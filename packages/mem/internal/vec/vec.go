package vec

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	MetaFile = "meta.json"
	FileMode = 0o600
	DirMode  = 0o700
)

type Part struct {
	Name  string
	Vecs  string
	Names string
}

var (
	PartDocs    = Part{Name: "docs", Vecs: "docs.f32", Names: "docs.keys"}
	PartQueries = Part{Name: "queries", Vecs: "queries.f32", Names: "queries.ids"}
)

type PartMeta struct {
	Count          int     `json:"count"`
	Source         string  `json:"source"`
	InputSHA256    string  `json:"input_sha256"`
	SentSHA256     string  `json:"sent_sha256"`
	Seconds        float64 `json:"seconds"`
	TextsPerSecond float64 `json:"texts_per_second"`
	Batch          int     `json:"batch"`
	Par            int     `json:"par"`
}

type Meta struct {
	Model          string          `json:"model"`
	URL            string          `json:"url"`
	ServerEndpoint string          `json:"server_endpoint,omitempty"`
	ServerInfo     json.RawMessage `json:"server_info,omitempty"`
	Dim            int             `json:"dim"`
	QueryPrefix    string          `json:"query_prefix"`
	DocPrefix      string          `json:"doc_prefix"`
	QuerySuffix    string          `json:"query_suffix"`
	DocSuffix      string          `json:"doc_suffix"`
	Docs           *PartMeta       `json:"docs,omitempty"`
	Queries        *PartMeta       `json:"queries,omitempty"`
	WallSeconds    float64         `json:"wall_seconds"`
	Texts          int             `json:"texts"`
	TextsPerSecond float64         `json:"texts_per_second"`
}

type Set struct {
	Names []string
	Dim   int
	Data  []float32
}

func (s *Set) Row(i int) []float32 { return s.Data[i*s.Dim : (i+1)*s.Dim] }

func Dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

func Normalize(v []float32) error {
	var s float64
	for _, x := range v {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return errors.New("non-finite component")
		}
		s += float64(x) * float64(x)
	}
	if !(s > 0) || math.IsInf(s, 0) {
		return errors.New("zero vector")
	}
	n := math.Sqrt(s)
	for i := range v {
		v[i] = float32(float64(v[i]) / n)
	}
	return nil
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Chmod(name, FileMode); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}

func WriteSet(dir string, p Part, names []string, data []float32, dim int) error {
	if dim <= 0 || len(data) != len(names)*dim {
		return fmt.Errorf("%s: %d floats do not make %d rows of %d", p.Name, len(data), len(names), dim)
	}
	for _, n := range names {
		if n == "" || strings.ContainsAny(n, "\r\n") {
			return fmt.Errorf("%s: name %q is empty or has a line break", p.Name, n)
		}
	}
	if err := os.MkdirAll(dir, DirMode); err != nil {
		return err
	}
	raw := make([]byte, 4*len(data))
	for i, x := range data {
		binary.LittleEndian.PutUint32(raw[4*i:], math.Float32bits(x))
	}
	if err := writeAtomic(filepath.Join(dir, p.Vecs), raw); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, p.Names), []byte(strings.Join(names, "\n")+"\n"))
}

func WriteMeta(dir string, m *Meta) error {
	if err := os.MkdirAll(dir, DirMode); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, MetaFile), append(data, '\n'))
}

func ReadMeta(dir string) (*Meta, error) {
	data, err := os.ReadFile(filepath.Join(dir, MetaFile))
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, MetaFile), err)
	}
	return &m, nil
}

func Load(dir string, p Part) (*Set, error) {
	m, err := ReadMeta(dir)
	if err != nil {
		return nil, err
	}
	rawNames, err := os.ReadFile(filepath.Join(dir, p.Names))
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, p.Vecs))
	if err != nil {
		return nil, err
	}
	names := strings.Split(strings.TrimSuffix(string(rawNames), "\n"), "\n")
	if len(rawNames) == 0 {
		names = nil
	}
	if m.Dim <= 0 || len(raw)%4 != 0 || len(raw)/4 != len(names)*m.Dim {
		return nil, fmt.Errorf("%s: %d bytes do not match %d names of dimension %d", filepath.Join(dir, p.Vecs), len(raw), len(names), m.Dim)
	}
	data := make([]float32, len(raw)/4)
	for i := range data {
		data[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[4*i:]))
	}
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if seen[n] {
			return nil, fmt.Errorf("%s: duplicate name %s", filepath.Join(dir, p.Names), n)
		}
		seen[n] = true
	}
	return &Set{Names: names, Dim: m.Dim, Data: data}, nil
}

func ExpandEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i++
				continue
			case 't':
				b.WriteByte('\t')
				i++
				continue
			case '\\':
				b.WriteByte('\\')
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
