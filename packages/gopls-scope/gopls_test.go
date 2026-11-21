package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type rpc struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func send(t *testing.T, w io.Writer, msg map[string]any) {
	t.Helper()
	msg["jsonrpc"] = "2.0"
	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeMessage(w, body); err != nil {
		t.Fatal(err)
	}
}

func diagnose(t *testing.T, gopls string, proxied bool, project string, files []string, logs io.Writer) map[string][]string {
	t.Helper()
	fromServer, serverOut := io.Pipe()
	serverIn, toServer := io.Pipe()
	done := make(chan error, 1)
	if proxied {
		go func() { done <- run([]string{gopls}, serverIn, serverOut, logs); _ = serverOut.Close() }()
	} else {
		cmd := exec.Command(gopls)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = serverIn, serverOut, logs
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		go func() { done <- cmd.Wait(); _ = serverOut.Close() }()
	}

	incoming := make(chan rpc, 64)
	go func() {
		defer close(incoming)
		r := bufio.NewReader(fromServer)
		for {
			body, err := readMessage(r)
			if err != nil {
				return
			}
			var m rpc
			if json.Unmarshal(body, &m) == nil {
				incoming <- m
			}
		}
	}()

	send(t, toServer, map[string]any{"id": 1, "method": "initialize", "params": map[string]any{
		"processId":        os.Getpid(),
		"rootUri":          uriOf(project),
		"rootPath":         project,
		"workspaceFolders": []folder{{URI: uriOf(project), Name: filepath.Base(project)}},
		"capabilities": map[string]any{
			"workspace":    map[string]any{"configuration": false, "workspaceFolders": false},
			"textDocument": map[string]any{"publishDiagnostics": map[string]any{}},
		},
	}})
	await := func(id string) {
		deadline := time.After(60 * time.Second)
		for {
			select {
			case m, ok := <-incoming:
				if !ok {
					t.Fatal("server closed the stream")
				}
				if m.Method == "" && string(m.ID) == id {
					return
				}
			case <-deadline:
				t.Fatalf("no response to request %s", id)
			}
		}
	}
	await("1")
	send(t, toServer, map[string]any{"method": "initialized", "params": map[string]any{}})
	for _, path := range files {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		send(t, toServer, map[string]any{"method": "textDocument/didOpen", "params": map[string]any{
			"textDocument": map[string]any{"uri": uriOf(path), "languageId": "go", "version": 1, "text": string(text)},
		}})
	}

	diags := map[string][]string{}
	overall := time.After(90 * time.Second)
	quiet := time.NewTimer(5 * time.Second)
collect:
	for {
		select {
		case m, ok := <-incoming:
			if !ok {
				t.Fatal("server closed the stream")
			}
			switch {
			case m.Method == "textDocument/publishDiagnostics":
				var p struct {
					URI         string `json:"uri"`
					Diagnostics []struct {
						Message string `json:"message"`
					} `json:"diagnostics"`
				}
				_ = json.Unmarshal(m.Params, &p)
				var msgs []string
				for _, d := range p.Diagnostics {
					msgs = append(msgs, d.Message)
				}
				diags[filePath(p.URI)] = msgs
				quiet.Reset(5 * time.Second)
			case m.Method != "" && len(m.ID) > 0:
				var result any
				if m.Method == "workspace/configuration" {
					var p struct {
						Items []json.RawMessage `json:"items"`
					}
					_ = json.Unmarshal(m.Params, &p)
					result = make([]any, len(p.Items))
				}
				send(t, toServer, map[string]any{"id": m.ID, "result": result})
			}
		case <-quiet.C:
			break collect
		case <-overall:
			break collect
		}
	}

	send(t, toServer, map[string]any{"id": 2, "method": "shutdown"})
	await("2")
	send(t, toServer, map[string]any{"method": "exit"})
	_ = toServer.Close()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("gopls did not exit")
	}
	return diags
}

func noise(msgs []string) []string {
	var found []string
	for _, m := range msgs {
		if strings.Contains(m, "not included in your workspace") || strings.Contains(m, "could not import") || strings.Contains(m, "undefined: helper") {
			found = append(found, m)
		}
	}
	return found
}

func TestGopls(t *testing.T) {
	gopls := os.Getenv("GOPLS")
	if gopls == "" {
		t.Skip("set GOPLS to a gopls binary to run against a real server")
	}
	tmp := t.TempDir()
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOPROXY", "off")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	t.Setenv("GOMODCACHE", filepath.Join(tmp, "modcache"))

	project := filepath.Join(tmp, "project")
	mod := filepath.Join(tmp, "scratch", "mod")
	write(t, project, "go.mod", "module example.com/project\n\ngo 1.26\n")
	write(t, project, "main.go", "package main\n\nfunc main() {}\n")
	write(t, mod, "go.mod", "module example.com/mod\n\ngo 1.26\n")
	write(t, mod, "inner/inner.go", "package inner\n\nconst X = 1\n")
	write(t, mod, "lib/helper.go", "package lib\n\nfunc helper() int { return 1 }\n")
	use := write(t, mod, "lib/use.go", "package lib\n\nimport \"example.com/mod/inner\"\n\nvar _ = helper() + inner.X\n\nvar broken int = \"x\"\n")
	loose := write(t, tmp, "scratch/loose/loose.go", "package loose\n\nimport \"example.com/mod/inner\"\n\nvar _ = inner.X + missing\n")
	files := []string{use, loose}

	t.Run("gopls alone", func(t *testing.T) {
		diags := diagnose(t, gopls, false, project, files, io.Discard)
		t.Logf("use.go: %q\nloose.go: %q", diags[use], diags[loose])
		if len(noise(diags[use])) == 0 {
			t.Errorf("expected gopls to misreport a module outside the workspace folder, got %q", diags[use])
		}
		if len(diags[loose]) == 0 {
			t.Errorf("expected gopls to report the loose file, got nothing")
		}
	})

	t.Run("behind gopls-scope", func(t *testing.T) {
		var logs lockedBuffer
		diags := diagnose(t, gopls, true, project, files, &logs)
		t.Logf("use.go: %q\nloose.go: %q", diags[use], diags[loose])
		if found := noise(diags[use]); len(found) > 0 {
			t.Errorf("workspace noise survived: %q", found)
		}
		typeError := false
		for _, m := range diags[use] {
			typeError = typeError || strings.Contains(m, `cannot use "x"`)
		}
		if !typeError {
			t.Errorf("the real type error in use.go is missing, got %q", diags[use])
		}
		if len(diags[loose]) != 0 {
			t.Errorf("a file outside any module should have no diagnostics, got %q", diags[loose])
		}
		if !strings.Contains(logs.String(), "workspace folder added: "+mod) {
			t.Errorf("no folder was added for %s; logs:\n%s", mod, logs.String())
		}
	})
}
