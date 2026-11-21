package ui

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"mem/internal/recall"
	"mem/internal/store"
)

func synthGraph(n, topics int, seed string) *Graph {
	g := &Graph{Built: "2026-01-01T00:00:00Z"}
	for t := 0; t < topics; t++ {
		g.Topics = append(g.Topics, TopicOut{ID: "t" + strconv.Itoa(t)})
	}
	weights := make([]float64, topics)
	var sum float64
	for t := range weights {
		weights[t] = 1 / math.Pow(float64(t+1), .9)
		sum += weights[t]
	}
	pick := func(key string) int {
		x := hash01(key, seed+"topic") * sum
		for t, w := range weights {
			if x < w {
				return t
			}
			x -= w
		}
		return topics - 1
	}
	byTopic := make([][]int, topics)
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("%s_%05d", seed, i)
		t := pick(key)
		g.Records = append(g.Records, Node{Key: key, Topic: t})
		byTopic[t] = append(byTopic[t], i)
	}
	for i := range g.Records {
		t := g.Records[i].Topic
		links := int(hash01(g.Records[i].Key, seed+"deg") * 4)
		for l := 0; l < links; l++ {
			var j int
			if hash01(g.Records[i].Key, seed+"x"+strconv.Itoa(l)) < .85 {
				m := byTopic[t]
				j = m[int(hash01(g.Records[i].Key, seed+"j"+strconv.Itoa(l))*float64(len(m)))%len(m)]
			} else {
				j = int(hash01(g.Records[i].Key, seed+"k"+strconv.Itoa(l))*float64(n)) % n
			}
			if j != i {
				g.Records[i].Out = append(g.Records[i].Out, g.Records[j].Key)
			}
		}
	}
	return g
}

func layoutBytes(t *testing.T, d *layoutData) []byte {
	t.Helper()
	raw, _, err := encodeLayout(d)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func overlapCount(in *layoutInput, d *layoutData) (pairs int) {
	type cell [2]int
	grid := map[cell][]int{}
	for i, nd := range in.nodes {
		p := d.Nodes[nd.key]
		c := cell{int(math.Floor(p.X / 24)), int(math.Floor(p.Y / 24))}
		grid[c] = append(grid[c], i)
	}
	for i, nd := range in.nodes {
		p := d.Nodes[nd.key]
		c := cell{int(math.Floor(p.X / 24)), int(math.Floor(p.Y / 24))}
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				for _, j := range grid[cell{c[0] + dx, c[1] + dy}] {
					if j <= i {
						continue
					}
					q := d.Nodes[in.nodes[j].key]
					if math.Hypot(p.X-q.X, p.Y-q.Y) < .8*(nd.r+in.nodes[j].r) {
						pairs++
					}
				}
			}
		}
	}
	return pairs
}

func checkInside(t *testing.T, in *layoutInput, d *layoutData) {
	t.Helper()
	for _, nd := range in.nodes {
		p, ok := d.Nodes[nd.key]
		if !ok {
			t.Fatalf("%s has no coordinates", nd.key)
		}
		tp := d.Topics[p.Topic]
		if r := math.Hypot(p.X-tp.X, p.Y-tp.Y); r > topicRadius(tp.Cap)+.05 {
			t.Fatalf("%s sits %.1f from the centre of %s, R = %.1f", nd.key, r, p.Topic, topicRadius(tp.Cap))
		}
	}
}

func TestLayoutDeterministicBytes(t *testing.T) {
	g := synthGraph(700, 12, "det")
	a := layoutBytes(t, computeFullLayout(newLayoutInput(g, nil)))
	b := layoutBytes(t, computeFullLayout(newLayoutInput(g, nil)))
	if !bytes.Equal(a, b) {
		t.Fatal("two layouts of one input differ")
	}
	rev := append([]Node(nil), g.Records...)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	g2 := *g
	g2.Records = rev
	if c := layoutBytes(t, computeFullLayout(newLayoutInput(&g2, nil))); !bytes.Equal(a, c) {
		t.Fatal("record order changes the layout")
	}
}

func TestLayoutPlacesEveryNodeInsideItsTerritory(t *testing.T) {
	g := synthGraph(1500, 20, "inside")
	in := newLayoutInput(g, nil)
	d := computeFullLayout(in)
	if len(d.Nodes) != len(g.Records) || len(d.Topics) != len(g.Topics) {
		t.Fatalf("layout holds %d nodes and %d topics, graph has %d and %d", len(d.Nodes), len(d.Topics), len(g.Records), len(g.Topics))
	}
	checkInside(t, in, d)
	if n := overlapCount(in, d); float64(n) > .01*float64(len(g.Records)) {
		t.Fatalf("%d overlapping pairs among %d nodes", n, len(g.Records))
	}
	ids := d.topicOrder()
	for i, a := range ids {
		for _, b := range ids[i+1:] {
			ta, tb := d.Topics[a], d.Topics[b]
			if gap := math.Hypot(ta.X-tb.X, ta.Y-tb.Y) - topicRadius(ta.Cap) - topicRadius(tb.Cap); gap < topicGap-1 {
				t.Fatalf("territories %s and %s are %.1f apart", a, b, gap)
			}
		}
	}
}

func extend(g *Graph, extra int, seed string) *Graph {
	out := *g
	out.Records = append([]Node(nil), g.Records...)
	for i := 0; i < extra; i++ {
		key := fmt.Sprintf("%s_new_%04d", seed, i)
		n := Node{Key: key, Topic: int(hash01(key, "topic") * float64(len(g.Topics)))}
		same := []string{}
		for _, r := range g.Records {
			if r.Topic == n.Topic {
				same = append(same, r.Key)
			}
		}
		if len(same) > 0 {
			n.Out = []string{same[int(hash01(key, "link")*float64(len(same)))%len(same)]}
		}
		out.Records = append(out.Records, n)
	}
	return &out
}

func TestIncrementalPlacementMovesNothingExisting(t *testing.T) {
	g := synthGraph(1400, 14, "inc")
	in := newLayoutInput(g, nil)
	d := computeFullLayout(in)
	before := map[string]layoutNode{}
	for k, v := range d.Nodes {
		before[k] = v
	}
	topicsBefore := map[string]layoutTopic{}
	for k, v := range d.Topics {
		topicsBefore[k] = *v
	}
	g2 := extend(g, 100, "inc")
	in2 := newLayoutInput(g2, nil)
	grown := 0
	count := map[string]int{}
	for _, nd := range in2.nodes {
		count[nd.topic]++
	}
	for id, c := range count {
		if capFor(c) > topicsBefore[id].Cap {
			grown++
		}
	}
	res := syncLayout(d, in2)
	if res.placed != 100 {
		t.Fatalf("placed %d of 100", res.placed)
	}
	moved := 0
	for k, v := range before {
		if d.Nodes[k] != v && len(res.bumped) == 0 {
			moved++
		}
	}
	if moved != 0 {
		t.Fatalf("%d existing coordinates changed (bumped topics: %v)", moved, res.bumped)
	}
	if len(res.bumped) != grown {
		t.Fatalf("bumped %v, expected %d", res.bumped, grown)
	}
	checkInside(t, in2, d)
}

func TestCapBumpTranslatesNeighboursRigidly(t *testing.T) {
	g := synthGraph(900, 10, "bump")
	in := newLayoutInput(g, nil)
	d := computeFullLayout(in)
	before := map[string]layoutNode{}
	for k, v := range d.Nodes {
		before[k] = v
	}
	topicsBefore := map[string]layoutTopic{}
	for k, v := range d.Topics {
		topicsBefore[k] = *v
	}
	target := "t0"
	have := 0
	for _, nd := range in.nodes {
		if nd.topic == target {
			have++
		}
	}
	grow := &Graph{Built: g.Built, Topics: g.Topics, Records: append([]Node(nil), g.Records...)}
	for i := 0; i < have; i++ {
		grow.Records = append(grow.Records, Node{Key: fmt.Sprintf("zz_%04d", i), Topic: 0})
	}
	in2 := newLayoutInput(grow, nil)
	res := syncLayout(d, in2)
	if len(res.bumped) == 0 || res.bumped[0] != target {
		t.Fatalf("bumped = %v", res.bumped)
	}
	if d.Topics[target].Cap <= topicsBefore[target].Cap || d.Topics[target].Cap0 != topicsBefore[target].Cap0 {
		t.Fatalf("cap %d -> %d, cap0 %d", topicsBefore[target].Cap, d.Topics[target].Cap, d.Topics[target].Cap0)
	}
	for k, old := range before {
		now := d.Nodes[k]
		ot, nt := topicsBefore[old.Topic], d.Topics[old.Topic]
		if math.Abs((now.X-nt.X)-(old.X-ot.X)) > .11 || math.Abs((now.Y-nt.Y)-(old.Y-ot.Y)) > .11 {
			t.Fatalf("%s changed its offset inside %s", k, old.Topic)
		}
	}
	ids := d.topicOrder()
	for i, a := range ids {
		for _, b := range ids[i+1:] {
			ta, tb := d.Topics[a], d.Topics[b]
			if gap := math.Hypot(ta.X-tb.X, ta.Y-tb.Y) - topicRadius(ta.Cap) - topicRadius(tb.Cap); gap < topicGap-1 {
				t.Fatalf("territories %s and %s are %.1f apart after the bump", a, b, gap)
			}
		}
	}
	checkInside(t, in2, d)
}

func TestNewTopicGetsAFreeTerritory(t *testing.T) {
	g := synthGraph(500, 8, "newtopic")
	d := computeFullLayout(newLayoutInput(g, nil))
	g2 := &Graph{Built: g.Built, Topics: append(append([]TopicOut(nil), g.Topics...), TopicOut{ID: "fresh"}), Records: append([]Node(nil), g.Records...)}
	for i := 0; i < 30; i++ {
		g2.Records = append(g2.Records, Node{Key: fmt.Sprintf("fresh_%02d", i), Topic: 8})
	}
	in := newLayoutInput(g2, nil)
	syncLayout(d, in)
	fresh := d.Topics["fresh"]
	if fresh == nil {
		t.Fatal("no territory for the new topic")
	}
	for id, o := range d.Topics {
		if id == "fresh" {
			continue
		}
		if gap := math.Hypot(fresh.X-o.X, fresh.Y-o.Y) - topicRadius(fresh.Cap) - topicRadius(o.Cap); gap < topicGap-1 {
			t.Fatalf("new territory is %.1f from %s", gap, id)
		}
	}
	checkInside(t, in, d)
}

func TestRemovedNodesAreForgottenAndRestoredOnesReplaced(t *testing.T) {
	g := synthGraph(300, 6, "rm")
	d := computeFullLayout(newLayoutInput(g, nil))
	g2 := &Graph{Built: g.Built, Topics: g.Topics, Records: g.Records[:200]}
	res := syncLayout(d, newLayoutInput(g2, nil))
	if res.removed != 100 || len(d.Nodes) != 200 {
		t.Fatalf("removed %d, %d nodes left", res.removed, len(d.Nodes))
	}
}

func TestCapSeries(t *testing.T) {
	prev := 0
	for n := 0; n < 3000; n++ {
		c := capFor(n)
		if c < n || c < prev {
			t.Fatalf("capFor(%d) = %d after %d", n, c, prev)
		}
		prev = c
	}
	if capFor(0) != 8 || capFor(7) != 8 {
		t.Fatalf("capFor small = %d %d", capFor(0), capFor(7))
	}
	if capSteps(8, 8) != 0 || capSteps(8, capFor(100)) < 2 || capSteps(capFor(100), 8) != 0 {
		t.Fatalf("capSteps = %d %d", capSteps(8, capFor(100)), capSteps(capFor(100), 8))
	}
	d := newLayoutData()
	d.Topics["a"] = &layoutTopic{Cap: capFor(10), Cap0: capFor(10)}
	d.Topics["b"] = &layoutTopic{Cap: capFor(60), Cap0: capFor(10)}
	if d.staleTopics() != 1 {
		t.Fatalf("stale topics = %d", d.staleTopics())
	}
}

func TestLayoutFileRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "l.json")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if readLayoutFile(filepath.Join(dir, "missing.json")) != nil {
		t.Fatal("a missing file loaded")
	}
	for name, body := range map[string]string{
		"empty": "", "garbage": "{not json", "wrong version": `{"v":1,"seed":1,"topics":{},"nodes":{}}`,
		"no maps": `{"v":2}`, "bad node": `{"v":2,"topics":{"a":{"x":0,"y":0,"cap":8,"c0":8}},"nodes":{"k":[1,2]}}`,
		"orphan node": `{"v":2,"topics":{},"nodes":{"k":[1,2,"gone"]}}`, "zero cap": `{"v":2,"topics":{"a":{"x":0,"y":0,"cap":0}},"nodes":{}}`,
	} {
		if readLayoutFile(write(body)) != nil {
			t.Fatalf("%s loaded", name)
		}
	}
	for name, body := range map[string]string{
		"null coordinates": `{"v":2,"seed":1,"topics":{"a":{"x":1,"y":2,"cap":8,"c0":8}},"nodes":{"k":[null,null,"a"]}}`,
		"null topic":       `{"v":2,"seed":1,"topics":{"a":{"x":1,"y":2,"cap":8,"c0":8}},"nodes":{"k":[1,2,null]}}`,
		"null topic x":     `{"v":2,"seed":1,"topics":{"a":{"x":null,"y":2,"cap":8,"c0":8}},"nodes":{}}`,
		"missing cap":      `{"v":2,"seed":1,"topics":{"a":{"x":1,"y":2}},"nodes":{}}`,
		"outside disc":     `{"v":2,"seed":1,"topics":{"a":{"x":1,"y":2,"cap":8,"c0":8}},"nodes":{"k":[900,2,"a"]}}`,
		"null node":        `{"v":2,"seed":1,"topics":{"a":{"x":1,"y":2,"cap":8,"c0":8}},"nodes":{"k":null}}`,
	} {
		if readLayoutFile(write(body)) != nil {
			t.Fatalf("%s loaded", name)
		}
	}
	if readLayoutFile(write(`{"v":2,"seed":1,"topics":{"a":{"x":1,"y":2,"cap":8,"c0":8}},"nodes":{"k":[1.5,2.5,"a"]}}`)) == nil {
		t.Fatal("a valid file was rejected")
	}
}

func TestLayoutFileRoundTripAndAtomicity(t *testing.T) {
	g := synthGraph(400, 7, "rt")
	d := computeFullLayout(newLayoutInput(g, nil))
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", layoutFileName("/some/mem.db"))
	raw := layoutBytes(t, d)
	if err := writeLayoutFile(path, raw); err != nil {
		t.Fatal(err)
	}
	back := readLayoutFile(path)
	if back == nil || !bytes.Equal(layoutBytes(t, back), raw) {
		t.Fatal("round trip changed the layout")
	}
	ents, _ := os.ReadDir(filepath.Dir(path))
	if len(ents) != 1 {
		t.Fatalf("temporary files left behind: %v", ents)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
	a1, a2, b := layoutFileName("/a/mem.db"), layoutFileName("/a/mem.db"), layoutFileName("/b/mem.db")
	if a1 == b || a1 != a2 {
		t.Fatal("file name is not keyed by the database path")
	}
}

func layoutFixture(t *testing.T, state string, relayout bool) *fixture {
	t.Helper()
	return layoutServer(newFixture(t), state, relayout)
}

func layoutServer(f *fixture, state string, relayout bool) *fixture {
	srv := New(f.st, Options{Host: "smok", Home: "/home/x", Now: func() time.Time { return tNow }, StateDir: state, Relayout: relayout}, false)
	return &fixture{st: f.st, srv: srv, h: srv.Handler()}
}

type apiGraph struct {
	Layout  layoutOut  `json:"layout"`
	Topics  []apiTopic `json:"topics"`
	Records []apiNode  `json:"records"`
}

type apiTopic struct {
	ID   string    `json:"id"`
	X    *float64  `json:"x"`
	Y    *float64  `json:"y"`
	R    float64   `json:"R"`
	Hull []float64 `json:"hull"`
	LX   float64   `json:"lx"`
	LY   float64   `json:"ly"`
	Cap  int       `json:"cap"`
}

type apiNode struct {
	K  string   `json:"k"`
	O  []string `json:"o"`
	RL string   `json:"rl"`
	X  []string `json:"x"`
	PX *float64 `json:"px"`
	PY *float64 `json:"py"`
}

func (f *fixture) graph(t *testing.T) apiGraph {
	t.Helper()
	var g apiGraph
	f.json(t, "/api/graph", &g)
	return g
}

func TestGraphAPICarriesLayoutAdditively(t *testing.T) {
	f := layoutFixture(t, t.TempDir(), false)
	if err := f.st.AddLinks(context.Background(), []store.Link{
		{Src: "user_owner", Dst: "reference_lonely", Rel: store.Relates},
		{Src: "user_owner", Dst: "reference_lonely", Rel: store.DerivedFrom, Note: "quote"},
		{Src: "user_owner", Dst: "project_state", Rel: store.Supersedes},
		{Src: "user_owner", Dst: "project_state", Rel: store.Relates},
	}); err != nil {
		t.Fatal(err)
	}
	g := f.graph(t)
	if g.Layout.State != "ready" || len(g.Layout.Rev) != 16 {
		t.Fatalf("layout = %+v", g.Layout)
	}
	if len(g.Records) != 9 {
		t.Fatalf("records = %d", len(g.Records))
	}
	for _, n := range g.Records {
		if n.PX == nil || n.PY == nil {
			t.Fatalf("%s has no coordinates", n.K)
		}
		if len(n.RL) != len(n.O) {
			t.Fatalf("%s: rl %q does not match o %v", n.K, n.RL, n.O)
		}
	}
	for _, tp := range g.Topics {
		if tp.X == nil || tp.Y == nil || tp.R <= 0 || len(tp.Hull) != hullBins || tp.Cap < 8 {
			t.Fatalf("topic %s = %+v", tp.ID, tp)
		}
		for _, h := range tp.Hull {
			if h < .7*tp.R-.1 || h > tp.R+.1 {
				t.Fatalf("hull %v outside [0.7R, R] of %s (R=%.1f)", h, tp.ID, tp.R)
			}
		}
	}
	var legacy Graph
	f.json(t, "/api/graph", &legacy)
	if len(legacy.Records) != 9 || legacy.Records[0].Key == "" {
		t.Fatal("legacy fields are gone")
	}
	by := map[string]apiNode{}
	for _, n := range g.Records {
		by[n.K] = n
	}
	if n := by["feedback_new"]; len(n.O) != 2 || n.O[1] != "feedback_old" || n.RL != "rs" {
		t.Fatalf("feedback_new = %+v", n)
	}
	if n := by["user_owner"]; len(n.O) != 2 || n.RL != "ds" {
		t.Fatalf("user_owner = %+v, supersedes and derived_from must outrank relates in either insertion order", n)
	}
	if len(by["reference_gamma"].X) == 0 {
		t.Fatal("unresolved links left the x field")
	}
}

func TestGraphAPILayoutIsPersistedAndReused(t *testing.T) {
	state := t.TempDir()
	f := layoutFixture(t, state, false)
	first := f.graph(t)
	path := filepath.Join(state, layoutFileName(f.st.Path()))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var d layoutData
	if err := json.Unmarshal(raw, &d); err != nil || !d.valid() || len(d.Nodes) != 9 {
		t.Fatalf("persisted layout: %v %+v", err, d)
	}
	second := layoutServer(f, state, false)
	again := second.graph(t)
	if again.Layout.Rev != first.Layout.Rev {
		t.Fatalf("rev %s then %s", first.Layout.Rev, again.Layout.Rev)
	}
	for i := range first.Records {
		if *first.Records[i].PX != *again.Records[i].PX || *first.Records[i].PY != *again.Records[i].PY {
			t.Fatalf("%s moved between two servers", first.Records[i].K)
		}
	}
}

func TestGraphAPIRecoversFromBadLayoutFiles(t *testing.T) {
	for name, body := range map[string]string{
		"corrupt": "{{{", "wrong version": `{"v":9,"seed":1,"topics":{},"nodes":{}}`, "empty": "",
	} {
		state := t.TempDir()
		f := layoutFixture(t, state, false)
		path := filepath.Join(state, layoutFileName(f.st.Path()))
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		g := f.graph(t)
		if g.Layout.State != "ready" || g.Records[0].PX == nil {
			t.Fatalf("%s: layout = %+v", name, g.Layout)
		}
		if readLayoutFile(path) == nil {
			t.Fatalf("%s: the file was not rewritten", name)
		}
	}
	state := t.TempDir()
	f := layoutFixture(t, state, false)
	if g := f.graph(t); g.Layout.State != "ready" {
		t.Fatalf("missing: %+v", g.Layout)
	}
}

func TestRelayoutIgnoresTheStoredFile(t *testing.T) {
	state := t.TempDir()
	f := layoutFixture(t, state, false)
	_ = f.graph(t)
	path := filepath.Join(state, layoutFileName(f.st.Path()))
	d := readLayoutFile(path)
	for k, n := range d.Nodes {
		n.X += 500
		d.Nodes[k] = n
	}
	for _, tp := range d.Topics {
		tp.X += 500
	}
	if err := writeLayoutFile(path, layoutBytes(t, d)); err != nil {
		t.Fatal(err)
	}
	kept := layoutServer(f, state, false).graph(t)
	fresh := layoutServer(f, state, true).graph(t)
	if *kept.Records[0].PX == *fresh.Records[0].PX {
		t.Fatal("--relayout reused the stored coordinates")
	}
	if back := readLayoutFile(path); back == nil || math.Abs(back.Nodes[fresh.Records[0].K].X-*fresh.Records[0].PX) > .11 {
		t.Fatal("--relayout did not rewrite the file")
	}
}

func TestLayoutSaveIsDebouncedAndOnlyOnChange(t *testing.T) {
	state := t.TempDir()
	f := layoutFixture(t, state, false)
	_ = f.graph(t)
	path := filepath.Join(state, layoutFileName(f.st.Path()))
	info, _ := os.Stat(path)
	f.srv.lay.delay = 80 * time.Millisecond
	_ = f.graph(t)
	info2, _ := os.Stat(path)
	if !info2.ModTime().Equal(info.ModTime()) {
		t.Fatal("an unchanged graph rewrote the file")
	}
	put(t, f.st, store.PutRequest{Key: "reference_added", Kind: store.Fact, Title: sp("Added"), Body: sp("Brand new fact.")})
	g := f.graph(t)
	if len(g.Records) != 10 || g.Records[len(g.Records)-1].PX == nil && g.Records[0].PX == nil {
		t.Fatalf("records = %d", len(g.Records))
	}
	before := readLayoutFile(path)
	if _, ok := before.Nodes["reference_added"]; ok {
		t.Fatal("the incremental change was written at once")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if d := readLayoutFile(path); d != nil {
			if _, ok := d.Nodes["reference_added"]; ok {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the debounced write never landed")
}

func TestGraphWithoutAStateDirStillCarriesCoordinates(t *testing.T) {
	f := layoutFixture(t, "", false)
	if g := f.graph(t); g.Layout.State != "ready" || g.Records[0].PX == nil {
		t.Fatalf("layout = %+v", g.Layout)
	}
}

func TestColdLayoutReportsBuildingState(t *testing.T) {
	f := layoutFixture(t, t.TempDir(), false)
	g, err := f.srv.Graph(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m := f.srv.lay
	m.mu.Lock()
	m.load()
	m.building = make(chan struct{})
	m.mu.Unlock()
	view, state := f.srv.layoutFor(context.Background(), g)
	if view != nil || state == nil || state.State != "building" {
		t.Fatalf("view=%v state=%v", view, state)
	}
	var out map[string]json.RawMessage
	f.json(t, "/api/graph", &out)
	var lay layoutOut
	_ = json.Unmarshal(out["layout"], &lay)
	if lay.State != "building" {
		t.Fatalf("layout = %s", out["layout"])
	}
	var recs []map[string]any
	_ = json.Unmarshal(out["records"], &recs)
	if len(recs) != 9 || recs[0]["px"] != nil {
		t.Fatal("a building layout must still carry the records, without coordinates")
	}
}

func TestGzipCompressesJSONAndLeavesOthersAlone(t *testing.T) {
	f := newFixture(t)
	get := func(target, enc string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Host = "127.0.0.1:7781"
		if enc != "" {
			req.Header.Set("Accept-Encoding", enc)
		}
		rec := httptest.NewRecorder()
		f.h.ServeHTTP(rec, req)
		return rec
	}
	plain := get("/api/graph", "")
	if plain.Header().Get("Content-Encoding") != "" {
		t.Fatal("compressed without being asked")
	}
	if plain.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatalf("plain answer lacks Vary: %v", plain.Header())
	}
	for _, enc := range []string{"gzip", "br, gzip;q=0.8", "GZIP"} {
		zipped := get("/api/graph", enc)
		if zipped.Header().Get("Content-Encoding") != "gzip" || zipped.Header().Get("Vary") != "Accept-Encoding" {
			t.Fatalf("%q: headers %v", enc, zipped.Header())
		}
		zr, err := gzip.NewReader(zipped.Body)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(zr)
		var a, b map[string]json.RawMessage
		if json.Unmarshal(body, &a) != nil || json.Unmarshal(plain.Body.Bytes(), &b) != nil || string(a["records"]) != string(b["records"]) {
			t.Fatalf("%q: gzip body differs from the plain one", enc)
		}
	}
	for _, enc := range []string{"gzip;q=0", "identity", "br"} {
		if get("/api/graph", enc).Header().Get("Content-Encoding") != "" {
			t.Fatalf("%q was compressed", enc)
		}
	}
	for _, p := range []string{"/app.js", "/app.css", "/"} {
		if rec := get(p, "gzip"); rec.Header().Get("Content-Encoding") != "gzip" || rec.Code != 200 {
			t.Fatalf("%s: %d %v", p, rec.Code, rec.Header())
		}
	}
	if rec := get("/api/detail?key=nope", "gzip"); rec.Code == 200 {
		t.Fatal("missing key answered 200")
	}
	head := httptest.NewRequest(http.MethodHead, "/api/graph", nil)
	head.Host = "127.0.0.1:7781"
	head.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, head)
	if rec.Header().Get("Content-Encoding") != "" {
		t.Fatal("HEAD was compressed")
	}
}

func TestGzipPassesStreamsThroughWithFlush(t *testing.T) {
	h := gzipMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: x\n\n"))
		w.(http.Flusher).Flush()
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") != "" || rec.Body.String() != "data: x\n\n" || !rec.Flushed {
		t.Fatalf("%v %q flushed=%v", rec.Header(), rec.Body.String(), rec.Flushed)
	}
}

func TestMeanVectorsFoldChunksIntoOneNormalisedVector(t *testing.T) {
	got := meanVectors([]recall.Entry{{Key: "a#1", Vec: []float32{3, 0}}, {Key: "a#2", Vec: []float32{0, 4}}, {Key: "b", Vec: []float32{0, 0}}})
	v := got["a"]
	if len(v) != 2 || math.Abs(float64(v[0])-.6) > 1e-6 || math.Abs(float64(v[1])-.8) > 1e-6 {
		t.Fatalf("a = %v", v)
	}
	if _, ok := got["b"]; ok {
		t.Fatal("a zero vector was kept")
	}
}

func TestIncrementalPlacementUsesVectorNeighbours(t *testing.T) {
	g := synthGraph(240, 3, "vec")
	vecs := map[string][]float32{}
	for _, r := range g.Records {
		v := make([]float32, 8)
		v[r.Topic%8] = 1
		v[4+int(hash01(r.Key, "c")*3)] += 1
		vecs[r.Key] = v
	}
	for i := range g.Records {
		g.Records[i].Out = nil
	}
	in := newLayoutInput(g, vecs)
	d := computeFullLayout(in)
	checkInside(t, in, d)
	g2 := extend(g, 20, "vec")
	vecs2 := map[string][]float32{}
	for k, v := range vecs {
		vecs2[k] = v
	}
	for _, r := range g2.Records[len(g.Records):] {
		vecs2[r.Key] = vecs[g.Records[0].Key]
	}
	in2 := newLayoutInput(g2, vecs2)
	syncLayout(d, in2)
	checkInside(t, in2, d)
}

func realGraph(t *testing.T) (*Graph, *store.Store) {
	t.Helper()
	path := os.Getenv("LAYOUT_REAL_DB")
	if path == "" {
		t.Skip("LAYOUT_REAL_DB is not set")
	}
	st, err := store.Open(context.Background(), path, store.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	g, err := Build(context.Background(), st, Options{Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	return g, st
}

func scaled(g *Graph, k int) *Graph {
	if k <= 1 {
		return g
	}
	out := &Graph{Built: g.Built, Topics: g.Topics}
	for c := 0; c < k; c++ {
		for _, r := range g.Records {
			n := r
			n.Key = fmt.Sprintf("%s~%d", r.Key, c)
			n.Out = nil
			for _, o := range r.Out {
				n.Out = append(n.Out, fmt.Sprintf("%s~%d", o, c))
			}
			out.Records = append(out.Records, n)
		}
	}
	return out
}

func TestRealDataLayout(t *testing.T) {
	g, _ := realGraph(t)
	in := newLayoutInput(g, nil)
	t0 := time.Now()
	d := computeFullLayout(in)
	cold := time.Since(t0)
	checkInside(t, in, d)
	pairs := overlapCount(in, d)
	d2 := computeFullLayout(newLayoutInput(g, nil))
	same := bytes.Equal(layoutBytes(t, d), layoutBytes(t, d2))
	raw := layoutBytes(t, d)
	var zb bytes.Buffer
	zw := gzip.NewWriter(&zb)
	_, _ = zw.Write(raw)
	_ = zw.Close()
	t.Logf("real: %d nodes %d topics cold=%v overlaps=%d (%.3f%% of nodes) deterministic=%v file=%d B gzip=%d B", len(in.nodes), len(d.Topics), cold, pairs, 100*float64(pairs)/float64(len(in.nodes)), same, len(raw), zb.Len())
	if cold > time.Second {
		t.Errorf("cold layout took %v", cold)
	}
	if !same {
		t.Error("not deterministic")
	}
	var worst float64
	for _, nd := range in.nodes {
		p := d.Nodes[nd.key]
		tp := d.Topics[p.Topic]
		worst = math.Max(worst, math.Hypot(p.X-tp.X, p.Y-tp.Y)/topicRadius(tp.Cap))
	}
	t.Logf("real: farthest node at %.3f R", worst)
	var keys []string
	for k := range d.Nodes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rem := map[string]bool{}
	for _, k := range keys {
		if hash01(k, "rm") < .02 {
			rem[k] = true
		}
	}
	g2 := &Graph{Built: g.Built, Topics: g.Topics}
	for _, r := range g.Records {
		if !rem[r.Key] {
			g2.Records = append(g2.Records, r)
		}
	}
	in2 := newLayoutInput(g2, nil)
	d3 := computeFullLayout(in2)
	before := map[string]layoutNode{}
	for k, v := range d3.Nodes {
		before[k] = v
	}
	t0 = time.Now()
	res := syncLayout(d3, in)
	inc := time.Since(t0)
	moved := 0
	for k, v := range before {
		if d3.Nodes[k] != v {
			moved++
		}
	}
	t.Logf("real incremental: placed %d in %v (%.3f ms/node) bumped=%v moved existing=%d", res.placed, inc, float64(inc.Microseconds())/1000/float64(max(1, res.placed)), res.bumped, moved)
	if per := float64(inc.Microseconds()) / 1000 / float64(max(1, res.placed)); per > 2 {
		t.Errorf("incremental %.2f ms/node", per)
	}
	if moved != 0 && len(res.bumped) == 0 {
		t.Errorf("%d existing nodes moved", moved)
	}
}

func TestRealDataScaledLayout(t *testing.T) {
	g, _ := realGraph(t)
	if os.Getenv("LAYOUT_SCALE") == "" {
		t.Skip("LAYOUT_SCALE is not set")
	}
	k, _ := strconv.Atoi(os.Getenv("LAYOUT_SCALE"))
	big := scaled(g, k)
	in := newLayoutInput(big, nil)
	t0 := time.Now()
	d := computeFullLayout(in)
	cold := time.Since(t0)
	checkInside(t, in, d)
	t.Logf("scaled x%d: %d nodes cold=%v overlaps=%d", k, len(in.nodes), cold, overlapCount(in, d))
	if cold > 12*time.Second {
		t.Errorf("cold layout took %v", cold)
	}
}

func TestOneStepBumpMovesNothingExisting(t *testing.T) {
	g := synthGraph(900, 10, "step")
	in := newLayoutInput(g, nil)
	d := computeFullLayout(in)
	before := map[string]layoutNode{}
	for k, v := range d.Nodes {
		before[k] = v
	}
	topicsBefore := map[string]layoutTopic{}
	for k, v := range d.Topics {
		topicsBefore[k] = *v
	}
	have := in.size["t3"]
	want := 0
	for n := have; ; n++ {
		if capFor(n) > d.Topics["t3"].Cap {
			want = n
			break
		}
	}
	if capFor(want) != capNext(d.Topics["t3"].Cap) {
		t.Fatalf("the test needs a single series step: %d -> %d", d.Topics["t3"].Cap, capFor(want))
	}
	grow := &Graph{Built: g.Built, Topics: g.Topics, Records: append([]Node(nil), g.Records...)}
	for i := 0; i < want-have; i++ {
		grow.Records = append(grow.Records, Node{Key: fmt.Sprintf("zz_%04d", i), Topic: 3})
	}
	res := syncLayout(d, newLayoutInput(grow, nil))
	if len(res.bumped) != 1 || res.bumped[0] != "t3" || res.moved != 0 {
		t.Fatalf("bumped %v, moved %d", res.bumped, res.moved)
	}
	for k, v := range before {
		if d.Nodes[k] != v {
			t.Fatalf("%s moved on a one-step bump", k)
		}
	}
	for k, v := range topicsBefore {
		if d.Topics[k].X != v.X || d.Topics[k].Y != v.Y {
			t.Fatalf("territory %s moved", k)
		}
	}
	if d.staleTopics() != 0 {
		t.Fatal("one step is not stale")
	}
}

func growTopic(g *Graph, topic int, to, serial int) *Graph {
	out := &Graph{Built: g.Built, Topics: g.Topics, Records: append([]Node(nil), g.Records...)}
	have := 0
	for _, r := range g.Records {
		if r.Topic == topic {
			have++
		}
	}
	for i := have; i < to; i++ {
		out.Records = append(out.Records, Node{Key: fmt.Sprintf("zz_%d_%04d", serial, i), Topic: topic})
	}
	return out
}

func TestConsecutiveBumpsReReserveRoom(t *testing.T) {
	g := synthGraph(900, 10, "chain")
	d := computeFullLayout(newLayoutInput(g, nil))
	size := newLayoutInput(g, nil).size["t3"]
	prevMoved := -1
	for step := 0; step < 7; step++ {
		cap0 := d.Topics["t3"].Cap
		to := size
		for capFor(to) == cap0 {
			to++
		}
		size = to
		g = growTopic(g, 3, to, step)
		in := newLayoutInput(g, nil)
		before := map[string]layoutNode{}
		for k, v := range d.Nodes {
			before[k] = v
		}
		res := syncLayout(d, in)
		if len(res.bumped) != 1 || res.bumped[0] != "t3" {
			t.Fatalf("step %d: bumped %v", step, res.bumped)
		}
		if prevMoved > 0 && res.moved+res.relocated != 0 {
			t.Fatalf("step %d moved %d and relocated %d territories right after a step that already re-reserved room", step, res.moved, res.relocated)
		}
		if res.moved+res.relocated == 0 {
			for k, v := range before {
				if d.Nodes[k] != v {
					t.Fatalf("step %d: %s moved on a free bump", step, k)
				}
			}
		}
		t.Logf("step %d: cap %d -> %d pushed %d relocated %d shifted %d", step, cap0, d.Topics["t3"].Cap, res.moved, res.relocated, res.shifted)
		prevMoved = res.moved + res.relocated
		ids := d.topicOrder()
		for i, a := range ids {
			for _, b := range ids[i+1:] {
				ta, tb := d.Topics[a], d.Topics[b]
				if gap := math.Hypot(ta.X-tb.X, ta.Y-tb.Y) - topicRadius(ta.Cap) - topicRadius(tb.Cap); gap < topicGap-1 {
					t.Fatalf("step %d: %s and %s are %.1f apart", step, a, b, gap)
				}
			}
		}
	}
}

func TestLayoutFileRejectsHugeCoordinates(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"node x":  `{"v":2,"seed":1,"topics":{"a":{"x":1,"y":2,"cap":8,"c0":8}},"nodes":{"k":[1e300,2,"a"]}}`,
		"node y":  `{"v":2,"seed":1,"topics":{"a":{"x":1,"y":2,"cap":8,"c0":8}},"nodes":{"k":[1,-1e30,"a"]}}`,
		"topic x": `{"v":2,"seed":1,"topics":{"a":{"x":1e9,"y":2,"cap":8,"c0":8}},"nodes":{}}`,
		"cap":     `{"v":2,"seed":1,"topics":{"a":{"x":1,"y":2,"cap":99999999999,"c0":8}},"nodes":{}}`,
	} {
		p := filepath.Join(dir, "l.json")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if readLayoutFile(p) != nil {
			t.Fatalf("%s loaded", name)
		}
	}
}

func TestGraphAPIAnswersWithAHugeCoordinateFile(t *testing.T) {
	state := t.TempDir()
	f := layoutFixture(t, state, false)
	path := filepath.Join(state, layoutFileName(f.st.Path()))
	body := `{"v":2,"seed":1,"topics":{"a":{"x":1,"y":2,"cap":8,"c0":8}},"nodes":{"user_owner":[1e300,2,"a"]}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	g := f.graph(t)
	if g.Layout.State != "ready" || len(g.Records) != 9 || g.Records[0].PX == nil {
		t.Fatalf("layout = %+v", g.Layout)
	}
}

func TestSparseLayoutCountsAsStale(t *testing.T) {
	big := synthGraph(400, 6, "sparse")
	empty := newLayoutData()
	if !sparseLayout(empty, big) {
		t.Fatal("an empty layout is not sparse for 400 records")
	}
	if sparseLayout(computeFullLayout(newLayoutInput(big, nil)), big) {
		t.Fatal("a complete layout is sparse")
	}
	if sparseLayout(empty, synthGraph(15, 3, "tiny")) {
		t.Fatal("a handful of records must stay incremental")
	}
	half := computeFullLayout(newLayoutInput(big, nil))
	n := 0
	for k := range half.Nodes {
		if n%2 == 0 {
			delete(half.Nodes, k)
		}
		n++
	}
	if sparseLayout(half, big) {
		t.Fatal("a half-covered layout should still grow incrementally")
	}
}

func TestGraphAPIRebuildsAnEmptyLayoutFileOnABigDB(t *testing.T) {
	state := t.TempDir()
	f := layoutFixture(t, state, false)
	for i := 0; i < 12; i++ {
		put(t, f.st, store.PutRequest{Key: fmt.Sprintf("reference_bulk_%02d", i), Kind: store.Fact, Title: sp("Bulk"), Body: sp("Imported later.")})
	}
	path := filepath.Join(state, layoutFileName(f.st.Path()))
	if err := os.WriteFile(path, []byte(`{"v":2,"seed":20261108,"topics":{},"nodes":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	g := f.graph(t)
	if g.Layout.State != "ready" || len(g.Records) != 21 {
		t.Fatalf("layout = %+v, records %d", g.Layout, len(g.Records))
	}
	gr, err := f.srv.Graph(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := computeFullLayout(newLayoutInput(gr, f.srv.layoutVectors(gr)))
	got := readLayoutFile(path)
	if got == nil || !bytes.Equal(layoutBytes(t, got), layoutBytes(t, want)) {
		t.Fatal("the sparse file was patched incrementally instead of rebuilt")
	}
}

func TestLayoutFileNameIgnoresRelativeSpelling(t *testing.T) {
	abs, err := filepath.Abs("rel/mem.db")
	if err != nil {
		t.Fatal(err)
	}
	if layoutFileName("rel/mem.db") != layoutFileName(abs) || layoutFileName("./rel/../rel/mem.db") != layoutFileName(abs) {
		t.Fatal("a relative and an absolute spelling of one path got different files")
	}
}

func TestGraphOutIsSafeWhileTheLayoutSyncs(t *testing.T) {
	f := layoutFixture(t, t.TempDir(), false)
	ctx := context.Background()
	g1, err := f.srv.Graph(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.srv.graphOut(ctx, g1)
	stop := make(chan struct{})
	done := make(chan struct{})
	for range 3 {
		go func() {
			defer func() { done <- struct{}{} }()
			for {
				select {
				case <-stop:
					return
				default:
					_ = f.srv.graphOut(ctx, g1)
				}
			}
		}()
	}
	for i := 0; i < 6; i++ {
		put(t, f.st, store.PutRequest{Key: fmt.Sprintf("reference_race_%02d", i), Kind: store.Fact, Title: sp("Race"), Body: sp("Concurrent.")})
		g2, err := f.srv.Graph(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_ = f.srv.graphOut(ctx, g2)
	}
	close(stop)
	for range 3 {
		<-done
	}
}

func movedAfterGrowth(t *testing.T, g *Graph, grow map[string]int) (res syncResult, movedNodes, movedTopics, total int, d *layoutData, in2 *layoutInput) {
	t.Helper()
	d = computeFullLayout(newLayoutInput(g, nil))
	before := map[string]layoutNode{}
	for k, v := range d.Nodes {
		before[k] = v
	}
	tb := map[string]layoutTopic{}
	for k, v := range d.Topics {
		tb[k] = *v
	}
	idx := map[string]int{}
	for i, tp := range g.Topics {
		idx[tp.ID] = i
	}
	out := &Graph{Built: g.Built, Topics: g.Topics, Records: append([]Node(nil), g.Records...)}
	for id, n := range grow {
		for i := 0; i < n; i++ {
			out.Records = append(out.Records, Node{Key: fmt.Sprintf("zz_%s_%04d", id, i), Topic: idx[id]})
		}
	}
	in2 = newLayoutInput(out, nil)
	res = syncLayout(d, in2)
	for k, v := range before {
		if d.Nodes[k] != v {
			movedNodes++
		}
	}
	for k, v := range tb {
		if d.Topics[k].X != v.X || d.Topics[k].Y != v.Y {
			movedTopics++
		}
	}
	return res, movedNodes, movedTopics, len(before), d, in2
}

func TestCollidingBumpMovesOnlyALocalChain(t *testing.T) {
	g := synthGraph(3000, 40, "local")
	small := ""
	var topics []string
	for _, tp := range g.Topics {
		topics = append(topics, tp.ID)
	}
	in := newLayoutInput(g, nil)
	for _, id := range topics {
		if in.size[id] > 0 && in.size[id] <= 12 {
			small = id
		}
	}
	if small == "" {
		t.Skip("no small topic")
	}
	res, mn, mt, total, d, in2 := movedAfterGrowth(t, g, map[string]int{small: 300})
	if len(res.bumped) == 0 {
		t.Fatal("no bump")
	}
	t.Logf("bump %v: moved %d of %d territories, %d of %d nodes", res.bumped, mt, len(d.Topics), mn, total)
	if res.moved+res.relocated == 0 {
		t.Fatal("the test needs a colliding bump")
	}
	if mt*3 > len(d.Topics) {
		t.Errorf("%d of %d territories moved", mt, len(d.Topics))
	}
	if mn*2 > total {
		t.Errorf("%d of %d nodes moved", mn, total)
	}
	ids := d.topicOrder()
	for i, a := range ids {
		for _, b := range ids[i+1:] {
			ta, tb := d.Topics[a], d.Topics[b]
			if gap := math.Hypot(ta.X-tb.X, ta.Y-tb.Y) - topicRadius(ta.Cap) - topicRadius(tb.Cap); gap < topicGap-1 {
				t.Fatalf("territories %s and %s are %.1f apart", a, b, gap)
			}
		}
	}
	checkInside(t, in2, d)
}

func TestRealDataCollidingBumpStaysLocal(t *testing.T) {
	g, _ := realGraph(t)
	res, mn, mt, total, d, in2 := movedAfterGrowth(t, g, map[string]int{"misc": 25, "infra_misc_tools": 25, "kernel_frag": 25, "llm": 25})
	t.Logf("real: bumped %v territory moves %d: %d of %d territories, %d of %d nodes moved", res.bumped, res.moved, mt, len(d.Topics), mn, total)
	checkInside(t, in2, d)
	if mn*10 > total {
		t.Errorf("%d of %d nodes moved", mn, total)
	}
}

func TestRealDataBurstIntoAnyTopicStaysLocal(t *testing.T) {
	g, _ := realGraph(t)
	burst := 100
	if v, err := strconv.Atoi(os.Getenv("LAYOUT_BURST")); err == nil && v > 0 {
		burst = v
	}
	base := computeFullLayout(newLayoutInput(g, nil))
	idx := map[string]int{}
	for i, tp := range g.Topics {
		idx[tp.ID] = i
	}
	worstFrac, worstTopic := 0.0, ""
	for _, id := range base.topicOrder() {
		d := &layoutData{V: base.V, Seed: base.Seed, Topics: map[string]*layoutTopic{}, Nodes: map[string]layoutNode{}}
		for k, v := range base.Topics {
			c := *v
			d.Topics[k] = &c
		}
		for k, v := range base.Nodes {
			d.Nodes[k] = v
		}
		out := &Graph{Built: g.Built, Topics: g.Topics, Records: append([]Node(nil), g.Records...)}
		for i := 0; i < burst; i++ {
			out.Records = append(out.Records, Node{Key: fmt.Sprintf("zz_%s_%04d", id, i), Topic: idx[id]})
		}
		in2 := newLayoutInput(out, nil)
		res := syncLayout(d, in2)
		moved, foreign := 0, 0
		for k, v := range base.Nodes {
			if d.Nodes[k] != v {
				moved++
				if v.Topic != id {
					foreign++
				}
			}
		}
		if foreign != 0 {
			t.Errorf("%s: %d nodes of other territories moved", id, foreign)
		}
		frac := float64(moved) / float64(len(base.Nodes))
		if frac > worstFrac {
			worstFrac, worstTopic = frac, id
		}
		if moved > 0 || os.Getenv("LAYOUT_VERBOSE") != "" {
			t.Logf("%s: size %d cap %d->%d bumped=%v pushed=%d relocated=%d shifted=%d moved %d (%.1f%%)", id, sizeOfTopic(g, idx[id]), base.Topics[id].Cap, d.Topics[id].Cap, res.bumped, res.moved, res.relocated, res.shifted, moved, 100*frac)
		}
		checkInside(t, in2, d)
		if pairs := overlapCount(in2, d); pairs != 0 {
			t.Errorf("%s: %d overlaps", id, pairs)
		}
	}
	t.Logf("worst %s %.2f%%", worstTopic, 100*worstFrac)
	if worstFrac > .1 {
		t.Errorf("a burst of %d into %s moved %.1f%% of existing nodes", burst, worstTopic, 100*worstFrac)
	}
}

func sizeOfTopic(g *Graph, topic int) int {
	n := 0
	for _, r := range g.Records {
		if r.Topic == topic {
			n++
		}
	}
	return n
}

func TestEqualizeRadiiSpreadsARimPileEvenly(t *testing.T) {
	const R = 100.0
	n := 1000
	pos := make([][2]float64, n)
	for k := range pos {
		a := float64(k) * 2.39996
		r := .99 * R
		if k%5 == 0 {
			r = .3 * R * hash01(strconv.Itoa(k), "r")
		}
		pos[k] = [2]float64{math.Cos(a) * r, math.Sin(a) * r}
	}
	equalizeRadii(pos, .94*R)
	var shell, outer int
	for _, p := range pos {
		r := math.Hypot(p[0], p[1])
		if r > .97*R {
			shell++
		}
		if r > .8*R {
			outer++
		}
		if r > .94*R+1e-6 {
			t.Fatalf("a node at %.2f R is beyond the target radius", r/R)
		}
	}
	want := 1 - .8*.8/(.94*.94)
	if got := float64(outer) / float64(n); math.Abs(got-want) > .04 {
		t.Fatalf("%.3f of the nodes sit beyond .8R, a uniform disc has %.3f", got, want)
	}
	if shell != 0 {
		t.Fatalf("%d nodes left in the outer shell", shell)
	}
}
