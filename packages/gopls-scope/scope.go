package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

type folder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

type message struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type document struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

type scope struct {
	skip []string
	home string
	log  io.Writer

	mu       sync.Mutex
	roots    []string
	added    map[string]map[string]bool
	moduleOf map[string]string
}

func newScope(skip []string, home string, log io.Writer) *scope {
	return &scope{
		skip:     skip,
		home:     home,
		log:      log,
		added:    map[string]map[string]bool{},
		moduleOf: map[string]string{},
	}
}

func (s *scope) fromClient(body []byte) [][]byte {
	var m message
	if json.Unmarshal(body, &m) != nil {
		return [][]byte{body}
	}
	switch m.Method {
	case "initialize":
		var params struct {
			RootURI          string   `json:"rootUri"`
			WorkspaceFolders []folder `json:"workspaceFolders"`
		}
		_ = json.Unmarshal(m.Params, &params)
		s.mu.Lock()
		s.addRoot(params.RootURI)
		for _, f := range params.WorkspaceFolders {
			s.addRoot(f.URI)
		}
		s.mu.Unlock()
	case "workspace/didChangeWorkspaceFolders":
		var params struct {
			Event struct {
				Added   []folder `json:"added"`
				Removed []folder `json:"removed"`
			} `json:"event"`
		}
		_ = json.Unmarshal(m.Params, &params)
		s.mu.Lock()
		for _, f := range params.Event.Added {
			s.addRoot(f.URI)
		}
		for _, f := range params.Event.Removed {
			s.removeRoot(f.URI)
		}
		s.mu.Unlock()
	case "textDocument/didOpen":
		if change := s.open(documentPath(m.Params)); change != nil {
			return [][]byte{change, body}
		}
	case "textDocument/didClose":
		if change := s.close(documentPath(m.Params)); change != nil {
			return [][]byte{body, change}
		}
	}
	return [][]byte{body}
}

func (s *scope) fromServer(body []byte) [][]byte {
	var m message
	if json.Unmarshal(body, &m) != nil || m.Method != "textDocument/publishDiagnostics" {
		return [][]byte{body}
	}
	var params map[string]json.RawMessage
	if json.Unmarshal(m.Params, &params) != nil {
		return [][]byte{body}
	}
	var uri string
	_ = json.Unmarshal(params["uri"], &uri)
	path := filePath(uri)
	if path == "" || s.inScope(path) || string(params["diagnostics"]) == "[]" {
		return [][]byte{body}
	}
	var whole map[string]json.RawMessage
	if json.Unmarshal(body, &whole) != nil {
		return [][]byte{body}
	}
	params["diagnostics"] = json.RawMessage("[]")
	whole["params"], _ = json.Marshal(params)
	out, err := json.Marshal(whole)
	if err != nil {
		return [][]byte{body}
	}
	return [][]byte{out}
}

func (s *scope) open(path string) []byte {
	if path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.moduleOf[path]; ok || s.underRoot(path) {
		return nil
	}
	mod := moduleRoot(filepath.Dir(path))
	if mod == "" || s.skipped(mod) {
		return nil
	}
	s.moduleOf[path] = mod
	if docs, ok := s.added[mod]; ok {
		docs[path] = true
		return nil
	}
	s.added[mod] = map[string]bool{path: true}
	_, _ = fmt.Fprintf(s.log, "gopls-scope: workspace folder added: %s\n", mod)
	return foldersChange([]string{mod}, nil)
}

func (s *scope) close(path string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	mod, ok := s.moduleOf[path]
	if !ok {
		return nil
	}
	delete(s.moduleOf, path)
	delete(s.added[mod], path)
	if len(s.added[mod]) > 0 {
		return nil
	}
	delete(s.added, mod)
	_, _ = fmt.Fprintf(s.log, "gopls-scope: workspace folder removed: %s\n", mod)
	return foldersChange(nil, []string{mod})
}

func (s *scope) inScope(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.underRoot(path) {
		return true
	}
	for mod := range s.added {
		if within(path, mod) {
			return true
		}
	}
	return false
}

func (s *scope) underRoot(path string) bool {
	for _, root := range s.roots {
		if within(path, root) {
			return true
		}
	}
	return false
}

func (s *scope) skipped(mod string) bool {
	if s.home != "" && within(s.home, mod) {
		return true
	}
	for _, dir := range s.skip {
		if within(mod, dir) {
			return true
		}
	}
	return false
}

func (s *scope) addRoot(uri string) {
	path := filePath(uri)
	if path == "" || slices.Contains(s.roots, path) {
		return
	}
	s.roots = append(s.roots, path)
}

func (s *scope) removeRoot(uri string) {
	path := filePath(uri)
	for i, root := range s.roots {
		if root == path {
			s.roots = append(s.roots[:i], s.roots[i+1:]...)
			return
		}
	}
}

func foldersChange(added, removed []string) []byte {
	toFolders := func(dirs []string) []folder {
		out := []folder{}
		for _, dir := range dirs {
			out = append(out, folder{URI: (&url.URL{Scheme: "file", Path: dir}).String(), Name: filepath.Base(dir)})
		}
		return out
	}
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  "workspace/didChangeWorkspaceFolders",
		"params": map[string]any{
			"event": map[string]any{"added": toFolders(added), "removed": toFolders(removed)},
		},
	}
	out, _ := json.Marshal(msg)
	return out
}

func documentPath(params json.RawMessage) string {
	var d document
	_ = json.Unmarshal(params, &d)
	return filePath(d.TextDocument.URI)
}

func filePath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || u.Path == "" {
		return ""
	}
	return filepath.Clean(u.Path)
}

func within(path, dir string) bool {
	return path == dir || dir == "/" || strings.HasPrefix(path, dir+"/")
}

func moduleRoot(dir string) string {
	for {
		if fi, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && fi.Mode().IsRegular() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
