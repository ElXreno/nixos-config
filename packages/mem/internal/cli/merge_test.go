package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mem/internal/store"
)

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMerge(t *testing.T) {
	sb := newSandbox(t)
	put := func(db, body string, args ...string) {
		t.Helper()
		a := append([]string{"--db", db, "put"}, args...)
		if code, out, errs := sb.run(body, a...); code != 0 {
			t.Fatalf("put %v: %d %s %s", args, code, out, errs)
		}
	}
	master := filepath.Join(sb.dir, "master.db")
	other := filepath.Join(sb.dir, "other.db")
	put(master, "bitsavers.org answers 403 to curl with the default User-Agent.", "site_bitsavers_org", "--kind", "state", "--ttl", "180d",
		"--scope", "global", "--title", "bitsavers.org blocks plain curl", "--trigger", `Bash:bitsavers\.org`)
	put(master, "Old firmware of the Acme X100 router lives under the vendor FTP pub/x100.", "route_acme_ftp", "--kind", "howto",
		"--scope", "global", "--title", "Acme X100 firmware on the vendor FTP")
	copyFile(t, master, other)
	base := sb.now

	sb.now = base.Add(time.Hour)
	put(other, "bitsavers.org answers 403 to curl with the default User-Agent; a browser User-Agent works.", "site_bitsavers_org", "--update")
	put(other, "snapshot.debian.org has the /mr/package/<name>/ JSON API listing every version.", "site_snapshot_debian_org", "--kind", "state",
		"--scope", "global", "--title", "snapshot.debian.org machine-readable API", "--trigger", `Bash:snapshot\.debian\.org`)
	put(other, "Acme moved the X100 firmware from FTP to the support portal; FTP pub/x100 is empty since 2024.", "route_acme_portal", "--kind", "howto",
		"--scope", "global", "--title", "Acme X100 firmware on the support portal", "--supersedes", "route_acme_ftp")
	sb.now = base.Add(2 * time.Hour)
	put(master, "Wayback CDX digests of 404 captures look like real files: check the status column.", "trap_wayback_cdx_404", "--kind", "incident",
		"--scope", "global", "--title", "Wayback CDX lists 404 captures")
	put(other, "Wayback CDX digests of 404 captures look like real files, always check the status column.", "trap_wayback_404_digest", "--kind", "incident",
		"--scope", "global", "--title", "Wayback CDX lists 404 captures too")

	since := base.Add(time.Minute).Format(time.RFC3339)
	code, out, errs := sb.run("", "--db", master, "merge", "--from", other, "--since", since, "--source", "digger:run1", "--dry-run")
	if code != 3 {
		t.Fatalf("dry run: %d %s %s", code, out, errs)
	}
	for _, want := range []string{
		"update\tsite_bitsavers_org",
		"new\tsite_snapshot_debian_org",
		"new\troute_acme_portal\t[howto] Acme X100 firmware on the support portal\tsupersedes route_acme_ftp",
		"conflict\ttrap_wayback_404_digest",
		"duplicate of trap_wayback_cdx_404",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run lacks %q:\n%s", want, out)
		}
	}
	if code, _, _ := sb.run("", "--db", master, "get", "site_snapshot_debian_org"); code == 0 {
		t.Fatal("dry run wrote a record")
	}

	code, out, errs = sb.run("", "--db", master, "merge", "--from", other, "--since", since, "--source", "digger:run1")
	if code != 3 {
		t.Fatalf("merge: %d %s %s", code, out, errs)
	}
	g := sb.getRecordIn(t, master, "site_bitsavers_org")
	if !strings.Contains(g.Record.Body, "browser User-Agent works") || g.Record.Source != "digger:run1" || g.Record.Version != 2 || g.Record.TTLDays != 180 {
		t.Errorf("updated record: %+v", g.Record)
	}
	if r := sb.getRecordIn(t, master, "site_snapshot_debian_org").Record; len(r.Triggers) != 1 || r.Kind != store.State {
		t.Errorf("new record: %+v", r)
	}
	if r := sb.getRecordIn(t, master, "route_acme_ftp").Record; r.Status != store.Superseded || r.SupersededBy != "route_acme_portal" {
		t.Errorf("superseded record: %+v", r)
	}
	if code, _, _ := sb.run("", "--db", master, "get", "trap_wayback_404_digest"); code == 0 {
		t.Error("the gate let a duplicate through")
	}

	code, out, _ = sb.run("", "--db", master, "merge", "--from", other, "--since", since, "--key", "site_bitsavers_org,site_snapshot_debian_org,route_acme_portal")
	if code != 0 || strings.Count(out, "same\t") != 3 {
		t.Errorf("second merge: %d\n%s", code, out)
	}
}

func (sb *sandbox) getRecordIn(t *testing.T, db, key string) getOutput {
	t.Helper()
	sb.vars["MEM_DB"] = db
	defer delete(sb.vars, "MEM_DB")
	return sb.getRecord(t, key)
}

func TestPutNoGate(t *testing.T) {
	sb := newSandbox(t)
	body := "Dig 2026-11-27: Redis LOLWUT version 5 source, files lolwut5.c from raw.githubusercontent.com."
	if code, out, errs := sb.run(body, "put", "find_run1", "--kind", "fact", "--scope", "global", "--title", "Dig: Redis LOLWUT 5 source"); code != 0 {
		t.Fatalf("first: %d %s %s", code, out, errs)
	}
	if code, _, _ := sb.run(body, "put", "find_run2", "--kind", "fact", "--scope", "global", "--title", "Dig: Redis LOLWUT 5 source"); code != 3 {
		t.Fatalf("the gate should flag the repeat, got %d", code)
	}
	if code, _, errs := sb.run(body, "put", "find_run2", "--kind", "fact", "--scope", "global", "--title", "Dig: Redis LOLWUT 5 source", "--no-gate"); code != 2 || !strings.Contains(errs, "--no-gate needs --why") {
		t.Fatalf("--no-gate without --why: %d %s", code, errs)
	}
	if code, out, errs := sb.run(body, "put", "find_run2", "--kind", "fact", "--scope", "global", "--title", "Dig: Redis LOLWUT 5 source", "--no-gate", "--why", "another dig"); code != 0 {
		t.Fatalf("--no-gate: %d %s %s", code, out, errs)
	}
}
