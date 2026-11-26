package recall

import (
	"context"
	"math"
	"sort"
	"strings"

	"mem/internal/store"
)

const (
	DefaultAutoTopicK      = 7
	DefaultAutoTopicMinCos = 0.45
	autoTopicMinShared     = 3
)

type AutoTopicOptions struct {
	Model  string
	K      int
	MinCos float64
}

type TopicPick struct {
	Key   string  `json:"key"`
	Topic string  `json:"topic"`
	Cos   float64 `json:"cos"`
}

type AutoTopicResult struct {
	Assigned []TopicPick `json:"assigned"`
	Skipped  int         `json:"skipped"`
}

type topicVec struct {
	key      string
	topic    string
	projects []string
	v        []float32
}

func normTopicVec(v []float32) []float32 {
	var n float64
	for _, x := range v {
		n += float64(x) * float64(x)
	}
	if n == 0 {
		return v
	}
	n = math.Sqrt(n)
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / n)
	}
	return out
}

func projectScopes(scopes []string) []string {
	var out []string
	for _, s := range scopes {
		if strings.HasPrefix(s, "project:") {
			out = append(out, s)
		}
	}
	return out
}

func sharesProject(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

type scored struct {
	topic string
	cos   float64
}

func vote(top []scored) (string, float64) {
	sum := map[string]float64{}
	for _, c := range top {
		sum[c.topic] += c.cos
	}
	best, bestSum := "", math.Inf(-1)
	for t, s := range sum {
		if s > bestSum || (s == bestSum && t < best) {
			best, bestSum = t, s
		}
	}
	for _, c := range top {
		if c.topic == best {
			return best, c.cos
		}
	}
	return best, 0
}

func AutoTopics(ctx context.Context, s *store.Store, o AutoTopicOptions) (AutoTopicResult, error) {
	var res AutoTopicResult
	if o.K < 1 {
		o.K = DefaultAutoTopicK
	}
	recs, err := s.List(ctx, store.Filter{})
	if err != nil {
		return res, err
	}
	byKey := make(map[string]store.Record, len(recs))
	missing := 0
	for _, r := range recs {
		byKey[r.Key] = r
		if r.Topic() == "" {
			missing++
		}
	}
	if missing == 0 {
		return res, nil
	}
	topics, err := s.Topics(ctx)
	if err != nil {
		return res, err
	}
	auto, err := s.AutoTopicKeys(ctx)
	if err != nil {
		return res, err
	}
	titles := make(map[string]string, len(topics))
	for _, t := range topics {
		titles[t.ID] = t.Title
	}
	var with, without []topicVec
	var buf []float32
	err = s.ScanVectors(ctx, o.Model, store.Query{}, func(v store.VecScan) error {
		r, ok := byKey[v.Key]
		if !ok {
			return nil
		}
		buf = store.DecodeVec(v.Blob, buf[:0])
		tv := topicVec{key: r.Key, topic: r.Topic(), projects: projectScopes(r.Scopes), v: normTopicVec(buf)}
		if tv.topic == "" {
			without = append(without, tv)
		} else if _, ok := titles[tv.topic]; ok && !auto[tv.key] {
			with = append(with, tv)
		}
		return nil
	})
	if err != nil {
		return res, err
	}
	res.Skipped = missing - len(without)
	var assign []store.TopicAssign
	for _, q := range without {
		var shared, all []scored
		for _, c := range with {
			var dot float64
			for i := range q.v {
				if i < len(c.v) {
					dot += float64(q.v[i]) * float64(c.v[i])
				}
			}
			sc := scored{c.topic, dot}
			all = append(all, sc)
			if sharesProject(q.projects, c.projects) {
				shared = append(shared, sc)
			}
		}
		pool := all
		if len(shared) >= autoTopicMinShared {
			pool = shared
		}
		if len(pool) == 0 {
			res.Skipped++
			continue
		}
		sort.Slice(pool, func(i, j int) bool { return pool[i].cos > pool[j].cos })
		if pool[0].cos < o.MinCos {
			res.Skipped++
			continue
		}
		topic, cos := vote(pool[:min(o.K, len(pool))])
		assign = append(assign, store.TopicAssign{Key: q.key, Topic: topic, Title: titles[topic]})
		res.Assigned = append(res.Assigned, TopicPick{Key: q.key, Topic: topic, Cos: math.Round(cos*1000) / 1000})
	}
	if len(assign) == 0 {
		return res, nil
	}
	if _, err := s.SetAutoTopics(ctx, assign); err != nil {
		return AutoTopicResult{}, err
	}
	return res, nil
}
