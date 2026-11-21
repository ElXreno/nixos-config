package vec

import (
	"net"
	"testing"
	"time"
)

func listen(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	return "http://" + ln.Addr().String()
}

func closedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return "http://" + addr
}

func TestPick(t *testing.T) {
	up, other, down := listen(t), listen(t), closedAddr(t)
	cases := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"single is returned without a dial", down, down},
		{"first reachable wins", up + "," + other, up},
		{"a dead first falls through", down + "," + up, up},
		{"spaces and empty parts are dropped", " " + down + " , ," + up + " ", up},
		{"the last one is the answer when nothing accepts", down + "," + down + "/x", down + "/x"},
		{"an unparsable entry is skipped", "not a url," + up, up},
	}
	for _, c := range cases {
		if got := Pick(t.Context(), c.in); got != c.want {
			t.Errorf("%s: Pick(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestPickDialLimit(t *testing.T) {
	up := listen(t)
	start := time.Now()
	if got := Pick(t.Context(), "http://192.0.2.1:8080,"+up); got != up {
		t.Fatalf("Pick = %q, want %q", got, up)
	}
	if d := time.Since(start); d > pickDial+200*time.Millisecond {
		t.Errorf("took %v, the dial limit is %v", d, pickDial)
	}
}

func TestClientsPickTheReachableURL(t *testing.T) {
	up, down := listen(t), closedAddr(t)
	if got := NewClient(Options{URL: down + "," + up + "/"}).c.url; got != up {
		t.Errorf("embed client url = %q, want %q", got, up)
	}
	if got := NewReranker(RerankOptions{URL: down + "," + up}).url; got != up {
		t.Errorf("reranker url = %q, want %q", got, up)
	}
}
