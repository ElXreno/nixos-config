package statefile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPreview(t *testing.T) {
	long := strings.Repeat("слово ", 40)
	rlo, lri, pdi, nbsp, emsp := string(rune(0x202e)), string(rune(0x2066)), string(rune(0x2069)), string(rune(0xa0)), string(rune(0x2003))
	for in, want := range map[string]string{
		"":                                       "",
		"  a\t\tb\n\n c  ":                       "a b c",
		"x\x00y\x1b[31mz":                        "x y [31mz",
		"left" + rlo + "right" + lri + "x" + pdi: "leftrightx",
		"ok\r\nnext" + nbsp + emsp + "word":      "ok next word",
		strings.Repeat("a", 100):                 strings.Repeat("a", 100),
		strings.Repeat("a", 101):                 strings.Repeat("a", 100),
		long:                                     strings.TrimSpace(string([]rune(long)[:100])),
		strings.Repeat("ж ", 60):                 strings.TrimSpace(string([]rune(strings.Repeat("ж ", 60))[:100])),
		"\xff\xfe bad":                           "\ufffd\ufffd bad",
		"emoji 👍🏽 " + strings.Repeat("я", 120): string([]rune("emoji 👍🏽 " + strings.Repeat("я", 120))[:100]),
	} {
		got := Preview(in)
		if got != want {
			t.Errorf("Preview(%q) = %q, want %q", in, got, want)
		}
		if n := utf8.RuneCountInString(got); n > PreviewRunes {
			t.Errorf("Preview(%q) has %d runes", in, n)
		}
		if !utf8.ValidString(got) {
			t.Errorf("Preview(%q) = %q is not valid UTF-8", in, got)
		}
	}
}

type row struct {
	N int    `json:"n"`
	S string `json:"s"`
}

func lines(t *testing.T, path string) []row {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []row
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var r row
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("line %q: %v", l, err)
		}
		out = append(out, r)
	}
	return out
}

func TestAppendCreatesPrivateFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if err := Append(dir, TurnsName, 1<<20, row{N: 1, S: "<b>&"}); err != nil {
		t.Fatal(err)
	}
	if err := Append(dir, TurnsName, 1<<20, row{N: 2}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, TurnsName)
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("file %v %v", fi, err)
	}
	di, err := os.Stat(dir)
	if err != nil || di.Mode().Perm() != 0o700 {
		t.Fatalf("dir %v %v", di, err)
	}
	if got := lines(t, path); len(got) != 2 || got[0].N != 1 || got[0].S != "<b>&" || got[1].N != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestAppendTightensAnOpenMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, TurnsName)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Append(dir, TurnsName, 1<<20, row{N: 1}); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
}

func TestAppendGuardsAPartialLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, TurnsName)
	if err := os.WriteFile(path, []byte(`{"n":1,"s":"cut`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Append(dir, TurnsName, 1<<20, row{N: 2}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	parts := strings.Split(strings.TrimSpace(string(data)), "\n")
	var r row
	if len(parts) != 2 || json.Unmarshal([]byte(parts[1]), &r) != nil || r.N != 2 {
		t.Fatalf("%q", data)
	}
}

func TestAppendRotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, TurnsName)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, 4097); err != nil {
		t.Fatal(err)
	}
	if err := Append(dir, TurnsName, 4096, row{N: 9}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path + OldSuffix); err != nil || fi.Size() != 4097 {
		t.Fatalf("old file %v %v", fi, err)
	}
	if got := lines(t, path); len(got) != 1 || got[0].N != 9 {
		t.Fatalf("%+v", got)
	}
}

func TestAppendFailures(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, TurnsName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Append(dir, TurnsName, 1<<20, row{N: 1}); err == nil {
		t.Fatal("append into a directory succeeded")
	}
	file := filepath.Join(dir, "plain")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Append(filepath.Join(file, "sub"), TurnsName, 1<<20, row{N: 1}); err == nil {
		t.Fatal("append under a file succeeded")
	}
	if _, err := os.Stat("/dev/full"); err == nil {
		full := t.TempDir()
		if err := os.Symlink("/dev/full", filepath.Join(full, TurnsName)); err != nil {
			t.Fatal(err)
		}
		if err := Append(full, TurnsName, 1<<20, row{N: 1}); err == nil {
			t.Fatal("append to /dev/full succeeded")
		}
	}
	if err := Append(t.TempDir(), TurnsName, 1<<20, func() {}); err == nil {
		t.Fatal("unencodable value appended")
	}
}

func TestCursorRoundTrip(t *testing.T) {
	for _, c := range []Cursor{{}, {ID: "0123456789abcdef", Off: 0}, {ID: "0123456789abcdef", Off: 1 << 40}} {
		got, err := ParseCursor(c.String())
		if err != nil || got != c {
			t.Errorf("%+v -> %q -> %+v %v", c, c.String(), got, err)
		}
	}
	for _, bad := range []string{"", "x", "c1", "c1--1", "c1--5", "c1-0123-5", "c1-0123456789ABCDEF-5", "c1-0123456789abcdef--5", "c1-0123456789abcdef-x",
		"c2-0123456789abcdef-5", "c1-0123456789abcdef-5-6", "c1-0123456789abcdeg-5", "c1-0123456789abcdef-99999999999999999999"} {
		if _, err := ParseCursor(bad); !errors.Is(err, ErrCursor) {
			t.Errorf("ParseCursor(%q) = %v", bad, err)
		}
	}
}

func write(t *testing.T, path, s string) {
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

func texts(ls []Line) []string {
	out := []string{}
	for _, l := range ls {
		out = append(out, string(l.Data))
	}
	return out
}

func readAll(t *testing.T, dir string, since *Cursor) ([]string, Cursor) {
	t.Helper()
	ls, c, err := Read(dir, TurnsName, since)
	if err != nil {
		t.Fatal(err)
	}
	return texts(ls), c
}

func same(a, b []string) bool { return strings.Join(a, "|") == strings.Join(b, "|") }

func TestReadFollowsAppendsAndRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, TurnsName)
	got, c0 := readAll(t, dir, nil)
	if len(got) != 0 || c0 != (Cursor{}) {
		t.Fatalf("empty dir: %q %+v", got, c0)
	}
	write(t, path, "a\nb\n")
	got, c1 := readAll(t, dir, &c0)
	if !same(got, []string{"a", "b"}) || c1.ID == "" || c1.Off != 4 {
		t.Fatalf("first read: %q %+v", got, c1)
	}
	write(t, path, "\n  \nc\npart")
	got, c2 := readAll(t, dir, &c1)
	if !same(got, []string{"c"}) || c2.ID != c1.ID || c2.Off != 10 {
		t.Fatalf("incremental read: %q %+v", got, c2)
	}
	got, c3 := readAll(t, dir, &c2)
	if len(got) != 0 || c3 != c2 {
		t.Fatalf("partial line returned: %q %+v", got, c3)
	}
	write(t, path, "ial\n")
	got, c4 := readAll(t, dir, &c3)
	if !same(got, []string{"partial"}) {
		t.Fatalf("completed line: %q %+v", got, c4)
	}
	write(t, path, "d\n")
	if err := os.Rename(path, path+OldSuffix); err != nil {
		t.Fatal(err)
	}
	got, c5 := readAll(t, dir, &c4)
	if !same(got, []string{"d"}) || c5.ID != c4.ID {
		t.Fatalf("rotated, no new file: %q %+v", got, c5)
	}
	write(t, path, "e\nf\n")
	got, c6 := readAll(t, dir, &c5)
	if !same(got, []string{"e", "f"}) || c6.ID == c5.ID {
		t.Fatalf("rotated, new file: %q %+v", got, c6)
	}
	got, _ = readAll(t, dir, &c4)
	if !same(got, []string{"d", "e", "f"}) {
		t.Fatalf("cursor into the old file: %q", got)
	}
	got, _ = readAll(t, dir, nil)
	if !same(got, []string{"a", "b", "c", "partial", "d", "e", "f"}) {
		t.Fatalf("full read: %q", got)
	}
	if err := os.Rename(path, path+OldSuffix); err != nil {
		t.Fatal(err)
	}
	write(t, path, "g\n")
	got, _ = readAll(t, dir, &c4)
	if !same(got, []string{"e", "f", "g"}) {
		t.Fatalf("cursor rotated out twice: %q", got)
	}
	got, _ = readAll(t, dir, &Cursor{})
	if !same(got, []string{"e", "f", "g"}) {
		t.Fatalf("empty cursor: %q", got)
	}
}

func TestReadIDsAreStableAndDistinct(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, TurnsName)
	write(t, path, `{"x":1`)
	if got, c := readAll(t, dir, nil); len(got) != 0 || c != (Cursor{}) {
		t.Fatalf("no complete line: %q %+v", got, c)
	}
	write(t, path, "}\n")
	ls, c1, err := Read(dir, TurnsName, nil)
	if err != nil || len(ls) != 1 || ls[0].File != c1.ID || ls[0].End != c1.Off {
		t.Fatalf("%+v %+v %v", ls, c1, err)
	}
	write(t, path, "more\n")
	_, c2, _ := Read(dir, TurnsName, nil)
	if c2.ID != c1.ID || c2.Off <= c1.Off {
		t.Fatalf("id changed on append: %+v %+v", c1, c2)
	}
	other := t.TempDir()
	write(t, filepath.Join(other, TurnsName), "other\n")
	_, c3, _ := Read(other, TurnsName, nil)
	if c3.ID == c1.ID {
		t.Fatal("two files share an id")
	}
}

func TestReadSkipsSpecialFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, TurnsName), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, c := readAll(t, dir, nil); len(got) != 0 || c != (Cursor{}) {
		t.Fatalf("%q %+v", got, c)
	}
}

func TestReadTruncatedFileStartsOver(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, TurnsName)
	write(t, path, "a\nb\nc\n")
	_, c, _ := Read(dir, TurnsName, nil)
	if err := os.WriteFile(path, []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := readAll(t, dir, &c); !same(got, []string{"a"}) {
		t.Fatalf("%q", got)
	}
}
