package ui

import (
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

type recallReport struct {
	Hop1                 int       `json:"hop1"`
	Hop2                 int       `json:"hop2"`
	Hop1Times            []float64 `json:"hop1Times"`
	Hop2Times            []float64 `json:"hop2Times"`
	ThirdHop             bool      `json:"thirdHop"`
	HiddenLit            bool      `json:"hiddenLit"`
	SeedInHops           bool      `json:"seedInHops"`
	EdgesReal            bool      `json:"edgesReal"`
	EdgesFromParent      bool      `json:"edgesFromParent"`
	EdgeCount            int       `json:"edgeCount"`
	CometsMid1           int       `json:"cometsMid1"`
	CometsMid2           int       `json:"cometsMid2"`
	UnderAnimatingDuring bool      `json:"underAnimatingDuring"`
	UnderAnimatingAfter  bool      `json:"underAnimatingAfter"`
	BreathMax            float64   `json:"breathMax"`
	BreathEnds           bool      `json:"breathEnds"`
	BreathTouched        bool      `json:"breathTouched"`
	BreathUntouched      bool      `json:"breathUntouched"`
	BreathOnce           bool      `json:"breathOnce"`
	BreathPeak           float64   `json:"breathPeak"`
	BreathLate           float64   `json:"breathLate"`
	PillAtIgnite         bool      `json:"pillAtIgnite"`
	PillLater            bool      `json:"pillLater"`
	PillOther            bool      `json:"pillOther"`
	FireEmpty            bool      `json:"fireEmpty"`
	HubHop1              int       `json:"hubHop1"`
	LoneHops             int       `json:"loneHops"`
	WaveCap              int       `json:"waveCap"`
	WaveCapKeepsNewest   []int     `json:"waveCapKeepsNewest"`
	ReducedHops          int       `json:"reducedHops"`
	ReducedEdges         int       `json:"reducedEdges"`
	ReducedUnder         bool      `json:"reducedUnder"`
	ReducedBreath        bool      `json:"reducedBreath"`
	ReducedTicks         int       `json:"reducedTicks"`
	Ticks                int       `json:"ticks"`
	TickCounts           []int     `json:"tickCounts"`
	TickHit              bool      `json:"tickHit"`
	TickMiss             bool      `json:"tickMiss"`
	TickMergedNodes      int       `json:"tickMergedNodes"`
	TickInside           bool      `json:"tickInside"`
	TickFading           int       `json:"tickFading"`
	TickFaded            int       `json:"tickFaded"`
	ObstacleRespected    bool      `json:"obstacleRespected"`
	ObstacleMoved        bool      `json:"obstacleMoved"`
	ExtraRespected       bool      `json:"extraRespected"`
	ExtraMoved           bool      `json:"extraMoved"`
	Expired              int       `json:"expired"`
	BadNumbers           int       `json:"badNumbers"`
	Draws                int       `json:"draws"`
	XcapHop2             int       `json:"xcapHop2"`
	XcapCross            int       `json:"xcapCross"`
	XcapSame             int       `json:"xcapSame"`
	BigHop1              int       `json:"bigHop1"`
	BigHop2              int       `json:"bigHop2"`
	BigEdges             int       `json:"bigEdges"`
	BreathTopics         int       `json:"breathTopics"`
	BreathKeepsSeed      bool      `json:"breathKeepsSeed"`
	InfoBreathing        int       `json:"infoBreathing"`
	BreathMany           int       `json:"breathMany"`
	InfoBreathingLate    int       `json:"infoBreathingLate"`
	CometAtIgnite        int       `json:"cometAtIgnite"`
	CometJustAfter       int       `json:"cometJustAfter"`
	CometEmberAlive      int       `json:"cometEmberAlive"`
	CometEmberGone       int       `json:"cometEmberGone"`
	LitAlive             int       `json:"litAlive"`
	LitGone              int       `json:"litGone"`
	CometHead            []float64 `json:"cometHead"`
	CometExpect          []float64 `json:"cometExpect"`
	Curve                float64   `json:"curve"`
	EmberMove            []float64 `json:"emberMove"`
	EmberQuad            []float64 `json:"emberQuad"`
	EmberExpect          []float64 `json:"emberExpect"`
	ReverseHead          []float64 `json:"reverseHead"`
	ReverseExpect        []float64 `json:"reverseExpect"`
	ReverseTrail         int       `json:"reverseTrail"`
	TimerCount           int       `json:"timerCount"`
	TimerDelay           float64   `json:"timerDelay"`
	TimerCleared         int       `json:"timerCleared"`
}

func runRecall(t *testing.T) recallReport {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	var js []byte
	for _, name := range []string{"gfx/theme.js", "gfx/recall.js"} {
		part, err := assetFS.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		js = append(append(js, part...), '\n')
	}
	path := filepath.Join(t.TempDir(), "recall-all.js")
	if err := os.WriteFile(path, js, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, "testdata/recall.js", path).Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("node: %v %s", err, stderr)
	}
	var r recallReport
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("node output %q: %v", out, err)
	}
	return r
}

func TestRecallWaveRunsTwoHopsAlongRealLinks(t *testing.T) {
	r := runRecall(t)
	if r.Hop1 != 5 || r.Hop2 != 15 || r.ThirdHop || r.SeedInHops {
		t.Fatalf("hops: %d first, %d second, third lit %v, seed re-lit %v", r.Hop1, r.Hop2, r.ThirdHop, r.SeedInHops)
	}
	if !reflect.DeepEqual(r.Hop1Times, []float64{140}) || !reflect.DeepEqual(r.Hop2Times, []float64{280}) {
		t.Fatalf("arrival times after the seed: %v then %v, want 140 then 280", r.Hop1Times, r.Hop2Times)
	}
	if !r.EdgesReal || !r.EdgesFromParent || r.EdgeCount != r.Hop1+r.Hop2 {
		t.Fatalf("comets: real links %v, leave their parent on time %v, %d edges", r.EdgesReal, r.EdgesFromParent, r.EdgeCount)
	}
	if r.HiddenLit {
		t.Fatal("a hidden record was lit")
	}
	if r.CometsMid1 != 5 || r.CometsMid2 != 20 {
		t.Fatalf("live comets mid-hop: %d then %d", r.CometsMid1, r.CometsMid2)
	}
	if r.LoneHops != 0 {
		t.Fatalf("a record with no links lit %d neighbours", r.LoneHops)
	}
}

func TestRecallFanOutAndWaveCaps(t *testing.T) {
	r := runRecall(t)
	if r.HubHop1 != 40 {
		t.Fatalf("a hub with 100 links sent %d comets, want the fan cap of 40", r.HubHop1)
	}
	if r.WaveCap != 3 || !reflect.DeepEqual(r.WaveCapKeepsNewest, []int{12, 13, 14}) {
		t.Fatalf("five turns left %d waves %v, want the newest three", r.WaveCap, r.WaveCapKeepsNewest)
	}
	if !r.FireEmpty {
		t.Fatal("a turn whose records are all hidden still made a wave")
	}
}

func TestRecallBreathesTouchedTerritoriesOnce(t *testing.T) {
	r := runRecall(t)
	if !r.BreathTouched || !r.BreathUntouched || !r.BreathOnce || !r.BreathEnds || r.BreathMax > 1 {
		t.Fatalf("breath: touched %v, untouched quiet %v, once %v, ends %v, max %v", r.BreathTouched, r.BreathUntouched, r.BreathOnce, r.BreathEnds, r.BreathMax)
	}
	if r.BreathPeak < .95 || r.BreathLate > .12 {
		t.Fatalf("breath peaks at %v and is still %v 1.8 s later", r.BreathPeak, r.BreathLate)
	}
	if !r.PillAtIgnite || !r.PillLater || !r.PillOther {
		t.Fatalf("pill glow: at ignite %v, gone later %v, other turn quiet %v", r.PillAtIgnite, r.PillLater, r.PillOther)
	}
	if !r.UnderAnimatingDuring || r.UnderAnimatingAfter || r.Expired != 0 {
		t.Fatalf("frames: wanted during %v, after %v, waves left %d", r.UnderAnimatingDuring, r.UnderAnimatingAfter, r.Expired)
	}
}

func TestRecallOffscreenTicks(t *testing.T) {
	r := runRecall(t)
	if r.Ticks != 2 || !reflect.DeepEqual(r.TickCounts, []int{1, 2}) || r.TickMergedNodes != 2 {
		t.Fatalf("ticks: %d, counts %v, merged nodes %d", r.Ticks, r.TickCounts, r.TickMergedNodes)
	}
	if !r.TickHit || !r.TickMiss || !r.TickInside {
		t.Fatalf("tick hit-test: hit %v, miss %v, inside the canvas %v", r.TickHit, r.TickMiss, r.TickInside)
	}
	if r.TickFading != 2 || r.TickFaded != 0 {
		t.Fatalf("ticks near the end of their 9 s: %d, after: %d", r.TickFading, r.TickFaded)
	}
	if !r.ObstacleRespected || !r.ObstacleMoved {
		t.Fatalf("a tick sat under a panel: respected %v, moved %v", r.ObstacleRespected, r.ObstacleMoved)
	}
	if !r.ExtraRespected || !r.ExtraMoved {
		t.Fatalf("a tick sat under a label: respected %v, moved %v", r.ExtraRespected, r.ExtraMoved)
	}
	if r.BadNumbers != 0 || r.Draws == 0 {
		t.Fatalf("draw calls: %d, with non-finite arguments %d", r.Draws, r.BadNumbers)
	}
}

func TestRecallReducedMotionDrawsNoWave(t *testing.T) {
	r := runRecall(t)
	if r.ReducedHops != 0 || r.ReducedEdges != 0 || r.ReducedUnder || !r.ReducedBreath {
		t.Fatalf("reduced motion: %d hops, %d comets, under animating %v, breath quiet %v", r.ReducedHops, r.ReducedEdges, r.ReducedUnder, r.ReducedBreath)
	}
	if r.ReducedTicks != 1 {
		t.Fatalf("reduced motion drew %d ticks, want the static marker", r.ReducedTicks)
	}
}

func TestRecallHopCapsAndLocality(t *testing.T) {
	r := runRecall(t)
	if r.BigHop1 != 260 || r.BigHop2 != 420 || r.BigEdges != 680 {
		t.Fatalf("eight hubs lit %d first and %d second hop records over %d comets, want 260, 420 and 680", r.BigHop1, r.BigHop2, r.BigEdges)
	}
	if r.XcapHop2 != 10 || r.XcapCross != 3 || r.XcapSame != 7 {
		t.Fatalf("second hop from one record: %d lit, %d across topics, %d inside, want 10, 3 and 7", r.XcapHop2, r.XcapCross, r.XcapSame)
	}
	if r.InfoBreathing != 4 || r.InfoBreathingLate != 0 || r.BreathMany != 8 {
		t.Fatalf("territories breathing 200 ms in: %d for one seed, %d for six seeds, 5 s in: %d, want 4, 8 and 0", r.InfoBreathing, r.BreathMany, r.InfoBreathingLate)
	}
	if r.BreathTopics != 4 || !r.BreathKeepsSeed {
		t.Fatalf("a wave from one seed across 15 territories breathed %d of them, seed territory kept %v, want 4 and true", r.BreathTopics, r.BreathKeepsSeed)
	}
}

func TestRecallCometTimingAndCurve(t *testing.T) {
	r := runRecall(t)
	if r.CometAtIgnite != 0 || r.CometJustAfter != 1 {
		t.Fatalf("comets at the instant of ignition %d and 1 ms later %d, want 0 and 1", r.CometAtIgnite, r.CometJustAfter)
	}
	if r.CometEmberAlive != 1 || r.CometEmberGone != 0 {
		t.Fatalf("ember 890 ms after landing: %d, 910 ms after: %d, want 1 and 0", r.CometEmberAlive, r.CometEmberGone)
	}
	if r.LitAlive != 1 || r.LitGone != 0 {
		t.Fatalf("glow 1590 ms after landing: %d, 1610 ms after: %d, want 1 and 0", r.LitAlive, r.LitGone)
	}
	if !near(r.CometHead, r.CometExpect) {
		t.Fatalf("comet head at mid hop %v, the edge curve gives %v", r.CometHead, r.CometExpect)
	}
	if !near(r.EmberMove, []float64{100, 100}) || !near(r.EmberQuad, r.EmberExpect) {
		t.Fatalf("ember curve starts at %v and bends through %v, the edge curve gives %v", r.EmberMove, r.EmberQuad, r.EmberExpect)
	}
	if !near(r.ReverseHead, r.ReverseExpect) || r.ReverseTrail == 0 {
		t.Fatalf("comet running against the stored link direction is at %v, want %v, trail segments %d", r.ReverseHead, r.ReverseExpect, r.ReverseTrail)
	}
	if r.Curve != 0.16 {
		t.Fatalf("edge curvature %v, want 0.16", r.Curve)
	}
}

func near(a, b []float64) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-6 {
			return false
		}
	}
	return true
}

func TestRecallExpiryWakesTheLoop(t *testing.T) {
	r := runRecall(t)
	if r.TimerCount != 1 || r.TimerDelay != 10660 || r.TimerCleared != 1 {
		t.Fatalf("expiry timers: %d armed, delay %v, cleared with the wave %d, want 1, 10660 and 1", r.TimerCount, r.TimerDelay, r.TimerCleared)
	}
}

func TestEdgeCurvatureHasOneSource(t *testing.T) {
	gl, err := assetFS.ReadFile("assets/gfx/gl.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gl), "nrm * len * ${G.theme.curve.toFixed(3)}") {
		t.Fatal("the edge shader does not take its curvature from theme.curve")
	}
	literal := regexp.MustCompile(`[-+*] (dx|dy) \* \.\d+`)
	for _, name := range []string{"gfx/recall.js", "gfx/overlay.js"} {
		src, err := assetFS.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if m := literal.Find(src); m != nil {
			t.Fatalf("%s hard-codes a curvature: %q", name, m)
		}
		if !strings.Contains(string(src), "T.curve") {
			t.Fatalf("%s does not use theme.curve", name)
		}
	}
}
