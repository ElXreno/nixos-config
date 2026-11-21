package ui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"mem/internal/store"
)

type netCase struct {
	Name string         `json:"name"`
	Old  map[string]any `json:"old"`
	Diff map[string]any `json:"diff"`
	Full map[string]any `json:"full"`
}

func netCases(t *testing.T) []netCase {
	t.Helper()
	f := newFixture(t)
	var out []netCase
	step := func(name string, fn func()) {
		old := getMap(t, f, "/api/graph")
		fn()
		diff := getMap(t, f, "/api/graph/diff?from="+old["rev"].(string))
		out = append(out, netCase{Name: name, Old: old, Diff: diff, Full: getMap(t, f, "/api/graph")})
	}
	step("put", func() {
		put(t, f.st, store.PutRequest{Key: "reference_net1", Kind: store.Fact, Title: sp("Net one"), Body: sp("one")})
	})
	step("update", func() {
		put(t, f.st, store.PutRequest{Key: "reference_beta", Kind: store.Fact, Title: sp("Beta again"), Body: sp("again"), Update: true})
	})
	step("links", func() {
		if err := f.st.AddLinks(t.Context(), []store.Link{{Src: "reference_net1", Dst: "reference_gamma", Rel: store.Relates}}); err != nil {
			t.Fatal(err)
		}
	})
	step("topicOrder", func() {
		var assign []store.TopicAssign
		for i := 0; i < 6; i++ {
			key := "reference_lift" + strconv.Itoa(i)
			put(t, f.st, store.PutRequest{Key: key, Kind: store.Fact, Title: sp("Lift"), Body: sp("lift")})
			assign = append(assign, store.TopicAssign{Key: key, Topic: "net", Title: "Networking"})
		}
		if _, err := f.st.SetTopics(t.Context(), assign); err != nil {
			t.Fatal(err)
		}
	})
	step("delete", func() {
		if _, err := f.st.DB().Exec(`DELETE FROM records WHERE key = 'reference_lonely'`); err != nil {
			t.Fatal(err)
		}
	})
	step("empty", func() {})
	return out
}

func TestNetClientApplyAndTransport(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	dir := t.TempDir()
	raw, err := json.Marshal(netCases(t))
	if err != nil {
		t.Fatal(err)
	}
	cases := filepath.Join(dir, "cases.json")
	if err := os.WriteFile(cases, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := assetFS.ReadFile("assets/net.js")
	if err != nil {
		t.Fatal(err)
	}
	js := filepath.Join(dir, "net.js")
	if err := os.WriteFile(js, src, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, "testdata/net.js", js, cases).Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("node: %v %s", err, stderr)
	}
	var checks map[string]bool
	if err := json.Unmarshal(out, &checks); err != nil {
		t.Fatalf("node output: %v: %s", err, out)
	}
	names := make([]string, 0, len(checks))
	for k := range checks {
		names = append(names, k)
	}
	sort.Strings(names)
	if len(names) < 38 {
		t.Fatalf("only %d checks ran: %v", len(names), names)
	}
	for _, k := range names {
		if !checks[k] {
			t.Errorf("check %s failed", k)
		}
	}
}

func TestEventSourceStaysInNetJS(t *testing.T) {
	net, err := assetFS.ReadFile("assets/net.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(normJS(string(net)), normJS("o.url || '/api/events'")) {
		t.Error("net.js does not default to the same-origin /api/events")
	}
	for _, name := range scriptOrder {
		src, err := assetFS.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if name != "net.js" && name != "app.js" && strings.Contains(string(src), "EventSource") {
			t.Errorf("%s mentions EventSource; the stream belongs to net.js", name)
		}
	}
}
