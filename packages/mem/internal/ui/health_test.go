package ui

import (
	"path/filepath"
	"testing"
	"time"

	"mem/internal/statefile"
)

func binned(h histogram) int {
	n := 0
	for _, c := range h.Counts {
		n += c
	}
	return n
}

func TestCalibrationSkipsKeysWithoutACosine(t *testing.T) {
	f, dir := stateFixture(t)
	vector := promptTurn("s1", "aaaaaaaaaaaaaaaa", "first prompt", 0)
	vector.Strong = []statefile.Key{{Key: "feedback_alpha", Cos: 0.81}, {Key: "lexical_strong", Cos: 0}}
	vector.Pointers = []statefile.Key{{Key: "reference_beta", Cos: 0.55}, {Key: "fused_novec", Cos: 0}}
	lexical := promptTurn("s1", "bbbbbbbbbbbbbbbb", "second prompt", time.Second)
	lexical.Mode, lexical.Embed = "vector", "timeout"
	lexical.Pointers = []statefile.Key{{Key: "relay-state", Cos: 0}}
	failure := statefile.Turn{Time: at(2 * time.Second), Event: statefile.EventTool, Session: "s1", Tool: "Bash", Status: "inject",
		Pointers: []statefile.Key{{Key: "lexical_failure", Cos: 0}}}
	appendJSON(t, filepath.Join(dir, statefile.TurnsName), vector, lexical, failure)
	appendJSON(t, filepath.Join(dir, statefile.ShadowName), shadowFix{Time: at(time.Second), Session: "s1", Prompt: "aaaaaaaaaaaaaaaa", Pool: "fused-reg", Embed: "ok",
		Keys: []string{"feedback_alpha"}, Hits: []shadowHitFix{{Key: "feedback_alpha", Cos: 0.81, Gate: "pass"}, {Key: "fused_novec", Cos: 0, Gate: "below-cos"}}})
	var h healthOut
	f.json(t, "/api/health", &h)
	c := h.Calibration
	if n := binned(c.StrongCos); n != 1 {
		t.Errorf("strong cosines binned = %d, want 1: %v", n, c.StrongCos.Counts)
	}
	if n := binned(c.PointerCos); n != 1 || c.PointerCos.Counts[0] != 0 {
		t.Errorf("pointer cosines binned = %d, want 1 and none below 0.3: %v", n, c.PointerCos.Counts)
	}
	if n := binned(c.ShadowCos); c.Shadow != 1 || n != 1 {
		t.Errorf("shadow turns %d, shadow cosines binned = %d, want 1 and 1: %v", c.Shadow, n, c.ShadowCos.Counts)
	}
	for _, r := range c.Sweep {
		want := 0
		if r.T <= 0.55 {
			want = 1
		}
		if r.Pointer != want {
			t.Errorf("sweep at %.2f counts %d pointers, want %d", r.T, r.Pointer, want)
		}
	}
}
