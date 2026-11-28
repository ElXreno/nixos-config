package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
)

type sev int

const (
	info sev = iota
	errs
	med
	high
	crit
)

var sevNames = map[sev]string{info: "INFO", errs: "ERR", med: "MED", high: "HIGH", crit: "CRIT"}

type finding struct {
	sev  sev
	file string
	msg  string
}

type tmpl struct {
	text  string
	files []string
}

type opts struct {
	deep    bool
	workers int
	base    string
	hf      *hfClient
}

type target struct {
	name     string
	mu       sync.Mutex
	findings []finding
	tmpls    map[string]*tmpl
	bases    []string
	scanned  map[string]int
}

func newTarget(name string) *target {
	return &target{name: name, tmpls: map[string]*tmpl{}, scanned: map[string]int{}}
}

func (t *target) count(kind string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.scanned[kind]++
}

func (t *target) scannedSummary() string {
	kinds := make([]string, 0, len(t.scanned))
	for k, n := range t.scanned {
		kinds = append(kinds, fmt.Sprintf("%s=%d", k, n))
	}
	sort.Strings(kinds)
	if len(kinds) == 0 {
		return "nothing"
	}
	return strings.Join(kinds, " ")
}

func (t *target) add(s sev, file, format string, a ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.findings = append(t.findings, finding{s, file, fmt.Sprintf(format, a...)})
}

func (t *target) addBase(b string) {
	b = strings.TrimSpace(strings.Trim(strings.TrimSpace(b), `"'`))
	if b == "" || strings.ContainsAny(b, " \t") {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !slices.Contains(t.bases, b) {
		t.bases = append(t.bases, b)
	}
}

func (t *target) addTemplate(file, text string) {
	h := shortHash(text)
	t.mu.Lock()
	tm, seen := t.tmpls[h]
	if !seen {
		tm = &tmpl{text: text}
		t.tmpls[h] = tm
	}
	tm.files = append(tm.files, file)
	t.mu.Unlock()
	if !seen {
		checkTemplate(t, file, text)
	}
}

func (t *target) recover(file string) {
	if e := recover(); e != nil {
		t.add(high, file, "cannot parse: %v", e)
	}
}

func (t *target) templateOrder() []string {
	hs := make([]string, 0, len(t.tmpls))
	for h, tm := range t.tmpls {
		sort.Strings(tm.files)
		hs = append(hs, h)
	}
	sort.Slice(hs, func(a, b int) bool { return t.tmpls[hs[a]].files[0] < t.tmpls[hs[b]].files[0] })
	return hs
}

func (t *target) worst() sev {
	w := info
	for _, f := range t.findings {
		w = max(w, f.sev)
	}
	return w
}

func (t *target) print() sev {
	var w strings.Builder
	sort.SliceStable(t.findings, func(a, b int) bool {
		if t.findings[a].sev != t.findings[b].sev {
			return t.findings[a].sev > t.findings[b].sev
		}
		return t.findings[a].file < t.findings[b].file
	})
	fmt.Fprintf(&w, "== %s\n", t.name)
	counts := map[sev]int{}
	for _, f := range t.findings {
		counts[f.sev]++
		fmt.Fprintf(&w, "%-4s %s: %s\n", sevNames[f.sev], f.file, strings.ReplaceAll(f.msg, "\n", "\n     "))
	}
	for _, h := range t.templateOrder() {
		tm := t.tmpls[h]
		fmt.Fprintf(&w, "template %s %d B: %s\n", h, len(tm.text), strings.Join(tm.files, ", "))
	}
	worst := t.worst()
	verdict := map[sev]string{info: "CLEAN", errs: "INCOMPLETE", med: "SUSPICIOUS", high: "SUSPICIOUS", crit: "MALICIOUS"}[worst]
	fmt.Fprintf(&w, "scanned: %s\n", t.scannedSummary())
	fmt.Fprintf(&w, "verdict: %s (crit=%d high=%d med=%d err=%d info=%d)\n", verdict, counts[crit], counts[high], counts[med], counts[errs], counts[info])
	fmt.Print(w.String())
	return worst
}

func main() {
	var o opts
	flag.BoolVar(&o.deep, "deep", true, "read Q8_0 and Q4_K tensor data of local GGUF files for payloads hidden in zero-scale blocks")
	flag.IntVar(&o.workers, "j", 8, "files scanned in parallel")
	flag.StringVar(&o.base, "base", "", "diff the chat templates against an official base: auto (declared base_model), owner/repo[@rev] or a template file")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: model-scan [flags] <file|dir|hf:owner/repo[@rev]|https://huggingface.co/owner/repo>...\n\nExit status: 0 clean, 1 incomplete (read or network errors), 2 findings of MED or above.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}
	o.hf = newHF()
	worst := info
	for _, arg := range flag.Args() {
		t := newTarget(arg)
		if repo, rev, ok := parseHF(arg); ok {
			scanRemote(t, repo, rev, o)
		} else {
			scanLocal(t, arg, o)
		}
		diffBase(t, o)
		worst = max(worst, t.print())
	}
	switch {
	case worst >= med:
		os.Exit(2)
	case worst == errs:
		os.Exit(1)
	}
}

func scanLocal(t *target, root string, o opts) {
	st, err := os.Stat(root)
	if err != nil {
		t.add(errs, root, "%v", err)
		return
	}
	var files []string
	if !st.IsDir() {
		files = []string{root}
	} else {
		walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				t.add(errs, p, "%v", err)
				return nil
			}
			if d.IsDir() {
				if p != root && (d.Name() == ".git" || d.Name() == ".cache" || d.Name() == ".no_exist") {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 {
				if s, err := os.Stat(p); err != nil || !s.Mode().IsRegular() {
					return nil
				}
			} else if !d.Type().IsRegular() {
				return nil
			}
			files = append(files, p)
			return nil
		})
		if walkErr != nil {
			t.add(errs, root, "%v", walkErr)
		}
	}
	parallel(o.workers, len(files), func(i int) {
		p := files[i]
		name := p
		if st.IsDir() {
			if rel, err := filepath.Rel(root, p); err == nil {
				name = rel
			}
		}
		f, err := os.Open(p)
		if err != nil {
			t.add(errs, name, "%v", err)
			return
		}
		defer func() { _ = f.Close() }()
		fst, err := f.Stat()
		if err != nil {
			t.add(errs, name, "%v", err)
			return
		}
		scanFile(t, name, f, fst.Size(), o, true)
	})
}

func parallel(n, total int, fn func(int)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(n, 1))
	for i := range total {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			fn(i)
		}()
	}
	wg.Wait()
}

var (
	scriptExt = setOf(".sh", ".bash", ".zsh", ".fish", ".ps1", ".psm1", ".bat", ".cmd", ".vbs", ".js", ".mjs", ".rb", ".pl", ".lua")
	nativeExt = setOf(".exe", ".dll", ".so", ".dylib", ".sys", ".scr", ".msi", ".com")
	pickleExt = setOf(".bin", ".pt", ".pth", ".ckpt", ".pkl", ".pickle", ".joblib")
	tmplJSON  = setOf("tokenizer_config.json", "chat_template.json", "processor_config.json")
)

func setOf(xs ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func scanFile(t *target, name string, ra io.ReaderAt, size int64, o opts, local bool) {
	defer t.recover(name)
	if size == 0 {
		return
	}
	lower := strings.ToLower(filepath.Base(name))
	ext := filepath.Ext(lower)
	var head []byte
	if local || pickleExt[ext] || ext == ".npy" || ext == ".npz" {
		head = make([]byte, min(size, 64))
		if _, err := ra.ReadAt(head, 0); err != nil && err != io.EOF {
			t.add(errs, name, "read: %v", err)
			return
		}
	}
	switch {
	case ext == ".gguf" || bytes.HasPrefix(head, []byte("GGUF")):
		t.count("gguf")
		scanGGUF(t, name, ra, size, o, local)
	case ext == ".safetensors":
		t.count("safetensors")
		scanSafetensors(t, name, ra, size, local)
	case ext == ".jinja" || lower == "modelfile":
		t.count("template")
		t.addTemplate(name, string(readSmall(t, name, ra, size)))
	case tmplJSON[lower]:
		t.count("json")
		scanTemplateJSON(t, name, readSmall(t, name, ra, size))
	case lower == "readme.md":
		for _, b := range readmeBases(string(readSmall(t, name, ra, size))) {
			t.addBase(b)
		}
	case ext == ".npy":
		t.count("npy")
		scanNpy(t, name, ra, size)
	case isZip(head) && (pickleExt[ext] || ext == ".npz"):
		t.count("zip")
		scanZip(t, name, ra, size)
	case pickleExt[ext] || (local && isPickle(head) && ext == ""):
		if scanPickleFile(t, name, ra, size, head, ext) {
			t.count("pickle")
		}
	case ext == ".py":
		t.count("python")
		scanPython(t, name, readSmall(t, name, ra, size))
	case scriptExt[ext]:
		t.count("script")
		t.add(info, name, "script: read it before running")
	case nativeExt[ext] || (local && isNative(head)):
		t.count("native")
		t.add(high, name, "native executable or library in a model repository")
	}
}

func readSmall(t *target, name string, ra io.ReaderAt, size int64) []byte {
	const limit = 16 << 20
	if size > limit {
		t.add(med, name, "unexpectedly large file (%d bytes): only the first %d scanned", size, limit)
		size = limit
	}
	b := make([]byte, size)
	n, err := ra.ReadAt(b, 0)
	if err != nil && err != io.EOF {
		t.add(errs, name, "read: %v", err)
	}
	return b[:n]
}

func isZip(h []byte) bool { return bytes.HasPrefix(h, []byte("PK\x03\x04")) }

func isPickle(h []byte) bool { return len(h) >= 2 && h[0] == 0x80 && h[1] >= 2 && h[1] <= 5 }

func isNative(h []byte) bool {
	switch {
	case bytes.HasPrefix(h, []byte("\x7fELF")):
		return true
	case bytes.HasPrefix(h, []byte{0xcf, 0xfa, 0xed, 0xfe}), bytes.HasPrefix(h, []byte{0xce, 0xfa, 0xed, 0xfe}):
		return true
	case bytes.HasPrefix(h, []byte("MZ")) && len(h) >= 64:
		pe := binary.LittleEndian.Uint32(h[60:])
		return pe >= 64 && pe < 1<<16
	}
	return false
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
