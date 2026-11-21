package vec

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func unit(a, b float64) []float32 {
	n := math.Hypot(a, b)
	return []float32{float32(a / n), float32(b / n)}
}

func writeRun(t *testing.T, dir string, keys []string, rows [][]float32) {
	t.Helper()
	var data []float32
	for _, r := range rows {
		data = append(data, r...)
	}
	if err := WriteSet(dir, PartDocs, keys, data, len(rows[0])); err != nil {
		t.Fatal(err)
	}
	if err := WriteMeta(dir, &Meta{Model: "m", Dim: len(rows[0])}); err != nil {
		t.Fatal(err)
	}
}

func TestDiff(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	writeRun(t, a, []string{"k1", "k2", "k3", "k4"}, [][]float32{unit(1, 0), unit(0, 1), unit(1, 1), unit(1, 2)})
	writeRun(t, b, []string{"k4", "k3", "k2", "k1"}, [][]float32{unit(1, 2), unit(1, 1), unit(1, 1), unit(1, 0)})
	res, err := Diff(a, b, []Part{PartDocs})
	if err != nil || len(res) != 1 {
		t.Fatalf("%v %v", res, err)
	}
	r := res[0]
	wantMin := 1 / math.Sqrt2
	if r.Count != 4 || math.Abs(r.Min-wantMin) > 1e-6 || math.Abs(r.Mean-(3+wantMin)/4) > 1e-6 || r.Part != "docs" {
		t.Errorf("%+v", r)
	}
	if r.P1 < r.Min || r.P1 > r.Mean {
		t.Errorf("p1 %v outside [min, mean]", r.P1)
	}
	same, err := Diff(a, a, []Part{PartDocs})
	if err != nil || math.Abs(same[0].Mean-1) > 1e-6 || math.Abs(same[0].Min-1) > 1e-6 {
		t.Errorf("self diff %+v %v", same, err)
	}
}

func TestDiffErrors(t *testing.T) {
	root := t.TempDir()
	a, b, c, d := filepath.Join(root, "a"), filepath.Join(root, "b"), filepath.Join(root, "c"), filepath.Join(root, "d")
	writeRun(t, a, []string{"k1", "k2"}, [][]float32{unit(1, 0), unit(0, 1)})
	writeRun(t, b, []string{"k1", "k3"}, [][]float32{unit(1, 0), unit(0, 1)})
	writeRun(t, c, []string{"k1", "k2"}, [][]float32{{1, 0, 0}, {0, 1, 0}})
	writeRun(t, d, []string{"k1"}, [][]float32{unit(1, 0)})
	for _, tc := range []struct{ x, y, want string }{{a, b, "key"}, {a, c, "dimension"}, {a, d, "key"}} {
		if _, err := Diff(tc.x, tc.y, []Part{PartDocs}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Diff(%s, %s) = %v, want %q", filepath.Base(tc.x), filepath.Base(tc.y), err, tc.want)
		}
	}
	if _, err := Diff(a, filepath.Join(root, "none"), []Part{PartDocs}); err == nil {
		t.Error("missing run accepted")
	}
}

func TestLoadRejectsTruncated(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "a")
	writeRun(t, dir, []string{"k1", "k2"}, [][]float32{unit(1, 0), unit(0, 1)})
	if err := WriteSet(dir, PartDocs, []string{"k1", "k2", "k3"}, []float32{1, 0, 0, 1}, 2); err == nil {
		t.Error("WriteSet accepted a row count that does not match the names")
	}
	if err := WriteSet(dir, PartDocs, []string{"k1\nk2"}, []float32{1, 0}, 2); err == nil {
		t.Error("WriteSet accepted a name with a newline")
	}
}

func TestLoadLayoutAndCorruption(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a")
	writeRun(t, dir, []string{"k1", "k2"}, [][]float32{{1, 0}, {0, -2.5}})
	raw, err := os.ReadFile(filepath.Join(dir, "docs.f32"))
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0, 0, 0x80, 0x3f, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x20, 0xc0}
	if !bytes.Equal(raw, want) {
		t.Fatalf("layout % x, want little-endian row-major float32 % x", raw, want)
	}
	set, err := Load(dir, PartDocs)
	if err != nil || set.Row(1)[1] != -2.5 || set.Row(0)[0] != 1 {
		t.Fatalf("%v %v", set, err)
	}
	for name, mod := range map[string]func([]byte) []byte{
		"truncated": func(b []byte) []byte { return b[:len(b)-1] },
		"short row": func(b []byte) []byte { return b[:len(b)-4] },
		"trailing":  func(b []byte) []byte { return append(slices.Clone(b), 0, 0, 0, 0) },
	} {
		if err := os.WriteFile(filepath.Join(dir, "docs.f32"), mod(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(dir, PartDocs); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
