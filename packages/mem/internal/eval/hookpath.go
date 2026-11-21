package eval

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"sync"
	"unicode/utf8"

	"mem/internal/recall"
	"mem/internal/store"
)

const (
	SetGold     = "gold"
	SetNegative = "negative"
	SetProject  = "project"
)

type HookStats struct {
	Set                string  `json:"set"`
	Prompts            int     `json:"prompts"`
	Injecting          int     `json:"prompts_with_injection"`
	InjectRate         float64 `json:"injection_rate"`
	Records            int     `json:"records_injected"`
	Cut                int     `json:"records_cut"`
	MeanRecords        float64 `json:"mean_records_per_prompt"`
	MeanChars          float64 `json:"mean_chars_per_prompt"`
	MeanCharsInjecting float64 `json:"mean_chars_per_injecting_prompt"`
	Recall             float64 `json:"injected_recall"`
	Hit                float64 `json:"injected_hit"`
	GoldInjected       int     `json:"gold_records_injected"`
	Precision          float64 `json:"injected_precision"`
}

type outcome struct {
	keys  []string
	cut   int
	chars int
}

func outcomeOf(inj recall.Injection) outcome {
	if inj.Text == "" {
		return outcome{}
	}
	return outcome{keys: inj.Keys, cut: len(inj.Cut), chars: utf8.RuneCountInString(inj.Text)}
}

func hookSearch(ctx context.Context, s *store.Store, qs []*query, st recall.Settings, nw int) error {
	jobs := make(chan *query)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for range nw {
		wg.Go(func() {
			for q := range jobs {
				hits, err := s.Search(ctx, recall.SearchQuery(q.Query, st.Pool, nil, nil))
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				q.cands = recall.Candidates(q.Query, hits)
				q.inj = st.Inject(q.Query, q.cands)
			}
		})
	}
	for _, q := range qs {
		jobs <- q
	}
	close(jobs)
	wg.Wait()
	return firstErr
}

func injectedRanks(q *query, keySlug map[string]string) []rankedHit {
	byKey := map[string]store.Hit{}
	for _, c := range q.cands {
		byKey[c.Hit.Record.Key] = c.Hit
	}
	var out []rankedHit
	for _, k := range q.inj.Keys {
		if q.inj.Text == "" {
			break
		}
		out = append(out, rankedHit{slug: keySlug[k], hit: byKey[k]})
	}
	return out
}

func hookStats(set string, qs []*query, outs []outcome, keySlug map[string]string) HookStats {
	st := HookStats{Set: set, Prompts: len(qs)}
	goldPrompts := 0
	for i, q := range qs {
		o := outs[i]
		st.Records += len(o.keys)
		st.Cut += o.cut
		st.MeanChars += float64(o.chars)
		if len(o.keys) > 0 {
			st.Injecting++
		}
		if len(q.gold) == 0 {
			continue
		}
		goldPrompts++
		n := 0
		for _, k := range o.keys {
			if slices.Contains(q.gold, keySlug[k]) {
				n++
			}
		}
		st.GoldInjected += n
		st.Recall += float64(n) / float64(len(q.gold))
		if n > 0 {
			st.Hit++
		}
	}
	if st.Injecting > 0 {
		st.MeanCharsInjecting = st.MeanChars / float64(st.Injecting)
	}
	if st.Prompts > 0 {
		st.InjectRate = float64(st.Injecting) / float64(st.Prompts)
		st.MeanRecords = float64(st.Records) / float64(st.Prompts)
		st.MeanChars /= float64(st.Prompts)
	}
	if goldPrompts > 0 {
		st.Recall /= float64(goldPrompts)
		st.Hit /= float64(goldPrompts)
	}
	if st.Records > 0 && goldPrompts > 0 {
		st.Precision = float64(st.GoldInjected) / float64(st.Records)
	}
	return st
}

func outcomes(qs []*query) []outcome {
	out := make([]outcome, len(qs))
	for i, q := range qs {
		out[i] = outcomeOf(q.inj)
	}
	return out
}

func sampleKey(seed uint64, p Prompt) uint64 {
	sum := sha256.Sum256(fmt.Appendf(nil, "%d\x00%s\x00%d", seed, p.Session, p.Turn))
	return binary.BigEndian.Uint64(sum[:8])
}

func sampleNegatives(pool []Prompt, n int, seed uint64) []Prompt {
	if n <= 0 {
		n = len(pool)
	}
	type keyed struct {
		k uint64
		p Prompt
	}
	ks := make([]keyed, len(pool))
	for i, p := range pool {
		ks[i] = keyed{sampleKey(seed, p), p}
	}
	sort.Slice(ks, func(i, j int) bool {
		if ks[i].k != ks[j].k {
			return ks[i].k < ks[j].k
		}
		if ks[i].p.Session != ks[j].p.Session {
			return ks[i].p.Session < ks[j].p.Session
		}
		return ks[i].p.Turn < ks[j].p.Turn
	})
	out := make([]Prompt, 0, min(n, len(ks)))
	for _, k := range ks[:min(n, len(ks))] {
		out = append(out, k.p)
	}
	return out
}

type Case struct {
	Name   string   `json:"name"`
	Prompt string   `json:"prompt"`
	Want   []string `json:"want,omitempty"`
	None   bool     `json:"none,omitempty"`
}

type CaseHit struct {
	Key   string  `json:"key"`
	Score float64 `json:"score"`
	recall.Relevance
}

type CaseResult struct {
	Name     string    `json:"name"`
	Hash     string    `json:"hash"`
	Want     []string  `json:"want,omitempty"`
	None     bool      `json:"none,omitempty"`
	Pass     bool      `json:"pass"`
	Injected []string  `json:"injected"`
	Chars    int       `json:"chars"`
	Top      []CaseHit `json:"top"`
}

func loadCases(path string) ([]Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cs []Case
	if err := json.Unmarshal(data, &cs); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i, c := range cs {
		if c.Name == "" || c.Prompt == "" || len(c.Want) == 0 && !c.None || len(c.Want) > 0 && c.None {
			return nil, fmt.Errorf("%s: case %d needs a name, a prompt and either want keys or none", path, i+1)
		}
	}
	return cs, nil
}

func judgeCase(c Case, q *query) CaseResult {
	o := outcomeOf(q.inj)
	res := CaseResult{Name: c.Name, Hash: PromptHash(c.Prompt), Want: c.Want, None: c.None, Injected: o.keys, Chars: o.chars}
	if res.Injected == nil {
		res.Injected = []string{}
	}
	for i, cand := range q.cands {
		if i >= 4 {
			break
		}
		res.Top = append(res.Top, CaseHit{Key: cand.Hit.Record.Key, Score: cand.Hit.Score, Relevance: cand.Relevance})
	}
	if c.None {
		res.Pass = len(o.keys) == 0
		return res
	}
	res.Pass = true
	for _, w := range c.Want {
		if !slices.Contains(o.keys, w) {
			res.Pass = false
		}
	}
	return res
}
