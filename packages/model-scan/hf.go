package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var knownBadAuthors = setOf("mir2k", "timdedid")

type hfClient struct {
	c      *http.Client
	token  string
	mu     sync.Mutex
	tcache map[string][]namedText
}

type namedText struct{ name, text string }

func newHF() *hfClient {
	return &hfClient{c: &http.Client{Timeout: 180 * time.Second}, token: os.Getenv("HF_TOKEN"), tcache: map[string][]namedText{}}
}

func (h *hfClient) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", "model-scan")
	if h.token != "" {
		req.Header.Set("Authorization", "Bearer "+h.token)
	}
	return h.c.Do(req)
}

func (h *hfClient) get(u string, limit int64) ([]byte, int, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := h.do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, resp.StatusCode, fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if int64(len(b)) > limit {
		return nil, 200, fmt.Errorf("GET %s: larger than %d bytes", u, limit)
	}
	return b, 200, err
}

func resolveURL(repo, rev, file string) string {
	parts := strings.Split(file, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return "https://huggingface.co/" + repo + "/resolve/" + url.PathEscape(rev) + "/" + strings.Join(parts, "/")
}

type httpRA struct {
	h      *hfClient
	url    string
	size   int64
	mu     sync.Mutex
	blocks map[int64][]byte
}

const raBlock = 4 << 20

func (r *httpRA) ReadAt(p []byte, off int64) (int, error) {
	n := 0
	for n < len(p) {
		pos := off + int64(n)
		if pos >= r.size {
			return n, io.EOF
		}
		bi := pos / raBlock
		b, err := r.block(bi)
		if err != nil {
			return n, err
		}
		c := copy(p[n:], b[pos-bi*raBlock:])
		if c == 0 {
			return n, io.ErrUnexpectedEOF
		}
		n += c
	}
	return n, nil
}

func (r *httpRA) block(i int64) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.blocks[i]; ok {
		return b, nil
	}
	start := i * raBlock
	end := min(start+raBlock, r.size) - 1
	req, err := http.NewRequest("GET", r.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	resp, err := r.h.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	want := end - start + 1
	switch {
	case resp.StatusCode == http.StatusPartialContent:
	case resp.StatusCode == http.StatusOK && start == 0 && r.size <= raBlock:
	default:
		return nil, fmt.Errorf("range %d-%d: %s", start, end, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, want))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != want {
		return nil, fmt.Errorf("range %d-%d: got %d bytes", start, end, len(b))
	}
	if len(r.blocks) >= 64 {
		clear(r.blocks)
	}
	r.blocks[i] = b
	return b, nil
}

type modelInfo struct {
	ID        string
	Author    string
	Sha       string
	CreatedAt time.Time
	Downloads int
	Gated     any
	Disabled  bool
	CardData  struct {
		BaseModel any `json:"base_model"`
	} `json:"cardData"`
	Siblings []struct {
		Rfilename string
		Size      int64
		LFS       *struct{ Size int64 } `json:"lfs"`
	}
	Security *struct {
		ScansDone       bool
		FilesWithIssues []json.RawMessage
	} `json:"securityRepoStatus"`
}

func (h *hfClient) info(repo, rev string) (*modelInfo, error) {
	u := "https://huggingface.co/api/models/" + repo
	if rev != "" {
		u += "/revision/" + url.PathEscape(rev)
	}
	b, _, err := h.get(u+"?blobs=true&securityStatus=true", 64<<20)
	if err != nil {
		return nil, err
	}
	var mi modelInfo
	if err := json.Unmarshal(b, &mi); err != nil {
		return nil, fmt.Errorf("model info: %w", err)
	}
	return &mi, nil
}

func baseList(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		var out []string
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func parseHF(arg string) (repo, rev string, ok bool) {
	switch {
	case strings.HasPrefix(arg, "hf:"):
		repo = strings.TrimPrefix(arg, "hf:")
		if i := strings.LastIndexByte(repo, '@'); i >= 0 {
			repo, rev = repo[:i], repo[i+1:]
		}
	case strings.HasPrefix(arg, "https://huggingface.co/"):
		parts := strings.Split(strings.Trim(strings.TrimPrefix(arg, "https://huggingface.co/"), "/"), "/")
		if len(parts) < 2 {
			return "", "", false
		}
		repo = parts[0] + "/" + parts[1]
		if len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "blob" || parts[2] == "resolve") {
			rev = parts[3]
		}
	default:
		return "", "", false
	}
	return repo, rev, strings.Count(repo, "/") == 1
}

func hfRepoFromURL(s string) string {
	s = strings.TrimSpace(s)
	for _, p := range []string{"https://huggingface.co/", "http://huggingface.co/", "huggingface.co/"} {
		if strings.HasPrefix(s, p) {
			parts := strings.Split(strings.Trim(strings.TrimPrefix(s, p), "/"), "/")
			if len(parts) >= 2 {
				return parts[0] + "/" + parts[1]
			}
		}
	}
	return ""
}

func readmeBases(s string) []string {
	if !strings.HasPrefix(s, "---") {
		return nil
	}
	end := strings.Index(s[3:], "\n---")
	if end < 0 {
		return nil
	}
	lines := strings.Split(s[3:3+end], "\n")
	var out []string
	for i, l := range lines {
		if !strings.HasPrefix(l, "base_model:") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(l, "base_model:"))
		if strings.HasPrefix(v, "[") {
			for _, x := range strings.Split(strings.Trim(v, "[]"), ",") {
				out = append(out, strings.Trim(strings.TrimSpace(x), `"'`))
			}
			continue
		}
		if v != "" {
			out = append(out, strings.Trim(v, `"'`))
			continue
		}
		for _, x := range lines[i+1:] {
			x = strings.TrimSpace(x)
			if !strings.HasPrefix(x, "- ") {
				break
			}
			out = append(out, strings.Trim(strings.TrimSpace(x[2:]), `"'`))
		}
	}
	return out
}

func remoteWanted(lower, ext string) bool {
	switch {
	case ext == ".gguf", ext == ".safetensors", ext == ".jinja", ext == ".npy", ext == ".npz", ext == ".py":
		return true
	case tmplJSON[lower], lower == "modelfile", pickleExt[ext], scriptExt[ext], nativeExt[ext]:
		return true
	}
	return false
}

func scanRemote(t *target, repo, rev string, o opts) {
	mi, err := o.hf.info(repo, rev)
	if err != nil {
		t.add(errs, repo, "%v", err)
		return
	}
	if knownBadAuthors[strings.ToLower(mi.Author)] {
		t.add(crit, repo, "author %s is on the known-malicious list", mi.Author)
	}
	if mi.Disabled {
		t.add(high, repo, "repository is disabled by Hugging Face")
	}
	t.add(info, repo, "author %s, created %s (%d days ago), %d downloads, revision %s, gated %v",
		mi.Author, mi.CreatedAt.Format("2006-01-02"), int(time.Since(mi.CreatedAt).Hours()/24), mi.Downloads, mi.Sha, mi.Gated)
	if s := mi.Security; s != nil {
		for _, f := range s.FilesWithIssues {
			t.add(high, repo, "Hugging Face security scan flags %s", clip(string(f), 300))
		}
		if !s.ScansDone {
			t.add(info, repo, "Hugging Face security scans have not finished")
		}
	}
	for _, b := range baseList(mi.CardData.BaseModel) {
		t.addBase(b)
	}
	sha := mi.Sha
	if sha == "" {
		sha = "main"
	}
	parallel(min(o.workers, 4), len(mi.Siblings), func(i int) {
		s := mi.Siblings[i]
		lower := strings.ToLower(filepath.Base(s.Rfilename))
		ext := filepath.Ext(lower)
		if !remoteWanted(lower, ext) {
			return
		}
		size := s.Size
		if s.LFS != nil && s.LFS.Size > 0 {
			size = s.LFS.Size
		}
		if size <= 0 {
			return
		}
		ra := &httpRA{h: o.hf, url: resolveURL(repo, sha, s.Rfilename), size: size, blocks: map[int64][]byte{}}
		scanFile(t, s.Rfilename, ra, size, o, false)
	})
}

func (h *hfClient) templates(ref string) ([]namedText, error) {
	h.mu.Lock()
	if c, ok := h.tcache[ref]; ok {
		h.mu.Unlock()
		return c, nil
	}
	h.mu.Unlock()
	repo, rev := ref, "main"
	if i := strings.LastIndexByte(ref, '@'); i >= 0 {
		repo, rev = ref[:i], ref[i+1:]
	}
	var out []namedText
	var firstErr error
	for _, f := range []string{"chat_template.jinja", "chat_template.json", "tokenizer_config.json"} {
		b, code, err := h.get(resolveURL(repo, rev, f), 16<<20)
		if code == http.StatusNotFound {
			continue
		}
		if err != nil {
			firstErr = err
			continue
		}
		name := ref + ":" + f
		if f == "chat_template.jinja" {
			out = append(out, namedText{name, string(b)})
			continue
		}
		var v struct {
			ChatTemplate json.RawMessage `json:"chat_template"`
		}
		if json.Unmarshal(b, &v) != nil || len(v.ChatTemplate) == 0 {
			continue
		}
		var s string
		if json.Unmarshal(v.ChatTemplate, &s) == nil {
			out = append(out, namedText{name, s})
			continue
		}
		var list []struct{ Name, Template string }
		if json.Unmarshal(v.ChatTemplate, &list) == nil {
			for _, x := range list {
				out = append(out, namedText{name + "#" + x.Name, x.Template})
			}
		}
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	h.mu.Lock()
	h.tcache[ref] = out
	h.mu.Unlock()
	return out, nil
}
