package ui

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"mem/internal/recall"
	"mem/internal/store"
)

const (
	NoTopic       = ""
	noTopicLabel  = "без темы"
	topicKeywords = 6
)

type Node struct {
	Key       string   `json:"k"`
	Type      string   `json:"t"`
	Topic     int      `json:"g"`
	Summary   string   `json:"d"`
	Out       []string `json:"o,omitempty"`
	Rel       string   `json:"rl,omitempty"`
	Broken    []string `json:"x,omitempty"`
	Modified  string   `json:"m"`
	Size      int      `json:"s"`
	Core      bool     `json:"c,omitempty"`
	Danger    bool     `json:"z,omitempty"`
	Open      bool     `json:"op,omitempty"`
	Status    string   `json:"st,omitempty"`
	Successor string   `json:"su,omitempty"`
	Scopes    []string `json:"sc,omitempty"`
	Version   int      `json:"ver"`
	TTLDays   int      `json:"tt,omitempty"`
	Chunks    int      `json:"ch,omitempty"`
	Source    string   `json:"src,omitempty"`
	Vector    string   `json:"vs,omitempty"`
	Triggers  int      `json:"tr,omitempty"`
	Verify    bool     `json:"vf,omitempty"`
	Verified  string   `json:"vd,omitempty"`
	Journal   int      `json:"j,omitempty"`
}

type TopicOut struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Keywords []string `json:"kw,omitempty"`
	Size     int      `json:"n"`
}

type Graph struct {
	Built    string     `json:"built"`
	Host     string     `json:"host"`
	Dir      string     `json:"dir"`
	DirShort string     `json:"dirShort"`
	Topics   []TopicOut `json:"topics"`
	Records  []Node     `json:"records"`
	Links    int        `json:"links"`
	Rows     int        `json:"rows"`

	meta  map[string]*nodeMeta
	base  baseHygiene
	res   *linkResolver
	byKey map[string]int
	vec   vecSummary
	fp    string
}

type nodeMeta struct {
	keys   []string
	kind   store.Kind
	status store.Status
	scopes []string
	topic  string
	core   bool
	vec    string
	asOf   time.Time
}

type Options struct {
	Host             string
	Home             string
	Now              func() time.Time
	StateDir         string
	Relayout         bool
	EmbedURL         string
	EmbedModel       string
	EmbedQueryPrefix string
	EmbedDocPrefix   string
	RerankURL        string
	HTTP             *http.Client
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now().UTC()
	}
	return time.Now().UTC()
}

func (o Options) queryPrefix() string {
	if o.EmbedQueryPrefix != "" {
		return o.EmbedQueryPrefix
	}
	return recall.DefaultQueryPrefix
}

func (o Options) docPrefix() string {
	if o.EmbedDocPrefix != "" {
		return o.EmbedDocPrefix
	}
	return recall.DefaultDocPrefix
}

func BaseKey(key string) (string, int) {
	i := strings.LastIndexByte(key, '#')
	if i <= 0 || i == len(key)-1 {
		return key, 0
	}
	n, err := strconv.Atoi(key[i+1:])
	if err != nil || n < 1 {
		return key, 0
	}
	return key[:i], n
}

type group struct {
	base string
	recs []store.Record
	nums []int
}

func groupRecords(recs []store.Record) []*group {
	by := map[string]*group{}
	var order []*group
	for _, r := range recs {
		base, n := r.Key, 0
		if r.Kind == store.Log {
			base, n = BaseKey(r.Key)
		}
		g := by[base]
		if g == nil {
			g = &group{base: base}
			by[base] = g
			order = append(order, g)
		}
		g.recs = append(g.recs, r)
		g.nums = append(g.nums, n)
	}
	for _, g := range order {
		idx := make([]int, len(g.recs))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool { return g.nums[idx[a]] < g.nums[idx[b]] })
		recs := make([]store.Record, len(idx))
		nums := make([]int, len(idx))
		for i, j := range idx {
			recs[i], nums[i] = g.recs[j], g.nums[j]
		}
		g.recs, g.nums = recs, nums
	}
	sort.Slice(order, func(i, j int) bool { return order[i].base < order[j].base })
	return order
}

func joinBodies(g *group) string {
	if len(g.recs) == 1 {
		return g.recs[0].Body
	}
	parts := make([]string, len(g.recs))
	for i, r := range g.recs {
		parts[i] = r.Body
	}
	return strings.Join(parts, "\n\n")
}

func groupTitle(g *group) string {
	t := g.recs[0].Title
	if len(g.recs) > 1 {
		if i := strings.Index(t, " / "); i > 0 {
			t = t[:i]
		}
	}
	return strings.TrimSpace(t)
}

func groupStatus(g *group) store.Status {
	for _, r := range g.recs {
		if r.Status == store.Current {
			return store.Current
		}
	}
	return g.recs[0].Status
}

func groupAsOf(g *group, fallback time.Time) time.Time {
	var t time.Time
	for _, r := range g.recs {
		if a := r.AsOf(); a.After(t) {
			t = a
		}
	}
	if t.IsZero() {
		for _, r := range g.recs {
			if r.CreatedAt.After(t) {
				t = r.CreatedAt
			}
		}
	}
	if t.IsZero() {
		return fallback
	}
	return t
}

func groupScopes(g *group) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range g.recs {
		for _, s := range r.Scopes {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	sort.Strings(out)
	return out
}

func groupTopic(g *group) string {
	for _, r := range g.recs {
		if t := r.Topic(); t != "" {
			return t
		}
	}
	return NoTopic
}

type rawLink struct{ src, dst, rel, note string }

func loadLinks(ctx context.Context, st *store.Store) ([]rawLink, error) {
	rows, err := st.DB().QueryContext(ctx, "SELECT src, dst, rel, note FROM links ORDER BY src, rel, dst")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []rawLink
	for rows.Next() {
		var l rawLink
		if err := rows.Scan(&l.src, &l.dst, &l.rel, &l.note); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func shortPath(path, home string) string {
	if home != "" && strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}

type vecSummary struct {
	Model   string
	Dim     int
	Rows    int
	Current int
	Fresh   int
	Stale   int
	Missing int
	ByKind  map[string]int
}

func vectorState(st *store.Store, ctx context.Context, opt Options, recs []store.Record) (map[string]string, vecSummary, error) {
	out := map[string]string{}
	sum := vecSummary{ByKind: map[string]int{}}
	models, err := st.VectorModels(ctx)
	if err != nil {
		return nil, sum, err
	}
	model := opt.EmbedModel
	if model == "" && len(models) == 1 {
		model = models[0].Model
	}
	if model == "" {
		return out, sum, nil
	}
	sum.Model = model
	for _, m := range models {
		if m.Model == model {
			sum.Rows, sum.Dim = m.Rows, m.Dim
		}
	}
	info, err := st.VectorInfo(ctx, model)
	if err != nil {
		return nil, sum, err
	}
	prefix := opt.docPrefix()
	for _, r := range recs {
		if r.Status != store.Current {
			continue
		}
		sum.Current++
		switch vi, ok := info[r.Key]; {
		case !ok:
			out[r.Key] = "missing"
			sum.Missing++
			sum.ByKind[string(r.Kind)]++
		case vi.SHA == store.TextSHA(store.DocText(prefix, r)):
			out[r.Key] = "fresh"
			sum.Fresh++
		default:
			out[r.Key] = "stale"
			sum.Stale++
		}
	}
	return out, sum, nil
}

func worstVector(states ...string) string {
	rank := map[string]int{"": 0, "fresh": 1, "stale": 2, "missing": 3}
	worst := ""
	for _, s := range states {
		if rank[s] > rank[worst] {
			worst = s
		}
	}
	return worst
}

func Build(ctx context.Context, st *store.Store, opt Options) (*Graph, error) {
	now := opt.now()
	recs, err := st.List(ctx, store.Filter{AllStatus: true})
	if err != nil {
		return nil, err
	}
	links, err := loadLinks(ctx, st)
	if err != nil {
		return nil, err
	}
	vecs, vsum, err := vectorState(st, ctx, opt, recs)
	if err != nil {
		return nil, err
	}
	infos, err := st.Topics(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := st.TopicMap(ctx, topicKeywords)
	if err != nil {
		return nil, err
	}
	groups := groupRecords(recs)
	baseOf := make(map[string]string, len(recs))
	for _, g := range groups {
		for _, r := range g.recs {
			baseOf[r.Key] = g.base
		}
	}
	lr := newLinkResolver(groups, links)

	outs := map[string][]string{}
	outSeen := map[[2]string]bool{}
	outRel := map[[2]string]byte{}
	broken := map[string][]string{}
	brokenSeen := map[[2]string]bool{}
	supersedes := map[string]string{}
	nLinks := map[[2]string]bool{}
	var dups [][3]string
	for _, l := range links {
		a, ok := baseOf[l.src]
		if !ok {
			continue
		}
		b, found := baseOf[l.dst]
		switch {
		case found && b != a:
			if !outSeen[[2]string{a, b}] {
				outSeen[[2]string{a, b}] = true
				outs[a] = append(outs[a], b)
				outRel[[2]string{a, b}] = relChar(l.rel)
				lo, hi := a, b
				if hi < lo {
					lo, hi = hi, lo
				}
				nLinks[[2]string{lo, hi}] = true
			} else if c := relChar(l.rel); relRank(c) > relRank(outRel[[2]string{a, b}]) {
				outRel[[2]string{a, b}] = c
			}
			if strings.HasPrefix(l.note, "distinct:") {
				dups = append(dups, [3]string{a, b, l.note})
			}
			if l.rel == string(store.Supersedes) {
				if _, ok := supersedes[b]; !ok {
					supersedes[b] = a
				}
			}
		case found:
		case l.rel == string(store.Unresolved):
			if !lr.valid(l.dst) || brokenSeen[[2]string{a, l.dst}] {
				continue
			}
			brokenSeen[[2]string{a, l.dst}] = true
			if keys, _ := lr.resolve(l.dst); len(keys) == 0 {
				broken[a] = append(broken[a], l.dst)
			}
		}
	}

	titleOf := map[string]string{}
	for _, t := range infos {
		titleOf[t.ID] = t.Title
	}
	kwOf := map[string][]string{}
	for _, e := range entries {
		kwOf[e.ID] = e.Keywords
	}
	g := &Graph{
		Built:    now.Format(time.RFC3339),
		Host:     opt.Host,
		Dir:      st.Path(),
		DirShort: shortPath(st.Path(), opt.Home),
		Records:  make([]Node, 0, len(groups)),
		Links:    len(nLinks),
		Rows:     len(recs),
		meta:     make(map[string]*nodeMeta, len(groups)),
		res:      lr,
		byKey:    make(map[string]int, len(groups)),
		vec:      vsum,
		base:     newBaseHygiene(),
	}
	for _, d := range dups {
		g.base.noteDuplicate(d[0], d[1], d[2])
	}
	topicSize := map[string]int{}
	topicIDs := make([]string, 0, len(groups))
	for _, gr := range groups {
		first := gr.recs[0]
		title := groupTitle(gr)
		body := joinBodies(gr)
		size := 0
		var states []string
		var triggers int
		var verify bool
		var verified time.Time
		for _, r := range gr.recs {
			size += len(r.Body)
			states = append(states, vecs[r.Key])
			triggers += len(r.Triggers)
			verify = verify || r.VerifyCmd != ""
			if r.VerifiedAt.After(verified) {
				verified = r.VerifiedAt
			}
		}
		status := groupStatus(gr)
		topic := groupTopic(gr)
		core := false
		for _, r := range gr.recs {
			core = core || r.Core
		}
		n := Node{
			Key:      gr.base,
			Type:     string(first.Kind),
			Summary:  truncRunes(title, 220),
			Out:      outs[gr.base],
			Rel:      relString(gr.base, outs[gr.base], outRel),
			Broken:   broken[gr.base],
			Modified: groupAsOf(gr, now).UTC().Format(time.RFC3339),
			Size:     size,
			Core:     core,
			Scopes:   groupScopes(gr),
			TTLDays:  first.TTLDays,
			Source:   sourceKind(first.Source),
			Triggers: triggers,
			Verify:   verify,
			Journal:  len(journalRe.FindAllStringIndex(body, -1)),
			Version:  1,
		}
		if !verified.IsZero() {
			n.Verified = verified.UTC().Format(time.RFC3339)
		}
		if first.Kind == store.State && n.TTLDays <= 0 {
			n.TTLDays = store.DefaultStateTTL
		}
		if status != store.Current {
			n.Status = string(status)
		} else {
			n.Vector = worstVector(states...)
		}
		for _, r := range gr.recs {
			n.Version = max(n.Version, r.Version)
		}
		if len(gr.recs) > 1 {
			n.Chunks = len(gr.recs)
		}
		if n.Summary == "" {
			n.Summary = firstLine(body)
		}
		if n.Summary == "" {
			n.Summary = gr.base
		}
		n.Danger = core && first.Kind == store.Rule && status == store.Current
		n.Open = first.Kind == store.State && status == store.Current && (first.TTLDays > 0 || first.VerifyCmd != "")
		if status == store.Superseded {
			if s, ok := baseOf[first.SupersededBy]; ok && s != gr.base {
				n.Successor = s
			} else if s, ok := supersedes[gr.base]; ok {
				n.Successor = s
			}
		}
		g.byKey[gr.base] = len(g.Records)
		g.Records = append(g.Records, n)
		keys := make([]string, len(gr.recs))
		for i, r := range gr.recs {
			keys[i] = r.Key
		}
		g.meta[gr.base] = &nodeMeta{keys: keys, kind: first.Kind, status: status, scopes: n.Scopes, topic: topic, core: core, vec: n.Vector, asOf: groupAsOf(gr, now)}
		topicIDs = append(topicIDs, topic)
		topicSize[topic]++
		g.base.collect(gr, &n, topic, baseOf, now)
	}
	g.base.finish(g, now)

	ids := make([]string, 0, len(topicSize))
	for id := range topicSize {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := ids[i], ids[j]
		if (a == NoTopic) != (b == NoTopic) {
			return b == NoTopic
		}
		if topicSize[a] != topicSize[b] {
			return topicSize[a] > topicSize[b]
		}
		return a < b
	})
	tix := make(map[string]int, len(ids))
	for i, id := range ids {
		label := titleOf[id]
		if id == NoTopic {
			label = noTopicLabel
		} else if label == "" {
			label = id
		}
		tix[id] = i
		g.Topics = append(g.Topics, TopicOut{ID: id, Label: label, Keywords: kwOf[id], Size: topicSize[id]})
	}
	if g.Topics == nil {
		g.Topics = []TopicOut{}
	}
	for i := range g.Records {
		g.Records[i].Topic = tix[topicIDs[i]]
	}
	return g, nil
}

func sourceKind(src string) string {
	kind, _, ok := strings.Cut(src, ":")
	if !ok {
		return src
	}
	return kind
}

func relChar(rel string) byte {
	switch store.Rel(rel) {
	case store.Supersedes:
		return 's'
	case store.DerivedFrom:
		return 'd'
	}
	return 'r'
}

func relRank(c byte) int {
	switch c {
	case 's':
		return 3
	case 'd':
		return 2
	case 'r':
		return 1
	}
	return 0
}

func relString(base string, outs []string, rels map[[2]string]byte) string {
	if len(outs) == 0 {
		return ""
	}
	b := make([]byte, len(outs))
	for i, o := range outs {
		b[i] = rels[[2]string{base, o}]
	}
	return string(b)
}
