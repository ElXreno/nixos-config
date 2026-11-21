package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func uriOf(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func notification(t *testing.T, method string, params any) []byte {
	t.Helper()
	out, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func didOpen(t *testing.T, path string) []byte {
	return notification(t, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": uriOf(path), "languageId": "go", "version": 1, "text": ""},
	})
}

func didClose(t *testing.T, path string) []byte {
	return notification(t, "textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": uriOf(path)}})
}

func publish(t *testing.T, path string) []byte {
	return notification(t, "textDocument/publishDiagnostics", map[string]any{
		"uri":         uriOf(path),
		"version":     1,
		"diagnostics": []map[string]any{{"message": "undefined: x", "severity": 1}},
	})
}

type change struct {
	Method string `json:"method"`
	Params struct {
		Event struct {
			Added   []folder `json:"added"`
			Removed []folder `json:"removed"`
		} `json:"event"`
	} `json:"params"`
}

func folderChange(t *testing.T, msg []byte) change {
	t.Helper()
	var c change
	if err := json.Unmarshal(msg, &c); err != nil || c.Method != "workspace/didChangeWorkspaceFolders" {
		t.Fatalf("not a workspace folder change: %s", msg)
	}
	return c
}

func diagnosticCount(t *testing.T, msg []byte) int {
	t.Helper()
	var m struct {
		Params struct {
			Diagnostics []json.RawMessage `json:"diagnostics"`
		} `json:"params"`
	}
	if err := json.Unmarshal(msg, &m); err != nil {
		t.Fatal(err)
	}
	return len(m.Params.Diagnostics)
}

func TestScope(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "project")
	mod := filepath.Join(tmp, "scratch", "mod")
	cache := filepath.Join(tmp, "modcache")
	write(t, mod, "go.mod", "module example.com/mod\n")
	write(t, cache, "example.com/dep@v1.0.0/go.mod", "module example.com/dep\n")
	inProject := write(t, project, "main.go", "")
	modA := write(t, mod, "a.go", "")
	modB := write(t, mod, "sub/b.go", "")
	loose := write(t, tmp, "scratch/loose/c.go", "")
	dep := write(t, cache, "example.com/dep@v1.0.0/dep.go", "")

	s := newScope([]string{cache}, "", io.Discard)
	initialize := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":"` + uriOf(project) + `","workspaceFolders":[{"uri":"` + uriOf(project) + `","name":"project"}]}}`)
	if out := s.fromClient(initialize); len(out) != 1 || !bytes.Equal(out[0], initialize) {
		t.Fatalf("initialize was changed: %q", out)
	}

	open := s.fromClient(didOpen(t, modA))
	if len(open) != 2 {
		t.Fatalf("first file of an outside module: want a folder change and the didOpen, got %d messages", len(open))
	}
	if c := folderChange(t, open[0]); len(c.Params.Event.Added) != 1 || c.Params.Event.Added[0].URI != uriOf(mod) || len(c.Params.Event.Removed) != 0 {
		t.Fatalf("wrong folder change: %+v", c)
	}
	if !bytes.Equal(open[1], didOpen(t, modA)) {
		t.Fatalf("didOpen was changed: %s", open[1])
	}
	for _, path := range []string{modB, inProject, loose, dep} {
		if out := s.fromClient(didOpen(t, path)); len(out) != 1 {
			t.Errorf("%s: want the didOpen alone, got %d messages", path, len(out))
		}
	}

	for path, want := range map[string]int{inProject: 1, modA: 1, modB: 1, loose: 0, dep: 0} {
		out := s.fromServer(publish(t, path))
		if len(out) != 1 {
			t.Fatalf("%s: want one message, got %d", path, len(out))
		}
		if got := diagnosticCount(t, out[0]); got != want {
			t.Errorf("%s: want %d diagnostics, got %d", path, want, got)
		}
	}

	if out := s.fromClient(didClose(t, modA)); len(out) != 1 {
		t.Fatalf("module still has an open file: want the didClose alone, got %d messages", len(out))
	}
	closing := s.fromClient(didClose(t, modB))
	if len(closing) != 2 || !bytes.Equal(closing[0], didClose(t, modB)) {
		t.Fatalf("last file of the module: want the didClose then a folder change, got %q", closing)
	}
	if c := folderChange(t, closing[1]); len(c.Params.Event.Removed) != 1 || c.Params.Event.Removed[0].URI != uriOf(mod) {
		t.Fatalf("wrong folder change: %+v", c)
	}
	if got := diagnosticCount(t, s.fromServer(publish(t, modA))[0]); got != 0 {
		t.Errorf("module folder was removed, its diagnostics should be cleared, got %d", got)
	}
	if out := s.fromClient(didOpen(t, modA)); len(out) != 2 {
		t.Errorf("reopening a removed module should add it again, got %d messages", len(out))
	}
}

func TestScopeSkipsHomeModule(t *testing.T) {
	home := t.TempDir()
	write(t, home, "go.mod", "module home\n")
	file := write(t, home, "notes/x.go", "")
	s := newScope(nil, home, io.Discard)
	if out := s.fromClient(didOpen(t, file)); len(out) != 1 {
		t.Fatalf("a go.mod in the home directory must not become a workspace folder, got %d messages", len(out))
	}
}

func TestFraming(t *testing.T) {
	var buf bytes.Buffer
	bodies := []string{`{"a":1}`, `{"b":"ü"}`, ""}
	for _, b := range bodies {
		if err := writeMessage(&buf, []byte(b)); err != nil {
			t.Fatal(err)
		}
	}
	raw := "Content-Type: application/vscode-jsonrpc; charset=utf-8\r\nContent-Length: 2\r\n\r\n{}"
	r := bufio.NewReader(io.MultiReader(&buf, strings.NewReader(raw)))
	for _, want := range append(bodies, "{}") {
		got, err := readMessage(r)
		if err != nil || string(got) != want {
			t.Fatalf("want %q, got %q (%v)", want, got, err)
		}
	}
	if _, err := readMessage(r); err != io.EOF {
		t.Fatalf("want io.EOF at the end, got %v", err)
	}
	if _, err := readMessage(bufio.NewReader(strings.NewReader("Content-Length: 5\r\n\r\n{}"))); err == nil {
		t.Fatal("a truncated body must be an error")
	}
}
