package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"mem/internal/statefile"
	"mem/internal/store"
)

type domReport struct {
	Backend         string          `json:"backend"`
	Frames          int             `json:"frames"`
	Writes          int             `json:"writes"`
	Nodes           int             `json:"nodes"`
	Tags            []string        `json:"tags"`
	Attrs           []string        `json:"attrs"`
	Hilite          []string        `json:"hilite"`
	Turns           int             `json:"turns"`
	Items           int             `json:"items"`
	Drawn           int             `json:"drawn"`
	Filtered        int             `json:"filtered"`
	Escaped         map[string]bool `json:"escaped"`
	TopicStable     bool            `json:"topicStable"`
	UndefinedWrites int             `json:"undefinedWrites"`
	Nav             map[string]any  `json:"nav"`
	Live            map[string]bool `json:"live"`
}

const (
	hostileImg  = `<img src=x onerror=window.__pwn=5>`
	hostileImgE = `&lt;img src=x onerror=window.__pwn=5&gt;`
)

func hostileState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sess := `s"><img src=x onerror=window.__pwn=6>`
	sha := "0123456789abcdef"
	turns := []statefile.Turn{
		{Time: "2026-11-07T11:00:00Z", Event: statefile.EventPrompt, Session: sess, PromptSHA: sha, Prompt: hostileImg + ` & <script>window.__pwn=7</script> "q" 'a'`, PromptChars: 300,
			Mode: "vector", Pool: `<i>reg</i>`, Status: "inject", Embed: `<b>ok</b>`, EmbedMs: 12, TotalMs: 40, Chars: 300,
			Harness: `<u>harness</u>`, Host: `<s>host</s>`, Project: `<p>proj</p>`, Cwd: `~/<q>cwd</q>`,
			Rerank: `<em>ok</em>`, RerankMs: 7,
			Strong:   []statefile.Key{{Key: "feedback_xss", Cos: 0.812}},
			Pointers: []statefile.Key{{Key: "reference_beta", Cos: 0.55}},
			Reranked: []statefile.Key{{Key: `<c>cand</c>`, Cos: 0.4}},
			Lexical:  []string{`x"><svg onload=window.__pwn=8>`}},
		{Time: "2026-11-07T11:00:05Z", Event: statefile.EventTool, Session: sess, Agent: `<b>agent</b>`, AgentType: `<i>atype</i>`, Tool: `<img src=y onerror=window.__pwn=9>`, Status: "inject",
			Keys: []string{"feedback_xss", `<u>nokey</u>`}, Chars: 90, Input: `echo <b>in</b>`, Fired: []statefile.Fired{{Key: "feedback_xss", Tool: `<t>tool</t>`, Regex: `<r>re</r>`}}},
		{Time: "2026-11-07T11:00:07Z", Event: statefile.EventSession, Session: sess, Status: "inject", Source: `<m>start</m>`, Keys: []string{"feedback_xss"}, Chars: 10, Topics: 3},
		{Time: "2026-11-07T11:00:08Z", Event: statefile.EventPrompt, Session: sess, Status: "skipped", Source: `<k>marker</k>`, Prompt: "skipped"},
		{Time: "not a time", Event: statefile.EventPrompt, Session: "s2", Prompt: "dropped", Status: "none"},
	}
	var b strings.Builder
	for _, tl := range turns {
		data, err := json.Marshal(tl)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, statefile.TurnsName), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	shadow, err := json.Marshal(map[string]any{"time": "2026-11-07T11:00:01Z", "session": sess, "prompt_sha": sha, "pool": `<b>fused</b>`, "embed": `<i>ok</i>`, "embed_ms": 3,
		"shadow_keys": []string{`<s>ghost</s>`, "project_state"}, "shadow_hits": []map[string]any{{"key": `<s>ghost</s>`, "cos": 0.7}, {"key": "project_state", "cos": 0.66}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, statefile.ShadowName), append(shadow, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	logLine := "2026-11-07T11:30:00Z user-prompt-submit session=s9 error 5ms <img src=z onerror=window.__pwn=14> failed\n"
	if err := os.WriteFile(filepath.Join(dir, statefile.HookLogName), []byte(logLine), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func hostileStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "mem.db"), store.Options{Now: func() time.Time { return t0 }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	img := `<img src="http://evil.invalid/p" onerror="window.__pwn=1">`
	body := img + "\n<script>window.__pwn=2</script>\n<svg onload=\"window.__pwn=3\"></svg>\n**" + img + "**\n[click](javascript:window.__pwn=4)\n" +
		"[x](reference_beta.md)\n[[reference_beta]]\n`" + img + "`\n```\n" + img + "\n```\n<style>@import url(http://evil.invalid/c.css)</style>\n" +
		"2026-01-01 " + img + "\n2026-01-02 x\n2026-01-03 y\n\n& amp quot lt gt 39 &amp; &lt;b&gt;\n"
	core := true
	trig := []store.Trigger{{Tool: "Bash", Regex: `<b>x</b>`}}
	put(t, st, store.PutRequest{Key: "feedback_xss", Kind: store.Rule, Title: sp(img), Body: sp(body), Core: &core, Triggers: &trig,
		Source: sp("src<img src=x onerror=window.__pwn=11>"), VerifyCmd: sp("v<svg onload=window.__pwn=12>")})
	put(t, st, store.PutRequest{Key: "reference_beta", Kind: store.Fact, Title: sp("Beta & amp <b>bold</b> \"q\" 'a'"), Body: sp("Beta body.")})
	put(t, st, store.PutRequest{Key: "project_state", Kind: store.State, Title: sp(img), Body: sp("x")})
	put(t, st, store.PutRequest{Key: "feedback_xss", Kind: store.Rule, Title: sp(img), Body: sp(body + "changed <b>line</b>\n"), Update: true, Why: "reason <img src=r onerror=window.__pwn=13>"})
	if err := st.AddLinks(context.Background(), []store.Link{
		{Src: "feedback_xss", Dst: "reference_beta", Rel: store.Relates, Note: "distinct: note <i>n</i>"},
		{Src: "feedback_xss", Dst: "x-" + img, Rel: store.Unresolved},
	}); err != nil {
		t.Fatal(err)
	}
	return st
}

var domNeedles = map[string]string{
	"prompt":      hostileImgE + ` &amp; `,
	"script":      `&lt;script&gt;window.__pwn=7&lt;/script&gt; &quot;q&quot; &#39;a&#39;`,
	"session":     `s&quot;&gt;&lt;img`,
	"tool":        `&lt;img src=y onerror=window.__pwn=9&gt;`,
	"agent":       `&lt;b&gt;agent&lt;/b&gt;`,
	"agentType":   `&lt;i&gt;atype&lt;/i&gt;`,
	"key":         `x&quot;&gt;&lt;svg onload=window.__pwn=8&gt;`,
	"nokey":       `&lt;u&gt;nokey&lt;/u&gt;`,
	"ghost":       `&lt;s&gt;ghost&lt;/s&gt;`,
	"embed":       `&lt;b&gt;ok&lt;/b&gt;`,
	"pool":        `&lt;i&gt;reg&lt;/i&gt;`,
	"sessionChip": `data-session="s&quot;&gt;&lt;img src=x onerror=window.__pwn=6&gt;"`,
	"harness":     `&lt;u&gt;harness&lt;/u&gt;`,
	"host":        `&lt;s&gt;host&lt;/s&gt;`,
	"project":     `&lt;p&gt;proj&lt;/p&gt;`,
	"cwd":         `~/&lt;q&gt;cwd&lt;/q&gt;`,
	"input":       `echo &lt;b&gt;in&lt;/b&gt;`,
	"firedRegex":  `&lt;r&gt;re&lt;/r&gt;`,
	"firedTool":   `&lt;t&gt;tool&lt;/t&gt;`,
	"marker":      `&lt;k&gt;marker&lt;/k&gt;`,
	"startMarker": `&lt;m&gt;start&lt;/m&gt;`,
	"candidate":   `&lt;c&gt;cand&lt;/c&gt;`,
	"rerankOut":   `&lt;em&gt;ok&lt;/em&gt;`,
	"source":      `src&lt;img src=x onerror=window.__pwn=11&gt;`,
	"verify":      `v&lt;svg onload=window.__pwn=12&gt;`,
	"trigger":     `&lt;b&gt;x&lt;/b&gt;`,
	"reason":      `reason &lt;img src=r onerror=window.__pwn=13&gt;`,
	"diff":        `changed &lt;b&gt;line&lt;/b&gt;`,
	"linkNote":    `distinct: note &lt;i&gt;n&lt;/i&gt;`,
	"dupNote":     `note &lt;i&gt;n&lt;/i&gt;`,
	"model":       `m&lt;i&gt;x&lt;/i&gt;`,
	"hookError":   `&lt;img src=z onerror=window.__pwn=14&gt; failed`,
	"title":       hostileImgE,
}

func runDOM(t *testing.T) domReport {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	st := hostileStore(t)
	h := New(st, Options{Host: "smok", Home: "/home/x", Now: func() time.Time { return tNow }, StateDir: hostileState(t), EmbedModel: "m<i>x</i>"}, false).Handler()
	rec := &fixture{h: h}
	get := func(target string) json.RawMessage {
		resp := rec.do(t, http.MethodGet, target)
		if resp.Code != http.StatusOK {
			t.Fatalf("%s = %d", target, resp.Code)
		}
		return json.RawMessage(append([]byte{}, resp.Body.Bytes()...))
	}
	var graph Graph
	if err := json.Unmarshal(get("/api/graph"), &graph); err != nil {
		t.Fatal(err)
	}
	details := map[string]json.RawMessage{}
	for _, n := range graph.Records {
		details[n.Key] = get("/api/detail?key=" + n.Key)
	}
	bundle := map[string]any{
		"graph": json.RawMessage(get("/api/graph")), "hygiene": get("/api/hygiene"), "turns": get("/api/turns"), "sessions": get("/api/sessions"),
		"health": get("/api/health"), "core": get("/api/core"), "search": get("/api/search?q=img&status=all&logs=1"), "details": details, "needles": domNeedles,
	}
	dir := t.TempDir()
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bundle.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	var js []byte
	for _, name := range scriptOrder {
		part, err := assetFS.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		js = append(append(js, part...), '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "all.js"), js, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "testdata/dom.js", filepath.Join(dir, "all.js"), filepath.Join(dir, "bundle.json"))
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("node: %v %s", err, stderr)
	}
	var r domReport
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("node output: %v", err)
	}
	return r
}

func TestRenderedHTMLCarriesNoRecordMarkup(t *testing.T) {
	r := runDOM(t)
	if r.Nodes != 3 || r.Writes < 40 {
		t.Fatalf("harness rendered %d nodes in %d writes", r.Nodes, r.Writes)
	}
	allowedTags := []string{"b", "button", "code", "details", "div", "em", "h2", "h3", "h4", "i", "label", "li", "line", "mark", "ol", "option", "p", "pre", "rect", "section", "select", "span", "summary",
		"svg", "circle", "path", "table", "tbody", "td", "th", "thead", "title", "tr", "u", "ul"}
	for _, tag := range r.Tags {
		if !slices.Contains(allowedTags, tag) {
			t.Fatalf("rendered a <%s> element, tags = %v", tag, r.Tags)
		}
	}
	allowedAttrs := []string{"aria-expanded", "aria-label", "aria-pressed", "aria-selected", "class", "cx", "cy", "d", "data-clear", "data-clearref", "data-crumb", "data-ex", "data-fhost", "data-fproj", "data-gall", "data-gi", "data-hist", "data-host", "data-id", "data-morefeed", "data-proj",
		"data-ref", "data-reset-filters", "data-reveal", "data-session", "data-slogs", "data-smode", "data-skind", "data-sstatus", "data-srerank", "data-status", "data-tabgo", "data-topic", "data-turn", "data-type",
		"disabled", "fill", "height", "id", "preserveaspectratio", "r", "role", "selected", "stroke", "stroke-dasharray", "stroke-opacity", "stroke-width", "style", "title", "value", "viewbox", "width", "x", "x1", "x2", "y", "y1", "y2"}
	for _, a := range r.Attrs {
		if !slices.Contains(allowedAttrs, a) || strings.HasPrefix(a, "on") {
			t.Fatalf("rendered attribute %q, attrs = %v", a, r.Attrs)
		}
	}
}

func TestRenderedServerStringsAreEscaped(t *testing.T) {
	r := runDOM(t)
	if r.Turns != 4 || r.Items < 8 {
		t.Fatalf("harness ingested %d turns, %d items, %d drawn", r.Turns, r.Items, r.Drawn)
	}
	for name := range domNeedles {
		if !r.Escaped[name] {
			t.Errorf("%s not rendered escaped", name)
		}
	}
}

func TestFilteringHidesNodes(t *testing.T) {
	r := runDOM(t)
	if r.Filtered != 2 {
		t.Fatalf("visible after hiding rules inside one project = %d", r.Filtered)
	}
}

func TestFallbackRendererRunsWithoutWebGL(t *testing.T) {
	r := runDOM(t)
	if r.Backend != "lite" {
		t.Fatalf("backend without webgl2 = %q, want lite", r.Backend)
	}
	if r.Frames < 1 {
		t.Fatalf("the fallback renderer drew %d frames", r.Frames)
	}
}

func TestFallbackRendererDragsAndSettles(t *testing.T) {
	r := runDOM(t)
	if len(r.Live) < 6 {
		t.Fatalf("live report has %d entries: %v", len(r.Live), r.Live)
	}
	for k, v := range r.Live {
		if !v {
			t.Errorf("live.%s = false", k)
		}
	}
}

func TestGraphReloadKeepsTopicFocusByIdentity(t *testing.T) {
	if r := runDOM(t); !r.TopicStable {
		t.Fatal("a graph reload moved the topic filter and focus to another topic")
	}
}

func TestRenderedHTMLHasNoUndefinedOrNaN(t *testing.T) {
	if r := runDOM(t); r.UndefinedWrites != 0 {
		t.Fatalf("%d renders carried undefined, NaN or [object", r.UndefinedWrites)
	}
}

func TestHighlightKeepsEntitiesIntact(t *testing.T) {
	r := runDOM(t)
	want := []string{
		"a &amp; b", "x &lt;b&gt; y", "q &quot;z&quot;", "it&#39;s",
		"<mark>Amp</mark> &amp; <mark>amp</mark>", "naïve <mark>test</mark>", "<mark>prefill</mark>",
	}
	if !slices.Equal(r.Hilite, want) {
		t.Fatalf("hilite = %q, want %q", r.Hilite, want)
	}
}

func TestNavigationInTheDOMHarness(t *testing.T) {
	r := runDOM(t)
	if len(r.Nav) < 45 {
		t.Fatalf("nav report has %d entries: %v", len(r.Nav), r.Nav)
	}
	for k, v := range r.Nav {
		switch x := v.(type) {
		case bool:
			if !x {
				t.Errorf("nav.%s = false", k)
			}
		}
	}
	want := map[string]any{"pushOnSelect": 1.0, "paletteActive": 0.0, "parseKeys": "cam,l,n,t,turn"}
	for k, w := range want {
		if r.Nav[k] != w {
			t.Errorf("nav.%s = %v, want %v", k, r.Nav[k], w)
		}
	}
	if u, _ := r.Nav["pushUrl"].(string); !strings.HasPrefix(u, "#n=") {
		t.Errorf("select pushed %q, want a #n= fragment", u)
	}
	if u, _ := r.Nav["topicUrl"].(string); !strings.HasPrefix(u, "#t=") || strings.Contains(u, "n=") {
		t.Errorf("up pushed %q, want a #t= fragment", u)
	}
}
