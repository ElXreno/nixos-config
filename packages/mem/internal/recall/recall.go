package recall

import (
	"mem/internal/store"
	"mem/internal/text"
)

const (
	DefaultMinCover   = 0.4
	DefaultMinShared  = 4
	DefaultMinTopic   = 1
	DefaultLead       = true
	DefaultLeadShared = 2
	DefaultPool       = 12
	DefaultLimit      = 4
	DefaultMaxChars   = 2500
	SearchDepth       = 60
)

const PromptHeader = "Agent memory: records related to this prompt (`mem get KEY` prints one in full, `mem search QUERY` finds more).\n"

type Gate struct {
	MinCover   float64 `json:"min_cover"`
	MinShared  int     `json:"min_shared"`
	MinTopic   int     `json:"min_topic"`
	Lead       bool    `json:"lead"`
	LeadShared int     `json:"lead_shared"`
}

func DefaultGate() Gate {
	return Gate{MinCover: DefaultMinCover, MinShared: DefaultMinShared, MinTopic: DefaultMinTopic, Lead: DefaultLead, LeadShared: DefaultLeadShared}
}

type Settings struct {
	Gate
	Pool     int `json:"pool"`
	Limit    int `json:"limit"`
	MaxChars int `json:"max_chars"`
}

func DefaultSettings() Settings {
	return Settings{Gate: DefaultGate(), Pool: DefaultPool, Limit: DefaultLimit, MaxChars: DefaultMaxChars}
}

func SearchQuery(query string, pool int, scopes, exclude []string) store.Query {
	return store.Query{
		Text:         query,
		Limit:        pool,
		Depth:        max(SearchDepth, pool),
		ExcludeKinds: []store.Kind{store.Log},
		Scopes:       scopes,
		ExcludeKeys:  exclude,
		Mode:         store.Fused,
	}
}

type Relevance struct {
	Cover  float64 `json:"cover"`
	Shared int     `json:"shared"`
	Topic  int     `json:"topic"`
	Terms  int     `json:"terms"`
}

func Measure(q text.Set, r store.Record) Relevance {
	if len(q) == 0 {
		return Relevance{}
	}
	shared := q.Intersect(text.NewSet(store.RecordText(r)))
	return Relevance{
		Cover:  float64(shared) / float64(len(q)),
		Shared: shared,
		Topic:  q.Intersect(text.NewSet(store.KeyText(r.Key) + "\n" + r.Title)),
		Terms:  len(q),
	}
}

func (g Gate) Passes(rv Relevance) bool {
	return rv.Terms > 0 && rv.Shared >= min(g.MinShared, rv.Terms) && rv.Cover >= g.MinCover && rv.Topic >= g.MinTopic
}

type Candidate struct {
	Hit       store.Hit `json:"hit"`
	Relevance Relevance `json:"relevance"`
}

func Candidates(query string, hits []store.Hit) []Candidate {
	q := text.NewSet(query)
	out := make([]Candidate, 0, len(hits))
	for _, h := range hits {
		if h.Record.Kind == store.Log {
			continue
		}
		out = append(out, Candidate{Hit: h, Relevance: Measure(q, h.Record)})
	}
	return out
}

func Leads(cands []Candidate) bool {
	if len(cands) == 0 {
		return false
	}
	top := cands[0].Hit
	return top.WordsRank == 1 && top.TriRank == 1 && (len(cands) == 1 || cands[1].Hit.TriRank == 0)
}

func (g Gate) passesLead(rv Relevance) bool {
	return rv.Terms > 0 && rv.Shared >= min(g.LeadShared, rv.Terms) && rv.Topic >= g.MinTopic
}

func (g Gate) Choose(cands []Candidate, limit int) []int {
	var out []int
	top := g.Lead && Leads(cands)
	for i, c := range cands {
		if limit > 0 && len(out) >= limit {
			break
		}
		if g.Passes(c.Relevance) || i == 0 && top && g.passesLead(c.Relevance) {
			out = append(out, i)
		}
	}
	return out
}

type Injection struct {
	Packed
	Picks []Candidate
}

func (st Settings) Picks(cands []Candidate) []Candidate {
	var out []Candidate
	for _, i := range st.Choose(cands, st.Limit) {
		out = append(out, cands[i])
	}
	return out
}

func (st Settings) Inject(query string, cands []Candidate) Injection {
	var inj Injection
	var recs []store.Record
	for _, c := range st.Picks(cands) {
		inj.Picks = append(inj.Picks, c)
		recs = append(recs, c.Hit.Record)
	}
	if len(recs) == 0 {
		return inj
	}
	inj.Packed = Pack(PromptHeader, recs, query, st.MaxChars)
	return inj
}
