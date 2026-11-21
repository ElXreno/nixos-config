package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mem/internal/store"
)

func TestTopicsCommands(t *testing.T) {
	sb := newSandbox(t)
	for k, body := range map[string]string{"alpha-rule": "pushes wait for authorization unique words", "bravo-fact": "the cooler firmware refuses commands unique words", "charlie-howto": "mount the pool with a read only flag"} {
		kind := "fact"
		if k == "alpha-rule" {
			kind = "rule"
		}
		if code, _, errs := sb.run(body, "put", "--key", k, "--kind", kind, "--scope", "global"); code != 0 {
			t.Fatalf("put %s: %d %s", k, code, errs)
		}
	}
	file := filepath.Join(sb.dir, "map.tsv")
	tsv := "alpha-rule\tgit\tGit and pushes\nbravo-fact\thw\tHardware notes\n\nghost-key\thw\tHardware notes\n"
	if err := os.WriteFile(file, []byte(tsv), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errs := sb.run("", "topics", "set", "--map", file)
	if code != 0 || !strings.Contains(out, "assigned 2") || !strings.Contains(out, "unknown keys 1") || !strings.Contains(errs, "ghost-key") {
		t.Fatalf("set: %d %q %q", code, out, errs)
	}
	code, out, _ = sb.run("", "topics", "set", "--map", file, "--json")
	var res store.TopicsResult
	if code != 0 || json.Unmarshal([]byte(out), &res) != nil || res.Assigned != 0 || res.Unchanged != 2 || res.Titles != 0 {
		t.Fatalf("second set: %d %q", code, out)
	}
	if code, _, _ := sb.run("", "topics", "set", "--map", file, "--strict"); code != 3 {
		t.Errorf("strict with an unknown key: exit %d", code)
	}
	code, out, _ = sb.run("", "topics", "list")
	if code != 0 || out != "git\t1\t0\tGit and pushes\nhw\t1\t0\tHardware notes\n" {
		t.Fatalf("list: %d %q", code, out)
	}
	code, out, _ = sb.run("", "topics", "list", "--json")
	var infos []store.TopicInfo
	if code != 0 || json.Unmarshal([]byte(out), &infos) != nil || len(infos) != 2 || infos[0].ID != "git" || infos[1].Records != 1 {
		t.Fatalf("list json: %d %q", code, out)
	}
	code, out, _ = sb.run("", "search", "unique words", "--scope", "topic:hw")
	if code != 0 || !strings.Contains(out, "bravo-fact") || strings.Contains(out, "alpha-rule") {
		t.Errorf("search in a topic: %d %q", code, out)
	}
	if code, _, _ := sb.run("pushes wait for the explicit authorization of the owner, unique words", "put", "--key", "alpha-rule", "--update", "--scope", "global"); code != 0 {
		t.Fatalf("update: %d", code)
	}
	if _, out, _ := sb.run("", "topics", "list"); !strings.HasPrefix(out, "git\t1\t0\t") {
		t.Errorf("update lost the topic: %q", out)
	}
	for _, bad := range []string{"alpha-rule\tgit\n", "alpha-rule\tBad Id\tTitle\n", "alpha-rule\tgit\tA\nbravo-fact\tgit\tB\n"} {
		if code, _, _ := sb.run(bad, "topics", "set", "--map", "-"); code != 2 {
			t.Errorf("bad map %q: exit %d", bad, code)
		}
	}
	if code, _, _ := sb.run("", "topics"); code != 2 {
		t.Errorf("bare topics: exit %d", code)
	}
}

func TestTopicsListEmpty(t *testing.T) {
	sb := newSandbox(t)
	sb.run("a unique body of words", "put", "--key", "k", "--kind", "fact", "--scope", "global")
	code, out, _ := sb.run("", "topics", "list", "--json")
	if code != 0 || strings.TrimSpace(out) != "[]" {
		t.Errorf("%d %q", code, out)
	}
	if code, out, _ := sb.run("", "topics", "list"); code != 0 || out != "" {
		t.Errorf("%d %q", code, out)
	}
}

func TestFreshPutIsSearchableWithoutVectors(t *testing.T) {
	sb := newSandbox(t)
	code, out, errs := sb.run("The wireguard-go tunnel on BIMBA needs a 16 MiB socket buffer to stop UDP receive drops.", "put", "--key", "wg-socket-buffer", "--kind", "fact", "--scope", "global")
	if code != 0 {
		t.Fatalf("put: %d %q %q", code, out, errs)
	}
	for _, mode := range []string{"fused", "words", "trigram"} {
		code, out, _ := sb.run("", "search", "wireguard socket buffer", "--mode", mode)
		if code != 0 || !strings.Contains(out, "wg-socket-buffer") {
			t.Errorf("mode %s: %d %q", mode, code, out)
		}
	}
	code, out, _ = sb.run("", "vec", "status", "--model", "m", "--json")
	if code != 0 || !strings.Contains(out, `"missing": 1`) {
		t.Errorf("vec status: %d %q", code, out)
	}
}

func TestSearchOfNegationWordsFindsRule(t *testing.T) {
	sb := newSandbox(t)
	code, out, errs := sb.run("Нельзя пушить в nixos-config без подтверждения владельца.", "put", "--key", "no-push-rule", "--kind", "rule", "--scope", "global")
	if code != 0 {
		t.Fatalf("put: %d %q %q", code, out, errs)
	}
	for _, q := range []string{"нельзя", "никогда нельзя", "и без", "в и"} {
		code, out, _ := sb.run("", "search", q)
		if code != 0 || !strings.Contains(out, "no-push-rule") {
			t.Errorf("search %q: %d %q", q, code, out)
		}
	}
}
