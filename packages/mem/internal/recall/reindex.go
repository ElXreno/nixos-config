package recall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"sync"
	"time"

	"mem/internal/store"
)

const (
	DefaultReindexBatch = 16
	DefaultReindexPar   = 2
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type ReindexOptions struct {
	Model     string
	DocPrefix string
	Embedder  Embedder
	Batch     int
	Par       int
	SkipLogs  bool
	Force     bool
	MaxDocs   int
	Progress  io.Writer
}

type ReindexResult struct {
	Current     int     `json:"current"`
	Expired     int     `json:"expired"`
	Fresh       int     `json:"fresh"`
	Embedded    int     `json:"embedded"`
	Raced       int     `json:"raced"`
	SkippedLogs int     `json:"skipped_logs"`
	Deferred    int     `json:"deferred"`
	Dim         int     `json:"dim"`
	Seconds     float64 `json:"seconds"`
}

type todo struct {
	key, text, sha string
}

func CheckVector(v []float32) error {
	if len(v) == 0 {
		return errors.New("empty vector")
	}
	var s float64
	for _, x := range v {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return errors.New("non-finite component")
		}
		s += float64(x) * float64(x)
	}
	if !(s > 0) {
		return errors.New("zero vector")
	}
	return nil
}

func Reindex(ctx context.Context, s *store.Store, o ReindexOptions) (ReindexResult, error) {
	var res ReindexResult
	if o.Model == "" || o.Embedder == nil {
		return res, errors.New("reindex needs a model and an embedder")
	}
	o.Batch, o.Par = max(o.Batch, 1), max(o.Par, 1)
	start := time.Now()
	recs, err := s.List(ctx, store.Filter{Statuses: []store.Status{store.Current, store.Expired}})
	if err != nil {
		return res, err
	}
	info, err := s.VectorInfo(ctx, o.Model)
	if err != nil {
		return res, err
	}
	var work []todo
	freshDim := map[int]int{}
	for _, r := range recs {
		if o.SkipLogs && r.Kind == store.Log {
			res.SkippedLogs++
			continue
		}
		if r.Status == store.Expired {
			res.Expired++
		} else {
			res.Current++
		}
		text := store.DocText(o.DocPrefix, r)
		sha := store.TextSHA(text)
		if vi, ok := info[r.Key]; ok && vi.SHA == sha && !o.Force {
			res.Fresh++
			freshDim[vi.Dim]++
			continue
		}
		work = append(work, todo{r.Key, text, sha})
	}
	if o.MaxDocs > 0 && len(work) > o.MaxDocs {
		slices.SortStableFunc(work, func(a, b todo) int { return len(a.text) - len(b.text) })
		res.Deferred = len(work) - o.MaxDocs
		work = work[:o.MaxDocs]
	}
	slices.SortStableFunc(work, func(a, b todo) int { return len(b.text) - len(a.text) })
	var batches [][]todo
	for i := 0; i < len(work); i += o.Batch {
		batches = append(batches, work[i:min(i+o.Batch, len(work))])
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		mu       sync.Mutex
		firstErr error
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
	jobs := make(chan []todo)
	for range min(o.Par, max(len(batches), 1)) {
		wg.Go(func() {
			for b := range jobs {
				texts := make([]string, len(b))
				for i, w := range b {
					texts[i] = w.text
				}
				vecs, err := o.Embedder.Embed(ctx, texts)
				if err != nil {
					fail(fmt.Errorf("embedding %d texts: %w", len(b), err))
					continue
				}
				if len(vecs) != len(b) {
					fail(fmt.Errorf("embedder returned %d vectors for %d texts", len(vecs), len(b)))
					continue
				}
				rows := make([]store.VecRow, len(b))
				bad := false
				for i, v := range vecs {
					if err := CheckVector(v); err != nil {
						fail(fmt.Errorf("record %s: %w", b[i].key, err))
						bad = true
						break
					}
					rows[i] = store.VecRow{Key: b[i].key, Vec: v, TextSHA: b[i].sha}
				}
				if bad {
					continue
				}
				mu.Lock()
				if err := checkDims(&res, freshDim, rows); err != nil {
					mu.Unlock()
					fail(err)
					continue
				}
				written, skipped, err := s.PutVectors(ctx, o.Model, o.DocPrefix, rows)
				if err != nil {
					mu.Unlock()
					fail(err)
					continue
				}
				res.Embedded += written
				res.Raced += skipped
				if o.Progress != nil {
					_, _ = fmt.Fprintf(o.Progress, "embedded %d/%d\n", res.Embedded+res.Raced, len(work))
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
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	wg.Wait()
	res.Seconds = time.Since(start).Seconds()
	if firstErr != nil {
		return res, firstErr
	}
	return res, ctx.Err()
}

func checkDims(res *ReindexResult, freshDim map[int]int, rows []store.VecRow) error {
	dim := len(rows[0].Vec)
	for _, r := range rows {
		if len(r.Vec) != dim {
			return fmt.Errorf("one response mixes dimensions %d and %d", dim, len(r.Vec))
		}
	}
	if res.Dim == 0 {
		res.Dim = dim
		for d, n := range freshDim {
			if d != dim {
				return fmt.Errorf("the model now returns dimension %d, %d kept vectors have dimension %d; rerun with --force", dim, n, d)
			}
		}
	}
	if dim != res.Dim {
		return fmt.Errorf("dimension %d in one response, %d in another", dim, res.Dim)
	}
	return nil
}
