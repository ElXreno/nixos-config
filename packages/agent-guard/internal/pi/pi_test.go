package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"agent-guard/internal/call"
)

func TestResolve(t *testing.T) {
	cases := []struct{ path, want string }{
		{"notes.md", "/w/repo/notes.md"},
		{"@notes.md", "/w/repo/notes.md"},
		{"@/etc/x", "/etc/x"},
		{"@~/x", "/home/u/x"},
		{"~", "/home/u"},
		{"~/a/../b", "/home/u/b"},
		{"a\u00A0b\u202Fc", "/w/repo/a b c"},
		{"file:///tmp/a%20b", "/tmp/a b"},
		{"file://localhost/tmp/x", "/tmp/x"},
		{"file://host/tmp/x", ""},
		{"../x", "/w/x"},
		{"/a/./b//c", "/a/b/c"},
		{"@@x", "/w/repo/@x"},
	}
	for _, c := range cases {
		if got := Resolve(c.path, "/w/repo", "/home/u"); got != c.want {
			t.Errorf("Resolve(%q): want %q got %q", c.path, c.want, got)
		}
	}
	if got := Resolve("x", "~/repo", "/home/u"); got != "/home/u/repo/x" {
		t.Errorf("a ~ cwd expands too, got %q", got)
	}
}

func payload(tool, input string) Payload {
	return Payload{Event: ToolCall, ToolName: tool, ToolCallID: "c1", Input: json.RawMessage(input), Cwd: "/w/repo", SessionID: "s"}
}

func TestDecode(t *testing.T) {
	cases := []struct {
		tool, input string
		guarded     bool
		want        call.Call
	}{
		{"bash", `{"command":"ls","timeout":5}`, true, call.Call{Kind: call.Shell, Command: "ls", Cwd: "/w/repo"}},
		{"process", `{"action":"start","name":"n","command":"make","cwd":"sub"}`, true, call.Call{Kind: call.Shell, Command: "make", Cwd: "/w/repo/sub"}},
		{"process", `{"action":"start","command":"make","cwd":"/abs"}`, true, call.Call{Kind: call.Shell, Command: "make", Cwd: "/abs"}},
		{"process", `{"action":"stop","id":"p1"}`, false, call.Call{Cwd: "/w/repo"}},
		{"edit", `{"path":"@a.go","edits":[{"oldText":"a","newText":"b"},{"oldText":"c","newText":"d"}]}`, true,
			call.Call{Kind: call.Edit, Cwd: "/w/repo", Change: call.Change{Path: "/w/repo/a.go", Apply: call.Original, Pairs: []call.Pair{{Old: "a", New: "b"}, {Old: "c", New: "d"}}}}},
		{"edit", `{"path":"a.go","oldText":"x","newText":""}`, true,
			call.Call{Kind: call.Edit, Cwd: "/w/repo", Change: call.Change{Path: "/w/repo/a.go", Apply: call.Original, Pairs: []call.Pair{{Old: "x", New: ""}}}}},
		{"write", `{"path":"~/n.md","content":""}`, true,
			call.Call{Kind: call.Write, Cwd: "/w/repo", Change: call.Change{Path: "/home/u/n.md", Write: true}}},
		{"read", `{"path":"x"}`, false, call.Call{Cwd: "/w/repo"}},
	}
	for _, c := range cases {
		got, guarded, err := Decode(payload(c.tool, c.input), "/home/u")
		if err != nil {
			t.Errorf("%s %s: %v", c.tool, c.input, err)
			continue
		}
		want := c.want
		want.Harness, want.Event, want.Tool, want.Session, want.CallID = "pi", ToolCall, c.tool, "s", "c1"
		if guarded != c.guarded || !reflect.DeepEqual(got, want) {
			t.Errorf("%s %s:\n got  %v %+v\n want %v %+v", c.tool, c.input, guarded, got, c.guarded, want)
		}
	}
	for _, bad := range []struct{ tool, input string }{{"bash", `"ls"`}, {"bash", `{"command":1}`}, {"edit", `{"edits":{}}`}} {
		if _, _, err := Decode(payload(bad.tool, bad.input), "/home/u"); err == nil {
			t.Errorf("%s %s: want an error", bad.tool, bad.input)
		}
	}
}

func TestExtensionToolsMatchTheCodec(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "pi", "agent-guard.ts"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`const TOOLS = new Set\(\[([^\]]*)\]\)`).FindSubmatch(src)
	if m == nil {
		t.Fatal("TOOLS not found in the extension")
	}
	var got []string
	for _, q := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(m[1], -1) {
		got = append(got, string(q[1]))
	}
	if !reflect.DeepEqual(got, Tools) {
		t.Errorf("the extension forwards %q, the codec decodes %q", got, Tools)
	}
}
