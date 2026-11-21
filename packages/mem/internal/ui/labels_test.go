package ui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type labelReport struct {
	Count         int     `json:"count"`
	Names         int     `json:"names"`
	UniqueNames   int     `json:"uniqueNames"`
	EmptyNames    int     `json:"emptyNames"`
	Overlaps      int     `json:"overlaps"`
	Outside       int     `json:"outside"`
	PanStable     bool    `json:"panStable"`
	ZoomStable    bool    `json:"zoomStable"`
	Deterministic bool    `json:"deterministic"`
	L1Shown       int     `json:"l1Shown"`
	L1Outside     int     `json:"l1Outside"`
	L1Overlaps    int     `json:"l1Overlaps"`
	Detached      int     `json:"detached"`
	Poses         int     `json:"poses"`
	PoseFlips     int     `json:"poseFlips"`
	Straddle      int     `json:"straddle"`
	ReducedFirst  bool    `json:"reducedFirstFrame"`
	MsPerFrame    float64 `json:"msPerFrame"`
	KFit          float64 `json:"kFit"`
}

func runLabels(t *testing.T) labelReport {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	var js []byte
	for _, name := range []string{"gfx/theme.js", "gfx/terrain.js", "gfx/labels.js"} {
		part, err := assetFS.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		js = append(append(js, part...), '\n')
	}
	path := filepath.Join(t.TempDir(), "labels-all.js")
	if err := os.WriteFile(path, js, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, "testdata/labels.js", path, "testdata/topics.json").Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("node: %v %s", err, stderr)
	}
	var r labelReport
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("node output %q: %v", out, err)
	}
	return r
}

func TestEveryTopicNameShowsAtOverview(t *testing.T) {
	r := runLabels(t)
	if r.Names != 49 || r.UniqueNames != r.Names || r.EmptyNames != 0 {
		t.Fatalf("short names: %d total, %d unique, %d empty", r.Names, r.UniqueNames, r.EmptyNames)
	}
	if r.Count < 48 {
		t.Fatalf("overview shows %d topic labels, want at least 48", r.Count)
	}
	if r.Overlaps != 0 || r.Outside != 0 {
		t.Fatalf("overview labels overlap %d times and %d leave the canvas", r.Overlaps, r.Outside)
	}
}

func TestLabelSetIsStable(t *testing.T) {
	r := runLabels(t)
	if !r.PanStable {
		t.Error("a 1 px pan changed the label set")
	}
	if !r.ZoomStable {
		t.Error("a 0.4% zoom changed the label set")
	}
	if !r.Deterministic {
		t.Error("two runs placed different labels")
	}
	if !r.ReducedFirst {
		t.Error("reduced motion still fades labels in")
	}
}

func TestLabelSetIsStableAcrossPoses(t *testing.T) {
	r := runLabels(t)
	if r.Poses < 60 || r.PoseFlips != 0 {
		t.Fatalf("%d label-set changes across %d poses under a 1 px pan or 0.4%% zoom", r.PoseFlips, r.Poses)
	}
}

func TestOverviewNamesStayOffNeighbours(t *testing.T) {
	r := runLabels(t)
	if r.Straddle != 0 {
		t.Fatalf("%d overview names sit on a neighbouring territory", r.Straddle)
	}
}

func TestLabelsStayInsideAtDeepZoom(t *testing.T) {
	r := runLabels(t)
	if r.L1Shown == 0 || r.L1Outside != 0 || r.L1Overlaps != 0 {
		t.Fatalf("territory zoom: %d shown, %d outside, %d overlapping", r.L1Shown, r.L1Outside, r.L1Overlaps)
	}
	if r.Detached != 0 {
		t.Fatalf("%d territory names float away from their territory", r.Detached)
	}
}

type nodeLabelReport struct {
	BigNodeLabels    int  `json:"bigOverviewNodeLabels"`
	BigSpill         int  `json:"bigOverviewSpill"`
	TopicLabels      int  `json:"topicLabels"`
	TopicOnLandmark  int  `json:"topicOnLandmark"`
	DupShown         int  `json:"dupShown"`
	DupDistinct      int  `json:"dupDistinct"`
	DupClipped       bool `json:"dupClipped"`
	HubPoses         int  `json:"hubPoses"`
	HubShown         int  `json:"hubShown"`
	HubNoRing        int  `json:"hubNoRing"`
	PanPoses         int  `json:"panPoses"`
	PanFlicker       int  `json:"panFlicker"`
	RevealBelow      bool `json:"revealBelow"`
	RevealShown      bool `json:"revealShown"`
	RevealOpaque     bool `json:"revealOpaque"`
	RevealHysteresis bool `json:"revealHysteresis"`
	RevealGone       bool `json:"revealGone"`
}

func runNodeLabels(t *testing.T) nodeLabelReport {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	var js []byte
	for _, name := range []string{"gfx/theme.js", "gfx/terrain.js", "gfx/labels.js"} {
		part, err := assetFS.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		js = append(append(js, part...), '\n')
	}
	path := filepath.Join(t.TempDir(), "labels-all.js")
	if err := os.WriteFile(path, js, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, "testdata/nodelabels.js", path, "testdata/topics.json").Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("node: %v %s", err, stderr)
	}
	var r nodeLabelReport
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("node output %q: %v", out, err)
	}
	return r
}

func TestKeyLabelsNeverCrossIntoNeighbourTerritories(t *testing.T) {
	r := runNodeLabels(t)
	if r.BigNodeLabels == 0 {
		t.Fatal("a large viewport shows no hub labels at overview, the check is vacuous")
	}
	if r.BigSpill != 0 {
		t.Fatalf("%d hub or key labels sit on a neighbouring territory at overview", r.BigSpill)
	}
}

func TestTopicNamesYieldToLandmarks(t *testing.T) {
	r := runNodeLabels(t)
	if r.TopicLabels < 48 {
		t.Fatalf("%d topic names at overview", r.TopicLabels)
	}
	if r.TopicOnLandmark != 0 {
		t.Fatalf("%d topic names cover a core, danger or hub glyph", r.TopicOnLandmark)
	}
}

func TestClippedKeysStayDistinctInOneNeighbourhood(t *testing.T) {
	r := runNodeLabels(t)
	if r.DupShown != 2 || r.DupDistinct != 2 || !r.DupClipped {
		t.Fatalf("two keys with one 28-char prefix: %d shown, %d distinct texts, clipped=%v", r.DupShown, r.DupDistinct, r.DupClipped)
	}
}

func TestHubLabelsCarryARing(t *testing.T) {
	r := runNodeLabels(t)
	if r.HubShown != r.HubPoses || r.HubNoRing != 0 {
		t.Fatalf("of %d hub poses %d labelled, %d without a ring", r.HubPoses, r.HubShown, r.HubNoRing)
	}
}

func TestKeyLabelsRevealOpaqueNotHalfFaded(t *testing.T) {
	r := runNodeLabels(t)
	if r.RevealBelow || !r.RevealShown || !r.RevealGone {
		t.Fatalf("zoom reveal: below the threshold shown %v, above it shown %v, far below gone %v", r.RevealBelow, r.RevealShown, r.RevealGone)
	}
	if !r.RevealOpaque {
		t.Fatal("a key label parked just above its zoom threshold stays semi-transparent")
	}
	if !r.RevealHysteresis {
		t.Fatal("a key label vanishes the moment the zoom dips under its threshold")
	}
}

func TestSlowContinuousPanKeepsLabelsFromFlickering(t *testing.T) {
	r := runNodeLabels(t)
	if r.PanPoses < 20 || r.PanFlicker > 25 {
		t.Fatalf("%d labels toggled three or more times over %d slow pans", r.PanFlicker, r.PanPoses)
	}
}
