package hook

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"mem/internal/recall"
	"mem/internal/store"
)

type pinEmbedder struct{ cos map[string]float32 }

func (p pinEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, tx := range texts {
		if strings.HasPrefix(tx, "Query: ") {
			out[i] = []float32{1, 0}
			continue
		}
		out[i] = []float32{0, 1}
		for marker, c := range p.cos {
			if strings.Contains(tx, marker) {
				out[i] = []float32{c, float32(math.Sqrt(1 - float64(c)*float64(c)))}
			}
		}
	}
	return out, nil
}

func (sb *sandbox) indexPinned(t *testing.T, emb recall.Embedder) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, sb.dbPath(), store.Options{Now: func() time.Time { return seedTime }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := recall.Reindex(ctx, s, recall.ReindexOptions{Model: fakeModel, DocPrefix: "Document: ", Embedder: emb, Batch: 4, Par: 1}); err != nil {
		t.Fatal(err)
	}
}

func f32(c float32) string { return strconv.FormatFloat(float64(c), 'g', -1, 64) }

const pointerPrompt = "Quokka wombat ping"

var (
	strongCos  = float32(0.70)
	pointerCos = float32(0.50)
	justBelow  = func(c float32) float32 { return math.Nextafter32(c, 0) }
)

func tierEmbedder() pinEmbedder {
	return pinEmbedder{cos: map[string]float32{
		"explicit authorization":    0.80,
		"bricks the Flydigi":        strongCos,
		"grows to half of RAM":      justBelow(strongCos),
		"installed by home-manager": pointerCos,
		"logs visitors":             justBelow(pointerCos),
		"bluetooth headset pairing": 0.55,
	}}
}

func tierFlags(extra ...string) []string {
	base := []string{"--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg",
		"--reg-cos", f32(strongCos), "--reg-pointer-cos", f32(pointerCos)}
	return append(base, extra...)
}

func tierSandbox(t *testing.T) *sandbox {
	t.Helper()
	sb := newSandbox(t)
	sb.seed(t)
	emb := tierEmbedder()
	sb.indexPinned(t, emb)
	useEmbedder(t, emb)
	return sb
}

func (sb *sandbox) context(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	code, out, errs := sb.run(stdin, args...)
	if code != 0 || errs != "" {
		t.Fatalf("exit %d stderr %q", code, errs)
	}
	if out == "" {
		return ""
	}
	var env envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not hook JSON: %v", err)
	}
	return env.HookSpecificOutput.AdditionalContext
}

func (sb *sandbox) prompt(t *testing.T, session string, args ...string) string {
	t.Helper()
	return sb.context(t, promptInput(t, sb, session, pointerPrompt), append([]string{UserPromptSubmit}, args...)...)
}

const (
	zfsLine      = "- `zfs-arc-ram` [fact, as of 2026-10-01]: ZFS ARC size after boot\n"
	settingsLine = "- `settings-json-copy` [fact, as of 2026-10-01]: settings.json is a copied file"
)

func TestPointerTiersAtThresholds(t *testing.T) {
	sb := tierSandbox(t)
	got := sb.prompt(t, "t1", tierFlags()...)
	for _, w := range []string{recall.PromptHeader, "`no-push-default` [rule, as of 2026-10-01]: No git push by default. Do not git push",
		"`bs3pro-0xdf` [rule, as of 2026-10-01]: Command 0xDF is never sent to the BS3 Pro. Command 0xDF bricks", recall.PointerHeader, zfsLine, settingsLine} {
		if !strings.Contains(got, w) {
			t.Errorf("output lacks %q:\n%s", w, got)
		}
	}
	for _, w := range []string{"collusion-wiki", "grows to half of RAM", "installed by home-manager", "bt-journal"} {
		if strings.Contains(got, w) {
			t.Errorf("output has %q:\n%s", w, got)
		}
	}
	if strings.Index(got, recall.PointerHeader) < strings.Index(got, "bs3pro-0xdf") {
		t.Errorf("pointers precede the strong records:\n%s", got)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "pointers zfs-arc-ram=") || !strings.Contains(line, "settings-json-copy=") {
		t.Errorf("log line %q", line)
	}
}

func TestPointersOffByDefault(t *testing.T) {
	sb := tierSandbox(t)
	got := sb.prompt(t, "off", "--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg", "--reg-cos", f32(strongCos))
	if !strings.Contains(got, "no-push-default") || strings.Contains(got, recall.PointerHeader) || strings.Contains(got, "zfs-arc-ram") {
		t.Errorf("default output:\n%s", got)
	}
}

func TestPointerTakeAndOnlyPointers(t *testing.T) {
	sb := tierSandbox(t)
	got := sb.prompt(t, "tk", tierFlags("--pointer-take", "1")...)
	if !strings.Contains(got, "zfs-arc-ram") || strings.Contains(got, "settings-json-copy") {
		t.Errorf("pointer-take 1:\n%s", got)
	}
	got = sb.prompt(t, "tk0", tierFlags("--pointer-take", "0")...)
	if strings.Contains(got, recall.PointerHeader) {
		t.Errorf("pointer-take 0:\n%s", got)
	}
	got = sb.prompt(t, "only", "--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg", "--reg-cos", "0.95", "--reg-pointer-cos", f32(pointerCos))
	if strings.Contains(got, recall.PromptHeader) || !strings.HasPrefix(got, recall.PointerHeader) || !strings.Contains(got, "`no-push-default`") {
		t.Errorf("pointers alone:\n%s", got)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "inject") {
		t.Errorf("log line %q", line)
	}
}

func TestLogPointers(t *testing.T) {
	sb := tierSandbox(t)
	pool := []string{"--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "all", "--reg-cos", "0.95", "--log-take", "1"}
	logCos := f32(0.55)
	got := sb.prompt(t, "lg1", append(pool, "--log-cos", f32(justBelow(0.55)))...)
	if !strings.Contains(got, "bluetooth headset pairing") || strings.Contains(got, recall.PointerHeader) {
		t.Errorf("log chunk at the strong floor:\n%s", got)
	}
	got = sb.prompt(t, "lg2", append(pool, "--log-cos", "0.6", "--log-pointer-cos", logCos)...)
	if !strings.HasPrefix(got, recall.PointerHeader+"- `bt-journal#1` [log, as of 2026-10-01]: 2026-04-12 bluetooth headset pairing") || strings.Count(got, "\n") != 1 {
		t.Errorf("log pointer:\n%s", got)
	}
	if strings.Contains(got, "no-push-default") {
		t.Errorf("regular pointers on without --reg-pointer-cos:\n%s", got)
	}
	got = sb.prompt(t, "lg3", append(pool, "--log-cos", "0.6", "--log-pointer-cos", f32(0.56))...)
	if got != "" {
		t.Errorf("log pointer cos above the cosine:\n%s", got)
	}
}

func pointerCount(text string) int {
	n := 0
	for _, k := range []string{"zfs-arc-ram", "settings-json-copy"} {
		if strings.Contains(text, "`"+k+"`") {
			n++
		}
	}
	return n
}

func TestPointerBudget(t *testing.T) {
	sb := tierSandbox(t)
	full := sb.prompt(t, "bf", tierFlags()...)
	strongOnly := sb.prompt(t, "bs", "--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg", "--reg-cos", f32(strongCos))
	if full == "" || strongOnly == "" || !strings.HasPrefix(full, strongOnly+"\n") {
		t.Fatalf("setup: strong text is not the prefix of the combined one:\n%s\n---\n%s", strongOnly, full)
	}
	hi, lo := utf8.RuneCountInString(full), utf8.RuneCountInString(strongOnly)+1
	need := utf8.RuneCountInString(strongOnly) + utf8.RuneCountInString(recall.PointerHeader+zfsLine)
	last := -1
	for max := lo - 3; max <= hi+2; max++ {
		got := sb.prompt(t, "bm"+strconv.Itoa(max), tierFlags("--max-chars", strconv.Itoa(max))...)
		if n := utf8.RuneCountInString(got); n > max {
			t.Errorf("max %d: %d chars", max, n)
		}
		if max < lo {
			continue
		}
		if !strings.HasPrefix(got, strongOnly) {
			t.Errorf("max %d: strong text changed", max)
		}
		if max < need && pointerCount(got) != 0 {
			t.Errorf("max %d: pointers without room for the first line", max)
		}
		if max == need && !strings.HasSuffix(got, strings.TrimRight(zfsLine, "\n")) {
			t.Errorf("max %d: the first pointer does not fit exactly:\n%s", max, got)
		}
		n := pointerCount(got)
		if n < last {
			t.Errorf("max %d: %d pointers after %d at a smaller budget", max, n, last)
		}
		last = n
		if max == hi && got != full {
			t.Errorf("exact budget lost text:\n%s", got)
		}
		if max == hi-1 && (pointerCount(got) != 1 || !strings.Contains(got, "zfs-arc-ram")) {
			t.Errorf("one char short should keep one pointer:\n%s", got)
		}
		if max == lo && got != strongOnly {
			t.Errorf("strong-only budget has a pointer header:\n%s", got)
		}
	}
	if last != 2 {
		t.Errorf("%d pointers at the full budget", last)
	}
}

func TestPointerDedupAcrossPromptsAndReset(t *testing.T) {
	sb := tierSandbox(t)
	first := sb.prompt(t, "d1", tierFlags()...)
	if !strings.Contains(first, "zfs-arc-ram") || !strings.Contains(first, "settings-json-copy") {
		t.Fatalf("first prompt:\n%s", first)
	}
	if got := sb.prompt(t, "d1", tierFlags()...); got != "" {
		t.Errorf("second prompt repeats:\n%s", got)
	}
	if line := sb.lastLog(t); !strings.Contains(line, "none") {
		t.Errorf("log line %q", line)
	}
	other := sb.prompt(t, "d2", tierFlags()...)
	if other != first {
		t.Errorf("another session sees a different text:\n%s", other)
	}
	promoted := sb.prompt(t, "d1", "--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg",
		"--reg-cos", f32(justBelow(strongCos)), "--reg-pointer-cos", f32(pointerCos))
	if strings.Contains(promoted, "no-push-default") {
		t.Errorf("seen strong record came back:\n%s", promoted)
	}
	if !strings.Contains(promoted, "The ZFS ARC grows to half of RAM") {
		t.Errorf("a pointed record is not injected in full when it passes the strong gate:\n%s", promoted)
	}
	if strings.Contains(promoted, "settings-json-copy") {
		t.Errorf("pointer repeated:\n%s", promoted)
	}
	if got := sb.prompt(t, "d1", tierFlags()...); got != "" {
		t.Errorf("nothing new is left, got:\n%s", got)
	}
	for _, source := range []string{"compact", "clear", "resume"} {
		sess := "r-" + source
		if got := sb.prompt(t, sess, tierFlags()...); got != first {
			t.Fatalf("%s: setup differs", source)
		}
		if got := sb.prompt(t, sess, tierFlags()...); got != "" {
			t.Fatalf("%s: setup repeats", source)
		}
		start, err := json.Marshal(map[string]string{"session_id": sess, "cwd": sb.root + "/repos/nixos-config", "hook_event_name": "SessionStart", "source": source})
		if err != nil {
			t.Fatal(err)
		}
		sb.context(t, string(start), SessionStart)
		got := sb.prompt(t, sess, tierFlags()...)
		if source == "resume" {
			if got != "" {
				t.Errorf("resume reset the sets:\n%s", got)
			}
			continue
		}
		if got != first {
			t.Errorf("%s: after the reset both tiers should come back:\n%s", source, got)
		}
	}
}

func TestPointerMarkersStayOutOfTheKeyList(t *testing.T) {
	sb := tierSandbox(t)
	sb.prompt(t, "m1", tierFlags()...)
	data, err := os.ReadFile(filepath.Join(sb.vars["XDG_STATE_HOME"], "agent-memory", "sessions", "m1.keys"))
	if err != nil {
		t.Fatal(err)
	}
	keys := strings.Fields(string(data))
	for _, want := range []string{"no-push-default", "bs3pro-0xdf"} {
		if !slices.Contains(keys, want) {
			t.Errorf("strong key %s missing from %v", want, keys)
		}
	}
	if slices.Contains(keys, "zfs-arc-ram") || slices.Contains(keys, "settings-json-copy") {
		t.Errorf("pointer marked as seen for strong injection: %v", keys)
	}
}

func TestPointerFlagsAreChecked(t *testing.T) {
	sb := tierSandbox(t)
	for _, extra := range [][]string{{"--pointer-take", "-1"}, {"--reg-pointer-cos", "-0.1"}, {"--log-pointer-cos", "-0.1"}} {
		code, out, _ := sb.run(promptInput(t, sb, "bad", pointerPrompt), append([]string{UserPromptSubmit}, tierFlags(extra...)...)...)
		if code != 0 || out != "" {
			t.Errorf("%v: exit %d stdout %q", extra, code, out)
		}
		if line := sb.lastLog(t); !strings.Contains(line, "bad flags") {
			t.Errorf("%v: log %q", extra, line)
		}
	}
}

func TestShadowLogsBothTiers(t *testing.T) {
	for _, shadow := range []string{"reg", "fused-reg"} {
		t.Run(shadow, func(t *testing.T) {
			sb := tierSandbox(t)
			args := tierFlags("--shadow-vec-pool", shadow)
			sb.prompt(t, "sh", args...)
			sb.prompt(t, "sh", args...)
			lines := sb.shadowLinesPointers(t)
			if len(lines) != 2 {
				t.Fatalf("%d shadow lines", len(lines))
			}
			l := lines[0]
			if !slices.Equal(l.LiveKeys, []string{"no-push-default", "bs3pro-0xdf"}) || !slices.Equal(l.LivePointers, []string{"zfs-arc-ram", "settings-json-copy"}) {
				t.Errorf("live %v %v", l.LiveKeys, l.LivePointers)
			}
			if !slices.Equal(l.ShadowKeys, []string{"no-push-default", "bs3pro-0xdf"}) || !slices.Equal(l.ShadowPointers, []string{"zfs-arc-ram", "settings-json-copy"}) {
				t.Errorf("shadow %v %v", l.ShadowKeys, l.ShadowPointers)
			}
			gates := map[string]string{}
			for _, h := range l.Hits {
				gates[h.Key] = h.Gate
			}
			for k, want := range map[string]string{"no-push-default": "pass", "bs3pro-0xdf": "pass", "zfs-arc-ram": "pointer", "settings-json-copy": "pointer", "collusion-wiki": "take"} {
				if gates[k] != want {
					t.Errorf("gate of %s is %q, want %q", k, gates[k], want)
				}
			}
			l = lines[1]
			if len(l.LiveKeys) != 0 || len(l.LivePointers) != 0 || len(l.ShadowKeys) != 0 || len(l.ShadowPointers) != 0 {
				t.Errorf("second prompt repeats: %+v", l)
			}
		})
	}
}

func TestShadowPointersWithLexicalLive(t *testing.T) {
	sb := tierSandbox(t)
	sb.prompt(t, "lx", "--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--shadow-vec-pool", "reg", "--reg-cos", f32(strongCos), "--reg-pointer-cos", f32(pointerCos))
	lines := sb.shadowLinesPointers(t)
	if len(lines) != 1 || lines[0].LiveMode != "lexical" || len(lines[0].ShadowPointers) != 2 || len(lines[0].LivePointers) != 0 {
		t.Errorf("%+v", lines)
	}
}

type shadowLinePointers struct {
	shadowLineT
	LivePointers   []string `json:"live_pointers"`
	ShadowPointers []string `json:"shadow_pointers"`
}

func (sb *sandbox) shadowLinesPointers(t *testing.T) []shadowLinePointers {
	t.Helper()
	base := sb.shadowLines(t)
	data, err := os.ReadFile(sb.shadowPath())
	if err != nil {
		t.Fatal(err)
	}
	var out []shadowLinePointers
	for i, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var p shadowLinePointers
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(l), &raw); err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{"live_pointers", "shadow_pointers"} {
			if string(raw[f]) == "" || string(raw[f]) == "null" {
				t.Fatalf("shadow line %d lacks %s: %s", i, f, l)
			}
		}
		if err := json.Unmarshal([]byte(l), &p); err != nil {
			t.Fatal(err)
		}
		p.shadowLineT = base[i]
		out = append(out, p)
	}
	return out
}
