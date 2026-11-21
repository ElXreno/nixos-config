package power

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-guard/internal/shell"
)

type supplies map[string]map[string]string

func fake(t *testing.T, s supplies) string {
	t.Helper()
	root := t.TempDir()
	for name, attrs := range s {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for k, v := range attrs {
			if err := os.WriteFile(filepath.Join(dir, k), []byte(v+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

func mains(online string) map[string]string {
	return map[string]string{"type": "Mains", "online": online}
}

func bat(status, capacity string) map[string]string {
	return map[string]string{"type": "Battery", "present": "1", "status": status, "capacity": capacity}
}

func TestRead(t *testing.T) {
	cases := []struct {
		name string
		s    supplies
		want string
		bat  bool
	}{
		{"no supplies", supplies{}, "AC, no battery", false},
		{"desktop with a mouse battery", supplies{"hidpp_battery_0": {"type": "Battery", "scope": "Device", "status": "Discharging", "capacity": "30"}}, "AC, no battery", false},
		{"laptop on AC at the charge limit", supplies{"ADP1": mains("1"), "BAT1": bat("Not charging", "63")}, "AC, battery 63%, not charging", false},
		{"laptop on battery", supplies{"ADP1": mains("0"), "BAT1": bat("Discharging", "41")}, "battery 41%, discharging", true},
		{"mains offline but charging", supplies{"ADP1": mains("0"), "BAT1": bat("Charging", "41")}, "AC, battery 41%, charging", false},
		{"mains offline wins over full", supplies{"ADP1": mains("0"), "BAT1": bat("Full", "100")}, "battery 100%, full", true},
		{"no external supply, discharging", supplies{"BAT0": bat("Discharging", "70")}, "battery 70%, discharging", true},
		{"no external supply, full", supplies{"BAT0": bat("Full", "100")}, "AC, battery 100%, full", false},
		{"usb-c sink only", supplies{"ADP1": mains("0"), "ucsi-source-psy-USBC000:001": {"type": "USB", "online": "1"}, "BAT0": bat("Discharging", "50")}, "AC, battery 50%, discharging", false},
		{"usb online 2", supplies{"usb": {"type": "USB", "online": "2"}, "BAT0": bat("Not charging", "80")}, "AC, battery 80%, not charging", false},
		{"device-scoped charger ignored", supplies{"ADP1": mains("0"), "phone": {"type": "USB", "scope": "Device", "online": "1"}, "BAT0": bat("Discharging", "50")}, "battery 50%, discharging", true},
		{"peripheral battery beside the system one", supplies{"ADP1": mains("1"), "BAT0": bat("Not charging", "80"), "hid-mouse": {"type": "Battery", "scope": "Device", "status": "Discharging", "capacity": "5"}}, "AC, battery 80%, not charging", false},
		{"two batteries by energy", supplies{"AC": mains("0"),
			"BAT0": {"type": "Battery", "status": "Unknown", "capacity": "40", "energy_now": "20000000", "energy_full": "50000000"},
			"BAT1": {"type": "Battery", "status": "Discharging", "capacity": "60", "energy_now": "15000000", "energy_full": "25000000"}}, "battery 47%, discharging", true},
		{"empty slot skipped", supplies{"AC": mains("0"), "BAT0": bat("Discharging", "55"), "BAT1": {"type": "Battery", "present": "0"}}, "battery 55%, discharging", true},
		{"capacity from charge", supplies{"AC": mains("0"), "BAT0": {"type": "Battery", "status": "Discharging", "charge_now": "3912000", "charge_full": "6168000"}}, "battery 63%, discharging", true},
		{"no capacity at all", supplies{"AC": mains("0"), "BAT0": {"type": "Battery", "status": "Discharging"}}, "battery, discharging", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Read(fake(t, c.s))
			if s.String() != c.want || s.OnBattery() != c.bat {
				t.Errorf("want %q (on battery %v), got %q (%v) %+v", c.want, c.bat, s.String(), s.OnBattery(), s)
			}
		})
	}
	if s := Read(filepath.Join(t.TempDir(), "missing")); s.Battery || !s.OnAC {
		t.Errorf("a missing root is a machine without a battery: %+v", s)
	}
	if s := Read(""); s.Battery || !s.OnAC {
		t.Errorf("an empty root reads nothing: %+v", s)
	}
}

func TestProbeReadsOnce(t *testing.T) {
	root := fake(t, supplies{"AC": mains("0"), "BAT0": bat("Discharging", "50")})
	p := &Probe{Root: root}
	if !p.State().OnBattery() {
		t.Fatal("on battery")
	}
	if err := os.WriteFile(filepath.Join(root, "AC", "online"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !p.State().OnBattery() {
		t.Error("one hook call reads the supplies once")
	}
}

var defaults = []string{
	"cargo build", "cargo test", "cmake --build", "docker build", "docker buildx build", "go build -race",
	"go test -race", "make --jobs*", "make -j*", "nh home build", "nh home switch", "nh os boot", "nh os build*",
	"nh os switch", "nh os test", "ninja !-t*", "nix build", "nix flake check", "nix-build", "nixos-rebuild boot",
	"nixos-rebuild build*", "nixos-rebuild dry-activate", "nixos-rebuild switch", "nixos-rebuild test",
	"nom build", "nom-build", "podman build",
}

func TestHeavy(t *testing.T) {
	patterns, err := Parse(defaults)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ command, want string }{
		{"nix build .#x", "nix build"},
		{"nix -L build .#x", "nix build"},
		{"nix --extra-experimental-features 'nix-command flakes' build .#x", "nix build"},
		{"nix eval .#build", ""},
		{"nix run nixpkgs#hello -- build", ""},
		{"nixos-rebuild build --flake .#KURWA", "nixos-rebuild build*"},
		{"nixos-rebuild --flake .#SMOK switch", "nixos-rebuild switch"},
		{"nixos-rebuild build-vm --flake .#SMOK", "nixos-rebuild build*"},
		{"nixos-rebuild --help", ""},
		{"nixos-rebuild list-generations", ""},
		{"nixos-rebuild --flake .#SMOK repl", ""},
		{"nix build --help", ""},
		{"nix build -h", ""},
		{"nix build --dry-run .#x", ""},
		{"nix build --version", ""},
		{"nix build --dry-run=false .#x", "nix build"},
		{"nix eval --raw .#x --apply build", ""},
		{"nix path-info --recursive nixpkgs#hello build", ""},
		{"nix flake check --no-build", ""},
		{"nix flake check -L", "nix flake check"},
		{"nix --accept-flake-config flake check", "nix flake check"},
		{"nom build .#x", "nom build"},
		{"nom-build -A hello", "nom-build"},
		{"nix flake check", "nix flake check"},
		{"nix flake show", ""},
		{"nix-build -A hello", "nix-build"},
		{"nix develop .#agent-guard -c go test ./...", ""},
		{"nix develop .#agent-guard -c go test -race ./...", "go test -race"},
		{"go build ./...", ""},
		{"go build -race -o x .", "go build -race"},
		{"go test -race=true ./...", "go test -race"},
		{"go test -count=1 -race ./...", "go test -race"},
		{"go test -race=false ./...", ""},
		{"cargo build --release", "cargo build"},
		{"cargo +nightly build", "cargo build"},
		{"cargo run -- test", ""},
		{"cargo run test", ""},
		{"cargo tree -e build", ""},
		{"make -j$(nproc)", "make -j*"},
		{"make -j 8 all", "make -j*"},
		{"make --jobs=8", "make --jobs*"},
		{"make install", ""},
		{"cmake --build build", "cmake --build"},
		{"cmake -B build", ""},
		{"ninja -C build", "ninja !-t*"},
		{"ninja -C build -t clean", ""},
		{"ninja -tcommands x", ""},
		{"docker build -t x .", "docker build"},
		{"docker buildx build .", "docker buildx build"},
		{"docker buildx ls", ""},
		{"podman build .", "podman build"},
		{"docker run img make -j8", ""},
		{"docker run --rm img cargo build", ""},
		{"docker logs build", ""},
		{"docker exec c nix build", ""},
		{"podman exec c nix build", ""},
		{"ssh bimba nix build .#a", ""},
		{`sh -c "cargo test"`, "cargo test"},
		{`nix-shell -p x --run "ninja -C b"`, "ninja !-t*"},
		{"sudo nice -n 19 nix build", "nix build"},
		{"echo nix build", ""},
		{"cd x && nix build", "nix build"},
		{"nh os build .", "nh os build*"},
		{"nh os switch .", "nh os switch"},
		{"nh os test", "nh os test"},
		{"nh os boot -H SMOK .", "nh os boot"},
		{"nh home switch", "nh home switch"},
		{"nh os info", ""},
		{"nh clean all", ""},
	}
	for _, c := range cases {
		got := ""
		if p, ok := Heavy(shell.Analyze(c.command).Commands, patterns); ok {
			got = p.Text
		}
		if got != c.want {
			t.Errorf("%s: want %q got %q", c.command, c.want, got)
		}
	}
}

func TestParse(t *testing.T) {
	for _, bad := range []string{"", "  ", "nix [build", "!nix build", "-j8", "ninja !", "ninja ![t"} {
		if _, err := Parse([]string{bad}); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	p, err := Parse([]string{"  nix   build "})
	if err != nil || p[0].Text != "nix build" {
		t.Errorf("words are normalised: %+v %v", p, err)
	}
}

func TestMessages(t *testing.T) {
	ac := State{Battery: true, OnAC: true, Capacity: 41}
	bat := State{Battery: true, Capacity: 40}
	cases := []struct {
		name string
		prev State
		seen bool
		cur  State
		want string
	}{
		{"first on battery", State{}, false, bat, "Power: on battery at 40%."},
		{"first on AC", State{}, false, ac, ""},
		{"still on AC", ac, true, ac, ""},
		{"still on battery", bat, true, State{Battery: true, Capacity: 12}, ""},
		{"unplugged", ac, true, bat, "Power: unplugged, now on battery at 40%."},
		{"plugged", bat, true, ac, "Power: back on AC, battery at 41%."},
		{"plugged, no capacity", bat, true, State{Battery: true, OnAC: true, Capacity: -1}, "Power: back on AC."},
		{"first on battery, no capacity", State{}, false, State{Battery: true, Capacity: -1}, "Power: on battery."},
	}
	for _, c := range cases {
		if got := Change(c.prev, c.seen, c.cur); got != c.want {
			t.Errorf("%s: want %q got %q", c.name, c.want, got)
		}
	}
	p, _ := Parse([]string{"nix build"})
	if got := HeavyNote(bat, p[0]); got != "Power: on battery at 40%, heavy build (`nix build`). If it can wait, `agent-guard power wait ac` run in the background exits once AC is back." {
		t.Errorf("heavy note: %q", got)
	}
	p, _ = Parse([]string{"ninja !-t* -C"})
	if got := HeavyNote(bat, p[0]); !strings.Contains(got, "heavy build (`ninja -C`)") || p[0].Text != "ninja !-t* -C" {
		t.Errorf("the note names the pattern without its exclusions: %q, text %q", got, p[0].Text)
	}
}

func TestMemory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "power")
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	m := Memory{Dir: dir, Now: func() time.Time { return now }}
	if _, _, err := m.Swap("", State{}); err == nil {
		t.Error("no session, nothing to remember")
	}
	if _, _, err := (Memory{}).Swap("k", State{}); err == nil {
		t.Error("no dir, nothing to remember")
	}
	bat := State{Battery: true, Capacity: 40}
	ac := State{Battery: true, OnAC: true, Capacity: 41}
	if _, seen, err := m.Swap("claude-code-s1", bat); err != nil || seen {
		t.Fatalf("first swap: seen %v err %v", seen, err)
	}
	prev, seen, err := m.Swap("claude-code-s1", ac)
	if err != nil || !seen || prev.OnAC || prev.Capacity != 40 {
		t.Fatalf("second swap returns the first state: %+v %v %v", prev, seen, err)
	}
	if prev, seen, _ := m.Swap("claude-code-s1", ac); !seen || !prev.OnAC {
		t.Errorf("third swap returns the second state: %+v", prev)
	}
	if _, seen, _ := m.Swap("pi-s1", ac); seen {
		t.Error("keys are separate")
	}
	info, err := os.Stat(filepath.Join(dir, "claude-code-s1.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("state file 0600: %v %v", info, err)
	}
	if _, _, err := m.Swap("../../escape/x", ac); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".._.._escape_x.json")); err != nil {
		t.Errorf("the key cannot leave the dir: %v", err)
	}
	old := filepath.Join(dir, "claude-code-old.json")
	if err := os.WriteFile(old, []byte(`{"ac":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stale := now.Add(-15 * 24 * time.Hour)
	if err := os.Chtimes(old, stale, stale); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dir, "pi-s1.json"), now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Swap("claude-code-new", ac); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("a new session prunes state older than two weeks: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pi-s1.json")); err != nil {
		t.Errorf("recent state stays: %v", err)
	}
}

func TestMemoryIsLocked(t *testing.T) {
	m := Memory{Dir: t.TempDir()}
	var wg sync.WaitGroup
	var mu sync.Mutex
	first := 0
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, seen, err := m.Swap("k", State{Battery: true})
			if err != nil {
				t.Error(err)
			}
			if !seen {
				mu.Lock()
				first++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if first != 1 {
		t.Errorf("exactly one concurrent hook sees the first observation, got %d", first)
	}
}

func TestWait(t *testing.T) {
	root := fake(t, supplies{"AC": mains("0"), "BAT0": bat("Discharging", "50")})
	read := func() State { return Read(root) }
	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(root, "AC", "online"), []byte("1\n"), 0o644)
	}()
	s, err := Wait(context.Background(), read, true, 5*time.Millisecond)
	if err != nil || !s.OnAC {
		t.Errorf("wait ac returns once plugged: %+v %v", s, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := Wait(ctx, read, false, time.Hour); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 5*time.Second {
		t.Errorf("the timeout ends the wait between polls: %v after %s", err, time.Since(start))
	}
	desktop := func() State { return Read(fake(t, supplies{})) }
	if _, err := Wait(context.Background(), desktop, false, time.Hour); !errors.Is(err, ErrNoBattery) {
		t.Errorf("waiting for battery without one fails at once: %v", err)
	}
	if s, err := Wait(context.Background(), desktop, true, time.Hour); err != nil || !s.OnAC {
		t.Errorf("a desktop is on AC: %+v %v", s, err)
	}
}
