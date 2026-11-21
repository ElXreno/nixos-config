package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mem/internal/store"
)

func (sb *sandbox) file(t *testing.T, name string, lines ...string) string {
	p := filepath.Join(sb.dir, name)
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func (sb *sandbox) getRecord(t *testing.T, key string) getOutput {
	code, out, errs := sb.run("", "get", "--json", key)
	var g getOutput
	if code != 0 || json.Unmarshal([]byte(out), &g) != nil {
		t.Fatalf("get %s: %d %s %s", key, code, out, errs)
	}
	return g
}

func hasLink(links []store.Link, dst string, rel store.Rel) (store.Link, bool) {
	for _, l := range links {
		if l.Dst == dst && l.Rel == rel {
			return l, true
		}
	}
	return store.Link{}, false
}

const recPush = `{"key":"push_needs_ack","kind":"rule","core":false,"title":"Push only on request","body":"Do not git push by default. After a commit wait for explicit authorization; every push is its own action.","scopes":{"hosts":["KURWA"],"projects":["nixos-config"],"tools":["git"]},"triggers":[{"tool":"Bash","regex":"git push"}],"ttl_days":null,"verify_cmd":null,"status":"current","superseded_by":null,"valid_from":"2026-05-01","links":[{"dst":"push_state","rel":"relates"}],"derived_from":["feedback_push"],"evidence":[{"slug":"feedback_push","quote":"explicit authorization"},{"slug":"reference_other","quote":"every push"}]}`

const recState = `{"key":"push_state","kind":"state","core":false,"title":"Seven commits unpushed","body":"agent-guard refactor: seven commits are not pushed to origin yet.","scopes":{"hosts":[],"projects":[],"tools":[]},"triggers":[],"ttl_days":30,"verify_cmd":"git status -sb","status":"current","superseded_by":null,"valid_from":"2026-10-20","links":[],"derived_from":["project_guard"],"evidence":[]}`

func seedLogs(t *testing.T, sb *sandbox) {
	for _, slug := range []string{"feedback_push", "project_guard"} {
		code, _, errs := sb.run(map[string]string{"feedback_push": "legacy notes about publishing commits", "project_guard": "refactor seven pending changesets"}[slug], "put", "--key", slug+"#1", "--kind", "log", "--scope", "project:nixos-config")
		if code != 0 {
			t.Fatalf("seed %s: %s", slug, errs)
		}
	}
}

func TestImportRecords(t *testing.T) {
	sb := newSandbox(t)
	seedLogs(t, sb)
	f := sb.file(t, "in.jsonl", recPush, recState)
	code, out, errs := sb.run("", "import-records", "--json", f)
	if code != 0 {
		t.Fatalf("import: %d %s %s", code, out, errs)
	}
	g := sb.getRecord(t, "push_needs_ack")
	r := g.Record
	if r.Kind != store.Rule || r.Status != store.Current || r.Title != "Push only on request" {
		t.Errorf("record: %+v", r)
	}
	if got := strings.Join(r.Scopes, ","); got != "host:kurwa,project:nixos-config" {
		t.Errorf("scopes %q", got)
	}
	if r.ValidFrom.Format("2006-01-02") != "2026-05-01" || r.CreatedAt.Format("2006-01-02") != "2026-05-01" {
		t.Errorf("valid_from %v created %v", r.ValidFrom, r.CreatedAt)
	}
	if len(r.Triggers) != 1 || r.Triggers[0].Regex != "git push" {
		t.Errorf("triggers %+v", r.Triggers)
	}
	if !strings.HasPrefix(r.Source, "migrate:") {
		t.Errorf("source %q", r.Source)
	}
	l, ok := hasLink(g.Links, "feedback_push#1", store.DerivedFrom)
	if !ok || !strings.Contains(l.Note, "explicit authorization") {
		t.Errorf("derived_from link %+v in %+v", l, g.Links)
	}
	l, ok = hasLink(g.Links, "reference_other", store.Relates)
	if !ok || !strings.Contains(l.Note, "every push") {
		t.Errorf("evidence-only link %+v in %+v", l, g.Links)
	}
	if _, ok := hasLink(g.Links, "push_state", store.Relates); !ok {
		t.Errorf("relates link missing in %+v", g.Links)
	}
	st := sb.getRecord(t, "push_state").Record
	if st.Kind != store.State || st.TTLDays != 30 || st.VerifyCmd != "git status -sb" || strings.Join(st.Scopes, ",") != "global" {
		t.Errorf("state: %+v", st)
	}
	again, _, errs := sb.run("", "import-records", f)
	if again != 3 || !strings.Contains(errs, "already exists") {
		t.Errorf("reimport exit %d: %s", again, errs)
	}
}

func TestImportRecordsGateAndDistinct(t *testing.T) {
	sb := newSandbox(t)
	a := `{"key":"never_push_a","kind":"rule","core":false,"title":"Never push unasked","body":"Never push to the remote by default: after committing, wait until the user explicitly authorizes the push.","scopes":{"hosts":[],"projects":[],"tools":[]},"triggers":[],"ttl_days":null,"verify_cmd":null,"status":"current","superseded_by":null,"valid_from":"2026-05-01","links":[],"derived_from":["s1"],"evidence":[]}`
	b := `{"key":"never_push_b","kind":"rule","core":false,"title":"No push by default","body":"Do not push to the remote by default: after committing, wait until the user explicitly authorizes the push.","scopes":{"hosts":[],"projects":[],"tools":[]},"triggers":[],"ttl_days":null,"verify_cmd":null,"status":"current","superseded_by":null,"valid_from":"2026-05-02","links":[],"derived_from":["s2"],"evidence":[]}`
	c := `{"key":"other_c","kind":"fact","core":false,"title":"Tailscale buffers","body":"Tailscale socketBufferSize is patched to 16 MB.","scopes":{"hosts":[],"projects":[],"tools":[]},"triggers":[],"ttl_days":null,"verify_cmd":null,"status":"current","superseded_by":null,"valid_from":"2026-05-03","links":[],"derived_from":["s3"],"evidence":[]}`
	f := sb.file(t, "in.jsonl", a, b, c)
	conflicts := filepath.Join(sb.dir, "conflicts.jsonl")
	code, _, errs := sb.run("", "import-records", "--conflicts", conflicts, f)
	if code != 3 || !strings.Contains(errs, "never_push_b") {
		t.Fatalf("conflict run: %d %s", code, errs)
	}
	data, err := os.ReadFile(conflicts)
	if err != nil || !strings.Contains(string(data), `"key":"never_push_b"`) || !strings.Contains(string(data), "never_push_a") {
		t.Fatalf("conflicts file: %q %v", data, err)
	}
	if code, _, _ := sb.run("", "get", "never_push_b"); code != 1 {
		t.Errorf("rejected record was written, exit %d", code)
	}
	if code, _, errs := sb.run("", "get", "other_c"); code != 0 {
		t.Errorf("unrelated record missing: %s", errs)
	}
	sb2 := newSandbox(t)
	f2 := sb2.file(t, "in.jsonl", a, b, c)
	dec2 := sb2.file(t, "distinct.jsonl", `{"key":"never_push_b","why":"b covers the agent, a covers the user"}`)
	if code, out, errs := sb2.run("", "import-records", "--distinct", dec2, f2); code != 0 {
		t.Fatalf("distinct run: %d %s %s", code, out, errs)
	}
	g := sb2.getRecord(t, "never_push_b")
	l, ok := hasLink(g.Links, "never_push_a", store.Relates)
	if !ok || !strings.Contains(l.Note, "distinct: b covers the agent") {
		t.Errorf("distinct link %+v in %+v", l, g.Links)
	}
}

func TestImportRecordsSupersessionOrder(t *testing.T) {
	sb := newSandbox(t)
	oldRec := `{"key":"hetzner_plan","kind":"fact","core":false,"title":"Hetzner is the pick","body":"Hetzner storage box with borg is the leading backup pick.","scopes":{"hosts":[],"projects":[],"tools":[]},"triggers":[],"ttl_days":null,"verify_cmd":null,"status":"superseded","superseded_by":"mega_s4","valid_from":"2026-08-01","links":[{"dst":"mega_s4","rel":"supersedes"}],"derived_from":["s1"],"evidence":[]}`
	newRec := `{"key":"mega_s4","kind":"fact","core":false,"title":"MEGA S4 with kopia","body":"Backups go to MEGA S4 through kopia; the provider list is closed.","scopes":{"hosts":[],"projects":[],"tools":[]},"triggers":[],"ttl_days":null,"verify_cmd":null,"status":"current","superseded_by":null,"valid_from":"2026-08-23","links":[{"dst":"hetzner_plan","rel":"supersedes"}],"derived_from":["s2"],"evidence":[]}`
	f := sb.file(t, "in.jsonl", newRec, oldRec)
	if code, out, errs := sb.run("", "import-records", f); code != 0 {
		t.Fatalf("import: %d %s %s", code, out, errs)
	}
	o := sb.getRecord(t, "hetzner_plan").Record
	if o.Status != store.Superseded || o.SupersededBy != "mega_s4" {
		t.Errorf("old record: %+v", o)
	}
	if _, ok := hasLink(sb.getRecord(t, "mega_s4").Links, "hetzner_plan", store.Supersedes); !ok {
		t.Errorf("supersedes link missing")
	}
	if n := sb.getRecord(t, "mega_s4").Record; n.Status != store.Current {
		t.Errorf("new record %+v", n)
	}
	missing := `{"key":"orphan_old","kind":"fact","core":false,"title":"Orphan","body":"Replaced by a record that does not exist anywhere.","scopes":{"hosts":[],"projects":[],"tools":[]},"triggers":[],"ttl_days":null,"verify_cmd":null,"status":"superseded","superseded_by":"nowhere","valid_from":"2026-08-01","links":[],"derived_from":["s9"],"evidence":[]}`
	f2 := sb.file(t, "bad.jsonl", missing)
	code, _, errs := sb.run("", "import-records", f2)
	if code != 2 || !strings.Contains(errs, "nowhere") {
		t.Errorf("dangling superseded_by: %d %s", code, errs)
	}
}

func TestImportRecordsValidationAndDryRun(t *testing.T) {
	sb := newSandbox(t)
	bad := `{"key":"bad_kind","kind":"note","core":false,"title":"x","body":"some body text","scopes":{"hosts":[],"projects":[],"tools":[]},"triggers":[],"ttl_days":null,"verify_cmd":null,"status":"current","superseded_by":null,"valid_from":"2026-05-01","links":[],"derived_from":["s1"],"evidence":[]}`
	good := strings.Replace(recState, `"key":"push_state"`, `"key":"dry_state"`, 1)
	f := sb.file(t, "in.jsonl", good, bad)
	code, _, errs := sb.run("", "import-records", f)
	if code != 2 || !strings.Contains(errs, "bad_kind") {
		t.Fatalf("invalid record: %d %s", code, errs)
	}
	if code, _, _ := sb.run("", "get", "dry_state"); code == 0 {
		t.Errorf("valid record written despite an invalid sibling")
	}
	f2 := sb.file(t, "good.jsonl", good)
	code, out, errs := sb.run("", "import-records", "--dry-run", f2)
	if code != 0 || !strings.Contains(out, "dry run") {
		t.Fatalf("dry run: %d %s %s", code, out, errs)
	}
	if code, _, _ := sb.run("", "get", "dry_state"); code == 0 {
		t.Errorf("dry run wrote a record")
	}
	if code, _, _ := sb.run("", "import-records"); code != 2 {
		t.Errorf("no files: exit %d", code)
	}
	dup := sb.file(t, "dup.jsonl", good, good)
	if code, _, errs := sb.run("", "import-records", dup); code != 2 || !strings.Contains(errs, "duplicate key") {
		t.Errorf("duplicate key in input: %d %s", code, errs)
	}
}

func TestImportRecordsCoreBudget(t *testing.T) {
	sb := newSandbox(t)
	big := `{"key":"big_core","kind":"rule","core":true,"title":"Big","body":"` + strings.Repeat("danger ", 1200) + `","scopes":{"hosts":[],"projects":[],"tools":[]},"triggers":[],"ttl_days":null,"verify_cmd":null,"status":"current","superseded_by":null,"valid_from":"2026-05-01","links":[],"derived_from":["s1"],"evidence":[]}`
	f := sb.file(t, "in.jsonl", big)
	if code, _, errs := sb.run("", "import-records", f); code == 0 || !strings.Contains(errs, "budget") {
		t.Errorf("over budget import: %d %s", code, errs)
	}
}

func TestImportRecordsRelatedFile(t *testing.T) {
	sb := newSandbox(t)
	mk := func(key, body string) string {
		return `{"key":"` + key + `","kind":"fact","core":false,"title":"Tailscale","body":"` + body + `","scopes":{"hosts":[],"projects":[],"tools":[]},"triggers":[],"ttl_days":null,"verify_cmd":null,"status":"current","superseded_by":null,"valid_from":"2026-05-01","links":[],"derived_from":["s1"],"evidence":[]}`
	}
	f := sb.file(t, "in.jsonl", mk("ts_one", "Tailscale socket buffer is sixteen megabytes."), mk("ts_two", "Tailscale exit nodes need an advertised route."))
	rel := filepath.Join(sb.dir, "related.jsonl")
	if code, _, errs := sb.run("", "import-records", "--rel", "0.05", "--related", rel, f); code != 0 {
		t.Fatalf("import: %d %s", code, errs)
	}
	data, err := os.ReadFile(rel)
	if err != nil || !strings.Contains(string(data), `"key":"ts_two"`) || !strings.Contains(string(data), `"ts_one"`) {
		t.Fatalf("related file: %q %v", data, err)
	}
}

func TestImportRecordsScopes(t *testing.T) {
	sb := newSandbox(t)
	mk := func(key string, core bool, hosts, body string) string {
		c := "false"
		if core {
			c = "true"
		}
		return `{"key":"` + key + `","kind":"rule","core":` + c + `,"title":"T","body":"` + body + `","scopes":{"hosts":[` + hosts + `],"projects":[],"tools":[]},"triggers":[],"ttl_days":null,"verify_cmd":null,"status":"current","superseded_by":null,"valid_from":"2026-05-01","links":[],"derived_from":["s1"],"evidence":[]}`
	}
	f := sb.file(t, "in.jsonl",
		mk("danger_core", true, `"BOBER"`, "NEVER touch the phone speakers without permission."),
		mk("plain_host", false, `"BIMBA"`, "Minecraft world lives on the BIMBA data pool."),
		mk("plain_none", false, ``, "Commit titles follow conventional commits."))
	if code, _, errs := sb.run("", "import-records", "--scope", "project:nixos-config", f); code != 0 {
		t.Fatalf("import: %d %s", code, errs)
	}
	want := map[string]string{
		"danger_core": "global,host:bober,project:nixos-config",
		"plain_host":  "host:bimba,project:nixos-config",
		"plain_none":  "global,project:nixos-config",
	}
	for k, w := range want {
		if got := strings.Join(sb.getRecord(t, k).Record.Scopes, ","); got != w {
			t.Errorf("%s scopes %q, want %q", k, got, w)
		}
	}
	if code, _, _ := sb.run("", "import-records", "--scope", "planet:earth", f); code != 2 {
		t.Errorf("bad scope exit %d", code)
	}
}
