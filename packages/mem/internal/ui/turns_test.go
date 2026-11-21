package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"mem/internal/env"
	"mem/internal/hook"
	"mem/internal/statefile"
	"mem/internal/store"
)

var tTurn = time.Date(2026, 11, 7, 11, 0, 0, 0, time.UTC)

func stateFixture(t *testing.T) (*fixture, string) {
	t.Helper()
	f := newFixture(t)
	dir := t.TempDir()
	srv := New(f.st, Options{Host: "smok", Home: "/home/x", Now: func() time.Time { return tNow }, StateDir: dir}, false)
	return &fixture{st: f.st, srv: srv, h: srv.Handler()}, dir
}

func appendRaw(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func appendJSON(t *testing.T, path string, vs ...any) {
	t.Helper()
	var b bytes.Buffer
	for _, v := range vs {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	appendRaw(t, path, b.String())
}

func at(d time.Duration) string { return tTurn.Add(d).Format(time.RFC3339) }

func promptTurn(session, sha, preview string, d time.Duration) statefile.Turn {
	return statefile.Turn{Time: at(d), Event: statefile.EventPrompt, Session: session, PromptSHA: sha, Prompt: preview, PromptChars: len([]rune(preview)),
		Mode: "vector", Pool: "reg", Status: "inject", Embed: "ok", EmbedMs: 12, TotalMs: 40, Chars: 300}
}

type shadowFix struct {
	Time     string          `json:"time"`
	Session  string          `json:"session"`
	Agent    string          `json:"agent,omitempty"`
	Prompt   string          `json:"prompt_sha"`
	Chars    int             `json:"prompt_chars"`
	Pool     string          `json:"pool"`
	LiveMode string          `json:"live_mode"`
	LiveKeys []string        `json:"live_keys"`
	Hits     []shadowHitFix  `json:"shadow_hits"`
	Keys     []string        `json:"shadow_keys"`
	Embed    string          `json:"embed"`
	EmbedMs  int64           `json:"embed_ms"`
	LivePtrs []string        `json:"live_pointers,omitempty"`
	Ptrs     *[]string       `json:"shadow_pointers,omitempty"`
	Extra    json.RawMessage `json:"x,omitempty"`
}

type shadowHitFix struct {
	Key  string  `json:"key"`
	Cos  float64 `json:"cos"`
	Log  bool    `json:"log"`
	Gate string  `json:"gate"`
}

type turnsResp struct {
	Turns   []turnOut `json:"turns"`
	Cursor  string    `json:"cursor"`
	Skipped int       `json:"skipped"`
}

func getTurns(t *testing.T, f *fixture, q url.Values) turnsResp {
	t.Helper()
	var r turnsResp
	f.json(t, "/api/turns?"+q.Encode(), &r)
	if r.Turns == nil || r.Cursor == "" {
		t.Fatalf("turns %v cursor %q", r.Turns, r.Cursor)
	}
	return r
}

func previews(r turnsResp) []string {
	out := []string{}
	for _, tr := range r.Turns {
		if tr.Event == statefile.EventTool {
			out = append(out, "tool:"+tr.Tool)
			continue
		}
		out = append(out, tr.Prompt)
	}
	return out
}

func keyNames(ks []statefile.Key) []string {
	out := []string{}
	for _, k := range ks {
		out = append(out, k.Key)
	}
	return out
}

func TestTurnsJoinTheShadow(t *testing.T) {
	f, dir := stateFixture(t)
	const sa, sb, sc = "aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb", "cccccccccccccccc"
	first := promptTurn("s1", sa, "first prompt", 0)
	first.Strong = []statefile.Key{{Key: "feedback_alpha", Cos: 0.81}}
	first.Pointers = []statefile.Key{{Key: "reference_beta", Cos: 0.55}}
	first.Lexical = []string{"reference_lonely"}
	tool := statefile.Turn{Time: at(2 * time.Second), Event: statefile.EventTool, Session: "s1", Tool: "Bash", Keys: []string{"reference_gamma"}, Status: "inject", Chars: 90}
	second := promptTurn("s2", sb, "second prompt", 5*time.Second)
	again := promptTurn("s1", sa, "first prompt", 120*time.Second)
	appendJSON(t, filepath.Join(dir, statefile.TurnsName), first, tool, second, again)
	ptrs := []string{"reference_net"}
	appendJSON(t, filepath.Join(dir, statefile.ShadowName),
		shadowFix{Time: at(0), Session: "s2", Prompt: sa, Pool: "fused-reg", Keys: []string{"decoy"}, Embed: "ok"},
		shadowFix{Time: at(time.Second), Session: "s1", Prompt: sa, Pool: "fused-reg", Embed: "ok", EmbedMs: 9, Keys: []string{"feedback_alpha", "user_owner"}, Ptrs: &ptrs,
			Hits: []shadowHitFix{{Key: "feedback_alpha", Cos: 0.81, Gate: "pass"}, {Key: "user_owner", Cos: 0.66, Gate: "pass"}, {Key: "reference_net", Cos: 0.52, Gate: "pointer"}}},
		shadowFix{Time: at(time.Second), Session: "s1", Prompt: sc, Pool: "fused-reg", Keys: []string{"wrong-sha"}, Embed: "ok"},
		shadowFix{Time: at(121 * time.Second), Session: "s1", Prompt: sa, Pool: "fused-reg", Keys: []string{"later"}, Embed: "timeout",
			Hits: []shadowHitFix{{Key: "later", Cos: 0.7, Gate: "pass"}}},
	)
	r := getTurns(t, f, nil)
	if !slices.Equal(previews(r), []string{"first prompt", "tool:Bash", "second prompt", "first prompt"}) {
		t.Fatalf("order %q", previews(r))
	}
	ids := map[string]bool{}
	for _, tr := range r.Turns {
		if tr.ID == "" || ids[tr.ID] || tr.Origin != "turns" {
			t.Fatalf("id %q origin %q", tr.ID, tr.Origin)
		}
		ids[tr.ID] = true
	}
	a := r.Turns[0]
	if a.Shadow == nil || a.Shadow.Pool != "fused-reg" || a.Shadow.Embed != "ok" || a.Shadow.EmbedMs != 9 {
		t.Fatalf("shadow of the first turn %+v", a.Shadow)
	}
	if !slices.Equal(keyNames(a.Shadow.Keys), []string{"feedback_alpha", "user_owner"}) || a.Shadow.Keys[1].Cos != 0.66 ||
		!slices.Equal(keyNames(a.Shadow.Pointers), []string{"reference_net"}) || a.Shadow.Pointers[0].Cos != 0.52 ||
		!slices.Equal(keyNames(a.Shadow.Only), []string{"user_owner", "reference_net"}) {
		t.Fatalf("shadow keys %+v", a.Shadow)
	}
	if a.Strong[0].Cos != 0.81 || a.Pointers[0].Key != "reference_beta" || !slices.Equal(a.Lexical, []string{"reference_lonely"}) || a.Embed != "ok" || a.TotalMs != 40 {
		t.Fatalf("live fields %+v", a)
	}
	if r.Turns[1].Shadow != nil || r.Turns[2].Shadow != nil {
		t.Fatalf("shadow joined across sessions or onto a tool turn: %+v %+v", r.Turns[1].Shadow, r.Turns[2].Shadow)
	}
	if d := r.Turns[3].Shadow; d == nil || !slices.Equal(keyNames(d.Keys), []string{"later"}) || d.Embed != "timeout" || !slices.Equal(keyNames(d.Only), []string{"later"}) || d.Pointers == nil {
		t.Fatalf("repeated prompt took the wrong shadow: %+v", d)
	}
	one := getTurns(t, f, url.Values{"session": {"s1"}, "limit": {"1"}})
	if len(one.Turns) != 1 || one.Turns[0].Shadow == nil || one.Turns[0].Shadow.Keys[0].Key != "later" {
		t.Fatalf("the last turn alone joined %+v", one.Turns)
	}
}

func TestTurnsCursorFollowsTheFile(t *testing.T) {
	f, dir := stateFixture(t)
	path := filepath.Join(dir, statefile.TurnsName)
	r0 := getTurns(t, f, nil)
	if len(r0.Turns) != 0 {
		t.Fatalf("%+v", r0)
	}
	appendJSON(t, path, promptTurn("s1", "", "one", 0), promptTurn("s1", "", "two", time.Second))
	r1 := getTurns(t, f, url.Values{"since": {r0.Cursor}})
	if !slices.Equal(previews(r1), []string{"one", "two"}) {
		t.Fatalf("%q", previews(r1))
	}
	if r := getTurns(t, f, url.Values{"since": {r1.Cursor}}); len(r.Turns) != 0 || r.Cursor != r1.Cursor {
		t.Fatalf("nothing new: %+v", r)
	}
	appendRaw(t, path, `{"time":"`+at(2*time.Second)+`","event":"prompt","session":"s1","prompt":"three","status":"none"`)
	r2 := getTurns(t, f, url.Values{"since": {r1.Cursor}})
	if len(r2.Turns) != 0 {
		t.Fatalf("partial line returned: %+v", r2)
	}
	appendRaw(t, path, "}\nnot json\n{\"time\":\"bad\",\"event\":\"prompt\"}\n{\"time\":\""+at(3*time.Second)+"\",\"event\":\"other\"}\n")
	r3 := getTurns(t, f, url.Values{"since": {r2.Cursor}})
	if !slices.Equal(previews(r3), []string{"three"}) {
		t.Fatalf("%q", previews(r3))
	}
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	appendJSON(t, path, promptTurn("s1", "", "four", 4*time.Second))
	r4 := getTurns(t, f, url.Values{"since": {r3.Cursor}})
	if !slices.Equal(previews(r4), []string{"four"}) {
		t.Fatalf("after rotation %q", previews(r4))
	}
	all := getTurns(t, f, nil)
	if !slices.Equal(previews(all), []string{"one", "two", "three", "four"}) {
		t.Fatalf("full read %q", previews(all))
	}
	for _, bad := range []string{"/api/turns?since=garbage", "/api/turns?since=c1-xyz-1", "/api/turns?limit=-1", "/api/turns?limit=x", "/api/turns?session=" + strings.Repeat("s", 300)} {
		if rec := f.do(t, http.MethodGet, bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d", bad[:min(len(bad), 40)], rec.Code)
		}
	}
}

func TestTurnsSessionFilterAndLimit(t *testing.T) {
	f, dir := stateFixture(t)
	path := filepath.Join(dir, statefile.TurnsName)
	for i, s := range []string{"s1", "s2", "s1", "s2", "s1"} {
		appendJSON(t, path, promptTurn(s, "", s+"-"+string(rune('a'+i)), time.Duration(i)*time.Second))
	}
	if r := getTurns(t, f, url.Values{"session": {"s1"}}); !slices.Equal(previews(r), []string{"s1-a", "s1-c", "s1-e"}) {
		t.Fatalf("%q", previews(r))
	}
	r := getTurns(t, f, url.Values{"limit": {"2"}})
	if !slices.Equal(previews(r), []string{"s2-d", "s1-e"}) || r.Skipped != 3 {
		t.Fatalf("%q skipped %d", previews(r), r.Skipped)
	}
	z := getTurns(t, f, url.Values{"limit": {"0"}})
	if len(z.Turns) != 0 || z.Skipped != 5 {
		t.Fatalf("limit 0: %+v", z)
	}
	appendJSON(t, path, promptTurn("s2", "", "s2-f", 9*time.Second), promptTurn("s1", "", "s1-g", 10*time.Second))
	if r := getTurns(t, f, url.Values{"since": {z.Cursor}, "session": {"s1"}}); !slices.Equal(previews(r), []string{"s1-g"}) {
		t.Fatalf("since with a session %q", previews(r))
	}
	if r := getTurns(t, f, url.Values{"session": {"nobody"}}); len(r.Turns) != 0 {
		t.Fatalf("%q", previews(r))
	}
}

func TestTurnsClipsOversizedFields(t *testing.T) {
	f, dir := stateFixture(t)
	big := promptTurn(strings.Repeat("s", 500), "", strings.Repeat("я", 5000), 0)
	for i := range 300 {
		big.Lexical = append(big.Lexical, strings.Repeat("k", 10)+string(rune('a'+i%26)))
	}
	appendJSON(t, filepath.Join(dir, statefile.TurnsName), big)
	r := getTurns(t, f, nil)
	if len(r.Turns) != 1 {
		t.Fatalf("%d turns", len(r.Turns))
	}
	tr := r.Turns[0]
	if len([]rune(tr.Prompt)) != statefile.PreviewRunes || len([]rune(tr.Session)) > 128 || len(tr.Lexical) > 100 {
		t.Fatalf("prompt %d session %d lexical %d", len([]rune(tr.Prompt)), len([]rune(tr.Session)), len(tr.Lexical))
	}
}

func TestTurnsWithoutAStateDir(t *testing.T) {
	f := newFixture(t)
	r := getTurns(t, f, nil)
	if len(r.Turns) != 0 {
		t.Fatalf("%+v", r)
	}
	var s struct {
		Sessions []sessionOut `json:"sessions"`
	}
	f.json(t, "/api/sessions", &s)
	if s.Sessions == nil || len(s.Sessions) != 0 {
		t.Fatalf("%+v", s)
	}
	g, _ := stateFixture(t)
	if r := getTurns(t, g, nil); len(r.Turns) != 0 {
		t.Fatalf("empty state dir %+v", r)
	}
}

func TestSessions(t *testing.T) {
	f, dir := stateFixture(t)
	path := filepath.Join(dir, statefile.TurnsName)
	appendJSON(t, path,
		promptTurn("s1", "", "a", 0),
		statefile.Turn{Time: at(10 * time.Second), Event: statefile.EventTool, Session: "s1", Agent: "x", Tool: "Bash", Status: "inject"},
		promptTurn("s2", "", "b", 20*time.Second),
		promptTurn("s1", "", "c", 30*time.Second),
		promptTurn("s3", "", "d", 60*time.Second),
	)
	var s struct {
		Sessions []sessionOut `json:"sessions"`
	}
	f.json(t, "/api/sessions", &s)
	want := []sessionOut{
		{Session: "s3", Test: true, Prompts: 1, First: at(60 * time.Second), Last: at(60 * time.Second)},
		{Session: "s1", Test: true, Prompts: 2, Tools: 1, First: at(0), Last: at(30 * time.Second)},
		{Session: "s2", Test: true, Prompts: 1, First: at(20 * time.Second), Last: at(20 * time.Second)},
	}
	if !slices.Equal(s.Sessions, want) {
		t.Fatalf("%+v", s.Sessions)
	}
}

func TestTurnsFromTheHookLogBeforeTheTurnsLog(t *testing.T) {
	f, dir := stateFixture(t)
	long := "hits 4; embed=ok 9ms pool reg; keys "
	for i := range 16 {
		long += fmt.Sprintf("feedback_al%03d=0.700,", i)
	}
	long += "reference_beta=0.612,reference_gamma=0.6"
	long = string([]rune(long)[:statefile.HookLogDetail]) + "..."
	appendRaw(t, filepath.Join(dir, statefile.HookLogName), strings.Join([]string{
		"2026-11-07T10:00:00Z user-prompt-submit session=old1 inject 41ms hits 3; embed=ok 12ms pool reg; keys feedback_alpha=0.712,reference_beta=lex-novec chars 900; pointers reference_gamma=0.551",
		"2026-11-07T10:00:05Z pre-tool-use session=old1 inject 3ms over 2500 chars, left out reference_gamma; tool Bash keys feedback_alpha,reference_beta chars 200",
		"2026-11-07T10:00:06Z user-prompt-submit session=old1 skip-command 0ms",
		"2026-11-07T10:00:07Z user-prompt-submit session=old1 none 5ms hits 0",
		"2026-11-07T10:00:08Z pre-tool-use session=old1 deny 1ms tool Write",
		"2026-11-07T10:00:09Z user-prompt-submit session=old2 inject 10ms hits 2; over 2500 chars, left out reference_beta; keys feedback_alpha=0.286/2/2,reference_beta=0.100/1/1 chars 300",
		"2026-11-07T10:00:10Z user-prompt-submit session=old2 error 7000ms over the 7s budget",
		"2026-11-07T10:00:11Z session-start session=old2 inject 4ms",
		"2026-11-07T10:00:12Z user-prompt-submit session=old2 inject 20ms " + long,
		"garbage line",
		"2026-11-07T10:00:13Z user-prompt-submit session=old2 embed=timeout 5ms",
		"2026-11-07T11:00:00Z user-prompt-submit session=new inject 9ms hits 1; keys feedback_alpha=lex chars 10",
		"",
	}, "\n"))
	appendJSON(t, filepath.Join(dir, statefile.TurnsName), promptTurn("new", "", "fresh", 0))
	r := getTurns(t, f, nil)
	if len(r.Turns) != 8 {
		t.Fatalf("%d turns: %q", len(r.Turns), previews(r))
	}
	h1, h2, h4, h6, h8, last := r.Turns[0], r.Turns[1], r.Turns[2], r.Turns[3], r.Turns[6], r.Turns[7]
	if e, s := r.Turns[4], r.Turns[5]; e.Status != "error" || e.Event != statefile.EventPrompt || e.TotalMs != 7000 || s.Event != statefile.EventSession || s.Session != "old2" || s.Status != "inject" {
		t.Fatalf("error and session rows from the hook log: %+v %+v", e, s)
	}
	if h1.Origin != "hook.log" || h1.Event != statefile.EventPrompt || h1.Session != "old1" || h1.Prompt != "" || h1.Mode != "vector" || h1.Pool != "reg" ||
		h1.Embed != "ok" || h1.EmbedMs != 12 || h1.TotalMs != 41 || h1.Chars != 900 || h1.Status != "inject" || h1.Time != "2026-11-07T10:00:00Z" {
		t.Fatalf("h1 %+v", h1)
	}
	if !slices.Equal(keyNames(h1.Strong), []string{"feedback_alpha"}) || h1.Strong[0].Cos != 0.712 || !slices.Equal(h1.Lexical, []string{"reference_beta"}) ||
		!slices.Equal(keyNames(h1.Pointers), []string{"reference_gamma"}) || h1.Pointers[0].Cos != 0.551 {
		t.Fatalf("h1 keys %+v", h1)
	}
	if h2.Event != statefile.EventTool || h2.Tool != "Bash" || !slices.Equal(h2.Keys, []string{"feedback_alpha", "reference_beta"}) || h2.Chars != 200 || h2.TotalMs != 3 {
		t.Fatalf("h2 %+v", h2)
	}
	if h4.Status != "none" || len(h4.Strong)+len(h4.Lexical) != 0 || h4.Mode != "lexical" {
		t.Fatalf("h4 %+v", h4)
	}
	if !slices.Equal(h6.Lexical, []string{"feedback_alpha"}) || len(h6.Strong) != 0 || h6.Mode != "lexical" || h6.Pool != "lexical" {
		t.Fatalf("h6 %+v", h6)
	}
	if k := keyNames(h8.Strong); len(k) != 17 || k[0] != "feedback_al000" || k[16] != "reference_beta" || h8.Strong[16].Cos != 0.612 || h8.Chars != 0 {
		t.Fatalf("h8 truncated detail %d %+v", len(k), h8.Strong)
	}
	if last.Origin != "turns" || last.Prompt != "fresh" {
		t.Fatalf("last %+v", last)
	}
	if r := getTurns(t, f, url.Values{"since": {r.Cursor}}); len(r.Turns) != 0 {
		t.Fatalf("hook log lines on a poll: %+v", r.Turns)
	}
	if r := getTurns(t, f, url.Values{"session": {"old2"}}); len(r.Turns) != 4 {
		t.Fatalf("session filter on the hook log: %q", previews(r))
	}
	var s struct {
		Sessions []sessionOut `json:"sessions"`
	}
	f.json(t, "/api/sessions", &s)
	if len(s.Sessions) != 3 || s.Sessions[0].Session != "new" || s.Sessions[2].Session != "old1" || s.Sessions[2].Prompts != 2 || s.Sessions[2].Tools != 1 {
		t.Fatalf("%+v", s.Sessions)
	}
	if err := os.Remove(filepath.Join(dir, statefile.TurnsName)); err != nil {
		t.Fatal(err)
	}
	if r := getTurns(t, f, nil); len(r.Turns) != 8 || r.Turns[7].Origin != "hook.log" || r.Turns[7].Session != "new" {
		t.Fatalf("without a turns log %q", previews(r))
	}
}

func TestTurnsWrittenByTheHook(t *testing.T) {
	root := t.TempDir()
	vars := map[string]string{"HOME": filepath.Join(root, "home"), "XDG_STATE_HOME": filepath.Join(root, "state"), "XDG_DATA_HOME": filepath.Join(root, "data")}
	dbPath := filepath.Join(root, "data", "agent-memory", "mem.db")
	st, err := store.Open(context.Background(), dbPath, store.Options{Now: func() time.Time { return t0 }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	trig := []store.Trigger{{Tool: "Bash", Regex: `\bgit\b.*\bpush\b`}}
	put(t, st, store.PutRequest{Key: "no-push-default", Kind: store.Rule, Scopes: []string{"global"}, Title: sp("No git push by default"),
		Body: sp("Do not git push by default. After a commit, wait for explicit authorization; every push is its own action."), Triggers: &trig})
	run := func(event string, in map[string]any) string {
		data, _ := json.Marshal(in)
		var out, errb bytes.Buffer
		e := env.Env{Stdin: bytes.NewReader(data), Stdout: &out, Stderr: &errb, Getenv: func(k string) string { return vars[k] },
			Now: func() time.Time { return tTurn }, Getwd: func() (string, error) { return root, nil }, Hostname: func() (string, error) { return "smok", nil }}
		if code := hook.Run([]string{event}, e); code != 0 {
			t.Fatalf("hook exit %d", code)
		}
		return out.String()
	}
	prompt := "Can I git   push the commit <b>now</b>,\nor do I wait for your authorization first?"
	if out := run(hook.UserPromptSubmit, map[string]any{"session_id": "live1", "hook_event_name": "UserPromptSubmit", "prompt": prompt, "cwd": root}); !strings.Contains(out, "no-push-default") {
		t.Fatalf("hook did not inject: %q", out)
	}
	run(hook.PreToolUse, map[string]any{"session_id": "live1", "agent_id": "a9", "hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": map[string]string{"command": "git push"}, "cwd": root})
	srv := New(st, Options{Now: func() time.Time { return tNow }, StateDir: filepath.Join(root, "state", "agent-memory")}, false)
	r := getTurns(t, &fixture{st: st, srv: srv, h: srv.Handler()}, nil)
	if len(r.Turns) != 2 {
		t.Fatalf("%d turns", len(r.Turns))
	}
	p, tool := r.Turns[0], r.Turns[1]
	if p.Prompt != strings.Join(strings.Fields(prompt), " ") || p.Session != "live1" || !slices.Equal(p.Lexical, []string{"no-push-default"}) || p.Status != "inject" || p.Time != tTurn.Format(time.RFC3339) {
		t.Fatalf("prompt turn %+v", p)
	}
	if tool.Event != statefile.EventTool || tool.Tool != "Bash" || tool.Agent != "a9" || !slices.Equal(tool.Keys, []string{"no-push-default"}) {
		t.Fatalf("tool turn %+v", tool)
	}
}
