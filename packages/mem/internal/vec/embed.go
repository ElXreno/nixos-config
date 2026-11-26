package vec

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	DefaultBatch   = 32
	DefaultPar     = 2
	DefaultRetries = 3
	maxInfoBytes   = 16 << 10
	maxErrBytes    = 160
	maxScanBytes   = 1 << 20
	defaultMaxBody = 64 << 20
	echoWindow     = 16
)

type Options struct {
	URL         string
	Model       string
	QueryPrefix string
	DocPrefix   string
	QuerySuffix string
	DocSuffix   string
	In          string
	Out         string
	Which       string
	DocsFile    string
	Batch       int
	Par         int
	Retries     int
	Backoff     time.Duration
	Timeout     time.Duration
	MaxBody     int64
	Client      *http.Client
}

type input struct {
	name string
	text string
}

type source struct {
	part   Part
	path   string
	prefix string
	suffix string
	idKey  string
}

func readInputs(path, idKey string) ([]input, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var out []input
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<28)
	line := 0
	for sc.Scan() {
		line++
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var row map[string]json.RawMessage
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			return nil, nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		var name, text string
		if err := json.Unmarshal(row[idKey], &name); err != nil || name == "" {
			return nil, nil, fmt.Errorf("%s:%d: missing %q", path, line, idKey)
		}
		if err := json.Unmarshal(row["text"], &text); err != nil {
			return nil, nil, fmt.Errorf("%s:%d: missing \"text\"", path, line)
		}
		if strings.TrimSpace(text) == "" {
			return nil, nil, fmt.Errorf("%s:%d: empty text for %s", path, line, name)
		}
		if seen[name] {
			return nil, nil, fmt.Errorf("%s:%d: duplicate %s %s", path, line, idKey, name)
		}
		seen[name] = true
		out = append(out, input{name, text})
	}
	if err := sc.Err(); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(out) == 0 {
		return nil, nil, fmt.Errorf("%s: no rows", path)
	}
	return out, raw, nil
}

func textTag(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:4])
}

type client struct {
	o    Options
	http *http.Client
	url  string
	urls []string
}

type embedResponse struct {
	Data []struct {
		Index     *int      `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func jsonStrings(v any, out *[]string) {
	switch x := v.(type) {
	case string:
		*out = append(*out, x)
	case []any:
		for _, e := range x {
			jsonStrings(e, out)
		}
	case map[string]any:
		for _, e := range x {
			jsonStrings(e, out)
		}
	}
}

func windows(s string, n int, visit func(string) bool) bool {
	for i := 0; i+n <= len(s); i++ {
		if visit(s[i : i+n]) {
			return true
		}
	}
	return false
}

func repeatsInput(body []byte, inputs []string) bool {
	if len(body) > maxScanBytes {
		body = body[:maxScanBytes]
	}
	views := []string{string(body)}
	var decoded any
	if json.Unmarshal(body, &decoded) == nil {
		jsonStrings(decoded, &views)
	}
	known := map[string]struct{}{}
	for _, in := range inputs {
		windows(in, echoWindow, func(w string) bool {
			known[w] = struct{}{}
			return false
		})
	}
	for _, v := range views {
		if windows(v, echoWindow, func(w string) bool {
			_, ok := known[w]
			return ok
		}) {
			return true
		}
	}
	return false
}

func redact(body []byte, inputs []string) string {
	if repeatsInput(body, inputs) {
		return "[body withheld, it repeats the input]"
	}
	s := strings.Join(strings.Fields(string(body)), " ")
	if len(s) > maxErrBytes {
		cut := maxErrBytes
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "..."
	}
	return s
}

type httpError struct {
	status int
	msg    string
	prompt int
	nCtx   int
}

func (e *httpError) Error() string { return fmt.Sprintf("http %d: %s", e.status, e.msg) }

func (e *httpError) ContextOverflow() (prompt, limit int, ok bool) {
	return e.prompt, e.nCtx, e.prompt > e.nCtx && e.nCtx > 0
}

func newHTTPError(status int, raw []byte, inputs []string) *httpError {
	he := &httpError{status: status, msg: redact(raw, inputs)}
	var body struct {
		Error struct {
			Type   string `json:"type"`
			Prompt int    `json:"n_prompt_tokens"`
			NCtx   int    `json:"n_ctx"`
		} `json:"error"`
	}
	if status == http.StatusBadRequest && json.Unmarshal(raw, &body) == nil && body.Error.Type == "exceed_context_size_error" {
		he.prompt, he.nCtx = body.Error.Prompt, body.Error.NCtx
	}
	return he
}

func (c *client) once(ctx context.Context, url string, inputs []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"model": c.o.Model, "input": inputs, "encoding_format": "float"})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	limit := c.o.MaxBody
	if limit <= 0 {
		limit = defaultMaxBody
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, newHTTPError(resp.StatusCode, raw, inputs)
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("response is larger than %d bytes", limit)
	}
	var er embedResponse
	if err := json.Unmarshal(raw, &er); err != nil {
		return nil, fmt.Errorf("response is not an embeddings list: %w", err)
	}
	if len(er.Data) != len(inputs) {
		return nil, fmt.Errorf("server returned %d embeddings for %d inputs", len(er.Data), len(inputs))
	}
	out := make([][]float32, len(inputs))
	for pos, d := range er.Data {
		idx := pos
		if d.Index != nil {
			idx = *d.Index
		}
		if idx < 0 || idx >= len(inputs) || out[idx] != nil {
			return nil, fmt.Errorf("embedding index %d is out of range or repeated", idx)
		}
		v := make([]float32, len(d.Embedding))
		for i, x := range d.Embedding {
			v[i] = float32(x)
		}
		out[idx] = v
	}
	return out, nil
}

func retryable(err error) bool {
	var he *httpError
	if errors.As(err, &he) {
		return he.status >= 500 || he.status == http.StatusTooManyRequests
	}
	return !errors.Is(err, context.Canceled)
}

func (c *client) embed(ctx context.Context, inputs []string) ([][]float32, error) {
	var out [][]float32
	var err error
	for i, url := range c.urls {
		if out, err = c.embedAt(ctx, url, inputs); err == nil || i == len(c.urls)-1 || !failover(err) {
			break
		}
	}
	return out, err
}

func (c *client) embedAt(ctx context.Context, url string, inputs []string) ([][]float32, error) {
	var err error
	for attempt := 0; attempt <= c.o.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.o.Backoff * time.Duration(1<<(attempt-1))):
			}
		}
		var out [][]float32
		if out, err = c.once(ctx, url, inputs); err == nil {
			return out, nil
		}
		if !retryable(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", c.o.Retries+1, err)
}

func (c *client) info(ctx context.Context) (string, json.RawMessage) {
	for _, ep := range []string{"/info", "/props"} {
		ictx, cancel := context.WithTimeout(ctx, 3*time.Second)
		req, err := http.NewRequestWithContext(ictx, http.MethodGet, c.url+ep, nil)
		if err != nil {
			cancel()
			continue
		}
		resp, err := c.http.Do(req)
		if err != nil {
			cancel()
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxInfoBytes+1))
		_ = resp.Body.Close()
		cancel()
		if resp.StatusCode == http.StatusOK && len(raw) <= maxInfoBytes && json.Valid(raw) {
			var compact bytes.Buffer
			if json.Compact(&compact, raw) == nil {
				return ep, compact.Bytes()
			}
		}
	}
	return "", nil
}

type embedded struct {
	names   []string
	data    []float32
	dim     int
	sent    string
	input   string
	seconds float64
}

func (c *client) run(ctx context.Context, s source, log io.Writer) (*embedded, error) {
	in, raw, err := readInputs(s.path, s.idKey)
	if err != nil {
		return nil, err
	}
	inSum := sha256.Sum256(raw)
	sent := make([]string, len(in))
	sentHash := sha256.New()
	for i, r := range in {
		sent[i] = s.prefix + r.text + s.suffix
		_, _ = sentHash.Write([]byte(sent[i]))
		_, _ = sentHash.Write([]byte{0})
	}
	order := make([]int, len(in))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return len(sent[b]) - len(sent[a]) })
	var batches [][]int
	for i := 0; i < len(order); i += c.o.Batch {
		batches = append(batches, order[i:min(i+c.o.Batch, len(order))])
	}
	results := make([][]float32, len(in))
	start := time.Now()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		mu       sync.Mutex
		firstErr error
		done     int
		dim      int
		lastLog  = start
		wg       sync.WaitGroup
	)
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		mu.Unlock()
	}
	jobs := make(chan []int)
	for range min(c.o.Par, len(batches)) {
		wg.Go(func() {
			for b := range jobs {
				texts := make([]string, len(b))
				for i, idx := range b {
					texts[i] = sent[idx]
				}
				vecs, err := c.embed(ctx, texts)
				if err != nil {
					fail(fmt.Errorf("%s batch starting at %s: %w", s.part.Name, in[b[0]].name, err))
					continue
				}
				mu.Lock()
				for i, v := range vecs {
					idx := b[i]
					if len(v) == 0 || dim != 0 && len(v) != dim {
						mu.Unlock()
						fail(fmt.Errorf("%s %s (text %s): dimension %d, expected %d", s.part.Name, in[idx].name, textTag(sent[idx]), len(v), dim))
						mu.Lock()
						break
					}
					if dim == 0 {
						dim = len(v)
					}
					if err := Normalize(v); err != nil {
						mu.Unlock()
						fail(fmt.Errorf("%s %s (text %s): %w", s.part.Name, in[idx].name, textTag(sent[idx]), err))
						mu.Lock()
						break
					}
					results[idx] = v
				}
				done += len(b)
				if log != nil && (done == len(in) || time.Since(lastLog) >= 5*time.Second) {
					lastLog = time.Now()
					_, _ = fmt.Fprintf(log, "%s %d/%d\n", s.part.Name, done, len(in))
				}
				mu.Unlock()
			}
		})
	}
	for _, b := range batches {
		select {
		case jobs <- b:
		case <-ctx.Done():
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	out := &embedded{dim: dim, sent: hex.EncodeToString(sentHash.Sum(nil)), input: hex.EncodeToString(inSum[:]), seconds: time.Since(start).Seconds()}
	out.data = make([]float32, 0, len(in)*dim)
	for i, r := range in {
		if len(results[i]) != dim {
			return nil, fmt.Errorf("%s %s: no vector", s.part.Name, r.name)
		}
		out.names = append(out.names, r.name)
		out.data = append(out.data, results[i]...)
	}
	return out, nil
}

func (o *Options) check() error {
	switch {
	case o.URL == "":
		return errors.New("--url is required")
	case o.Model == "":
		return errors.New("--model is required")
	case o.In == "" && (o.Which != "docs" || o.DocsFile == ""):
		return errors.New("--in is required")
	case o.Out == "":
		return errors.New("--out is required")
	case o.Which != "docs" && o.Which != "queries" && o.Which != "both":
		return fmt.Errorf("--which must be docs, queries or both, not %q", o.Which)
	case o.Batch < 1:
		return errors.New("--batch must be at least 1")
	case o.Par < 1:
		return errors.New("--par must be at least 1")
	case o.Retries < 0:
		return errors.New("--retries must not be negative")
	}
	return nil
}

func sameRun(a, b *Meta) error {
	if a.Model != b.Model || a.QueryPrefix != b.QueryPrefix || a.DocPrefix != b.DocPrefix || a.QuerySuffix != b.QuerySuffix || a.DocSuffix != b.DocSuffix {
		return errors.New("the output directory holds a different model, prefix or suffix from the part kept; use a fresh directory")
	}
	return nil
}

func Embed(ctx context.Context, o Options, log io.Writer) (*Meta, error) {
	if err := o.check(); err != nil {
		return nil, err
	}
	if o.Backoff == 0 {
		o.Backoff = time.Second
	}
	if o.Timeout == 0 {
		o.Timeout = 10 * time.Minute
	}
	urls := order(context.Background(), o.URL)
	c := &client{o: o, http: o.Client, url: urls[0], urls: urls}
	if c.http == nil {
		c.http = &http.Client{Timeout: o.Timeout}
	}
	var srcs []source
	if o.Which != "queries" {
		path := filepath.Join(o.In, "docs.jsonl")
		if o.DocsFile != "" {
			path = o.DocsFile
		}
		srcs = append(srcs, source{PartDocs, path, o.DocPrefix, o.DocSuffix, "key"})
	}
	if o.Which != "docs" {
		srcs = append(srcs, source{PartQueries, filepath.Join(o.In, "queries.jsonl"), o.QueryPrefix, o.QuerySuffix, "id"})
	}
	meta := &Meta{Model: o.Model, URL: o.URL, QueryPrefix: o.QueryPrefix, DocPrefix: o.DocPrefix, QuerySuffix: o.QuerySuffix, DocSuffix: o.DocSuffix}
	prev, err := ReadMeta(o.Out)
	switch {
	case err == nil:
		if o.Which != "both" && (o.Which == "queries" && prev.Docs != nil || o.Which == "docs" && prev.Queries != nil) {
			if err := sameRun(prev, meta); err != nil {
				return nil, err
			}
			meta.Dim = prev.Dim
			if o.Which == "queries" {
				meta.Docs = prev.Docs
			} else {
				meta.Queries = prev.Queries
			}
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	meta.ServerEndpoint, meta.ServerInfo = c.info(ctx)
	start := time.Now()
	done := make([]*embedded, len(srcs))
	for i, s := range srcs {
		if done[i], err = c.run(ctx, s, log); err != nil {
			return nil, err
		}
		if meta.Dim != 0 && meta.Dim != done[i].dim {
			return nil, fmt.Errorf("dimension %d for %s, %d elsewhere in this run", done[i].dim, s.part.Name, meta.Dim)
		}
		meta.Dim = done[i].dim
	}
	for i, s := range srcs {
		d := done[i]
		if err := WriteSet(o.Out, s.part, d.names, d.data, d.dim); err != nil {
			return nil, err
		}
		pm := &PartMeta{Count: len(d.names), Source: s.path, InputSHA256: d.input, SentSHA256: d.sent, Seconds: d.seconds,
			TextsPerSecond: float64(len(d.names)) / max(d.seconds, 1e-9), Batch: o.Batch, Par: o.Par}
		if s.part.Name == PartDocs.Name {
			meta.Docs = pm
		} else {
			meta.Queries = pm
		}
		meta.Texts += len(d.names)
	}
	meta.WallSeconds = time.Since(start).Seconds()
	meta.TextsPerSecond = float64(meta.Texts) / max(meta.WallSeconds, 1e-9)
	if err := WriteMeta(o.Out, meta); err != nil {
		return nil, err
	}
	return meta, nil
}
