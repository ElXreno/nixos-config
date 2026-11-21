package cli

import (
	"bytes"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"mem/internal/env"
)

func (sb *sandbox) runPipe(t *testing.T, payload string, closeWriter bool, args ...string) (int, string, string, time.Duration) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()
	if payload != "" {
		if _, err := w.WriteString(payload); err != nil {
			t.Fatal(err)
		}
	}
	if closeWriter {
		_ = w.Close()
	}
	var out, errb bytes.Buffer
	e := env.Env{
		Stdin:    r,
		Stdout:   &out,
		Stderr:   &errb,
		Getenv:   func(k string) string { return sb.vars[k] },
		Now:      func() time.Time { return sb.now },
		Getwd:    func() (string, error) { return sb.dir, nil },
		Hostname: func() (string, error) { return "SMOK", nil },
	}
	start := time.Now()
	code := Run(args, e)
	return code, out.String(), errb.String(), time.Since(start)
}

func TestPutUpdateIdlePipe(t *testing.T) {
	sb := newSandbox(t)
	if code, _, errs := sb.run("", "put", "pipe-note", "--kind", "fact", "--scope", "global", "--title", "Old", "--body", "orchard lanterns drift over basalt"); code != 0 {
		t.Fatalf("create: exit %d: %s", code, errs)
	}

	code, _, errs, took := sb.runPipe(t, "", false, "put", "pipe-note", "--update", "--title", "New")
	if code != 0 || took > 2*time.Second || !strings.Contains(errs, "nothing on stdin, body unchanged") {
		t.Fatalf("idle pipe: exit %d after %s: %s", code, took, errs)
	}
	_, out, _ := sb.run("", "get", "pipe-note")
	if !strings.Contains(out, "title: New") || !strings.Contains(out, "orchard lanterns drift over basalt") {
		t.Fatalf("idle pipe update lost data:\n%s", out)
	}

	code, _, errs, _ = sb.runPipe(t, "copper herons nest in quiet reeds", true, "put", "pipe-note", "--update")
	if code != 0 {
		t.Fatalf("pipe with data: exit %d: %s", code, errs)
	}
	_, out, _ = sb.run("", "get", "pipe-note")
	if !strings.Contains(out, "copper herons nest in quiet reeds") {
		t.Fatalf("pipe body not applied:\n%s", out)
	}

	code, _, errs, _ = sb.runPipe(t, "", true, "put", "pipe-note", "--update", "--title", "Closed")
	if code != 0 || strings.Contains(errs, "nothing on stdin") {
		t.Fatalf("closed pipe: exit %d: %s", code, errs)
	}
}

func (sb *sandbox) runFile(stdin *os.File, args ...string) (int, string, string, time.Duration) {
	var out, errb bytes.Buffer
	e := env.Env{
		Stdin:    stdin,
		Stdout:   &out,
		Stderr:   &errb,
		Getenv:   func(k string) string { return sb.vars[k] },
		Now:      func() time.Time { return sb.now },
		Getwd:    func() (string, error) { return sb.dir, nil },
		Hostname: func() (string, error) { return "SMOK", nil },
	}
	start := time.Now()
	code := Run(args, e)
	return code, out.String(), errb.String(), time.Since(start)
}

func socketPair(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	near, far := os.NewFile(uintptr(fds[0]), "stdin"), os.NewFile(uintptr(fds[1]), "peer")
	t.Cleanup(func() {
		_ = near.Close()
		_ = far.Close()
	})
	return near, far
}

func TestIdleSocketOnStdinNeverBlocks(t *testing.T) {
	old := stdinStall
	stdinStall = 150 * time.Millisecond
	t.Cleanup(func() { stdinStall = old })
	sb := newSandbox(t)
	if code, _, errs := sb.run("", "put", "sock-note", "--kind", "fact", "--scope", "global", "--title", "Old", "--body", "orchard lanterns drift over basalt"); code != 0 {
		t.Fatalf("create: exit %d: %s", code, errs)
	}

	idle, _ := socketPair(t)
	code, _, errs, took := sb.runFile(idle, "put", "sock-note", "--update", "--title", "New")
	if code != 0 || took > 2*time.Second || !strings.Contains(errs, "nothing on stdin, body unchanged") {
		t.Fatalf("metadata update on an idle socket: exit %d after %s: %s", code, took, errs)
	}
	if _, out, _ := sb.run("", "get", "sock-note"); !strings.Contains(out, "title: New") || !strings.Contains(out, "orchard lanterns drift over basalt") {
		t.Fatalf("the update lost data:\n%s", out)
	}

	idle, _ = socketPair(t)
	code, _, errs, took = sb.runFile(idle, "put", "sock-new", "--kind", "fact", "--scope", "global", "--title", "T")
	if code != 2 || took > 2*time.Second || !strings.Contains(errs, "mem put: no body: nothing arrives on stdin") {
		t.Fatalf("a new record without a body on an idle socket: exit %d after %s: %s", code, took, errs)
	}
	if code, _, _ := sb.run("", "get", "sock-new"); code != 1 {
		t.Fatalf("a record was written without a body: exit %d", code)
	}

	idle, _ = socketPair(t)
	code, _, errs, took = sb.runFile(idle, "edit", "sock-note", "--old", "basalt", "--new", "-")
	if code != 2 || took > 2*time.Second || !strings.Contains(errs, "mem edit: - nothing arrives on stdin") {
		t.Fatalf("edit --new - on an idle socket: exit %d after %s: %s", code, took, errs)
	}

	near, far := socketPair(t)
	if _, err := far.WriteString("copper herons nest in quiet reeds"); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Shutdown(int(far.Fd()), syscall.SHUT_WR); err != nil {
		t.Fatal(err)
	}
	code, out, errs, _ := sb.runFile(near, "put", "sock-new", "--kind", "fact", "--scope", "global", "--title", "T")
	if code != 0 || !strings.Contains(out, "sock-new\tcreated") {
		t.Fatalf("a body sent over a socket: exit %d %s %s", code, out, errs)
	}
	if _, out, _ := sb.run("", "get", "--body", "sock-new"); out != "copper herons nest in quiet reeds\n" {
		t.Fatalf("body %q", out)
	}
}
