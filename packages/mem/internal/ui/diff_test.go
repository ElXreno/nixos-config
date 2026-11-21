package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"mem/internal/store"
)

func applyDiffJSON(old, diff map[string]any) map[string]any {
	oldTopics := old["topics"].([]any)
	oldID := make([]string, len(oldTopics))
	byID := map[string]any{}
	for i, t := range oldTopics {
		id := t.(map[string]any)["id"].(string)
		oldID[i] = id
		byID[id] = t
	}
	var topics []any
	newIdx := map[string]int{}
	for i, t := range diff["topics"].([]any) {
		m := t.(map[string]any)
		id := m["id"].(string)
		newIdx[id] = i
		if same, _ := m["same"].(bool); same {
			topics = append(topics, byID[id])
		} else {
			topics = append(topics, t)
		}
	}
	del := map[string]bool{}
	for _, k := range diff["del"].([]any) {
		del[k.(string)] = true
	}
	repl := map[string]any{}
	for _, list := range []string{"add", "set"} {
		for _, n := range diff[list].([]any) {
			repl[n.(map[string]any)["k"].(string)] = n
		}
	}
	recs := map[string]any{}
	for _, r := range old["records"].([]any) {
		m := r.(map[string]any)
		k := m["k"].(string)
		if del[k] {
			continue
		}
		if _, ok := repl[k]; ok {
			continue
		}
		c := map[string]any{}
		for f, v := range m {
			c[f] = v
		}
		c["g"] = float64(newIdx[oldID[int(m["g"].(float64))]])
		recs[k] = c
	}
	for k, n := range repl {
		recs[k] = n
	}
	keys := make([]string, 0, len(recs))
	for k := range recs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := map[string]any{}
	for k, v := range old {
		out[k] = v
	}
	for _, k := range []string{"rev", "built", "links", "rows", "layout"} {
		out[k] = diff[k]
	}
	out["topics"] = topics
	list := make([]any, len(keys))
	for i, k := range keys {
		list[i] = recs[k]
	}
	out["records"] = list
	return out
}

func getMap(t *testing.T, f *fixture, target string) map[string]any {
	t.Helper()
	var m map[string]any
	f.json(t, target, &m)
	return m
}

func assertDiffEqualsFull(t *testing.T, f *fixture, old map[string]any) map[string]any {
	t.Helper()
	diff := getMap(t, f, "/api/graph/diff?from="+old["rev"].(string))
	full := getMap(t, f, "/api/graph")
	got := applyDiffJSON(old, diff)
	if !reflect.DeepEqual(got, full) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(full)
		t.Fatalf("apply(diff) != full graph\napplied %d bytes, full %d bytes\ndiff add=%v set=%v del=%v", len(a), len(b), len(diff["add"].([]any)), len(diff["set"].([]any)), diff["del"])
	}
	return diff
}

func TestGraphCarriesRevision(t *testing.T) {
	f := newFixture(t)
	a := getMap(t, f, "/api/graph")
	b := getMap(t, f, "/api/graph")
	rev, _ := a["rev"].(string)
	if len(rev) != 16 || b["rev"] != rev {
		t.Fatalf("rev = %q then %q", rev, b["rev"])
	}
}

func TestGraphDiffEqualsFullGraph(t *testing.T) {
	f := newFixture(t)
	old := getMap(t, f, "/api/graph")
	empty := getMap(t, f, "/api/graph/diff?from="+old["rev"].(string))
	if len(empty["add"].([]any))+len(empty["set"].([]any))+len(empty["del"].([]any)) != 0 || empty["rev"] != old["rev"] {
		t.Fatalf("diff from the current revision is not empty: %v", empty)
	}
	steps := []func(){
		func() {
			put(t, f.st, store.PutRequest{Key: "reference_fresh", Kind: store.Fact, Title: sp("Fresh"), Body: sp("Fresh body about zfs.")})
		},
		func() {
			put(t, f.st, store.PutRequest{Key: "reference_beta", Kind: store.Fact, Title: sp("Beta fact revised"), Body: sp("Revised."), Update: true})
		},
		func() {
			if err := f.st.AddLinks(t.Context(), []store.Link{{Src: "reference_fresh", Dst: "reference_gamma", Rel: store.Relates}}); err != nil {
				t.Fatal(err)
			}
		},
		func() {
			if _, err := f.st.SetTopics(t.Context(), []store.TopicAssign{{Key: "reference_fresh", Topic: "net", Title: "Networking"}}); err != nil {
				t.Fatal(err)
			}
		},
		func() {
			for i := 0; i < 4; i++ {
				put(t, f.st, store.PutRequest{Key: "reference_bulk" + strconv.Itoa(i), Kind: store.Fact, Title: sp("Bulk"), Body: sp("bulk")})
			}
			if _, err := f.st.SetTopics(t.Context(), []store.TopicAssign{
				{Key: "reference_bulk0", Topic: "net", Title: "Networking"}, {Key: "reference_bulk1", Topic: "net", Title: "Networking"},
				{Key: "reference_bulk2", Topic: "net", Title: "Networking"}, {Key: "reference_bulk3", Topic: "net", Title: "Networking"},
			}); err != nil {
				t.Fatal(err)
			}
		},
		func() {
			if _, err := f.st.DB().Exec(`DELETE FROM records WHERE key = 'reference_lonely'`); err != nil {
				t.Fatal(err)
			}
		},
	}
	for i, step := range steps {
		step()
		d := assertDiffEqualsFull(t, f, old)
		if d["from"] != old["rev"] || d["rev"] == old["rev"] {
			t.Fatalf("step %d: diff revs from=%v rev=%v old=%v", i, d["from"], d["rev"], old["rev"])
		}
		old = getMap(t, f, "/api/graph")
	}
}

func TestGraphDiffRemapsTopicIndexes(t *testing.T) {
	f := newFixture(t)
	for _, k := range []string{"misc_a", "misc_b"} {
		put(t, f.st, store.PutRequest{Key: "reference_" + k, Kind: store.Fact, Title: sp("Misc"), Body: sp("misc")})
	}
	if _, err := f.st.SetTopics(t.Context(), []store.TopicAssign{{Key: "reference_misc_a", Topic: "misc", Title: "Misc"}, {Key: "reference_misc_b", Topic: "misc", Title: "Misc"}}); err != nil {
		t.Fatal(err)
	}
	old := getMap(t, f, "/api/graph")
	before := map[string]string{}
	for _, r := range old["records"].([]any) {
		m := r.(map[string]any)
		before[m["k"].(string)] = old["topics"].([]any)[int(m["g"].(float64))].(map[string]any)["id"].(string)
	}
	var assign []store.TopicAssign
	for i := 0; i < 6; i++ {
		key := "reference_lift" + strconv.Itoa(i)
		put(t, f.st, store.PutRequest{Key: key, Kind: store.Fact, Title: sp("Lift"), Body: sp("lift")})
		assign = append(assign, store.TopicAssign{Key: key, Topic: "net", Title: "Networking"})
	}
	if _, err := f.st.SetTopics(t.Context(), assign); err != nil {
		t.Fatal(err)
	}
	diff := assertDiffEqualsFull(t, f, old)
	full := getMap(t, f, "/api/graph")
	oldOrder, newOrder := []string{}, []string{}
	for _, tp := range old["topics"].([]any) {
		oldOrder = append(oldOrder, tp.(map[string]any)["id"].(string))
	}
	for _, tp := range full["topics"].([]any) {
		newOrder = append(newOrder, tp.(map[string]any)["id"].(string))
	}
	if reflect.DeepEqual(oldOrder, newOrder) {
		t.Fatalf("topic order did not change: %v", oldOrder)
	}
	sets := diff["set"].([]any)
	for _, s := range sets {
		k := s.(map[string]any)["k"].(string)
		if _, existed := before[k]; existed {
			t.Fatalf("diff re-sent %s although only the topic order moved", k)
		}
	}
	for _, r := range full["records"].([]any) {
		m := r.(map[string]any)
		id := full["topics"].([]any)[int(m["g"].(float64))].(map[string]any)["id"].(string)
		if was, ok := before[m["k"].(string)]; ok && was != id {
			t.Fatalf("%s moved from topic %s to %s", m["k"], was, id)
		}
	}
}

func TestGraphDiffUnknownRevisionIsGone(t *testing.T) {
	f := newFixture(t)
	for target, want := range map[string]int{
		"/api/graph/diff?from=deadbeefdeadbeef": http.StatusGone,
		"/api/graph/diff?from=":                 http.StatusBadRequest,
		"/api/graph/diff":                       http.StatusBadRequest,
	} {
		if rec := f.do(t, http.MethodGet, target); rec.Code != want {
			t.Fatalf("%s = %d, want %d: %s", target, rec.Code, want, rec.Body.String())
		}
	}
}

func TestGraphDiffRingKeepsSixteenRevisions(t *testing.T) {
	f := newFixture(t)
	first := getMap(t, f, "/api/graph")["rev"].(string)
	var revs []string
	for i := 0; i < 16; i++ {
		put(t, f.st, store.PutRequest{Key: "reference_ring" + strconv.Itoa(i), Kind: store.Fact, Title: sp("Ring"), Body: sp("ring")})
		revs = append(revs, getMap(t, f, "/api/graph")["rev"].(string))
	}
	if rec := f.do(t, http.MethodGet, "/api/graph/diff?from="+first); rec.Code != http.StatusGone {
		t.Fatalf("revision %d back = %d, want 410", 16, rec.Code)
	}
	for i, rev := range revs[:len(revs)-1] {
		if rec := f.do(t, http.MethodGet, "/api/graph/diff?from="+rev); rec.Code != http.StatusOK {
			t.Fatalf("revision %d of %d = %d", i, len(revs), rec.Code)
		}
	}
	if n := len(f.srv.rv.ring); n != 16 {
		t.Fatalf("ring holds %d revisions", n)
	}
}

func TestGraphDiffDateRolloverIsEmpty(t *testing.T) {
	f := newFixture(t)
	clock := tNow
	srv := New(f.st, Options{Host: "smok", Home: "/home/x", Now: func() time.Time { return clock }}, false)
	g := &fixture{st: f.st, srv: srv, h: srv.Handler()}
	old := getMap(t, g, "/api/graph")
	clock = clock.Add(26 * time.Hour)
	d := getMap(t, g, "/api/graph/diff?from="+old["rev"].(string))
	if len(d["add"].([]any))+len(d["set"].([]any))+len(d["del"].([]any)) != 0 {
		t.Fatalf("a date-only rollover produced a diff: add=%d set=%d del=%v", len(d["add"].([]any)), len(d["set"].([]any)), d["del"])
	}
	if d["built"] == old["built"] || d["rev"] == old["rev"] {
		t.Fatalf("rollover kept built=%v rev=%v", d["built"], d["rev"])
	}
	if got, full := applyDiffJSON(old, d), getMap(t, g, "/api/graph"); !reflect.DeepEqual(got, full) {
		t.Fatal("apply(rollover diff) != full graph")
	}
}

func TestGraphDiffIsSmallAfterOnePut(t *testing.T) {
	f := newFixture(t)
	old := getMap(t, f, "/api/graph")
	put(t, f.st, store.PutRequest{Key: "reference_tiny", Kind: store.Fact, Title: sp("Tiny"), Body: sp("Tiny body.")})
	rec := f.do(t, http.MethodGet, "/api/graph/diff?from="+old["rev"].(string))
	if rec.Code != http.StatusOK || rec.Body.Len() > 20<<10 {
		t.Fatalf("diff = %d, %d bytes", rec.Code, rec.Body.Len())
	}
	var d map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if len(d["add"].([]any)) != 1 || len(d["set"].([]any)) != 0 {
		t.Fatalf("one put gave add=%d set=%d", len(d["add"].([]any)), len(d["set"].([]any)))
	}
}

func TestGraphDiffAppliesAcrossManyPuts(t *testing.T) {
	f := newFixture(t)
	old := getMap(t, f, "/api/graph")
	for i := 0; i < 5; i++ {
		put(t, f.st, store.PutRequest{Key: fmt.Sprintf("reference_many%d", i), Kind: store.Fact, Title: sp("Many"), Body: sp("many")})
	}
	assertDiffEqualsFull(t, f, old)
}
