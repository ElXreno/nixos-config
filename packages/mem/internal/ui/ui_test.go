package ui

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	"mem/internal/store"
)

func TestMain(m *testing.M) {
	time.Local = time.UTC
	os.Exit(m.Run())
}

var (
	t0    = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	tNow  = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	skip  = store.Gate{Skip: true}
	local = []string{"project:legacy"}
)

type fixture struct {
	st  *store.Store
	srv *Server
	h   http.Handler
}

func put(t *testing.T, st *store.Store, req store.PutRequest) {
	t.Helper()
	req.Gate = skip
	if req.Scopes == nil {
		req.Scopes = local
	}
	if _, err := st.Put(context.Background(), req); err != nil {
		t.Fatalf("put %s: %v", req.Key, err)
	}
}

func sp(s string) *string { return &s }

func newFixture(t *testing.T) *fixture {
	t.Helper()
	clock := t0
	core := true
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "mem.db"), store.Options{Now: func() time.Time { return clock }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	put(t, st, store.PutRequest{Key: "feedback_alpha", Kind: store.Rule, Title: sp("Alpha rule"), Body: sp("Alpha says see [[reference_beta]] and `reference_beta`."), Core: &core,
		Triggers: &[]store.Trigger{{Tool: "Bash", Regex: `alpha\b`}}})
	put(t, st, store.PutRequest{Key: "reference_beta", Kind: store.Fact, Title: sp("Beta fact"), Body: sp("Beta fact about the kernel build.")})
	put(t, st, store.PutRequest{Key: "reference_lonely", Kind: store.Fact, Title: sp("Lonely fact"), Body: sp("Nobody links here.")})
	put(t, st, store.PutRequest{Key: "reference_gamma", Kind: store.Fact, Title: sp("Gamma"), Body: sp("Gamma notes.\n2026-01-01 first\n2026-02-01 second\n2026-03-01 third")})
	put(t, st, store.PutRequest{Key: "project_state", Kind: store.State, Title: sp("State of things"), Body: sp("Open work item.")})
	put(t, st, store.PutRequest{Key: "user_owner", Kind: store.Fact, Title: sp("About the owner"), Body: sp("Prefers terse answers.")})
	put(t, st, store.PutRequest{Key: "project_big#1", Kind: store.Log, Title: sp("Big log"), Body: sp("First chunk of the large log about zfs readahead.")})
	put(t, st, store.PutRequest{Key: "project_big#2", Kind: store.Log, Title: sp("Big log / part two"), Body: sp("Second chunk with the nvidia driver details.")})
	put(t, st, store.PutRequest{Key: "feedback_old", Kind: store.Rule, Title: sp("Old rule"), Body: sp("The old way of building.")})
	clock = t0.Add(24 * time.Hour)
	put(t, st, store.PutRequest{Key: "feedback_new", Kind: store.Rule, Title: sp("New rule"), Body: sp("The new way of building."), Supersedes: []string{"feedback_old"}})
	clock = t0.Add(48 * time.Hour)
	put(t, st, store.PutRequest{Key: "feedback_alpha", Kind: store.Rule, Title: sp("Alpha rule"), Body: sp("Alpha says see [[reference_beta]] again, revised."), Update: true})
	if err := st.AddLinks(context.Background(), []store.Link{
		{Src: "feedback_alpha", Dst: "reference_beta", Rel: store.Relates},
		{Src: "reference_gamma", Dst: "reference_beta", Rel: store.Relates},
		{Src: "reference_gamma", Dst: "reference_net_hub", Rel: store.Unresolved},
		{Src: "reference_gamma", Dst: "missing-key", Rel: store.Unresolved},
		{Src: "reference_gamma", Dst: "MEMORY", Rel: store.Unresolved},
		{Src: "reference_gamma", Dst: "old-slug", Rel: store.Unresolved},
		{Src: "reference_gamma", Dst: "project", Rel: store.Unresolved},
		{Src: "reference_beta", Dst: "old-slug#1", Rel: store.DerivedFrom, Note: "evidence quote"},
		{Src: "feedback_new", Dst: "feedback_alpha", Rel: store.Relates, Note: "distinct: different scope"},
		{Src: "project_big#1", Dst: "reference_beta", Rel: store.Relates},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetTopics(context.Background(), []store.TopicAssign{
		{Key: "reference_gamma", Topic: "net", Title: "Networking"}, {Key: "feedback_alpha", Topic: "kernel", Title: "Kernel work"},
		{Key: "reference_beta", Topic: "kernel", Title: "Kernel work"}, {Key: "project_big#1", Topic: "kernel", Title: "Kernel work"}, {Key: "project_big#2", Topic: "kernel", Title: "Kernel work"},
	}); err != nil {
		t.Fatal(err)
	}
	srv := New(st, Options{Host: "smok", Home: "/home/x", Now: func() time.Time { return tNow }}, false)
	return &fixture{st: st, srv: srv, h: srv.Handler()}
}

func (f *fixture) do(t *testing.T, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	req.Host = "127.0.0.1:7781"
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

func (f *fixture) json(t *testing.T, target string, v any) {
	t.Helper()
	rec := f.do(t, http.MethodGet, target)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d %s", target, rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("GET %s content type %q", target, ct)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("GET %s: %v", target, err)
	}
}

func TestRecords(t *testing.T) {
	f := newFixture(t)
	var l struct {
		Total   int               `json:"total"`
		Records []json.RawMessage `json:"records"`
	}
	f.json(t, "/api/records?kind=rule&status=current", &l)
	if l.Total != 2 || len(l.Records) != 2 {
		t.Fatalf("rules current = %d/%d", l.Total, len(l.Records))
	}
	if strings.Contains(string(l.Records[0]), `"body"`) {
		t.Fatalf("body must be omitted by default: %s", l.Records[0])
	}
	f.json(t, "/api/records?kind=rule,fact&limit=2&offset=1&body=1", &l)
	if l.Total != 7 || len(l.Records) != 2 || !strings.Contains(string(l.Records[0]), `"body"`) {
		t.Fatalf("paged = %d/%d %s", l.Total, len(l.Records), l.Records[0])
	}
	f.json(t, "/api/records?scope=host:other", &l)
	if l.Total != 0 {
		t.Fatalf("scope filter total = %d", l.Total)
	}
	f.json(t, "/api/records?topic=net", &l)
	if l.Total != 1 {
		t.Fatalf("topic filter total = %d", l.Total)
	}
	for _, bad := range []string{"/api/records?kind=nope", "/api/records?status=nope", "/api/records?scope=bogus", "/api/records?limit=-1", "/api/records?limit=x"} {
		if rec := f.do(t, http.MethodGet, bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d", bad, rec.Code)
		}
	}
}

func TestRecordLinksHistory(t *testing.T) {
	f := newFixture(t)
	var rec struct {
		Record struct {
			Key     string   `json:"key"`
			Body    string   `json:"body"`
			Version int      `json:"version"`
			Chunks  []string `json:"chunks"`
		} `json:"record"`
		Links []store.Link `json:"links"`
	}
	f.json(t, "/api/record?key=feedback_alpha", &rec)
	if rec.Record.Key != "feedback_alpha" || rec.Record.Version != 2 || !strings.Contains(rec.Record.Body, "revised") || len(rec.Links) == 0 {
		t.Fatalf("record = %+v", rec)
	}
	f.json(t, "/api/record?key=project_big", &rec)
	if rec.Record.Key != "project_big" || !slices.Equal(rec.Record.Chunks, []string{"project_big#1", "project_big#2"}) || !strings.Contains(rec.Record.Body, "nvidia") {
		t.Fatalf("grouped record = %+v", rec.Record)
	}
	if len(rec.Links) != 1 || rec.Links[0].Src != "project_big#1" {
		t.Fatalf("grouped links = %+v", rec.Links)
	}
	var lk struct {
		Links []store.Link `json:"links"`
	}
	f.json(t, "/api/links?key=reference_beta", &lk)
	if len(lk.Links) != 4 {
		t.Fatalf("links of beta = %+v", lk.Links)
	}
	var h struct {
		Versions []store.Version `json:"versions"`
	}
	f.json(t, "/api/history?key=feedback_alpha", &h)
	if len(h.Versions) != 1 || h.Versions[0].Version != 1 || strings.Contains(h.Versions[0].Body, "again") || !strings.Contains(h.Versions[0].Body, "and") {
		t.Fatalf("history = %+v", h.Versions)
	}
	f.json(t, "/api/history?key=reference_beta", &h)
	if h.Versions == nil || len(h.Versions) != 0 {
		t.Fatalf("empty history must be an empty array: %+v", h.Versions)
	}
	for target, code := range map[string]int{
		"/api/record?key=nope":      http.StatusNotFound,
		"/api/record":               http.StatusBadRequest,
		"/api/record?key=BAD%20key": http.StatusBadRequest,
		"/api/links?key=*":          http.StatusBadRequest,
		"/api/history?key=a[0]":     http.StatusBadRequest,
	} {
		if rec := f.do(t, http.MethodGet, target); rec.Code != code {
			t.Fatalf("%s = %d, want %d", target, rec.Code, code)
		}
	}
}

func TestReadOnly(t *testing.T) {
	f := newFixture(t)
	for _, p := range Patterns() {
		target := strings.ReplaceAll(p, "{$}", "")
		for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			rec := f.do(t, m, target)
			if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
				t.Fatalf("%s %s = %d allow %q", m, target, rec.Code, rec.Header().Get("Allow"))
			}
		}
	}
	for _, m := range []string{http.MethodPost, http.MethodDelete} {
		if rec := f.do(t, m, "/api/does-not-exist"); rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s unknown = %d", m, rec.Code)
		}
	}
	var n int
	if err := f.st.DB().QueryRow("SELECT count(*) FROM records").Scan(&n); err != nil || n != 10 {
		t.Fatalf("records after mutation attempts = %d %v", n, err)
	}
}

func TestReadOnlyStoreHandle(t *testing.T) {
	f := newFixture(t)
	path := f.st.Path()
	ro, err := store.Open(context.Background(), path, store.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ro.Close() }()
	srv := New(ro, Options{Now: func() time.Time { return tNow }}, false)
	for _, target := range []string{"/api/graph", "/api/stats", "/api/search?q=alpha", "/api/history?key=feedback_alpha", "/api/record?key=project_big"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Host = "localhost:1"
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s on a read-only handle = %d %s", target, rec.Code, rec.Body.String())
		}
	}
	if _, err := ro.DB().Exec("DELETE FROM records"); err == nil {
		t.Fatal("the read-only handle accepted a write")
	}
}

func TestSecurityHeaders(t *testing.T) {
	f := newFixture(t)
	for _, target := range append(assetTargets(), "/api/stats", "/missing") {
		h := f.do(t, http.MethodGet, target).Header()
		if h.Get("Content-Security-Policy") != CSP {
			t.Fatalf("%s CSP = %q", target, h.Get("Content-Security-Policy"))
		}
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Referrer-Policy") != "no-referrer" || h.Get("Cache-Control") != "no-store" || h.Get("X-Frame-Options") != "DENY" {
			t.Fatalf("%s headers = %v", target, h)
		}
	}
}

func TestCSPDirectives(t *testing.T) {
	dirs := map[string]string{}
	for _, d := range strings.Split(CSP, ";") {
		name, val, _ := strings.Cut(strings.TrimSpace(d), " ")
		dirs[name] = val
	}
	want := map[string]string{
		"default-src": "'none'", "script-src": "'self'", "connect-src": "'self'", "font-src": "'none'",
		"form-action": "'none'", "base-uri": "'none'", "frame-ancestors": "'none'",
	}
	for k, v := range want {
		if dirs[k] != v {
			t.Fatalf("CSP %s = %q, want %q", k, dirs[k], v)
		}
	}
	if strings.Contains(dirs["script-src"], "unsafe") || strings.Contains(CSP, "http:") || strings.Contains(CSP, "https:") || strings.Contains(CSP, "*") {
		t.Fatalf("CSP too loose: %s", CSP)
	}
}

var (
	externalURL = regexp.MustCompile(`(?i)(?:https?:)?//[a-z0-9][a-z0-9.-]*\.[a-z]{2,}|@import|<link[^>]+href=["']?https?:|url\(\s*["']?https?:|src=["']?https?:|\bimport\(|\bnew\s+WebSocket\b|\bnew\s+EventSource\b|\bsendBeacon\b|\bXMLHttpRequest\b`)
	inlineJS    = regexp.MustCompile(`(?i)<script(?:\s[^>]*)?>[^<]|\son[a-z]+=["']|javascript:`)
)

func TestServedAssetsAreOffline(t *testing.T) {
	f := newFixture(t)
	for _, target := range assetTargets() {
		rec := f.do(t, http.MethodGet, target)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s = %d", target, rec.Code)
		}
		body := rec.Body.String()
		if m := externalURL.FindString(body); m != "" {
			t.Fatalf("%s references an external resource: %q", target, m)
		}
		if strings.Contains(body, "fonts.googleapis") || strings.Contains(body, "fonts.gstatic") {
			t.Fatalf("%s references a font host", target)
		}
		if target == "/" {
			if m := inlineJS.FindString(body); m != "" {
				t.Fatalf("index has inline script or handler: %q", m)
			}
			for _, src := range regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllStringSubmatch(body, -1) {
				if src[1] != "data:," && (!strings.HasPrefix(src[1], "/") || strings.HasPrefix(src[1], "//")) {
					t.Fatalf("index loads %q", src[1])
				}
			}
		}
	}
	for _, name := range append(slices.Clone(scriptOrder), loaderName) {
		data, err := assetFS.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range regexp.MustCompile(`fetch\(\s*['"`+"`"+`]([^'"`+"`"+`]*)`).FindAllStringSubmatch(string(data), -1) {
			if !strings.HasPrefix(m[1], "/api/") {
				t.Fatalf("%s fetches %q", name, m[1])
			}
		}
	}
}

func assetTargets() []string {
	return append([]string{"/", "/app.css", "/" + loaderName}, scriptURLs()...)
}

func TestAssetsTunedBehaviourKept(t *testing.T) {
	js := normJS(allJS(t))
	for _, want := range []string{
		`localStorage.getItem('mem.'+k)`, `localStorage.setItem('mem.'+k,v)`, `lim={lw:[180,560],rw:[300,1400]}`,
		`else{cam.x=D.cx+p.x-D.x`, `n.hx=hx;n.hy=hy`, `GFX.grab(D.n,`, `GFX.drop()`,
		`Math.min(2, window.devicePixelRatio || 1)`, `getContext('webgl2'`, `G.lite.create(cv)`, `powerPreference: 'low-power'`,
		`ov.addEventListener('pointerdown'`,
	} {
		if !strings.Contains(js, normJS(want)) {
			t.Fatalf("assets lost %q", want)
		}
	}
	css, _ := assetFS.ReadFile("assets/app.css")
	if !strings.Contains(string(css), "max-width:min(1320px,100%)") {
		t.Fatal("centre column cap 1320px lost")
	}
}

func TestRendererIsOnDemand(t *testing.T) {
	js := allJS(t)
	for _, banned := range []string{"setInterval(draw", "sim.alpha", "forceSimulation", "reheat", "computeBends"} {
		if strings.Contains(js, banned) {
			t.Errorf("assets carry %q: frames come from the live simulation or an event, never a timer", banned)
		}
	}
	scene, err := assetFS.ReadFile("assets/gfx/scene.js")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(allJS(t), "requestAnimationFrame(frame)"); n != 1 {
		t.Errorf("assets schedule the frame loop in %d places, want only invalidate", n)
	}
	if !strings.Contains(string(scene), "if (animating)") {
		t.Error("scene.js frame does not stop requesting frames when nothing moves")
	}
	for _, want := range []string{"host.visible()", "live.awake()", "if (!wander && !live.awake()) return false;"} {
		if !strings.Contains(string(scene), want) {
			t.Errorf("scene.js lost %q: the loop must pause with the tab and sleep under reduced motion", want)
		}
	}
}

func TestLegacyForceSimulationStaysGone(t *testing.T) {
	f := newFixture(t)
	if rec := f.do(t, http.MethodGet, "/gfx/live.js"); rec.Code != http.StatusOK || !slices.Contains(scriptOrder, "gfx/live.js") {
		t.Errorf("/gfx/live.js = %d, listed %v: the live simulation must ship", rec.Code, slices.Contains(scriptOrder, "gfx/live.js"))
	}
	if rec := f.do(t, http.MethodGet, "/sim.js"); rec.Code != http.StatusNotFound {
		t.Errorf("/sim.js = %d, want 404", rec.Code)
	}
	if _, err := assetFS.ReadFile("assets/sim.js"); err == nil {
		t.Error("assets/sim.js is embedded again")
	}
	html, err := assetFS.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if all := allJS(t) + string(html) + strings.Join(scriptOrder, " "); strings.Contains(all, "sim.js") || regexp.MustCompile(`\bsim\.[a-z]`).MatchString(all) {
		t.Error("the page still refers to the old force simulation")
	}
}

func TestScriptOrderIsComplete(t *testing.T) {
	f := newFixture(t)
	listed := map[string]bool{loaderName: true}
	for _, name := range scriptOrder {
		listed[name] = true
		rec := f.do(t, http.MethodGet, "/"+name)
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/javascript") {
			t.Fatalf("/%s = %d %q", name, rec.Code, rec.Header().Get("Content-Type"))
		}
		src := rec.Body.String()
		if !strings.HasPrefix(normJS(src), "'usestrict';") {
			t.Errorf("%s is not a strict classic script", name)
		}
		if regexp.MustCompile(`(?m)^\s*(import|export)\s`).MatchString(src) {
			t.Errorf("%s uses module syntax", name)
		}
	}
	err := fs.WalkDir(assetFS, "assets", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".js") {
			return err
		}
		if rel := strings.TrimPrefix(p, "assets/"); !listed[rel] {
			t.Errorf("%s is embedded but missing from scriptOrder", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	loader := f.do(t, http.MethodGet, "/"+loaderName).Body.String()
	if strings.Contains(loader, loaderScriptTag) {
		t.Fatal("loader.js still carries the script list placeholder")
	}
	var urls []string
	m := regexp.MustCompile(`const SCRIPTS = (\[[^\]]*\]);`).FindStringSubmatch(loader)
	if m == nil || json.Unmarshal([]byte(m[1]), &urls) != nil || !slices.Equal(urls, scriptURLs()) {
		t.Fatalf("loader.js list = %v, want %v", m, scriptURLs())
	}
}

func TestThemeMatchesCSS(t *testing.T) {
	theme, err := assetFS.ReadFile("assets/gfx/theme.js")
	if err != nil {
		t.Fatal(err)
	}
	css, _ := assetFS.ReadFile("assets/app.css")
	root := regexp.MustCompile(`(?s):root\{(.*?)\}`).FindStringSubmatch(string(css))
	if root == nil {
		t.Fatal("no :root block")
	}
	cssVars := map[string]string{}
	for _, m := range regexp.MustCompile(`(--[a-z0-9]+):\s*(#[0-9a-fA-F]{6})`).FindAllStringSubmatch(root[1], -1) {
		cssVars[m[1]] = strings.ToLower(m[2])
	}
	names := map[string]string{"--canvas": "canvas", "--bg": "bg", "--panel": "panel", "--line": "line", "--mute1": "m1", "--mute2": "m2", "--text": "text", "--text2": "text2",
		"--bright": "bright", "--live": "live", "--danger": "danger", "--link": "link"}
	for _, k := range []string{"rule", "fact", "state", "incident", "howto", "log"} {
		names["--"+k] = "kind." + k
	}
	if len(cssVars) < len(names) {
		t.Fatalf("css declares %d colour tokens, want at least %d", len(cssVars), len(names))
	}
	for v, key := range names {
		want := cssVars[v]
		field := strings.TrimPrefix(key, "kind.")
		var m []string
		if strings.HasPrefix(key, "kind.") {
			m = regexp.MustCompile(`kind = \{[^}]*\b` + field + `:\s*['"](#[0-9a-fA-F]{6})['"]`).FindStringSubmatch(string(theme))
		} else {
			m = regexp.MustCompile(`\bc = \{[^}]*\b` + field + `:\s*['"](#[0-9a-fA-F]{6})['"]`).FindStringSubmatch(string(theme))
		}
		if want == "" || m == nil || strings.ToLower(m[1]) != want {
			t.Errorf("%s: css %q, theme %v", v, want, m)
		}
	}
}

func TestHostHeaderGuard(t *testing.T) {
	f := newFixture(t)
	for host, code := range map[string]int{
		"127.0.0.1:7781": http.StatusOK, "localhost:7781": http.StatusOK, "[::1]:7781": http.StatusOK, "127.0.0.1": http.StatusOK,
		"evil.example:7781": http.StatusForbidden, "192.168.1.5:7781": http.StatusForbidden, "attacker.test": http.StatusForbidden,
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		f.h.ServeHTTP(rec, req)
		if rec.Code != code {
			t.Fatalf("Host %q = %d, want %d", host, rec.Code, code)
		}
	}
	remote := New(f.st, Options{Now: func() time.Time { return tNow }}, true)
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Host = "memory.example:7781"
	rec := httptest.NewRecorder()
	remote.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("--allow-remote Host = %d", rec.Code)
	}
}

func TestCheckListen(t *testing.T) {
	cases := []struct {
		addr   string
		remote bool
		ok     bool
	}{
		{"127.0.0.1:7781", false, true},
		{"127.0.0.1:0", false, true},
		{"127.5.5.5:80", false, true},
		{"[::1]:7781", false, true},
		{"localhost:7781", false, true},
		{"LOCALHOST:7781", false, true},
		{"0.0.0.0:7781", false, false},
		{":7781", false, false},
		{"[::]:7781", false, false},
		{"192.168.1.10:7781", false, false},
		{"100.64.1.2:7781", false, false},
		{"example.com:7781", false, false},
		{"127.0.0.1.evil.com:80", false, false},
		{"127.0.0.1", false, false},
		{"127.0.0.1:99999", false, false},
		{"127.0.0.1:abc", false, false},
		{"", false, false},
		{"0.0.0.0:7781", true, true},
		{":7781", true, true},
		{"100.64.1.2:7781", true, true},
		{"0.0.0.0:99999", true, false},
	}
	for _, c := range cases {
		err := CheckListen(c.addr, c.remote)
		if (err == nil) != c.ok {
			t.Errorf("CheckListen(%q, %v) = %v, want ok=%v", c.addr, c.remote, err, c.ok)
		}
	}
}

func TestCheckBound(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	if err := CheckBound(ln, false); err != nil {
		t.Fatal(err)
	}
	wide, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Skip("no wildcard listen:", err)
	}
	defer func() { _ = wide.Close() }()
	if err := CheckBound(wide, false); err == nil {
		t.Fatal("a wildcard listener passed the loopback check")
	}
	if err := CheckBound(wide, true); err != nil {
		t.Fatal(err)
	}
}

func TestServeAndShutdown(t *testing.T) {
	f := newFixture(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, f.h) }()
	resp, err := http.Get("http://" + ln.Addr().String() + "/api/stats")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"nodes":9`) {
		t.Fatalf("live GET = %d %s", resp.StatusCode, body)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
	if _, err := http.Get("http://" + ln.Addr().String() + "/"); err == nil {
		t.Fatal("server still accepts after shutdown")
	}
}

func TestBaseKey(t *testing.T) {
	for in, want := range map[string][2]any{
		"a#1": {"a", 1}, "a#12": {"a", 12}, "a": {"a", 0}, "a#": {"a#", 0}, "#1": {"#1", 0}, "a#x": {"a#x", 0}, "a#0": {"a#0", 0}, "a#1#2": {"a#1", 2},
	} {
		b, n := BaseKey(in)
		if b != want[0] || n != want[1] {
			t.Errorf("BaseKey(%q) = %q,%d want %v", in, b, n, want)
		}
	}
}

func normJS(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		if r == '"' {
			return '\''
		}
		return r
	}, s)
}

func allJS(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, name := range append(slices.Clone(scriptOrder), loaderName) {
		data, err := assetFS.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestAssetsCarryNoLegacyVocabulary(t *testing.T) {
	html, _ := assetFS.ReadFile("assets/index.html")
	css, _ := assetFS.ReadFile("assets/app.css")
	all := allJS(t) + string(html) + string(css)
	for _, stale := range []string{"MEMORY.md", "этап B", "хаб", "в индексе", "Пометка в", "verdict", "aliasOf", "SMOK завис", "prefill strata", "readahead zfs", "Преемник в старой памяти", "от 10 КБ"} {
		if strings.Contains(all, stale) {
			t.Errorf("assets still carry %q", stale)
		}
	}
	for _, found := range []string{"Core-записи", "Здоровье и калибровка", "data-smode", "Битые ссылки"} {
		if !strings.Contains(all, found) {
			t.Errorf("assets lost %q", found)
		}
	}
}

func TestGraphPayloadCarriesNoBodies(t *testing.T) {
	f := newFixture(t)
	body := f.do(t, http.MethodGet, "/api/graph").Body.String()
	for _, leak := range []string{"Beta fact about the kernel build", "Alpha says see", `"b":`} {
		if strings.Contains(body, leak) {
			t.Fatalf("graph payload still embeds record bodies: %q", leak)
		}
	}
}
