package hook

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"mem/internal/statefile"
)

const pushPrompt = "Can I git push the commit to the remote now, or do I wait for your authorization first?"

func (sb *sandbox) turnsPath() string {
	return filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", statefile.TurnsName)
}

func (sb *sandbox) turns(t *testing.T) []statefile.Turn {
	t.Helper()
	data, err := os.ReadFile(sb.turnsPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []statefile.Turn
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var tl statefile.Turn
		if err := json.Unmarshal([]byte(l), &tl); err != nil {
			t.Fatalf("turns line is not JSON: %v", err)
		}
		out = append(out, tl)
	}
	return out
}

func injected(t *testing.T, out string) string {
	t.Helper()
	var env envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not hook JSON: %v", err)
	}
	return env.HookSpecificOutput.AdditionalContext
}

func keysOf(ks []statefile.Key) []string {
	out := []string{}
	for _, k := range ks {
		out = append(out, k.Key)
	}
	return out
}

func TestTurnsLineForEachSearchedPrompt(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	_, out, _ := sb.run(promptInput(t, sb, "s1", "  "+pushPrompt+"\n"), UserPromptSubmit)
	sb.run(promptInput(t, sb, "s1", "write a haiku about autumn leaves falling in the park"), UserPromptSubmit)
	sb.run(promptInput(t, sb, "s1", "/compact now"), UserPromptSubmit)
	sb.run(promptInput(t, sb, "s1", "   "), UserPromptSubmit)
	lines := sb.turns(t)
	if len(lines) != 2 {
		t.Fatalf("%d turns lines, want one per prompt that reached the search: %+v", len(lines), lines)
	}
	a, b := lines[0], lines[1]
	if a.Event != statefile.EventPrompt || a.Session != "s1" || a.Agent != "" || a.Status != "inject" || a.Mode != "lexical" || a.Pool != "lexical" {
		t.Errorf("first line %+v", a)
	}
	if a.PromptSHA != promptSHA(pushPrompt) || a.Prompt != pushPrompt || a.PromptCut || a.PromptChars != utf8.RuneCountInString(pushPrompt) {
		t.Errorf("prompt fields %q %q %d", a.PromptSHA, a.Prompt, a.PromptChars)
	}
	if _, err := time.Parse(time.RFC3339, a.Time); err != nil || a.Time != hookTime.Format(time.RFC3339) {
		t.Errorf("time %q %v", a.Time, err)
	}
	if !slices.Equal(a.Lexical, []string{"no-push-default"}) || len(a.Strong) != 0 || len(a.Pointers) != 0 || a.Embed != "" {
		t.Errorf("keys %+v", a)
	}
	if a.Chars != utf8.RuneCountInString(injected(t, out)) || a.TotalMs < 0 {
		t.Errorf("chars %d, injected %d, total %d", a.Chars, utf8.RuneCountInString(injected(t, out)), a.TotalMs)
	}
	if b.Status != "none" || len(b.Lexical) != 0 || b.Chars != 0 || b.Prompt == "" {
		t.Errorf("second line %+v", b)
	}
	fi, err := os.Stat(sb.turnsPath())
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("turns file %v %v", fi, err)
	}
}

func TestTurnsPreviewIsCollapsedAndCut(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	body := strings.Repeat("пуш  коммита\tв\n\nремоут ", 12)
	prompt := "git   push\n\n" + body
	sb.run(promptInput(t, sb, "p1", prompt), UserPromptSubmit)
	lines := sb.turns(t)
	if len(lines) != 1 {
		t.Fatalf("%d lines", len(lines))
	}
	want := string([]rune(strings.Join(strings.Fields(prompt), " "))[:statefile.PreviewRunes])
	if lines[0].Prompt != want || !lines[0].PromptCut || utf8.RuneCountInString(lines[0].Prompt) != statefile.PreviewRunes {
		t.Errorf("preview %q, want %q", lines[0].Prompt, want)
	}
	if lines[0].PromptChars != utf8.RuneCountInString(strings.TrimSpace(prompt)) || lines[0].PromptSHA != promptSHA(strings.TrimSpace(prompt)) {
		t.Errorf("%+v", lines[0])
	}
}

func TestTurnsPreviewLeavesOutTheAnswerTail(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	code, out, _ := sb.run(sb.fixture(t, "prompt-short"), UserPromptSubmit)
	if code != 0 || !strings.Contains(out, "zfs-arc-ram") {
		t.Fatalf("the short prompt did not take the answer tail: %d %q", code, out)
	}
	lines := sb.turns(t)
	if len(lines) != 1 || lines[0].Prompt != "да, давай" || lines[0].PromptChars != 9 || lines[0].PromptSHA != promptSHA("да, давай") {
		t.Fatalf("%+v", lines)
	}
	data, _ := os.ReadFile(sb.turnsPath())
	for _, w := range []string{"ARC", "zfs_arc_max", "Shall I"} {
		if strings.Contains(string(data), w) {
			t.Errorf("turns line carries the answer tail %q", w)
		}
	}
}

func TestTurnsPreviewStaysOutOfHookLogAndShadow(t *testing.T) {
	const marker = "zxqvturnsmarker"
	prompt := "Can I git push " + marker + " upstream now?"
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{})
	args := append(append([]string{UserPromptSubmit}, shadowBase...), "--vec-pool", "reg", "--shadow-vec-pool", "fused-all")
	sb.run(promptInput(t, sb, "m1", prompt), args...)
	useEmbedder(t, echoErrEmbedder{})
	sb.run(promptInput(t, sb, "m2", prompt), args...)
	turns, _ := os.ReadFile(sb.turnsPath())
	if strings.Count(string(turns), marker) != 2 {
		t.Fatalf("turns file lacks the preview: %d", strings.Count(string(turns), marker))
	}
	for _, p := range []string{sb.shadowPath(), sb.logPath()} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), marker) || strings.Contains(string(data), "git push") {
			t.Errorf("%s carries prompt text", filepath.Base(p))
		}
	}
	if l := sb.shadowLines(t); len(l) != 2 || l[0].Prompt != sb.turns(t)[0].PromptSHA {
		t.Errorf("shadow and turns do not share the prompt hash: %+v", l)
	}
}

func TestTurnsVectorTiersCarryCosines(t *testing.T) {
	sb := tierSandbox(t)
	got := sb.prompt(t, "t1", tierFlags()...)
	if !strings.Contains(got, "bs3pro-0xdf") || !strings.Contains(got, "settings-json-copy") {
		t.Fatalf("tier output %q", got)
	}
	lines := sb.turns(t)
	if len(lines) != 1 {
		t.Fatalf("%d lines", len(lines))
	}
	l := lines[0]
	if l.Mode != "vector" || l.Pool != "reg" || l.Embed != "ok" || l.EmbedMs < 0 || l.Status != "inject" || len(l.Lexical) != 0 {
		t.Errorf("%+v", l)
	}
	if !slices.Equal(keysOf(l.Strong), []string{"no-push-default", "bs3pro-0xdf"}) || !slices.Equal(keysOf(l.Pointers), []string{"zfs-arc-ram", "settings-json-copy"}) {
		t.Fatalf("strong %+v pointers %+v", l.Strong, l.Pointers)
	}
	for _, k := range append(append([]statefile.Key{}, l.Strong...), l.Pointers...) {
		want := map[string]float32{"no-push-default": 0.8, "bs3pro-0xdf": strongCos, "zfs-arc-ram": justBelow(strongCos), "settings-json-copy": pointerCos}[k.Key]
		if d := k.Cos - roundCos(float64(want)); d > 1e-9 || d < -1e-9 {
			t.Errorf("%s cos %v, want %v", k.Key, k.Cos, roundCos(float64(want)))
		}
	}
	if l.Chars != utf8.RuneCountInString(got) {
		t.Errorf("chars %d, injected %d", l.Chars, utf8.RuneCountInString(got))
	}
}

func TestTurnsEmbedFailureFallsBackToLexical(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.indexVectors(t)
	useEmbedder(t, &keywordEmbedder{err: errors.New("dial tcp: connection refused")})
	sb.run(promptInput(t, sb, "f1", pushPrompt), append([]string{UserPromptSubmit}, vecFlags...)...)
	useEmbedder(t, &keywordEmbedder{delay: 3 * time.Second})
	sb.run(promptInput(t, sb, "f2", pushPrompt), append(append([]string{UserPromptSubmit}, vecFlags...), "--embed-timeout", "80ms")...)
	lines := sb.turns(t)
	if len(lines) != 2 {
		t.Fatalf("%d lines", len(lines))
	}
	for i, want := range []string{"response", "timeout"} {
		l := lines[i]
		if l.Mode != "vector" || l.Embed != want || l.Status != "inject" || !slices.Equal(l.Lexical, []string{"no-push-default"}) || len(l.Strong) != 0 {
			t.Errorf("line %d: %+v", i, l)
		}
	}
	if lines[1].EmbedMs < 60 {
		t.Errorf("timeout embed ms %d", lines[1].EmbedMs)
	}
}

func TestTurnsPreToolUseInjections(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	for _, f := range []string{"ptu-bash-0xdf", "ptu-bash-0xdf", "ptu-write-memory", "ptu-bash-ls", "ptu-bash-push-agent"} {
		if code, _, _ := sb.run(sb.fixture(t, f), PreToolUse); code != 0 {
			t.Fatalf("%s exit %d", f, code)
		}
	}
	lines := sb.turns(t)
	if len(lines) != 2 {
		t.Fatalf("%d lines, want one per injection: %+v", len(lines), lines)
	}
	a, b := lines[0], lines[1]
	if a.Event != statefile.EventTool || a.Tool != "Bash" || a.Session != "s1" || a.Agent != "" || !slices.Equal(a.Keys, []string{"bs3pro-0xdf"}) || a.Status != "inject" || a.Chars <= 0 {
		t.Errorf("first %+v", a)
	}
	if a.Prompt != "" || a.PromptSHA != "" {
		t.Errorf("tool line carries prompt fields %+v", a)
	}
	if b.Agent != "a1" || !slices.Equal(b.Keys, []string{"no-push-default"}) {
		t.Errorf("subagent %+v", b)
	}
	if a.Input != "flydigictl raw 0xDF 0x01" || b.Input != "git -C . push --force-with-lease" || b.AgentType != "verifier" {
		t.Errorf("tool previews %q %q agent type %q", a.Input, b.Input, b.AgentType)
	}
	data, _ := os.ReadFile(sb.turnsPath())
	if strings.Contains(string(data), "tool_input") || strings.Contains(string(data), "tool_use_id") {
		t.Error("turns line carries the raw tool input")
	}
}

func TestTurnsLogCanBeTurnedOff(t *testing.T) {
	for _, c := range []struct {
		event, stdin string
	}{{UserPromptSubmit, "prompt-push"}, {PreToolUse, "ptu-bash-0xdf"}} {
		ref, off := newSandbox(t), newSandbox(t)
		ref.seed(t)
		off.seed(t)
		codeA, outA, errA := ref.run(ref.fixture(t, c.stdin), c.event)
		codeB, outB, errB := off.run(off.fixture(t, c.stdin), c.event, "--no-turns-log")
		if codeA != codeB || outA != outB || errA != errB || outA == "" {
			t.Errorf("%s: output changed by --no-turns-log: %q vs %q", c.event, outA, outB)
		}
		if _, err := os.Stat(off.turnsPath()); err == nil {
			t.Errorf("%s: turns file written with --no-turns-log", c.event)
		}
		if len(ref.turns(t)) != 1 {
			t.Errorf("%s: no turns line by default", c.event)
		}
	}
	sb := newSandbox(t)
	sb.seed(t)
	if code, _, _ := sb.run(sb.fixture(t, "session-start-startup"), SessionStart, "--no-turns-log"); code != 0 || strings.Contains(sb.lastLog(t), "bad flags") {
		t.Errorf("session-start rejected --no-turns-log: %q", sb.lastLog(t))
	}
}

func TestTurnsLogIsRotated(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	if err := os.MkdirAll(filepath.Dir(sb.turnsPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sb.turnsPath(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(sb.turnsPath(), statefile.MaxTurnsSize+1); err != nil {
		t.Fatal(err)
	}
	sb.run(sb.fixture(t, "prompt-push"), UserPromptSubmit)
	if fi, err := os.Stat(sb.turnsPath() + ".1"); err != nil || fi.Size() != statefile.MaxTurnsSize+1 {
		t.Errorf("old log not kept: %v %v", fi, err)
	}
	if l := sb.turns(t); len(l) != 1 {
		t.Errorf("%d lines in the fresh log", len(l))
	}
}

func TestTurnsLineAfterAPartialLineStaysParseable(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	if err := os.MkdirAll(filepath.Dir(sb.turnsPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sb.turnsPath(), []byte(`{"time":"2026-11-07T00:00:00Z","sess`), 0o600); err != nil {
		t.Fatal(err)
	}
	sb.run(sb.fixture(t, "prompt-push"), UserPromptSubmit)
	data, _ := os.ReadFile(sb.turnsPath())
	parts := strings.Split(strings.TrimSpace(string(data)), "\n")
	var l statefile.Turn
	if len(parts) != 2 || json.Unmarshal([]byte(parts[1]), &l) != nil || l.Session != "s1" {
		t.Errorf("%q", data)
	}
}

func TestTurnsWriteFailureNeverChangesTheHook(t *testing.T) {
	breakers := map[string]func(t *testing.T, path string){
		"directory": func(t *testing.T, path string) {
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"disk full": func(t *testing.T, path string) {
			if _, err := os.Stat("/dev/full"); err != nil {
				t.Skip("no /dev/full")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("/dev/full", path); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, breakIt := range breakers {
		t.Run(name, func(t *testing.T) {
			for _, c := range []struct {
				event, fixture string
			}{{UserPromptSubmit, "prompt-push"}, {PreToolUse, "ptu-bash-0xdf"}} {
				ref, sb := newSandbox(t), newSandbox(t)
				ref.seed(t)
				sb.seed(t)
				_, want, _ := ref.run(ref.fixture(t, c.fixture), c.event)
				breakIt(t, sb.turnsPath())
				code, out, errs := sb.run(sb.fixture(t, c.fixture), c.event)
				if code != 0 || out != want || errs != "" || want == "" {
					t.Errorf("%s: %d %q %q, want %q", c.event, code, out, errs, want)
				}
				data, _ := os.ReadFile(sb.logPath())
				if !strings.Contains(string(data), "turns:") || strings.Contains(string(data), "git push") {
					t.Errorf("%s: log %q", c.event, data)
				}
				if !strings.Contains(string(data), c.event+" session=s1 inject") {
					t.Errorf("%s: the live log line is gone: %q", c.event, data)
				}
			}
		})
	}
}

func TestNonUserPromptsAreSkipped(t *testing.T) {
	cases := []struct {
		prompt, source string
		args           []string
	}{
		{"<agent-message from=\"verifier\">Can I git push the commit to the remote now, or do I wait for your authorization first?</agent-message>", "<agent-message", nil},
		{"  \n<task-notification>\n<task-id>b1</task-id> git push the commit to the remote, authorization</task-notification>", "<task-notification", nil},
		{"<system-reminder>Can I git push the commit to the remote now, or do I wait for your authorization first?</system-reminder>", "<system-reminder", nil},
		{"<local-command-stdout>git push the commit to the remote, authorization</local-command-stdout>", "<local-command-", nil},
		{"[cron nightly] Can I git push the commit to the remote now, or do I wait for your authorization first?", "[cron nightly]", []string{"--skip-prefix", "[scheduled]", "--skip-prefix", "[cron nightly]"}},
		{"[scheduled] Can I git push the commit to the remote now, or do I wait for your authorization first?", "[scheduled]", []string{"--skip-prefix", "[scheduled]", "--skip-prefix", "[cron nightly]"}},
	}
	for _, c := range cases {
		t.Run(c.source, func(t *testing.T) {
			sb := newSandbox(t)
			sb.seed(t)
			code, out, errs := sb.run(promptInput(t, sb, "sk", c.prompt), append([]string{UserPromptSubmit}, c.args...)...)
			if code != 0 || out != "" || errs != "" {
				t.Errorf("exit %d stdout %q stderr %q", code, out, errs)
			}
			lines := sb.turns(t)
			if len(lines) != 1 || lines[0].Status != "skipped" || lines[0].Source != c.source || lines[0].PromptSHA != promptSHA(strings.TrimSpace(c.prompt)) || len(lines[0].Lexical) != 0 {
				t.Fatalf("turns %+v", lines)
			}
			if line := sb.lastLog(t); !strings.Contains(line, "user-prompt-submit session=sk skipped") || !strings.Contains(line, "source "+c.source) {
				t.Errorf("log line %q", line)
			}
			if _, err := os.Stat(filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", "sessions", "sk.keys")); err == nil {
				t.Error("a skipped prompt touched the session set")
			}
		})
	}
}

func TestPromptsMentioningMarkersLaterAreNotSkipped(t *testing.T) {
	for _, body := range []string{
		pushPrompt + " <agent-message from=\"x\">",
		pushPrompt + "\n<task-notification> and <system-reminder> and <local-command-stdout> show up in the transcript",
		"[scheduled] " + pushPrompt,
	} {
		sb := newSandbox(t)
		sb.seed(t)
		_, out, _ := sb.run(promptInput(t, sb, "ns", body), UserPromptSubmit)
		if !strings.Contains(body, "transcript") && !strings.Contains(out, "no-push-default") {
			t.Errorf("%q skipped: %q", body, out)
		}
		if l := sb.turns(t); len(l) != 1 || l[0].Status == "skipped" || l[0].Source != "" || l[0].Status == "" {
			t.Errorf("turns %+v", l)
		}
	}
}
