package legacy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"mem/internal/env"
	"mem/internal/store"
)

func TestFrontmatter(t *testing.T) {
	cases := []struct {
		name, src            string
		has                  bool
		wantName, wantDesc   string
		wantType, wantLayout string
		wantMod              string
		wantBody             string
	}{
		{
			name:     "top level layout",
			src:      "---\nname: Machine inventory\ndescription: Quick reference for clan machines\ntype: reference\noriginSessionId: 8053a831\nmodified: 2026-10-14T06:16:18.726Z\n---\n# Machines\n\nbody\n",
			has:      true,
			wantName: "Machine inventory", wantDesc: "Quick reference for clan machines", wantType: "reference", wantLayout: "top",
			wantMod: "2026-10-14T06:16:18Z", wantBody: "# Machines\n\nbody\n",
		},
		{
			name:     "metadata layout with quoted description",
			src:      "---\nname: feedback_use_fd\ndescription: \"Search tooling - fd, not \\\"find\\\"; never full-scan /nix/store\"\nmetadata: \n  node_type: memory\n  type: feedback\n  originSessionId: 7641aed7\n  modified: 2026-11-01T10:00:00Z\n---\nUse fd.\n",
			has:      true,
			wantName: "feedback_use_fd", wantDesc: `Search tooling - fd, not "find"; never full-scan /nix/store`, wantType: "feedback", wantLayout: "metadata",
			wantMod: "2026-11-01T10:00:00Z", wantBody: "Use fd.\n",
		},
		{
			name:     "single quotes, folded and literal blocks, CRLF",
			src:      "---\r\nname: 'it''s'\r\ndescription: >\r\n  folded\r\n  text\r\nnotes: |\r\n  a\r\n  b\r\ntype: Project\r\n---\r\nx\r\n",
			has:      true,
			wantName: "it's", wantDesc: "folded text", wantType: "project", wantLayout: "top", wantBody: "x\n",
		},
		{
			name:     "byte order mark and unknown layout",
			src:      bom + "---\nname: n\ndescription: d\n---\nbody",
			has:      true,
			wantName: "n", wantDesc: "d", wantLayout: "partial", wantBody: "body",
		},
		{
			name:       "no frontmatter",
			src:        "# DDNet data\n\n## Formats\n",
			wantLayout: "none", wantBody: "# DDNet data\n\n## Formats\n",
		},
		{
			name:       "unterminated frontmatter is body",
			src:        "---\nname: x\nno end",
			wantLayout: "none", wantBody: "---\nname: x\nno end",
		},
		{
			name:       "empty frontmatter",
			src:        "---\n---\nbody",
			has:        true,
			wantLayout: "partial", wantBody: "body",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fm, body, has := splitFrontmatter(c.src)
			if has != c.has {
				t.Fatalf("has = %v, want %v", has, c.has)
			}
			mod := ""
			if m := fm.Modified(); !m.IsZero() {
				mod = m.Format(time.RFC3339)
			}
			got := []string{fm.Name(), fm.Description(), fm.Type(), layoutOf(fm, has), mod, body}
			want := []string{c.wantName, c.wantDesc, c.wantType, c.wantLayout, c.wantMod, c.wantBody}
			if !slices.Equal(got, want) {
				t.Fatalf("got  %q\nwant %q", got, want)
			}
		})
	}
}

func TestKindFor(t *testing.T) {
	cases := []struct {
		slug, fmType string
		want         store.Kind
	}{
		{"feedback_use_fd", "", store.Rule},
		{"reference_machines", "feedback", store.Fact},
		{"project_tormoz_todo", "", store.State},
		{"user_soldering_beginner", "", store.Fact},
		{"self_notes", "", store.Fact},
		{"cuda", "feedback", store.Rule},
		{"cuda", "project", store.State},
		{"cuda", "user", store.Fact},
	}
	for _, c := range cases {
		if got := kindFor(c.slug, c.fmType); got != c.want {
			t.Errorf("kindFor(%q, %q) = %s, want %s", c.slug, c.fmType, got, c.want)
		}
	}
}

func TestTitleFor(t *testing.T) {
	mk := func(name, desc string) frontmatter {
		return frontmatter{top: map[string]string{"name": name, "description": desc}, nested: map[string]map[string]string{}}
	}
	cases := []struct {
		slug string
		fm   frontmatter
		body string
		want string
	}{
		{"feedback_use_fd", mk("feedback_use_fd", "fd not find"), "", "fd not find"},
		{"reference_machines", mk("Machine inventory", "Quick reference"), "", "Machine inventory: Quick reference"},
		{"x", mk("Inventory", "Inventory of   machines"), "", "Inventory of machines"},
		{"commit-title-only", mk("commit title only", ""), "", ""},
		{"x", mk("", ""), "# Heading here\n\ntext", "Heading here"},
		{"x", mk("", ""), "text first\n# Heading", ""},
	}
	for _, c := range cases {
		if got := titleFor(c.slug, c.fm, c.body); got != c.want {
			t.Errorf("titleFor(%q) = %q, want %q", c.slug, got, c.want)
		}
	}
}

func journal(entries int) string {
	var b strings.Builder
	b.WriteString("# Saga\n\nIntro line about the investigation.\n\n")
	for i := 1; i <= entries; i++ {
		_, _ = fmt.Fprintf(&b, "- 2026-10-%02d: entry %d, measured %d W at 5200 MHz, see nvme0n1 and port 127.0.0.1:809%d. %s\n",
			i, i, 40+i, i%10, strings.Repeat("Подробности замера и выводы. ", 6))
	}
	b.WriteString("\n```\n[[:space:]] [[not_a_link]]\n```\n")
	return b.String()
}

func TestJournalAndChunks(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		lines int
	}{
		{"dated header only", "2026-10-14: verified once.\n\nPlain text.", 1},
		{"list journal", "- 2026-10-01: a\n- 2026-10-02: b\n* **2026-10-03** c", 3},
		{"heading journal", "## 2026-10-01 first\n\n### [2026-10-02] second", 2},
		{"dates inside fences are ignored", "```\n2026-10-01 log\n2026-10-02 log\n```\n2026-10-03 real", 1},
		{"date not at line start", "fixed on 2026-10-01 and 2026-10-02", 0},
	}
	for _, c := range cases {
		if got := journalLines(c.body); got != c.lines {
			t.Errorf("%s: journalLines = %d, want %d", c.name, got, c.lines)
		}
	}
	body := journal(30)
	chunks := chunkBody(body, DefaultChunkRunes)
	if len(chunks) < 3 {
		t.Fatalf("got %d chunks for a %d-char journal", len(chunks), runes(body))
	}
	var joined strings.Builder
	for i, c := range chunks {
		if n := runes(c.text); n > DefaultChunkRunes*3/2 {
			t.Errorf("chunk %d has %d chars", i+1, n)
		}
		if c.heading != "Saga" {
			t.Errorf("chunk %d heading %q", i+1, c.heading)
		}
		if i > 0 && !dateLedRe.MatchString(c.text) && !strings.HasPrefix(c.text, "```") {
			t.Errorf("chunk %d does not start at an entry: %.40q", i+1, c.text)
		}
		joined.WriteString(c.text + "\n")
	}
	for _, line := range strings.Split(body, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.Contains(joined.String(), line) {
			t.Errorf("line lost in chunking: %.60q", line)
		}
	}
	long := strings.Repeat("слово ", 1000)
	for i, c := range chunkBody(long, 1200) {
		if n := runes(c.text); n > 1800 || n == 0 {
			t.Errorf("long line chunk %d has %d chars", i+1, n)
		}
	}
	if got := chunkBody("short", 1200); len(got) != 1 || got[0].text != "short" {
		t.Errorf("short body chunks = %+v", got)
	}
}

func TestLinkTargets(t *testing.T) {
	src := "See [[feedback_use_fd]], [[Machine inventory|machines]], [[project_x#section]], [[project_x]] and [[a.md]].\n" +
		"Inline `[[code_link]]` and\n```\n[[fenced_link]]\n```\n"
	want := []string{"feedback_use_fd", "Machine inventory", "project_x", "a"}
	if got := linkTargets(src); !slices.Equal(got, want) {
		t.Fatalf("linkTargets = %q, want %q", got, want)
	}
	r := newResolver()
	for slug, name := range map[string]string{
		"feedback_use_fd":         "feedback_use_fd",
		"reference_machines":      "Machine inventory",
		"feedback_commit_title":   "commit-title",
		"feedback_build_deploy":   "x",
		"project_build_deploy":    "y",
		"reference_smok_hub":      "SMOK hub",
		"project_zfs-bclone-eio":  "",
		"user_soldering_beginner": "",
	} {
		r.add(slug, name)
	}
	cases := []struct {
		target string
		want   string
		ok     bool
	}{
		{"feedback_use_fd", "feedback_use_fd", true},
		{"machine inventory", "reference_machines", true},
		{"commit-title", "feedback_commit_title", true},
		{"commit_title", "feedback_commit_title", true},
		{"project_zfs_bclone_eio", "project_zfs-bclone-eio", true},
		{"soldering-beginner", "user_soldering_beginner", true},
		{"build_deploy", "", false},
		{"missing", "", false},
	}
	for _, c := range cases {
		got, ok := r.resolve(c.target)
		if got != c.want || ok != c.ok {
			t.Errorf("resolve(%q) = %q %v, want %q %v", c.target, got, ok, c.want, c.ok)
		}
	}
}

var fixture = map[string]string{
	"MEMORY.md":             "- feedback_use_fd · reference_machines\n",
	"reference_smok_hub.md": "---\nname: SMOK hub\ndescription: hub\ntype: reference\n---\n- [[reference_machines]]\n",
	"archive.md":            "---\nname: archive\ndescription: old\ntype: reference\n---\n[[feedback_use_fd]]\n",
	"feedback_use_fd.md": "---\nname: feedback_use_fd\ndescription: \"fd, not find\"\nmetadata: \n  node_type: memory\n  type: feedback\n  originSessionId: 1\n  modified: 2026-11-01T10:00:00Z\n---\n" +
		"Use `fd`, never `find` in /nix/store. Related: [[Machine inventory]], [[reference_smok_hub]], [[gone_file]], [[feedback_use_fd]].\n",
	"reference_machines.md": "---\nname: Machine inventory\ndescription: Quick reference for clan machines\ntype: reference\nmodified: 2026-10-14T06:16:18.726Z\n---\n" +
		"SMOK: Ultra 9 290HX. See [[project_saga]].\n",
	"project_saga.md":    "---\nname: project_saga\ndescription: Long investigation\nmetadata:\n  type: project\n---\n" + journal(16),
	"project_small.md":   "---\nname: project_small\ndescription: Small state\ntype: project\n---\n2026-10-14: pushed? no.\n",
	"reference_ddnet.md": "# DDNet data\n\nFormats and [[reference_machines]].\n",
	"notes.txt":          "not markdown",
	".hidden.md":         "---\nname: hidden\n---\nx",
}

func writeFixture(t *testing.T, files map[string]string) string {
	dir := filepath.Join(t.TempDir(), "memory")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "nested.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func snapshot(t *testing.T, dir string) string {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s %v %d %d\n", path, info.Mode(), info.Size(), info.ModTime().UnixNano())
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			_, _ = h.Write(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func openTest(t *testing.T, c *clock) *store.Store {
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "mem.db"), store.Options{Now: c.now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func keysOf(recs []store.Record) []string {
	var out []string
	for _, r := range recs {
		out = append(out, r.Key)
	}
	slices.Sort(out)
	return out
}

func TestLoadPlan(t *testing.T) {
	dir := writeFixture(t, fixture)
	plan, err := Load(dir, Options{Project: "NixOS-Config"})
	if err != nil {
		t.Fatal(err)
	}
	st := plan.Stats
	if st.Files != 8 || st.Indexes != 3 || st.Records != 4 || st.LogFiles != 1 || st.LogByDates != 1 || st.Skipped != 0 {
		t.Fatalf("stats %+v", st)
	}
	if st.Layouts["top"] != 4 || st.Layouts["metadata"] != 2 || st.Layouts["none"] != 2 {
		t.Fatalf("layouts %+v", st.Layouts)
	}
	byKey := map[string]store.Record{}
	for _, r := range plan.Records {
		byKey[r.Key] = r
		if !slices.Equal(r.Scopes, []string{"project:nixos-config"}) {
			t.Errorf("%s scopes %v", r.Key, r.Scopes)
		}
		if !strings.HasPrefix(r.Source, "legacy:"+dir+"/") {
			t.Errorf("%s source %q", r.Key, r.Source)
		}
	}
	for key, want := range map[string]store.Kind{
		"feedback_use_fd": store.Rule, "reference_machines": store.Fact, "project_small": store.State,
		"reference_ddnet": store.Fact, "project_saga#1": store.Log, "project_saga#2": store.Log,
	} {
		if byKey[key].Kind != want {
			t.Errorf("%s kind %q, want %s", key, byKey[key].Kind, want)
		}
	}
	for _, k := range []string{"MEMORY", "reference_smok_hub", "archive", "project_saga", "notes", "hidden", "nested"} {
		if _, ok := byKey[k]; ok {
			t.Errorf("%s must not be a record", k)
		}
	}
	if r := byKey["reference_machines"]; r.Title != "Machine inventory: Quick reference for clan machines" ||
		!r.ValidFrom.Equal(time.Date(2026, 10, 14, 6, 16, 18, 0, time.UTC)) {
		t.Errorf("reference_machines title %q valid_from %v", r.Title, r.ValidFrom)
	}
	if r := byKey["reference_ddnet"]; r.Title != "DDNet data" {
		t.Errorf("no-frontmatter title %q", r.Title)
	}
	if r := byKey["project_saga#2"]; r.Title != "Long investigation / Saga" || r.Source != "legacy:"+filepath.Join(dir, "project_saga.md") {
		t.Errorf("chunk title %q source %q", r.Title, r.Source)
	}
	wantLinks := []store.Link{
		{Src: "feedback_use_fd", Dst: "reference_machines", Rel: store.Relates},
		{Src: "feedback_use_fd", Dst: "reference_smok_hub", Rel: store.Unresolved},
		{Src: "feedback_use_fd", Dst: "gone_file", Rel: store.Unresolved},
		{Src: "reference_ddnet", Dst: "reference_machines", Rel: store.Relates},
		{Src: "reference_machines", Dst: "project_saga#1", Rel: store.Relates},
	}
	if !slices.Equal(plan.Links, wantLinks) {
		t.Errorf("links\n got %v\nwant %v", plan.Links, wantLinks)
	}
	if st.Links != 3 || st.ToIndex != 1 || st.Unresolved != 1 {
		t.Errorf("link stats %+v", st)
	}
	again, err := Load(dir, Options{Project: "nixos-config"})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(again.Records) != fmt.Sprint(plan.Records) || fmt.Sprint(again.Links) != fmt.Sprint(plan.Links) {
		t.Error("Load is not deterministic")
	}
	big := writeFixture(t, map[string]string{"reference_big.md": "---\nname: big\n---\n" + strings.Repeat("Большой справочник по железу.\n\n", 400)})
	bp, err := Load(big, Options{Project: "p", LogBytes: 10000})
	if err != nil {
		t.Fatal(err)
	}
	if bp.Stats.LogBySize != 1 || bp.Stats.Chunks < 9 || bp.Records[0].Key != "reference_big#1" {
		t.Errorf("size threshold: %+v first %q", bp.Stats, bp.Records[0].Key)
	}
	if _, err := Load(dir, Options{Project: "bad scope!"}); err == nil {
		t.Error("invalid project accepted")
	}
	if _, err := Load(filepath.Join(dir, "missing"), Options{Project: "p"}); err == nil {
		t.Error("missing dir accepted")
	}
}

func TestImportIdempotent(t *testing.T) {
	ctx := context.Background()
	dir := writeFixture(t, fixture)
	before := snapshot(t, dir)
	c := &clock{t: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)}
	s := openTest(t, c)
	plan, res, err := Import(ctx, s, dir, Options{Project: "nixos-config"}, false)
	if err != nil {
		t.Fatal(err)
	}
	n := len(plan.Records)
	if len(res.Created) != n || len(res.Updated) != 0 || res.Unchanged != 0 {
		t.Fatalf("first import %+v", res)
	}
	c.t = c.t.Add(time.Hour)
	_, res, err = Import(ctx, s, dir, Options{Project: "nixos-config"}, false)
	if err != nil || len(res.Created) != 0 || len(res.Updated) != 0 || res.Unchanged != n || len(res.Stale) != 0 {
		t.Fatalf("second import %+v %v", res, err)
	}
	if after := snapshot(t, dir); after != before {
		t.Fatal("import modified the legacy directory")
	}
	r, err := s.Get(ctx, "feedback_use_fd")
	if src := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC); err != nil || r.Version != 1 || !r.ValidFrom.Equal(src) || !r.AsOf().Equal(src) {
		t.Fatalf("record after reimport %+v %v", r, err)
	}
	links, err := s.Links(ctx, "reference_machines")
	if err != nil || len(links) != 3 {
		t.Fatalf("links of reference_machines %v %v", links, err)
	}
	hits, err := s.Search(ctx, store.Query{Text: "127.0.0.1:8095", Limit: 3})
	if err != nil || len(hits) == 0 || !strings.HasPrefix(hits[0].Record.Key, "project_saga#") {
		t.Fatalf("search chunk %v %v", hits, err)
	}

	edit := map[string]string{}
	for k, v := range fixture {
		edit[k] = v
	}
	edit["reference_machines.md"] = strings.Replace(fixture["reference_machines.md"], "Ultra 9 290HX", "Ultra 9 290HX, 96 GB", 1)
	edit["reference_ddnet.md"] = fixture["reference_ddnet.md"] + "\nMore.\n"
	delete(edit, "project_small.md")
	edit["project_saga.md"] = "---\nname: project_saga\ndescription: Long investigation\nmetadata:\n  type: project\n---\n" + journal(3)
	writeTo := func(files map[string]string) {
		for _, name := range []string{"project_small.md", "project_saga.md"} {
			_ = os.Remove(filepath.Join(dir, name))
		}
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	writeTo(edit)
	body := "Edited by the agent after import."
	if _, err := s.Put(ctx, store.PutRequest{Key: "reference_ddnet", Update: true, Body: &body, Why: "distilled"}); err != nil {
		t.Fatal(err)
	}
	edited, res, err := Import(ctx, s, dir, Options{Project: "nixos-config"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Updated, []string{"project_saga#1", "reference_machines"}) || !slices.Equal(res.Kept, []string{"reference_ddnet"}) {
		t.Fatalf("third import updated %v kept %v", res.Updated, res.Kept)
	}
	var wantStale []string
	for _, k := range keysOf(plan.Records) {
		if !slices.Contains(keysOf(edited.Records), k) {
			wantStale = append(wantStale, k)
		}
	}
	stale := slices.Clone(res.Stale)
	slices.Sort(stale)
	if len(wantStale) < 3 || !slices.Contains(wantStale, "project_small") || !slices.Equal(stale, wantStale) || len(res.Retired) != 0 {
		t.Fatalf("stale %v, want %v, retired %v", stale, wantStale, res.Retired)
	}
	hist, err := s.History(ctx, "reference_machines")
	if err != nil || len(hist) != 1 || hist[0].Reason != "import: legacy" {
		t.Fatalf("history %+v %v", hist, err)
	}
	if r, _ := s.Get(ctx, "reference_ddnet"); r.Body != body {
		t.Fatalf("agent edit overwritten: %q", r.Body)
	}
	_, res, err = Import(ctx, s, dir, Options{Project: "nixos-config"}, true)
	if err != nil || len(res.Retired) != len(stale) {
		t.Fatalf("retire stale %+v %v", res, err)
	}
	cur, err := s.List(ctx, store.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if got := keysOf(cur); slices.Contains(got, "project_small") || slices.Contains(got, "project_saga#2") || !slices.Contains(got, "project_saga#1") {
		t.Fatalf("current after retire %v", got)
	}
}

func TestRun(t *testing.T) {
	dir := writeFixture(t, fixture)
	tmp := t.TempDir()
	vars := map[string]string{"HOME": tmp, "XDG_DATA_HOME": filepath.Join(tmp, "data"), "MEM_CORE": filepath.Join(tmp, "core.md")}
	run := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		e := env.Env{
			Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb,
			Getenv:   func(k string) string { return vars[k] },
			Now:      func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) },
			Hostname: func() (string, error) { return "smok", nil },
		}
		return Run(args, e), out.String(), errb.String()
	}
	db := filepath.Join(tmp, "data", "agent-memory", "mem.db")
	steps := []struct {
		name   string
		args   []string
		code   int
		stdout []string
		stderr []string
		db     bool
	}{
		{name: "missing flags", args: []string{"--dir", dir}, code: 2, stderr: []string{"--dir and --project are required"}},
		{name: "positional", args: []string{"--dir", dir, "--project", "p", "extra"}, code: 2, stderr: []string{"unexpected argument"}},
		{name: "bad project", args: []string{"--dir", dir, "--project", "a b"}, code: 2, stderr: []string{"valid scope"}},
		{name: "missing dir", args: []string{"--dir", filepath.Join(dir, "nope"), "--project", "p"}, code: 1},
		{name: "dry run", args: []string{"--dir", dir, "--project", "nixos-config", "--dry-run"},
			stdout: []string{"files 8: 3 indexes skipped, 0 unusable, 4 records, 1 log files (0 by size, 1 by journal lines)", "frontmatter: top 4, metadata 2, none 2", "dry run, nothing written"}},
		{name: "import", args: []string{"--dir", dir, "--project", "nixos-config"}, db: true,
			stdout: []string{"links: 3 resolved, 1 to index files, 1 unresolved", "created, 0 updated, 0 unchanged"}},
		{name: "reimport json", args: []string{"--dir", dir, "--project", "nixos-config", "--json"}, db: true,
			stdout: []string{`"unchanged": `, `"layouts": {`}},
	}
	for _, st := range steps {
		code, out, errs := run(st.args...)
		if code != st.code {
			t.Errorf("%s: exit %d, want %d\n%s%s", st.name, code, st.code, out, errs)
			continue
		}
		for _, w := range st.stdout {
			if !strings.Contains(out, w) {
				t.Errorf("%s: stdout lacks %q:\n%s", st.name, w, out)
			}
		}
		for _, w := range st.stderr {
			if !strings.Contains(errs, w) {
				t.Errorf("%s: stderr lacks %q:\n%s", st.name, w, errs)
			}
		}
		if _, err := os.Stat(db); (err == nil) != st.db {
			t.Errorf("%s: db exists = %v, want %v", st.name, err == nil, st.db)
		}
	}
	if _, err := os.Stat(vars["MEM_CORE"]); err == nil {
		t.Error("import wrote the core file")
	}
}

func TestStateTTL(t *testing.T) {
	ctx := context.Background()
	files := map[string]string{
		"project_old.md":   "---\nname: project_old\ndescription: Old state\ntype: project\nmodified: 2026-10-01T09:00:00Z\n---\nRefactor is not pushed.\n",
		"project_fresh.md": "---\nname: project_fresh\ndescription: Fresh state\ntype: project\nmodified: 2026-11-05T09:00:00Z\n---\nKernel pinned to 7.2.\n",
		"project_saga.md":  fixture["project_saga.md"],
	}
	dir := writeFixture(t, files)
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		ttl     int
		wantTTL int
		past    []string
	}{
		{0, 0, []string{"project_old"}},
		{60, 60, nil},
	} {
		plan, err := Load(dir, Options{Project: "p", StateTTL: c.ttl})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range plan.Records {
			want := c.wantTTL
			if r.Kind == store.Log {
				want = 0
			}
			if r.TTLDays != want {
				t.Errorf("state-ttl %d: %s ttl %d, want %d", c.ttl, r.Key, r.TTLDays, want)
			}
		}
		if got := PastTTL(plan, now); !slices.Equal(got, c.past) {
			t.Errorf("state-ttl %d: past ttl %v, want %v", c.ttl, got, c.past)
		}
		s := openTest(t, &clock{t: now})
		if _, err := Apply(ctx, s, plan, false); err != nil {
			t.Fatalf("state-ttl %d: %v", c.ttl, err)
		}
		expired, err := s.Expire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got := keysOf(expired); !slices.Equal(got, c.past) {
			t.Errorf("state-ttl %d: expired %v, want %v", c.ttl, got, c.past)
		}
	}
}
