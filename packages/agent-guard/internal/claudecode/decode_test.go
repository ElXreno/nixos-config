package claudecode

import (
	"encoding/json"
	"reflect"
	"testing"

	"agent-guard/internal/call"
	"agent-guard/internal/harness"
)

func TestDecode(t *testing.T) {
	cases := []struct {
		tool, input string
		want        call.Call
	}{
		{"Bash", `{"command":"ls -la","description":"x"}`, call.Call{Tool: "Bash", Kind: call.Shell, Command: "ls -la"}},
		{"Edit", `{"file_path":"/r/a.go","old_string":"a","new_string":"b","replace_all":true}`,
			call.Call{Tool: "Edit", Kind: call.Edit, Change: call.Change{Path: "/r/a.go", Pairs: []call.Pair{{Old: "a", New: "b", All: true}}}}},
		{"MultiEdit", `{"file_path":"/r/a.go","edits":[{"old_string":"a","new_string":"b"},{"old_string":"c","new_string":"d","replace_all":true}]}`,
			call.Call{Tool: "MultiEdit", Kind: call.Edit, Change: call.Change{Path: "/r/a.go", Pairs: []call.Pair{{Old: "a", New: "b"}, {Old: "c", New: "d", All: true}}}}},
		{"MultiEdit", `{"file_path":"/r/a.go","edits":[]}`,
			call.Call{Tool: "MultiEdit", Kind: call.Edit, Change: call.Change{Path: "/r/a.go", Pairs: []call.Pair{}}}},
		{"Write", `{"file_path":"/r/a.go","content":""}`,
			call.Call{Tool: "Write", Kind: call.Write, Change: call.Change{Path: "/r/a.go", Write: true}}},
		{"Read", `{"file_path":"/r/a.go","limit":10}`, call.Call{Tool: "Read", Change: call.Change{Path: "/r/a.go"}}},
		{"Grep", `{"pattern":"x","path":"/r"}`, call.Call{Tool: "Grep", Change: call.Change{Path: "/r"}}},
		{"WebFetch", `{"url":"https://x","prompt":"p"}`, call.Call{Tool: "WebFetch", URL: "https://x"}},
		{"mcp__x__y", `{"content":[{"type":"text"}],"command":5}`, call.Call{Tool: "mcp__x__y"}},
		{"Agent", ``, call.Call{Tool: "Agent"}},
		{"Agent", `null`, call.Call{Tool: "Agent"}},
	}
	for _, tc := range cases {
		got, err := DecodeTool(tc.tool, json.RawMessage(tc.input))
		if err != nil {
			t.Errorf("%s %s: %v", tc.tool, tc.input, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s %s:\n got  %+v\n want %+v", tc.tool, tc.input, got, tc.want)
		}
	}
}

func TestDecodeRejects(t *testing.T) {
	for _, tc := range []struct{ tool, input string }{
		{"Bash", `"ls"`},
		{"Bash", `[1]`},
		{"Bash", `{"command":5}`},
		{"Edit", `{"file_path":"/r/a.go","old_string":1}`},
		{"MultiEdit", `{"file_path":"/r/a.go","edits":{}}`},
		{"Read", `"x"`},
	} {
		if _, err := DecodeTool(tc.tool, json.RawMessage(tc.input)); err == nil {
			t.Errorf("%s %s: want an error", tc.tool, tc.input)
		}
	}
}

func TestDecodeCarriesTheEnvelope(t *testing.T) {
	in := Input{SessionID: "s", Cwd: "/r", HookEventName: "PreToolUse", ToolName: "Bash", ToolInput: json.RawMessage(`{"command":"ls"}`), ToolUseID: "t"}
	got, err := Decode(in)
	if err != nil {
		t.Fatal(err)
	}
	want := call.Call{Harness: harness.ClaudeCode, Event: "PreToolUse", Tool: "Bash", Kind: call.Shell, Session: "s", CallID: "t", Cwd: "/r", Command: "ls"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if got.Summary() != "ls" {
		t.Errorf("summary %q", got.Summary())
	}
}
