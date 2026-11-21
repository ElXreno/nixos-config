package hook

import (
	"context"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"
)

const dialSlack = 400 * time.Millisecond

func TestPrewarmAddr(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"http://127.0.0.1:8080", "127.0.0.1:8080", true},
		{"http://tei.example", "tei.example:80", true},
		{"https://tei.example", "tei.example:443", true},
		{"http://tei.example:9/v1/embeddings", "tei.example:9", true},
		{"http://[::1]:8080", "[::1]:8080", true},
		{"http://[::1]", "[::1]:80", true},
		{"", "", false},
		{"not a url", "", false},
		{"%%", "", false},
		{"unix:///run/tei.sock", "", false},
		{"http://:8080", "", false},
	}
	for _, c := range cases {
		got, ok := prewarmAddr(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("%q: %q %v, want %q %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestPrewarmListeningSocket(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	got := make(chan struct{}, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		got <- struct{}{}
		buf := make([]byte, 1)
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		_, err = c.Read(buf)
		if err == nil {
			t.Errorf("the prewarm sent data")
		}
		_ = c.Close()
	}()
	start := time.Now()
	out := prewarm(t.Context(), "http://"+ln.Addr().String())
	if d := time.Since(start); d > dialSlack {
		t.Errorf("took %v", d)
	}
	if out != "ok" {
		t.Errorf("outcome %q", out)
	}
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Error("the listener saw no connection")
	}
}

func TestPrewarmRefusedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	start := time.Now()
	out := prewarm(t.Context(), "http://"+addr)
	if d := time.Since(start); d > dialSlack {
		t.Errorf("took %v", d)
	}
	if out != "down" {
		t.Errorf("outcome %q", out)
	}
}

func TestPrewarmUnroutableAddress(t *testing.T) {
	start := time.Now()
	out := prewarm(t.Context(), "http://192.0.2.1:8080")
	if d := time.Since(start); d > dialSlack {
		t.Errorf("took %v, the dial limit is %v", d, prewarmDial)
	}
	if out != "timeout" && out != "down" && out != "ok" {
		t.Errorf("outcome %q", out)
	}
}

func TestPrewarmBadAndCancelled(t *testing.T) {
	if out := prewarm(t.Context(), ""); out != "" {
		t.Errorf("empty url: %q", out)
	}
	if out := prewarm(t.Context(), "not a url"); out != "skip" {
		t.Errorf("bad url: %q", out)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	defer cancel()
	start := time.Now()
	prewarm(ctx, "http://192.0.2.1:8080")
	if d := time.Since(start); d > dialSlack {
		t.Errorf("cancelled context took %v", d)
	}
}

func sessionStartInput(sb *sandbox, session string) string {
	return `{"session_id":"` + session + `","cwd":"` + sb.root + `/repos/nixos-config","hook_event_name":"SessionStart","source":"startup"}`
}

func TestSessionStartPrewarmsTheEmbedder(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	conns := make(chan struct{}, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conns <- struct{}{}
			_ = c.Close()
		}
	}()
	url := "http://" + ln.Addr().String()
	cases := []struct {
		name string
		args []string
		env  string
	}{
		{"flag", []string{"--embed-url", url}, ""},
		{"environment", nil, url},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sb := newSandbox(t)
			sb.seed(t)
			sb.vars["MEM_EMBED_URL"] = c.env
			sb.check(t, spec{name: "session start", event: SessionStart, stdin: sessionStartInput(sb, "pw1"), args: c.args,
				has: []string{"smok-apex-d2"}, log: []string{"inject", "prewarm=ok"}})
			select {
			case <-conns:
			case <-time.After(time.Second):
				t.Error("no connection reached the listener")
			}
		})
	}
}

func TestSessionStartPrewarmNeverFailsTheHook(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refused := "http://" + ln.Addr().String()
	_ = ln.Close()
	for name, url := range map[string]string{"refused": refused, "unroutable": "http://192.0.2.1:8080", "garbage": "::not a url::"} {
		t.Run(name, func(t *testing.T) {
			sb := newSandbox(t)
			sb.seed(t)
			sb.vars["MEM_EMBED_URL"] = url
			start := time.Now()
			sb.check(t, spec{name: "session start", event: SessionStart, stdin: sessionStartInput(sb, "pw2"),
				has: []string{"smok-apex-d2"}, log: []string{"inject", "prewarm="}})
			if d := time.Since(start); d > time.Second {
				t.Errorf("took %v", d)
			}
			if line := sb.lastLog(t); name != "unroutable" && strings.Contains(line, "prewarm=ok") {
				t.Errorf("log line %q", line)
			}
		})
	}
}

func TestSessionStartWithoutEmbedURLDoesNotPrewarm(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.check(t, spec{name: "session start", event: SessionStart, stdin: sessionStartInput(sb, "pw3"), has: []string{"smok-apex-d2"}})
	if line := sb.lastLog(t); strings.Contains(line, "prewarm") {
		t.Errorf("log line %q", line)
	}
}

func TestPrewarmBlackholedAddress(t *testing.T) {
	old := prewarmControl
	prewarmControl = func(ctx context.Context, _, _ string, _ syscall.RawConn) error {
		<-ctx.Done()
		return ctx.Err()
	}
	t.Cleanup(func() { prewarmControl = old })
	start := time.Now()
	out := prewarm(t.Context(), "http://192.0.2.1:8080")
	took := time.Since(start)
	if out != "timeout" {
		t.Errorf("outcome %q", out)
	}
	if took < prewarmDial/2 || took > dialSlack {
		t.Errorf("timeout after %v, the dial limit is %v", took, prewarmDial)
	}
}

func TestSessionStartSurvivesABlackholedEmbedder(t *testing.T) {
	old := prewarmControl
	prewarmControl = func(ctx context.Context, _, _ string, _ syscall.RawConn) error {
		<-ctx.Done()
		return ctx.Err()
	}
	t.Cleanup(func() { prewarmControl = old })
	sb := newSandbox(t)
	sb.seed(t)
	sb.vars["MEM_EMBED_URL"] = "http://192.0.2.1:8080"
	start := time.Now()
	sb.check(t, spec{name: "session start", event: SessionStart, stdin: sessionStartInput(sb, "pw4"),
		has: []string{"smok-apex-d2"}, log: []string{"inject", "prewarm=timeout"}})
	if d := time.Since(start); d > 800*time.Millisecond {
		t.Errorf("took %v", d)
	}
}
