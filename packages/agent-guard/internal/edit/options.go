package edit

import (
	"os"
	"path/filepath"
	"strings"

	"agent-guard/internal/checks"
	"agent-guard/internal/harness"
	"agent-guard/internal/treesitter"
)

const maxRead = 1 << 20

type Options struct {
	Grammars treesitter.Grammars
	Vocab    harness.Vocabulary
	Read     func(path string) (string, bool)
	Previous func(path string) (string, bool)
	Memory   func(path string) bool
	Note     func(note string)
	Scratch  string
}

func underDir(path, dir string) bool {
	if dir == "" {
		return false
	}
	return strings.HasPrefix(filepath.Clean(path), filepath.Clean(dir)+string(filepath.Separator))
}

func ReadFile(path string) (string, bool) {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxRead {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func Missing(string) (string, bool) { return "", false }

func (o Options) read(path string) (string, bool) {
	if o.Read == nil {
		return ReadFile(path)
	}
	return o.Read(path)
}

func (o Options) previous(path string) string {
	read := o.Previous
	if read == nil {
		read = ReadFile
	}
	s, _ := read(path)
	return s
}

func (o Options) memory(path string) bool {
	return o.Memory != nil && o.Memory(path)
}

func MemoryMatcher(patterns []string) func(path, cwd string) bool {
	return func(path, cwd string) bool {
		if !filepath.IsAbs(path) {
			return false
		}
		p := filepath.Clean(path)
		for _, pat := range patterns {
			if !filepath.IsAbs(pat) {
				anchor := checks.GitRoot(filepath.Dir(p))
				if anchor == "" {
					anchor = cwd
				}
				if anchor == "" {
					continue
				}
				pat = filepath.Join(anchor, pat)
			}
			if under(p, pat) {
				return true
			}
		}
		return false
	}
}

func under(p, pattern string) bool {
	want := strings.Split(strings.Trim(filepath.Clean(pattern), "/"), "/")
	got := strings.Split(strings.Trim(p, "/"), "/")
	if len(got) <= len(want) {
		return false
	}
	for i, w := range want {
		if ok, _ := filepath.Match(w, got[i]); !ok {
			return false
		}
	}
	return true
}

func (o Options) note(s string) {
	if o.Note != nil {
		o.Note(s)
	}
}
