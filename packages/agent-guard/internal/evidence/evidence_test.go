package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	session    = "The session serves operations only after Ghidra reports default auto-analysis complete."
	addresses  = "Default-space addresses use lowercase 0x-prefixed hexadecimal; other address spaces use <percent-encoded-space>:0x<hex>."
	defect     = "Known provider defect ghidra-9996 (jump-table-flow-references-omitted): see https://github.com/NationalSecurityAgency/ghidra/issues/9996."
	pseudocode = "Pseudocode is Ghidra decompiler output, not original source."
)

var (
	subject = map[string]any{"identity": map[string]any{"kind": "file", "sha256": "3dbe", "format": "elf"}, "name": "hello"}
	profile = map[string]any{"provider": map[string]any{"id": "ghidra"}, "parameters": map[string]any{"architecture": "x86_64"}, "digest": "8f13"}
	rows    = []any{map[string]any{"address": "0x1025a0", "value": "main"}}
)

func record(id, op string, raw, normalized any, limitations ...string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{
		"evidence_id":           id,
		"evidence_format":       2,
		"subject":               subject,
		"provider":              map[string]any{"id": "ghidra", "version": "12.2"},
		"predicate_type":        "rea.analysis",
		"confidence":            "observed",
		"authority":             "shipped-artifact",
		"analysis_profile":      profile,
		"operation":             op,
		"raw_result":            raw,
		"normalized_result":     normalized,
		"environment":           nil,
		"limitations":           limitations,
		"relationship_coverage": nil,
		"locations":             []any{},
		"evidence_links":        []any{},
	})
	return strings.TrimSpace(b.String())
}

func memory(t *testing.T) Memory {
	t.Helper()
	return Memory{Dir: filepath.Join(t.TempDir(), "evidence"), Now: func() time.Time { return time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC) }}
}

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, s)
	}
	return v
}

func compact(t *testing.T, m Memory, key, in string) (string, bool) {
	t.Helper()
	out, ok, err := m.Compact(key, in)
	if err != nil {
		t.Fatal(err)
	}
	return out, ok
}

func TestFirstResultKeepsEveryLimitation(t *testing.T) {
	m := memory(t)
	out, ok := compact(t, m, "cc-s", record("ev_1", "search_procedures", rows, rows, session, addresses, defect))
	if !ok {
		t.Fatal("a raw_result equal to normalized_result and empty fields still shrink the first result")
	}
	v := decode(t, out)
	if got := v["limitations"].([]any); len(got) != 3 {
		t.Errorf("first result keeps all limitations, got %v", got)
	}
	if _, ok := v[shownKey]; ok {
		t.Error("first result has nothing shown earlier")
	}
	if v["subject"].(map[string]any)["name"] != "hello" || v["analysis_profile"].(map[string]any)["digest"] != "8f13" {
		t.Errorf("first result keeps the envelope: %v", out)
	}
	if v["raw_result"] != "same as normalized_result" {
		t.Errorf("raw_result equal to normalized_result is replaced, got %v", v["raw_result"])
	}
	for _, k := range []string{"environment", "relationship_coverage", "locations", "evidence_links"} {
		if _, ok := v[k]; ok {
			t.Errorf("empty %s is dropped", k)
		}
	}
	if !strings.Contains(out, "<percent-encoded-space>") {
		t.Errorf("strings are not re-escaped: %s", out)
	}
}

func TestMemberOrderKept(t *testing.T) {
	out, _ := compact(t, memory(t), "cc-s", `{"z":1,"evidence_id":"ev_9","locations":[],"a":{"y":2,"b":3}}`)
	if want := `{"z":1,"evidence_id":"ev_9","a":{"y":2,"b":3}}`; out != want {
		t.Errorf("got %s, want %s", out, want)
	}
}

func TestRepeatsAreDropped(t *testing.T) {
	m := memory(t)
	compact(t, m, "cc-s", record("ev_1", "search_procedures", rows, rows, session, addresses, defect))
	out, ok := compact(t, m, "cc-s", record("ev_2", "procedure_pseudo_code", nil, "void main(void) {}", session, pseudocode, addresses, defect))
	if !ok {
		t.Fatal("second result is rewritten")
	}
	v := decode(t, out)
	if got := v["limitations"].([]any); len(got) != 1 || got[0] != pseudocode {
		t.Errorf("only the new limitation stays, got %v", got)
	}
	if v[shownKey] != float64(3) {
		t.Errorf("%s counts the dropped ones, got %v", shownKey, v[shownKey])
	}
	sameAs, _ := json.Marshal(v[sameAsKey])
	if want := `{"evidence_id":"ev_1","fields":["analysis_profile","authority","confidence","evidence_format","predicate_type","provider","subject"]}`; string(sameAs) != want {
		t.Errorf("repeated envelope collapses into one pointer to the first record, got %s", sameAs)
	}
	for k := range envelopeGroup {
		if _, ok := v[k]; ok {
			t.Errorf("collapsed %s is gone", k)
		}
	}
	if v["raw_result"] != nil {
		t.Errorf("a null raw_result stays null, got %v", v["raw_result"])
	}
	if v["normalized_result"] != "void main(void) {}" {
		t.Errorf("result itself is untouched, got %v", v["normalized_result"])
	}
}

func with(t *testing.T, rec, key string, val any) string {
	t.Helper()
	v := decode(t, rec)
	v[key] = val
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(b.String())
}

func TestChangedEnvelopeKeepsWhatDiffers(t *testing.T) {
	m := memory(t)
	compact(t, m, "cc-s", record("ev_1", "search_procedures", rows, rows, session))
	out, _ := compact(t, m, "cc-s", with(t, record("ev_2", "xrefs", rows, rows, session), "confidence", "inferred"))
	v := decode(t, out)
	if _, ok := v[sameAsKey]; ok {
		t.Errorf("an envelope with a changed field is not collapsed whole: %s", out)
	}
	if v["confidence"] != "inferred" || v["provider"] == nil {
		t.Errorf("small envelope fields stay as they are: %s", out)
	}
	if v["subject"] != "same as in ev_1" || v["analysis_profile"] != "same as in ev_1" {
		t.Errorf("large repeated parts still point at the first record: %v %v", v["subject"], v["analysis_profile"])
	}
}

func TestNearDuplicatesMerge(t *testing.T) {
	exit := func(at string) string {
		return "Ghidra Listing reports a terminal call at " + at + " to 0x1022e0 <EXTERNAL>::exit (hasNoReturn=true)."
	}
	both := func(at, to string) string { return "Call at " + at + " jumps to " + to + "." }
	out, _ := compact(t, memory(t), "cc-s", record("ev_1", "procedure_pseudo_code", nil, "x", pseudocode,
		exit("0x1026fa"), both("0x1", "0x2"), exit("0x1027a1"), both("0x3", "0x4"), exit("0x1028d3")))
	got := decode(t, out)["limitations"].([]any)
	want := []any{
		pseudocode,
		"Ghidra Listing reports a terminal call at [0x1026fa, 0x1027a1, 0x1028d3] to 0x1022e0 <EXTERNAL>::exit (hasNoReturn=true).",
		both("0x1", "0x2"),
		both("0x3", "0x4"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("limitation %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMultilineResultDetached(t *testing.T) {
	code := "void main(void)\n{\n  puts(\"hi \\\\ <there>\");\n}\n"
	out, ok := compact(t, memory(t), "cc-s", record("ev_1", "procedure_pseudo_code", nil, code, pseudocode))
	if !ok {
		t.Fatal("a multi-line result is always rewritten")
	}
	head, tail, found := strings.Cut(out, "\n\n"+detachedHeader+"\n")
	if !found || tail != code {
		t.Fatalf("the text follows the JSON verbatim, got %q", out)
	}
	if v := decode(t, head); v["normalized_result"] != detachedNote || v["evidence_id"] != "ev_1" {
		t.Errorf("the JSON part points at the text: %s", head)
	}

	single, _ := compact(t, memory(t), "cc-s", record("ev_2", "search_procedures", rows, "one line", pseudocode))
	if strings.Contains(single, detachedHeader) {
		t.Errorf("a one-line result stays inside the JSON: %s", single)
	}
	nested := `{"records":[` + record("ev_3", "procedure_pseudo_code", nil, code) + `]}`
	if out, _ := compact(t, memory(t), "cc-s", nested); strings.Contains(out, detachedHeader) {
		t.Errorf("only a top-level record is detached: %s", out)
	}
}

func TestEveryLimitationRepeated(t *testing.T) {
	m := memory(t)
	compact(t, m, "cc-s", record("ev_1", "search_procedures", rows, rows, session, addresses))
	out, _ := compact(t, m, "cc-s", record("ev_2", "search_procedures", rows, rows, session, addresses))
	v := decode(t, out)
	if _, ok := v["limitations"]; ok {
		t.Errorf("an emptied limitations list is dropped: %s", out)
	}
	if v[shownKey] != float64(2) {
		t.Errorf("%s = 2, got %v", shownKey, v[shownKey])
	}
}

func TestDifferentRawResultStays(t *testing.T) {
	m := memory(t)
	candidates := append([]any{map[string]any{"address": "0x10e010", "value": "__libc_start_main"}}, rows...)
	out, _ := compact(t, m, "cc-s", record("ev_1", "search_procedures", candidates, rows, session))
	v := decode(t, out)
	if got, ok := v["raw_result"].([]any); !ok || len(got) != 2 {
		t.Errorf("a raw_result that differs stays whole, got %v", v["raw_result"])
	}
}

func TestRepeatsWithinOneResult(t *testing.T) {
	m := memory(t)
	b, _ := json.Marshal(map[string]any{"records": []json.RawMessage{
		json.RawMessage(record("ev_1", "xrefs", rows, rows, session, defect)),
		json.RawMessage(record("ev_2", "xrefs", rows, rows, session, defect)),
	}})
	out, _ := compact(t, m, "cc-s", string(b))
	var v struct {
		Records []map[string]any `json:"records"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	if got := v.Records[0]["limitations"].([]any); len(got) != 2 {
		t.Errorf("first nested record keeps its limitations, got %v", got)
	}
	if v.Records[1][shownKey] != float64(2) || v.Records[1][sameAsKey].(map[string]any)["evidence_id"] != "ev_1" {
		t.Errorf("second nested record refers to the first: %v", v.Records[1])
	}
}

func TestAgentsAndForget(t *testing.T) {
	m := memory(t)
	in := record("ev_1", "search_procedures", rows, rows, session, defect)
	compact(t, m, "claude-code-s1", in)
	compact(t, m, "claude-code-s1-a1", in)
	other := "claude-code-s10"
	compact(t, m, other, in)

	out, _ := compact(t, m, "claude-code-s1-a2", record("ev_2", "xrefs", rows, rows, session, defect))
	if _, ok := decode(t, out)[shownKey]; ok {
		t.Error("a subagent that saw nothing gets every limitation")
	}

	if err := m.Forget("claude-code-s1"); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"claude-code-s1", "claude-code-s1-a1"} {
		out, _ := compact(t, m, k, record("ev_3", "xrefs", rows, rows, session, defect))
		if _, ok := decode(t, out)[shownKey]; ok {
			t.Errorf("%s forgot what it was shown", k)
		}
	}
	out, _ = compact(t, m, other, record("ev_4", "xrefs", rows, rows, session, defect))
	if decode(t, out)[shownKey] != float64(2) {
		t.Error("another session whose id shares the prefix keeps its memory")
	}
}

func TestLargeResultTeachesNothing(t *testing.T) {
	m := memory(t)
	big := strings.Repeat("x", recordLimit)
	compact(t, m, "cc-s", record("ev_1", "procedure_pseudo_code", nil, big, session, defect))
	out, _ := compact(t, m, "cc-s", record("ev_2", "xrefs", rows, rows, session, defect))
	if got, ok := decode(t, out)["limitations"].([]any); !ok || len(got) != 2 {
		t.Errorf("a result over %d bytes may be cut by the harness, so its limitations count as unseen, got %v", recordLimit, got)
	}
}

func TestOtherOutputsPassThrough(t *testing.T) {
	m := memory(t)
	for _, in := range []string{
		`{"result":null}`,
		"plain text",
		`{"limitations":[1,2]}`,
		`{"evidence_id":"ev_1","limitations":[]}`,
		`{"broken"`,
	} {
		out, ok := compact(t, m, "cc-s", in)
		if ok || out != in {
			t.Errorf("%q passes through unchanged, got %q", in, out)
		}
	}
	if _, err := os.Stat(filepath.Join(m.Dir, "cc-s.json")); err != nil {
		t.Errorf("JSON outputs still create the state file: %v", err)
	}
}

func TestMatches(t *testing.T) {
	patterns := []string{"mcp__plugin_hm_rea__*"}
	if !Matches(patterns, "mcp__plugin_hm_rea__search_procedures") {
		t.Error("rea tool matches")
	}
	if Matches(patterns, "mcp__plugin_hm_ghidra-mcp__decompile") || Matches(nil, "mcp__plugin_hm_rea__xrefs") {
		t.Error("other tools and an empty list do not match")
	}
	if ValidPattern("[") == nil || ValidPattern("") == nil {
		t.Error("malformed and empty patterns are rejected")
	}
}
