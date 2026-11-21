package hook

import (
	"context"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"mem/internal/recall"
)

func TestShadowLineHasNoPointerFieldsWithoutPointerFlags(t *testing.T) {
	sb := tierSandbox(t)
	sb.prompt(t, "np", "--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg",
		"--shadow-vec-pool", "fused-reg", "--reg-cos", f32(strongCos))
	data, err := os.ReadFile(sb.shadowPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "pointers") {
		t.Errorf("shadow line carries pointer fields with the tier off: %s", data)
	}
}

func TestShadowLineKeepsPointerFieldsWhenTheTierIsOn(t *testing.T) {
	sb := tierSandbox(t)
	args := tierFlags("--shadow-vec-pool", "fused-reg")
	sb.prompt(t, "wp", args...)
	sb.prompt(t, "wp", args...)
	for i, l := range strings.Split(strings.TrimSpace(readFile(t, sb.shadowPath())), "\n") {
		for _, f := range []string{`"live_pointers":`, `"shadow_pointers":`} {
			if !strings.Contains(l, f) {
				t.Errorf("line %d lacks %s", i, f)
			}
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var keyRe = regexp.MustCompile("- `([^`]+)` \\[")

func TestNoRecordInBothTiersAtAnyBudget(t *testing.T) {
	sb := tierSandbox(t)
	full := sb.prompt(t, "uf", tierFlags("--reg-pointer-cos", "0.01", "--pointer-take", "9")...)
	hi := utf8.RuneCountInString(full)
	for max := 1; max <= hi+2; max++ {
		got := sb.prompt(t, "ub"+strconv.Itoa(max), tierFlags("--reg-pointer-cos", "0.01", "--pointer-take", "9", "--max-chars", strconv.Itoa(max))...)
		if n := utf8.RuneCountInString(got); n > max {
			t.Errorf("max %d: %d chars", max, n)
		}
		seen := map[string]bool{}
		for _, m := range keyRe.FindAllStringSubmatch(got, -1) {
			if seen[m[1]] {
				t.Errorf("max %d: %s listed twice:\n%s", max, m[1], got)
			}
			seen[m[1]] = true
		}
	}
}

func TestNoRecordInBothTiersWithLexicalRegulars(t *testing.T) {
	sb := tierSandbox(t)
	got := sb.prompt(t, "lr", tierFlags("--vec-lex-regular", "--reg-pointer-cos", "0.01", "--pointer-take", "9")...)
	seen := map[string]bool{}
	for _, m := range keyRe.FindAllStringSubmatch(got, -1) {
		if seen[m[1]] {
			t.Errorf("%s listed twice:\n%s", m[1], got)
		}
		seen[m[1]] = true
	}
}

func fullBacklogListener(t *testing.T) string {
	t.Helper()
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = syscall.Close(fd) })
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		t.Skip(err)
	}
	if err := syscall.Listen(fd, 0); err != nil {
		t.Skip(err)
	}
	sa, err := syscall.Getsockname(fd)
	if err != nil {
		t.Skip(err)
	}
	addr := "127.0.0.1:" + strconv.Itoa(sa.(*syscall.SockaddrInet4).Port)
	var held []net.Conn
	t.Cleanup(func() {
		for _, c := range held {
			_ = c.Close()
		}
	})
	for range 8 {
		c, err := net.DialTimeout("tcp", addr, 150*time.Millisecond)
		if err != nil {
			return addr
		}
		held = append(held, c)
	}
	t.Skip("the accept queue never filled")
	return ""
}

func TestPrewarmAgainstAFullBacklogReturnsAtTheLimit(t *testing.T) {
	addr := fullBacklogListener(t)
	start := time.Now()
	out := prewarm(t.Context(), "http://"+addr)
	took := time.Since(start)
	if out != "timeout" {
		t.Errorf("outcome %q", out)
	}
	if took < prewarmDial/2 || took > dialSlack {
		t.Errorf("took %v, the limit is %v", took, prewarmDial)
	}
}

func TestSessionStartAgainstAFullBacklogStaysFast(t *testing.T) {
	addr := fullBacklogListener(t)
	sb := newSandbox(t)
	sb.seed(t)
	sb.vars["MEM_EMBED_URL"] = "http://" + addr
	start := time.Now()
	sb.check(t, spec{name: "session start", event: SessionStart, stdin: sessionStartInput(sb, "bl1"),
		has: []string{"smok-apex-d2"}, log: []string{"inject", "prewarm=timeout"}})
	if d := time.Since(start); d > 800*time.Millisecond {
		t.Errorf("took %v", d)
	}
}

func TestPrewarmWithAHangingResolverReturnsAtTheLimit(t *testing.T) {
	old := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	t.Cleanup(func() { net.DefaultResolver = old })
	start := time.Now()
	out := prewarm(t.Context(), "http://tei.example:8080")
	took := time.Since(start)
	if out == "ok" {
		t.Errorf("outcome %q", out)
	}
	if took > dialSlack {
		t.Errorf("took %v, the limit is %v", took, prewarmDial)
	}
}

func TestStrongTierIsUntouchedByPointersAtAnyBudget(t *testing.T) {
	sb := tierSandbox(t)
	wide := []string{"--reg-pointer-cos", "0.01", "--pointer-take", "9"}
	hi := utf8.RuneCountInString(sb.prompt(t, "sf", tierFlags(wide...)...))
	for max := 1; max <= hi+2; max++ {
		m := strconv.Itoa(max)
		strong := sb.prompt(t, "ss"+m, "--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg",
			"--reg-cos", f32(strongCos), "--max-chars", m)
		both := sb.prompt(t, "sb"+m, tierFlags(append(wide, "--max-chars", m)...)...)
		switch {
		case strong != "" && !strings.HasPrefix(both, strong):
			t.Errorf("max %d: the strong text changed:\n%s\n---\n%s", max, strong, both)
		case strong == "" && strings.Contains(both, recall.PromptHeader):
			t.Errorf("max %d: strong tier appeared with pointers on:\n%s", max, both)
		}
	}
}

func TestAStronglyInjectedRecordNeverComesBackAsAPointer(t *testing.T) {
	sb := tierSandbox(t)
	strongOnly := []string{"--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg", "--reg-cos", f32(strongCos)}
	if got := sb.prompt(t, "sp", strongOnly...); !strings.Contains(got, "no-push-default") {
		t.Fatalf("setup:\n%s", got)
	}
	got := sb.prompt(t, "sp", "--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg",
		"--reg-cos", "0.95", "--reg-pointer-cos", f32(pointerCos))
	for _, k := range []string{"no-push-default", "bs3pro-0xdf"} {
		if strings.Contains(got, "`"+k+"`") {
			t.Errorf("%s came back as a pointer:\n%s", k, got)
		}
	}
	if !strings.Contains(got, "zfs-arc-ram") {
		t.Errorf("fresh pointer missing:\n%s", got)
	}
}

func TestAPointerLeftOutForRoomIsOfferedAgain(t *testing.T) {
	sb := tierSandbox(t)
	full := sb.prompt(t, "pf", tierFlags()...)
	strongOnly := sb.prompt(t, "ps", "--embed-url", "http://embedder.invalid", "--embed-model", fakeModel, "--vec-pool", "reg", "--reg-cos", f32(strongCos))
	need := utf8.RuneCountInString(strongOnly) + utf8.RuneCountInString(recall.PointerHeader+zfsLine)
	first := sb.prompt(t, "pr", tierFlags("--max-chars", strconv.Itoa(need))...)
	if !strings.Contains(first, "zfs-arc-ram") || strings.Contains(first, "settings-json-copy") {
		t.Fatalf("setup:\n%s", first)
	}
	second := sb.prompt(t, "pr", tierFlags()...)
	if strings.Contains(second, "zfs-arc-ram") || !strings.Contains(second, "settings-json-copy") {
		t.Errorf("second prompt:\n%s\nfull:\n%s", second, full)
	}
}
