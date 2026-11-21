package cli

import (
	"strings"
	"testing"
)

func TestUIRefusesNonLoopback(t *testing.T) {
	sb := newSandbox(t)
	for _, addr := range []string{"0.0.0.0:0", ":0", "192.168.1.5:7781", "example.com:80", "[::]:0"} {
		code, out, errOut := sb.run("", "ui", "--listen", addr)
		if code != 2 || out != "" || !strings.Contains(errOut, "non-loopback") {
			t.Fatalf("ui --listen %s = %d %q %q", addr, code, out, errOut)
		}
	}
}

func TestUIUsageErrors(t *testing.T) {
	sb := newSandbox(t)
	if code, _, errOut := sb.run("", "ui", "extra"); code != 2 || !strings.Contains(errOut, "positional") {
		t.Fatalf("positional = %d %q", code, errOut)
	}
	if code, _, _ := sb.run("", "ui", "--listen", "127.0.0.1:notaport"); code != 2 {
		t.Fatalf("bad port = %d", code)
	}
	if code, _, errOut := sb.run("", "ui", "--listen", "127.0.0.1:0"); code != 1 || !strings.Contains(errOut, "no memory database") {
		t.Fatalf("missing db = %d %q", code, errOut)
	}
}

func TestUIListedInUsage(t *testing.T) {
	sb := newSandbox(t)
	if code, out, _ := sb.run("", "help"); code != 0 || !strings.Contains(out, "ui [--listen ADDR]") {
		t.Fatalf("help = %d, ui missing from usage", code)
	}
}
