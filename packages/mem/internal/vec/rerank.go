package vec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

const defaultRerankBody = 1 << 20

var ErrBadRerank = errors.New("unusable rerank reply")

type RerankOptions struct {
	URL     string
	Model   string
	Timeout time.Duration
	MaxBody int64
	Client  *http.Client
}

type Reranker struct {
	o    RerankOptions
	http *http.Client
	url  string
	urls []string
}

func NewReranker(o RerankOptions) *Reranker {
	if o.Timeout <= 0 {
		o.Timeout = time.Minute
	}
	if o.MaxBody <= 0 {
		o.MaxBody = defaultRerankBody
	}
	urls := order(context.Background(), o.URL)
	r := &Reranker{o: o, http: o.Client, url: urls[0], urls: urls}
	if r.http == nil {
		r.http = &http.Client{Timeout: o.Timeout}
	}
	return r
}

type rerankReply struct {
	Results []struct {
		Index *int     `json:"index"`
		Score *float64 `json:"relevance_score"`
	} `json:"results"`
}

func (r *Reranker) Rerank(ctx context.Context, query string, docs []string) ([]float64, error) {
	if len(docs) == 0 {
		return []float64{}, nil
	}
	req := map[string]any{"query": query, "documents": docs, "top_n": len(docs)}
	if r.o.Model != "" {
		req["model"] = r.o.Model
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var scores []float64
	for i, url := range r.urls {
		if scores, err = r.rerankAt(ctx, url, body, query, docs); err == nil || i == len(r.urls)-1 || !failover(err) {
			break
		}
	}
	return scores, err
}

func (r *Reranker) rerankAt(ctx context.Context, url string, body []byte, query string, docs []string) ([]float64, error) {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/v1/rerank", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	resp, err := r.http.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, r.o.MaxBody+1))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{resp.StatusCode, redact(raw, append([]string{query}, docs...))}
	}
	if int64(len(raw)) > r.o.MaxBody {
		return nil, fmt.Errorf("%w: larger than %d bytes", ErrBadRerank, r.o.MaxBody)
	}
	var rr rerankReply
	if err := json.Unmarshal(raw, &rr); err != nil {
		return nil, fmt.Errorf("%w: not a rerank list: %v", ErrBadRerank, err)
	}
	if len(rr.Results) != len(docs) {
		return nil, fmt.Errorf("%w: %d results for %d documents", ErrBadRerank, len(rr.Results), len(docs))
	}
	scores := make([]float64, len(docs))
	filled := make([]bool, len(docs))
	for _, x := range rr.Results {
		if x.Index == nil || x.Score == nil {
			return nil, fmt.Errorf("%w: a result lacks its index or score", ErrBadRerank)
		}
		i := *x.Index
		if i < 0 || i >= len(docs) || filled[i] {
			return nil, fmt.Errorf("%w: index %d is out of range or repeated", ErrBadRerank, i)
		}
		scores[i], filled[i] = *x.Score, true
	}
	if err := CheckScores(scores, len(docs)); err != nil {
		return nil, err
	}
	return scores, nil
}

func CheckScores(scores []float64, n int) error {
	if len(scores) != n {
		return fmt.Errorf("%w: %d scores for %d documents", ErrBadRerank, len(scores), n)
	}
	for i, s := range scores {
		if math.IsNaN(s) || s < 0 || s > 1 {
			return fmt.Errorf("%w: score %v of document %d is outside [0,1]", ErrBadRerank, s, i)
		}
	}
	return nil
}
