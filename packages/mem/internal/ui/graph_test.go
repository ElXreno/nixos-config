package ui

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"mem/internal/store"
)

func nodesByKey(g Graph) map[string]Node {
	by := map[string]Node{}
	for _, n := range g.Records {
		by[n.Key] = n
	}
	return by
}

func TestGraphNodes(t *testing.T) {
	f := newFixture(t)
	var g Graph
	f.json(t, "/api/graph", &g)
	by := nodesByKey(g)
	if len(g.Records) != 9 {
		t.Fatalf("nodes = %d, want 9 (chunks of project_big collapse)", len(g.Records))
	}
	big := by["project_big"]
	if big.Type != "log" || big.Chunks != 2 || big.Summary != "Big log" || big.Size != len("First chunk of the large log about zfs readahead.")+len("Second chunk with the nvidia driver details.") {
		t.Fatalf("project_big = %+v", big)
	}
	if by["user_owner"].Type != "fact" || by["feedback_alpha"].Type != "rule" || by["project_state"].Type != "state" {
		t.Fatalf("types: %+v %+v %+v", by["user_owner"], by["feedback_alpha"], by["project_state"])
	}
	if a := by["feedback_alpha"]; a.Version != 2 || !slices.Equal(a.Out, []string{"reference_beta"}) || !a.Core || !a.Danger || a.Triggers != 1 {
		t.Fatalf("feedback_alpha = %+v", a)
	}
	if a := by["feedback_new"]; a.Core || a.Danger {
		t.Fatalf("a non-core rule is flagged: %+v", a)
	}
	if st := by["project_state"]; !st.Open || st.TTLDays != store.DefaultStateTTL || st.Core {
		t.Fatalf("project_state = %+v", st)
	}
	if o := by["feedback_old"]; o.Successor != "feedback_new" || o.Status != "superseded" || o.Danger {
		t.Fatalf("feedback_old = %+v", o)
	}
	if gm := by["reference_gamma"]; gm.Journal != 3 || !slices.Equal(gm.Broken, []string{"missing-key", "reference_net_hub"}) {
		t.Fatalf("reference_gamma broken = %v journal %d", gm.Broken, gm.Journal)
	}
	if !slices.Equal(by["project_big"].Out, []string{"reference_beta"}) {
		t.Fatalf("project_big out = %v", by["project_big"].Out)
	}
	for _, n := range g.Records {
		if n.Topic < 0 || n.Topic >= len(g.Topics) {
			t.Fatalf("%s topic %d of %d", n.Key, n.Topic, len(g.Topics))
		}
		if _, err := time.Parse(time.RFC3339, n.Modified); err != nil {
			t.Fatalf("%s modified %q", n.Key, n.Modified)
		}
	}
	if g.Host != "smok" || g.DirShort != g.Dir || g.Links != 5 || g.Rows != 10 {
		t.Fatalf("header: host %q dir %q links %d rows %d", g.Host, g.DirShort, g.Links, g.Rows)
	}
}

func TestTopicsComeFromTheStore(t *testing.T) {
	f := newFixture(t)
	var g Graph
	f.json(t, "/api/graph", &g)
	by := nodesByKey(g)
	if got := g.Topics[by["reference_gamma"].Topic]; got.ID != "net" || got.Label != "Networking" {
		t.Fatalf("gamma topic = %+v", got)
	}
	if got := g.Topics[by["project_big"].Topic]; got.ID != "kernel" || got.Label != "Kernel work" || got.Size != 3 {
		t.Fatalf("log group topic = %+v", got)
	}
	last := g.Topics[len(g.Topics)-1]
	if last.ID != NoTopic || last.Label != noTopicLabel || last.Size != 5 {
		t.Fatalf("the no-topic bucket must come last: %+v", g.Topics)
	}
	if by["reference_lonely"].Topic != len(g.Topics)-1 {
		t.Fatalf("an untopiced record is in topic %d", by["reference_lonely"].Topic)
	}
	var out struct {
		Topics []topicOut `json:"topics"`
	}
	f.json(t, "/api/topics?members=1", &out)
	total := 0
	for _, tp := range out.Topics {
		total += tp.Size
		if len(tp.Members) != tp.Size {
			t.Fatalf("topic %s members %d size %d", tp.Label, len(tp.Members), tp.Size)
		}
	}
	if total != 9 || len(out.Topics) != 3 {
		t.Fatalf("topics = %+v", out.Topics)
	}
	if strings.Contains(f.do(t, http.MethodGet, "/api/topics").Body.String(), `"hubs"`) {
		t.Fatal("topics still report hubs")
	}
}

func section(h Hygiene, id string) HygieneSection {
	for _, s := range h.Sections {
		if s.ID == id {
			return s
		}
	}
	return HygieneSection{}
}

func keysOfItems(its []HygieneItem) []string {
	out := []string{}
	for _, it := range its {
		out = append(out, it.Key)
	}
	return out
}

func TestHygieneCountsOnlyActionableItems(t *testing.T) {
	f := newFixture(t)
	var h Hygiene
	f.json(t, "/api/hygiene", &h)
	if b := section(h, "broken"); len(b.Items) != 2 || b.Items[0].Key != "reference_gamma" || b.Items[0].Detail != "missing-key" || b.Items[1].Detail != "reference_net_hub" || !b.Counted {
		t.Fatalf("broken = %+v", b)
	}
	for _, id := range []string{"superseded", "journals", "verify", "duplicates", "scopes"} {
		if section(h, id).Counted {
			t.Fatalf("%s must not be counted", id)
		}
	}
	if s := section(h, "superseded"); !slices.Equal(keysOfItems(s.Items), []string{"feedback_old"}) || s.Items[0].Detail != "-> feedback_new" {
		t.Fatalf("superseded = %+v", s)
	}
	if !slices.Equal(keysOfItems(section(h, "journals").Items), []string{"reference_gamma"}) {
		t.Fatalf("journals = %+v", section(h, "journals"))
	}
	if !slices.Equal(keysOfItems(section(h, "stale").Items), []string{"project_state"}) {
		t.Fatalf("stale = %+v", section(h, "stale"))
	}
	orphans := section(h, "orphans")
	if !slices.Contains(keysOfItems(orphans.Items), "reference_lonely") || slices.Contains(keysOfItems(orphans.Items), "reference_beta") {
		t.Fatalf("orphans = %+v", orphans)
	}
	for _, it := range orphans.Items {
		if it.Key == "reference_lonely" && !slices.Equal(it.Why, []string{reasonNoTopic}) {
			t.Fatalf("reasons = %v", it.Why)
		}
	}
	distinct := map[string]bool{}
	for _, id := range []string{"broken", "orphans", "expired", "stale", "dangling", "triggers"} {
		for _, it := range section(h, id).Items {
			distinct[it.Key] = true
		}
	}
	if h.Count != len(distinct) || h.Count == 0 {
		t.Fatalf("count = %d, distinct counted keys %d", h.Count, len(distinct))
	}
	if d := section(h, "duplicates"); len(d.Items) != 1 || !strings.Contains(d.Items[0].Detail, "different scope") {
		t.Fatalf("duplicates = %+v", d)
	}
	if sc := section(h, "scopes"); len(sc.Items) != 1 || sc.Items[0].Key != "project:legacy" || sc.Items[0].Kind != scopeKind {
		t.Fatalf("scopes = %+v", sc)
	}
	if len(h.Imports) != 1 || h.Imports[0].Project != "legacy" || h.Imports[0].Untopiced != 5 || h.Imports[0].Logs != 1 {
		t.Fatalf("imports = %+v", h.Imports)
	}
	for _, s := range h.Sections {
		if s.Items == nil {
			t.Fatalf("%s must encode as an array", s.ID)
		}
	}
	if h.Surfaced.Applied || h.Surfaced.WindowDays != surfacedDays {
		t.Fatalf("surfaced = %+v", h.Surfaced)
	}
}

func TestBrokenLinksResolveByKeyThenDerivedThenPrefix(t *testing.T) {
	f := newFixture(t)
	g, err := f.srv.Graph(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		token string
		keys  []string
		how   string
	}{
		{"reference_lonely", []string{"reference_lonely"}, resolveExact},
		{"project_big#2", []string{"project_big"}, resolveExact},
		{"reference-lonely", []string{"reference_lonely"}, resolveExact},
		{"old-slug", []string{"reference_beta"}, resolveDerived},
		{"project", []string{"project_big", "project_state"}, resolvePrefix},
		{"missing-key", nil, ""},
		{"MEMORY", nil, ""},
		{"SendMessage", nil, ""},
		{"a b", nil, ""},
		{"fe", nil, ""},
	}
	for _, c := range cases {
		keys, how := g.res.resolve(c.token)
		if !slices.Equal(keys, c.keys) || how != c.how {
			t.Errorf("%q = %v %q, want %v %q", c.token, keys, how, c.keys, c.how)
		}
	}
	if g.res.valid("MEMORY") || g.res.valid("SendMessage") || !g.res.valid("use-fd-not-find") {
		t.Fatal("token validity")
	}
	by := nodesByKey(*g)
	if !slices.Equal(by["reference_gamma"].Broken, []string{"missing-key", "reference_net_hub"}) {
		t.Fatalf("only the unresolvable token stays broken: %v", by["reference_gamma"].Broken)
	}
}

func TestStats(t *testing.T) {
	f := newFixture(t)
	var s statsOut
	f.json(t, "/api/stats", &s)
	if s.Rows != 10 || s.Nodes != 9 || s.ByKind["log"] != 2 || s.ByKind["rule"] != 3 || s.ByStatus["superseded"] != 1 || s.Topics != 2 || s.Untopiced != 5 || s.Core != 1 {
		t.Fatalf("stats = %+v", s)
	}
	if s.Links["relates"] != 4 || s.Links["supersedes"] != 1 || s.Links["unresolved"] != 5 || s.Links["derived_from"] != 1 || s.History < 1 || s.Schema < 2 {
		t.Fatalf("stats links/history/schema = %+v", s)
	}
	if s.Hygiene["superseded"] != 1 || s.Hygiene["broken"] != 2 || s.Hygiene["count"] == 0 {
		t.Fatalf("stats hygiene = %+v", s.Hygiene)
	}
}

func TestGraphCacheFollowsWrites(t *testing.T) {
	f := newFixture(t)
	g1, err := f.srv.Graph(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	g2, _ := f.srv.Graph(t.Context())
	if g1 != g2 {
		t.Fatal("graph is rebuilt on every request")
	}
	var fp1, fp2 map[string]string
	f.json(t, "/api/fingerprint", &fp1)
	put(t, f.st, store.PutRequest{Key: "reference_fresh", Kind: store.Fact, Title: sp("Fresh"), Body: sp("Fresh arrival.")})
	g3, _ := f.srv.Graph(t.Context())
	if g3 == g1 || len(g3.Records) != len(g1.Records)+1 {
		t.Fatalf("graph did not follow a write: %d -> %d", len(g1.Records), len(g3.Records))
	}
	f.json(t, "/api/fingerprint", &fp2)
	if fp1["fingerprint"] == "" || fp1["fingerprint"] == fp2["fingerprint"] {
		t.Fatalf("fingerprint did not move on a write: %v %v", fp1, fp2)
	}
	if _, err := f.st.DB().Exec("UPDATE records SET status = 'expired' WHERE key = 'project_state'"); err != nil {
		t.Fatal(err)
	}
	var fp3 map[string]string
	f.json(t, "/api/fingerprint", &fp3)
	if fp3["fingerprint"] == fp2["fingerprint"] {
		t.Fatal("fingerprint did not move on a status change")
	}
	if _, err := f.st.SetTopics(t.Context(), []store.TopicAssign{{Key: "reference_fresh", Topic: "moved_here", Title: "Moved here"}}); err != nil {
		t.Fatal(err)
	}
	var fp4 map[string]string
	f.json(t, "/api/fingerprint", &fp4)
	if _, err := f.st.SetTopics(t.Context(), []store.TopicAssign{{Key: "reference_fresh", Topic: "moved_there", Title: "Moved here"}}); err != nil {
		t.Fatal(err)
	}
	var fp5 map[string]string
	f.json(t, "/api/fingerprint", &fp5)
	if fp4["fingerprint"] == fp3["fingerprint"] || fp5["fingerprint"] == fp4["fingerprint"] {
		t.Fatal("fingerprint did not move when a record changed topic")
	}
}

func TestExpiredIsCounted(t *testing.T) {
	f := newFixture(t)
	if _, err := f.st.DB().Exec("UPDATE records SET status = 'expired' WHERE key = 'project_state'"); err != nil {
		t.Fatal(err)
	}
	var h Hygiene
	f.json(t, "/api/hygiene", &h)
	if !slices.Equal(keysOfItems(section(h, "expired").Items), []string{"project_state"}) || len(section(h, "stale").Items) != 0 || !section(h, "expired").Counted {
		t.Fatalf("expired = %+v stale = %+v", section(h, "expired"), section(h, "stale"))
	}
}

func TestVectorStateFeedsOrphans(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	prefix := "Document: "
	var rows []store.VecRow
	for _, k := range []string{"reference_beta", "feedback_alpha"} {
		rec, err := f.st.Get(ctx, k)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, store.VecRow{Key: k, Vec: []float32{1, 0, 0}, TextSHA: store.TextSHA(store.DocText(prefix, rec))})
	}
	rec, _ := f.st.Get(ctx, "reference_gamma")
	rows = append(rows, store.VecRow{Key: "reference_gamma", Vec: []float32{0, 1, 0}, TextSHA: store.TextSHA(store.DocText(prefix, rec))})
	if _, _, err := f.st.PutVectors(ctx, "fake", prefix, rows); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.DB().Exec("UPDATE vectors SET text_sha256 = 'old' WHERE key = 'reference_gamma'"); err != nil {
		t.Fatal(err)
	}
	srv := New(f.st, Options{Now: func() time.Time { return tNow }, EmbedModel: "fake"}, false)
	g, err := srv.Graph(ctx)
	if err != nil {
		t.Fatal(err)
	}
	by := nodesByKey(*g)
	if by["reference_beta"].Vector != "fresh" || by["reference_gamma"].Vector != "stale" || by["reference_lonely"].Vector != "missing" || by["project_big"].Vector != "missing" {
		t.Fatalf("vector states: beta %q gamma %q lonely %q big %q", by["reference_beta"].Vector, by["reference_gamma"].Vector, by["reference_lonely"].Vector, by["project_big"].Vector)
	}
	if by["feedback_old"].Vector != "" {
		t.Fatalf("a superseded record carries a vector state %q", by["feedback_old"].Vector)
	}
	h := srv.hygieneOf(g)
	why := map[string][]string{}
	for _, it := range section(h, "orphans").Items {
		why[it.Key] = it.Why
	}
	if !slices.Equal(why["reference_lonely"], []string{reasonNoTopic, reasonNoVector}) || !slices.Equal(why["reference_gamma"], []string{reasonStaleVec}) || why["reference_beta"] != nil {
		t.Fatalf("orphan reasons: %v", why)
	}
}

func TestLegacyHubTokensResolveToTheirMembers(t *testing.T) {
	f := newFixture(t)
	put(t, f.st, putReq("reference_net_card", "Net card", "Card."))
	put(t, f.st, putReq("reference_net_switch", "Net switch", "Switch."))
	g, err := f.srv.Graph(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	keys, how := g.res.resolve("reference_net_hub")
	if !slices.Equal(keys, []string{"reference_net_card", "reference_net_switch"}) || how != resolvePrefix {
		t.Fatalf("hub token = %v %q", keys, how)
	}
	if !slices.Equal(nodesByKey(*g)["reference_gamma"].Broken, []string{"missing-key"}) {
		t.Fatalf("broken = %v", nodesByKey(*g)["reference_gamma"].Broken)
	}
}

func TestHygieneListsRetiredRecordsUncounted(t *testing.T) {
	f := newFixture(t)
	if _, err := f.st.Retire(t.Context(), "reference_beta", "gone"); err != nil {
		t.Fatal(err)
	}
	var h Hygiene
	f.json(t, "/api/hygiene", &h)
	r := section(h, "retired")
	if r.ID != "retired" || r.Counted || !slices.Equal(keysOfItems(r.Items), []string{"reference_beta"}) {
		t.Fatalf("retired = %+v", r)
	}
}
