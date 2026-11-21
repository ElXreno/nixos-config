package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func put(t *testing.T, sb *sandbox, key, body string, extra ...string) {
	t.Helper()
	args := append([]string{"put", key, "--kind", "fact", "--scope", "global", "--title", "Title of " + key, "--body", body}, extra...)
	if code, _, errs := sb.run("", args...); code != 0 {
		t.Fatalf("put %s: %d %s", key, code, errs)
	}
}

func TestGetSeveralKeysAndBody(t *testing.T) {
	sb := newSandbox(t)
	put(t, sb, "alpha", "alpha line one\nalpha line two")
	put(t, sb, "beta", "granite basalt obsidian quartz")

	code, full, _ := sb.run("", "get", "alpha")
	_, tail, found := strings.Cut(full, "---\n")
	if code != 0 || !found {
		t.Fatalf("get alpha: %d %q", code, full)
	}
	code, body, errs := sb.run("", "get", "--body", "alpha")
	if code != 0 || body != tail || body != "alpha line one\nalpha line two\n" {
		t.Fatalf("get --body alpha = %d %q %s, want the text after --- %q", code, body, errs, tail)
	}

	code, out, _ := sb.run("", "get", "--body", "alpha", "beta")
	want := "=== alpha\nalpha line one\nalpha line two\n=== beta\ngranite basalt obsidian quartz\n"
	if code != 0 || out != want {
		t.Fatalf("get --body alpha beta = %d %q, want %q", code, out, want)
	}

	code, out, _ = sb.run("", "get", "alpha", "beta")
	if code != 0 || strings.Count(out, "\nkey: ")+1 != 2 || !strings.HasPrefix(out, "key: alpha\n") || !strings.Contains(out, "\n\nkey: beta\n") {
		t.Fatalf("get alpha beta = %d %q", code, out)
	}

	code, out, errs = sb.run("", "get", "--body", "alpha", "ghost", "beta")
	if code != 1 || !strings.Contains(errs, "not found: ghost") || strings.Count(out, "=== ") != 2 {
		t.Fatalf("get with a missing key = %d %q %q", code, out, errs)
	}

	code, out, _ = sb.run("", "get", "--json", "alpha", "beta")
	var many []getOutput
	if code != 0 || json.Unmarshal([]byte(out), &many) != nil || len(many) != 2 || many[1].Record.Key != "beta" {
		t.Fatalf("get --json alpha beta = %d %s", code, out)
	}

	if code, _, errs = sb.run("", "get", "--body", "--history", "alpha"); code != 2 || !strings.Contains(errs, "--body cannot be combined") {
		t.Fatalf("get --body --history = %d %s", code, errs)
	}
	if code, _, _ = sb.run("", "get"); code != 2 {
		t.Fatalf("get without keys = %d", code)
	}
	if code, _, errs = sb.run("", "get", "ghost"); code != 1 || !strings.Contains(errs, "not found: ghost") {
		t.Fatalf("get ghost = %d %s", code, errs)
	}
}

func TestEdit(t *testing.T) {
	sb := newSandbox(t)
	original := "The embedder listens on port 8096.\nThe reranker listens on port 8101.\nBoth run on BIMBA."
	put(t, sb, "ports", original)
	body := func() string {
		_, out, _ := sb.run("", "get", "--body", "ports")
		return strings.TrimSuffix(out, "\n")
	}

	code, out, errs := sb.run("", "edit", "ports", "--old", "port 8096", "--new", "port 8196", "--why", "moved")
	if code != 0 || out != "ports\tupdated\tversion 2\n" {
		t.Fatalf("edit = %d %q %s", code, out, errs)
	}
	edited := strings.Replace(original, "port 8096", "port 8196", 1)
	if got := body(); got != edited {
		t.Fatalf("body after edit = %q, want %q", got, edited)
	}
	_, hist, _ := sb.run("", "history", "ports")
	if !strings.Contains(hist, "=== version 1") || !strings.Contains(hist, "update: moved") || !strings.Contains(hist, "port 8096") {
		t.Fatalf("history lacks the previous version:\n%s", hist)
	}

	code, _, errs = sb.run("", "edit", "ports", "--old", "port 9999", "--new", "x")
	if code != 3 || !strings.Contains(errs, "does not occur in ports") || body() != edited {
		t.Fatalf("edit of absent text = %d %s, body %q", code, errs, body())
	}
	code, _, errs = sb.run("", "edit", "ports", "--old", "listens on", "--new", "serves")
	if code != 3 || !strings.Contains(errs, "occurs 2 times in ports") || !strings.Contains(errs, "--all") || body() != edited {
		t.Fatalf("ambiguous edit = %d %s, body %q", code, errs, body())
	}
	if _, v, _ := sb.run("", "get", "ports"); !strings.Contains(v, "version: 2") {
		t.Fatalf("a rejected edit bumped the version:\n%s", v)
	}

	code, out, errs = sb.run("", "edit", "ports", "--old", "listens on", "--new", "serves", "--all")
	if code != 0 || !strings.Contains(out, "version 3") || strings.Count(body(), "serves") != 2 {
		t.Fatalf("edit --all = %d %q %s, body %q", code, out, errs, body())
	}

	code, _, errs = sb.run("Both run on BIMBA, CPU only.\n", "edit", "ports", "--old", "Both run on BIMBA.", "--new", "-")
	if code != 0 || !strings.HasSuffix(body(), "Both run on BIMBA, CPU only.") {
		t.Fatalf("edit --new - = %d %s, body %q", code, errs, body())
	}
	code, _, errs = sb.run("", "edit", "ports", "--old", "\nBoth run on BIMBA, CPU only.", "--new", "")
	if code != 0 || strings.Contains(body(), "BIMBA") {
		t.Fatalf("edit that deletes = %d %s, body %q", code, errs, body())
	}

	for name, args := range map[string][]string{
		"no key":       {"edit", "--old", "a", "--new", "b"},
		"no new":       {"edit", "ports", "--old", "a"},
		"empty old":    {"edit", "ports", "--old", "", "--new", "b"},
		"same text":    {"edit", "ports", "--old", "serves", "--new", "serves"},
		"stdin twice":  {"edit", "ports", "--old", "-", "--new", "-"},
		"unknown flag": {"edit", "ports", "--old", "a", "--new", "b", "--nope"},
	} {
		if code, _, errs := sb.run("", args...); code != 2 {
			t.Errorf("%s: exit %d, want 2: %s", name, code, errs)
		}
	}
	if code, _, errs = sb.run("", "edit", "ghost", "--old", "a", "--new", "b"); code != 1 || !strings.Contains(errs, "not found") {
		t.Fatalf("edit ghost = %d %s", code, errs)
	}
}

func TestEditKeepsCoreFileAndRetiredRecords(t *testing.T) {
	sb := newSandbox(t)
	put(t, sb, "core-rule", "Never send 0xDF to the cooler.", "--core")
	if code, _, errs := sb.run("", "edit", "core-rule", "--old", "0xDF", "--new", "0xEF"); code != 0 {
		t.Fatalf("edit core: %d %s", code, errs)
	}
	_, core, _ := sb.run("", "core", "--out", "-")
	if !strings.Contains(core, "0xEF") || strings.Contains(core, "0xDF") {
		t.Fatalf("core after edit:\n%s", core)
	}
	if code, _, errs := sb.run("", "retire", "core-rule", "--why", "sold"); code != 0 {
		t.Fatalf("retire: %s", errs)
	}
	if code, _, errs := sb.run("", "edit", "core-rule", "--old", "0xEF", "--new", "0xFF"); code != 3 || !strings.Contains(errs, "retired") {
		t.Fatalf("edit of a retired record = %d %s", code, errs)
	}
}

func TestTTLUnits(t *testing.T) {
	sb := newSandbox(t)
	ttl := func(key string) int {
		_, out, _ := sb.run("", "get", "--json", key)
		var g getOutput
		if err := json.Unmarshal([]byte(out), &g); err != nil {
			t.Fatalf("get --json %s: %v %s", key, err, out)
		}
		return g.Record.TTLDays
	}
	bodies := []string{"granite basalt obsidian", "walrus narwhal beluga", "copper zinc cobalt", "maple birch willow", "violin cello oboe", "saffron cumin fennel"}
	for i, c := range []struct {
		arg  string
		want int
	}{{"30", 30}, {"30d", 30}, {"2w", 14}, {"720h", 30}, {"24H", 1}, {" 45d", 45}} {
		key := "state-" + string(rune('a'+i))
		code, _, errs := sb.run("", "put", key, "--kind", "state", "--scope", "global", "--body", bodies[i], "--ttl", c.arg)
		if code != 0 {
			t.Errorf("--ttl %q = exit %d: %s", c.arg, code, errs)
			continue
		}
		if got := ttl(key); got != c.want {
			t.Errorf("--ttl %q stored %d days, want %d", c.arg, got, c.want)
		}
	}
	for _, bad := range []string{"36h", "1.5d", "-3", "3m", "d", ""} {
		code, _, errs := sb.run("", "put", "bad-ttl", "--kind", "state", "--scope", "global", "--body", "x y z", "--ttl", bad)
		if code != 2 || !strings.Contains(errs, "want days as N or Nd, weeks as Nw, or hours as Nh in whole days") {
			t.Errorf("--ttl %q = exit %d, want 2 with the accepted forms: %s", bad, code, errs)
		}
	}
	if code, _, errs := sb.run("", "put", "state-a", "--update", "--ttl", "8w"); code != 0 || ttl("state-a") != 56 {
		t.Errorf("--update --ttl 8w = %d ttl %d: %s", code, ttl("state-a"), errs)
	}
}

func TestTTLMustLieBetweenOneDayAndAHundredYears(t *testing.T) {
	sb := newSandbox(t)
	ttl := func(key string) int {
		_, out, _ := sb.run("", "get", "--json", key)
		var g getOutput
		if err := json.Unmarshal([]byte(out), &g); err != nil {
			t.Fatalf("get --json %s: %v %s", key, err, out)
		}
		return g.Record.TTLDays
	}
	const want = "want days as N or Nd, weeks as Nw, or hours as Nh in whole days, from 1 to 36500 days"
	for _, bad := range []string{"0", "0d", "0w", "0h", "36501", "36501d", "5215w", "876024h", "999999", "200000w", "2000000000000000000w", "9223372036854775807", "9223372036854775800h"} {
		code, _, errs := sb.run("", "put", "bad-ttl", "--kind", "state", "--scope", "global", "--body", "x y z", "--ttl", bad)
		if code != 2 || !strings.Contains(errs, want) {
			t.Errorf("state --ttl %q = exit %d, want 2 with the accepted forms and the range: %s", bad, code, errs)
		}
	}
	code, _, errs := sb.run("", "put", "zero-fact", "--kind", "fact", "--scope", "global", "--body", "walrus narwhal beluga", "--ttl", "0h")
	if code != 2 {
		t.Errorf("fact --ttl 0h = exit %d, want 2: %s", code, errs)
	}
	code, _, errs = sb.run("", "put", "ttl-fact", "--kind", "fact", "--scope", "global", "--body", "falcon heron osprey", "--ttl", "30")
	if code != 2 || !strings.Contains(errs, "ttl applies to state records only") {
		t.Errorf("fact --ttl 30 = exit %d, want 2: %s", code, errs)
	}
	bodies := []string{"granite basalt obsidian", "maple birch willow", "violin cello oboe", "saffron cumin fennel"}
	for i, c := range []struct {
		arg  string
		want int
	}{{"1", 1}, {"36500", 36500}, {"5214w", 36498}, {"876000h", 36500}} {
		key := "edge-state-" + string(rune('a'+i))
		if code, _, errs := sb.run("", "put", key, "--kind", "state", "--scope", "global", "--body", bodies[i], "--ttl", c.arg); code != 0 {
			t.Errorf("--ttl %q = exit %d: %s", c.arg, code, errs)
			continue
		}
		if got := ttl(key); got != c.want {
			t.Errorf("--ttl %q stored %d days, want %d", c.arg, got, c.want)
		}
	}
	sb.now = sb.now.Add(time.Hour)
	if code, out, errs := sb.run("", "expire"); code != 0 || strings.Contains(out, "expired") {
		t.Errorf("a ttl of up to 36500 days expires within an hour: %d %q %q", code, out, errs)
	}
	if code, _, errs := sb.run("", "put", "long-state", "--kind", "state", "--scope", "global", "--body", "copper zinc cobalt", "--ttl", "30"); code != 0 {
		t.Fatalf("put long-state: %s", errs)
	}
	code, _, errs = sb.run("", "put", "long-state", "--update", "--ttl", "0")
	if code != 2 || ttl("long-state") != 30 {
		t.Errorf("--update --ttl 0 = exit %d, ttl %d, want exit 2 and ttl 30: %s", code, ttl("long-state"), errs)
	}
}

func TestJSONListsAreNeverNull(t *testing.T) {
	sb := newSandbox(t)
	put(t, sb, "alpha", "alpha body")
	code, out, errs := sb.run("", "search", "--mode", "fused", "--json", "zzzqqq")
	if code != 0 || strings.TrimSpace(out) != "[]" {
		t.Errorf("search --json with no hits: %d %q %q, want []", code, out, errs)
	}
	code, out, errs = sb.run("", "get", "--json", "nope1", "nope2")
	if code != 1 || strings.TrimSpace(out) != "[]" || !strings.Contains(errs, "not found: nope1") || !strings.Contains(errs, "not found: nope2") {
		t.Errorf("get --json with every key missing: %d %q %q, want [] and exit 1", code, out, errs)
	}
}

func TestConflictListThreshold(t *testing.T) {
	sb := newSandbox(t)
	put(t, sb, "no-push-default", "Do not git push by default. After a commit, wait for explicit authorization; every push is its own action.")
	code, out, errs := sb.run("", "put", "freetoken-fork-push", "--kind", "fact", "--scope", "global", "--title", "FreeToken fork push allowed",
		"--body", "Pushing to the FreeToken fork is allowed without asking.")
	if code != 0 || out != "freetoken-fork-push\tcreated\tversion 1\n" {
		t.Fatalf("a faint neighbour prints a block: %d %q %q", code, out, errs)
	}
	code, out, _ = sb.run("", "put", "freetoken-fork-push", "--update", "--rel", "0.1", "--body", "Pushing to the FreeToken fork is allowed without asking first.")
	if code != 0 || !strings.Contains(out, "check for conflict:") || !strings.Contains(out, "no-push-default  similarity 0.1") {
		t.Fatalf("--rel 0.1 brings the faint neighbour back: %d %q", code, out)
	}

	put(t, sb, "smok-touchpad-strip-heat", "The light bar next to the trackpad warms the glass to 41 degrees after an hour of gaming; a thermal camera shows the hot spot left of the hinge, and the fans do not react to it.")
	if code, _, errs := sb.run("", "put", "smok-touchpad-strip-heat", "--update", "--title", "SMOK touchpad strip heats the glass"); code != 0 {
		t.Fatalf("retitle: %s", errs)
	}
	code, out, _ = sb.run("", "put", "smok-touchpad-strip-power", "--kind", "fact", "--scope", "global", "--title", "SMOK touchpad strip power states",
		"--body", "On wall power the strip under the palm rest stays lit in every profile; the embedded controller dims it only on battery below 20 percent, and the vendor tool has no switch for it.")
	if code != 0 || !strings.Contains(out, "check for conflict:") || !strings.Contains(out, "smok-touchpad-strip-heat  similarity 0.1") || strings.Contains(out, "no-push-default") {
		t.Fatalf("a neighbour on the same subject by its title is listed below the threshold: %d %q", code, out)
	}
}
