package checks

import (
	"strings"
	"testing"

	"agent-guard/internal/shell"
	"agent-guard/internal/verdict"
)

func TestNoWrite(t *testing.T) {
	cfg := &Settings{Home: "/home/test", DeniedWrites: []DeniedWrite{
		{Path: "/proc/sys/vm/drop_caches", Reason: "tell the owner how much memory is needed and wait."},
		{Path: "/proc/sys/vm/compact_memory", Reason: "tell the owner how much memory is needed and wait."},
	}}
	ctx := Context{Cwd: "/home/test/repo"}
	cases := []struct {
		cmd  string
		deny string
	}{
		{"echo 3 > /proc/sys/vm/drop_caches", "/proc/sys/vm/drop_caches"},
		{"echo 1 >> /proc/sys/vm/compact_memory", "/proc/sys/vm/compact_memory"},
		{"echo 3 >| /proc/sys/vm/drop_caches", "/proc/sys/vm/drop_caches"},
		{"echo 3 &> /proc/sys/vm/drop_caches", "/proc/sys/vm/drop_caches"},
		{"echo 3 > /proc/sys/vm/../vm/drop_caches", "/proc/sys/vm/drop_caches"},
		{"cd /proc/sys/vm && echo 3 > drop_caches", "/proc/sys/vm/drop_caches"},
		{"sync; echo 3 > /proc/sys/vm/drop_caches; echo 1 > /proc/sys/vm/compact_memory", "/proc/sys/vm/drop_caches, /proc/sys/vm/compact_memory"},
		{`ssh root@localhost bash -c "sync; echo 3 > /proc/sys/vm/drop_caches; echo 1 > /proc/sys/vm/compact_memory"`, "/proc/sys/vm/drop_caches, /proc/sys/vm/compact_memory"},
		{"ssh other 'echo 3 > /proc/sys/vm/drop_caches'", "/proc/sys/vm/drop_caches"},
		{"ssh other 'cd /proc/sys/vm && echo 1 > compact_memory'", "/proc/sys/vm/compact_memory"},
		{"ssh -p 2222 -o Foo=bar root@h \"echo 3 > /proc/sys/vm/drop_caches\"", "/proc/sys/vm/drop_caches"},
		{"bash -c 'echo 3 > /proc/sys/vm/drop_caches'", "/proc/sys/vm/drop_caches"},
		{"sh -c \"sudo sh -c 'echo 3 > /proc/sys/vm/drop_caches'\"", "/proc/sys/vm/drop_caches"},
		{"sudo bash -c 'echo 3 > /proc/sys/vm/drop_caches'", "/proc/sys/vm/drop_caches"},
		{"docker exec c sh -c 'echo 3 > /proc/sys/vm/drop_caches'", "/proc/sys/vm/drop_caches"},
		{"cat <<EOF > /proc/sys/vm/drop_caches\n3\nEOF", "/proc/sys/vm/drop_caches"},
		{"bash <<EOF\nsync\necho 3 > /proc/sys/vm/drop_caches\nEOF", "/proc/sys/vm/drop_caches"},
		{"ssh root@h bash <<EOF\necho 1 > /proc/sys/vm/compact_memory\nEOF", "/proc/sys/vm/compact_memory"},
		{"echo 3 | tee /proc/sys/vm/drop_caches", "/proc/sys/vm/drop_caches"},
		{"echo 3 | sudo tee -a /proc/sys/vm/drop_caches >/dev/null", "/proc/sys/vm/drop_caches"},
		{"ssh root@h 'echo 3 | tee /proc/sys/vm/drop_caches'", "/proc/sys/vm/drop_caches"},
		{"sysctl -w vm.drop_caches=3", "/proc/sys/vm/drop_caches"},
		{"sysctl --write vm.drop_caches=3", "/proc/sys/vm/drop_caches"},
		{"sysctl vm.drop_caches=3", "/proc/sys/vm/drop_caches"},
		{"sudo sysctl -w vm.compact_memory=1", "/proc/sys/vm/compact_memory"},
		{"sysctl -w vm/drop_caches=3", "/proc/sys/vm/drop_caches"},
		{"sysctl -w vm.swappiness=10 vm.drop_caches=3", "/proc/sys/vm/drop_caches"},
		{"sysctl -q -w vm.compact_memory=1", "/proc/sys/vm/compact_memory"},
		{"ssh h 'sysctl -w vm.drop_caches=3'", "/proc/sys/vm/drop_caches"},
		{"ssh h sysctl vm.compact_memory=1", "/proc/sys/vm/compact_memory"},
		{"bash -c 'sysctl -w vm.drop_caches=3'", "/proc/sys/vm/drop_caches"},
		{"cat /proc/sys/vm/drop_caches", ""},
		{"rg -n . /proc/sys/vm/compact_memory", ""},
		{"ssh h 'cat /proc/sys/vm/drop_caches'", ""},
		{"echo hi > /tmp/x < /proc/sys/vm/drop_caches", ""},
		{"echo 10 > /proc/sys/vm/swappiness", ""},
		{"ssh h 'echo 10 > /proc/sys/vm/swappiness'", ""},
		{"echo 3 > /proc/sys/vm/drop_caches_x", ""},
		{"echo 1 > drop_caches", ""},
		{"ssh h 'echo 1 > drop_caches'", ""},
		{"sysctl vm.drop_caches", ""},
		{"sysctl -n vm.drop_caches", ""},
		{"sysctl -a", ""},
		{"sysctl -w vm.swappiness=10", ""},
		{"sysctl vm.swappiness=10", ""},
		{"echo vm.drop_caches=3", ""},
		{"grep drop_caches /etc/sysctl.conf", ""},
	}
	for _, c := range cases {
		v := runOneWith(NoWrite, shell.Analyze(c.cmd), ctx, cfg)
		if got := len(v) > 0 && v[0].Level == verdict.Deny; got != (c.deny != "") {
			t.Errorf("%q: want deny %q, got %v", c.cmd, c.deny, v)
			continue
		}
		if c.deny != "" && !strings.Contains(v[0].Message, "writing "+c.deny+", on this host or any other: tell the owner") {
			t.Errorf("%q: message %q", c.cmd, v[0].Message)
		}
	}
	if v := runOneWith(NoWrite, shell.Analyze("echo 3 > /proc/sys/vm/drop_caches"), ctx, &Settings{Home: "/home/test"}); len(v) != 0 {
		t.Errorf("no denied writes, no verdict: %v", v)
	}
}
