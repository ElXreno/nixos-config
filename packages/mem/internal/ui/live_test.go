package ui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type liveReport struct {
	RestMax           float64 `json:"restMax"`
	RestSleeps        bool    `json:"restSleeps"`
	RestExact         bool    `json:"restExact"`
	WanderMax         float64 `json:"wanderMax"`
	WanderAwake       bool    `json:"wanderAwake"`
	WanderAliveShare  float64 `json:"wanderAliveShare"`
	WanderLinkStretch float64 `json:"wanderLinkStretch"`
	AfterWanderRest   float64 `json:"afterWanderRestMax"`
	AfterWanderSleeps bool    `json:"afterWanderSleeps"`
	DragSelf          float64 `json:"dragSelf"`
	DragNear          float64 `json:"dragNear"`
	DragTwo           float64 `json:"dragTwo"`
	DragFar           float64 `json:"dragFar"`
	DragOther         float64 `json:"dragOther"`
	DragPinned        bool    `json:"dragPinned"`
	DragAwake         bool    `json:"dragAwake"`
	ReleaseEarly      float64 `json:"releaseEarly"`
	ReleaseHalfSecond float64 `json:"releaseHalfSecond"`
	ReleaseTwoSeconds float64 `json:"releaseTwoSeconds"`
	ReleaseDone       float64 `json:"releaseDone"`
	ReleaseSleeps     bool    `json:"releaseSleeps"`
	ReleaseExact      bool    `json:"releaseExact"`
	SameFollows       float64 `json:"sameFollows"`
	CrossFollows      float64 `json:"crossFollows"`
	HiddenStays       bool    `json:"hiddenStays"`
	CollisionPushes   bool    `json:"collisionPushes"`
	FitOK             bool    `json:"fitOk"`
	HullInside        int     `json:"hullInside"`
	HullMembers       int     `json:"hullMembers"`
	HullWorstMargin   float64 `json:"hullWorstMargin"`
	HullMaxGap        float64 `json:"hullMaxGap"`
	HullFarSamples    int     `json:"hullFarSamples"`
	HullBoundsContain bool    `json:"hullBoundsContain"`
	HullCenterNear    bool    `json:"hullCenterNearCloud"`
	HullR             float64 `json:"hullR"`
	SoloR             float64 `json:"soloR"`
	HullFollows       bool    `json:"hullFollows"`
	HitFinds          bool    `json:"hitFinds"`
	HitSkipsHidden    bool    `json:"hitSkipsHidden"`
	HitStale          bool    `json:"hitStale"`
	HitFollows        bool    `json:"hitFollows"`
}

func runLive(t *testing.T) liveReport {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	var js []byte
	for _, name := range []string{"gfx/theme.js", "gfx/terrain.js", "gfx/live.js", "gfx/hit.js"} {
		part, err := assetFS.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		js = append(append(js, part...), '\n')
	}
	path := filepath.Join(t.TempDir(), "live-all.js")
	if err := os.WriteFile(path, js, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, "testdata/live.js", path).Output()
	if err != nil {
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("node: %v %s", err, stderr)
	}
	var r liveReport
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("node output %q: %v", out, err)
	}
	return r
}

func TestLiveRestStateEqualsTheServerMap(t *testing.T) {
	r := runLive(t)
	if r.RestMax != 0 || !r.RestExact || !r.RestSleeps {
		t.Errorf("without wander the lattice left home: max %.4f px, exact %v, asleep %v", r.RestMax, r.RestExact, r.RestSleeps)
	}
	if r.WanderMax > 5 || r.WanderMax < 1.5 {
		t.Errorf("wander strays %.2f px from home, want a few px", r.WanderMax)
	}
	if r.WanderAliveShare < .5 || !r.WanderAwake {
		t.Errorf("wander moves %.0f%% of the nodes by more than 1 px, awake %v: the map is not alive", r.WanderAliveShare*100, r.WanderAwake)
	}
	if r.WanderLinkStretch > 4 {
		t.Errorf("wander stretches a link by %.2f px: the field is not coherent", r.WanderLinkStretch)
	}
	if r.AfterWanderRest != 0 || !r.AfterWanderSleeps {
		t.Errorf("after wander stops the map is %.4f px off home, asleep %v", r.AfterWanderRest, r.AfterWanderSleeps)
	}
}

func TestLiveDragPullsNeighboursMoreThanDistantNodes(t *testing.T) {
	r := runLive(t)
	if !r.DragPinned || !r.DragAwake {
		t.Fatalf("pin did not hold: pinned %v awake %v", r.DragPinned, r.DragAwake)
	}
	if r.DragSelf > .01 {
		t.Errorf("the pinned node is %.3f px from the cursor", r.DragSelf)
	}
	if r.DragNear < 8 {
		t.Errorf("a linked neighbour followed a 100 px drag by only %.2f px", r.DragNear)
	}
	if r.DragTwo < 3 || r.DragTwo >= r.DragNear {
		t.Errorf("second hop moved %.2f px, first hop %.2f px: want a visible but weaker pull", r.DragTwo, r.DragNear)
	}
	if r.DragFar > 1 || r.DragOther > 1 || r.DragNear < 10*r.DragFar {
		t.Errorf("distant nodes moved %.3f and %.3f px against %.2f px for the neighbour", r.DragFar, r.DragOther, r.DragNear)
	}
	if r.SameFollows <= r.CrossFollows {
		t.Errorf("a same-topic link pulled %.2f px, a cross-topic link %.2f px: same-topic must be stronger", r.SameFollows, r.CrossFollows)
	}
	if !r.HiddenStays {
		t.Error("a hidden node was pulled by the drag")
	}
	if !r.CollisionPushes {
		t.Error("two overlapping nodes were not pushed apart")
	}
}

func TestLiveReleaseSpringsHome(t *testing.T) {
	r := runLive(t)
	if r.ReleaseEarly < 40 {
		t.Errorf("0.1 s after release the node is only %.1f px from home: it must glide, not jump", r.ReleaseEarly)
	}
	if r.ReleaseHalfSecond > r.ReleaseEarly/3 {
		t.Errorf("0.5 s after release the node is still %.1f px away (%.1f at 0.1 s)", r.ReleaseHalfSecond, r.ReleaseEarly)
	}
	if r.ReleaseTwoSeconds > 1.5 {
		t.Errorf("2 s after release the farthest node is still %.2f px from home", r.ReleaseTwoSeconds)
	}
	if r.ReleaseDone != 0 || !r.ReleaseExact || !r.ReleaseSleeps {
		t.Errorf("the map did not settle on the server layout: %.4f px, exact %v, asleep %v", r.ReleaseDone, r.ReleaseExact, r.ReleaseSleeps)
	}
}

func TestTerritoryHullHugsLiveMembers(t *testing.T) {
	r := runLive(t)
	if !r.FitOK || r.HullInside != r.HullMembers || r.HullWorstMargin < 5 {
		t.Errorf("hull covers %d of %d members, worst margin %.2f px", r.HullInside, r.HullMembers, r.HullWorstMargin)
	}
	if r.HullFarSamples != 0 || r.HullMaxGap > 12+24 {
		t.Errorf("hull leaves %d boundary samples more than 36 px from any node, widest gap %.1f px: an empty blob", r.HullFarSamples, r.HullMaxGap)
	}
	if !r.HullBoundsContain || !r.HullCenterNear {
		t.Errorf("bounds contain members %v, centre follows the cloud %v", r.HullBoundsContain, r.HullCenterNear)
	}
	if r.HullR > 45 {
		t.Errorf("a six node topic gets radius %.1f", r.HullR)
	}
	if r.SoloR < 12 || r.SoloR > 22 {
		t.Errorf("a one node topic gets radius %.1f, want node radius plus padding", r.SoloR)
	}
	if !r.HullFollows {
		t.Error("the hull did not follow members that moved 60 px")
	}
}

func TestHitIndexFollowsRebuilds(t *testing.T) {
	r := runLive(t)
	if !r.HitFinds || !r.HitSkipsHidden || !r.HitStale || !r.HitFollows {
		t.Errorf("hit index: finds %v, skips hidden %v, stale before rebuild %v, follows after %v", r.HitFinds, r.HitSkipsHidden, r.HitStale, r.HitFollows)
	}
}
