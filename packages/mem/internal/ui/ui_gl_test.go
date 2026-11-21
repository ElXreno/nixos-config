package ui

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const glProbeHTML = `<!doctype html><canvas id=c></canvas><script src=/probe.js></script>`

const glProbeJS = `const out = {};
const g = document.getElementById('c').getContext('webgl2');
out.gl = !!g;
if (g) {
  const s = g.createShader(g.VERTEX_SHADER);
  g.shaderSource(s, '#version 300 es\nvoid main(){gl_Position=vec4(0);}');
  g.compileShader(s);
  out.compile = g.getShaderParameter(s, g.COMPILE_STATUS);
}
const publish = () => { document.title = JSON.stringify(out); };
try {
  const w = new Worker('/worker.js');
  w.onmessage = e => { out.worker = e.data; publish(); };
  w.onerror = () => { out.worker = 'err'; publish(); };
} catch (e) { out.worker = 'throw'; publish(); }
try {
  const b = new Worker(URL.createObjectURL(new Blob(['postMessage(1)'])));
  b.onerror = () => { out.blob = 'blocked'; publish(); };
  b.onmessage = () => { out.blob = 'allowed'; publish(); };
} catch (e) { out.blob = 'throw'; publish(); }
`

type browser struct {
	t       *testing.T
	session string
	bin     string
}

func newBrowser(t *testing.T, label string) *browser {
	t.Helper()
	bin, err := exec.LookPath("agent-browser")
	if err != nil {
		t.Skip("agent-browser is not installed")
	}
	b := &browser{t: t, bin: bin, session: fmt.Sprintf("mem-gl-%s-%d", label, os.Getpid())}
	t.Cleanup(b.close)
	return b
}

func (b *browser) close() { _ = exec.Command(b.bin, "--session", b.session, "close").Run() }

func (b *browser) run(args ...string) string {
	b.t.Helper()
	cmd := exec.Command(b.bin, append([]string{"--session", b.session}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		b.t.Fatalf("agent-browser %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (b *browser) eval(js string) string {
	b.t.Helper()
	out := b.run("eval", js)
	lines := strings.Split(out, "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func (b *browser) evalJSON(js string, v any) {
	b.t.Helper()
	raw := b.eval(js)
	if s, err := strconv.Unquote(raw); err == nil {
		raw = s
	}
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		b.t.Fatalf("eval result %q: %v", raw, err)
	}
}

func (b *browser) waitTrue(js string, within time.Duration) {
	b.t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if b.eval(js) == "true" {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	b.t.Fatalf("condition never held: %s", js)
}

func TestGLUnderTheRealCSP(t *testing.T) {
	b := newBrowser(t, "csp")
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(glProbeHTML))
	})
	mux.HandleFunc("/probe.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = w.Write([]byte(glProbeJS))
	})
	mux.HandleFunc("/worker.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = w.Write([]byte(`postMessage('worker-ok')`))
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", CSP)
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(func() { b.close(); srv.Close() })
	b.run("open", srv.URL+"/")
	b.waitTrue(`document.title.includes('"worker"') && document.title.includes('"blob"')`, 15*time.Second)
	var got struct {
		GL      bool   `json:"gl"`
		Compile bool   `json:"compile"`
		Blob    string `json:"blob"`
		Worker  string `json:"worker"`
	}
	b.evalJSON(`document.title`, &got)
	if !got.GL || !got.Compile {
		t.Fatalf("webgl2 under the CSP: %+v", got)
	}
	if got.Blob != "blocked" || got.Worker != "worker-ok" {
		t.Fatalf("worker policy under the CSP: %+v", got)
	}
}

func TestGLRendererUnderTheRealHeader(t *testing.T) {
	b := newBrowser(t, "scene")
	f := newFixture(t)
	srv := httptest.NewServer(f.h)
	t.Cleanup(func() { b.close(); srv.Close() })
	b.run("open", srv.URL+"/")
	b.run("set", "viewport", "1600", "900")
	b.waitTrue(`!!(window.__gfx && window.__gfx.stats().frames > 0)`, 20*time.Second)
	var st struct {
		Backend string `json:"backend"`
		Nodes   int    `json:"nodes"`
		Edges   int    `json:"edges"`
		Frames  int    `json:"frames"`
	}
	b.evalJSON(`JSON.stringify(window.__gfx.stats())`, &st)
	if st.Backend != "webgl2" || st.Nodes != 9 || st.Edges == 0 {
		t.Fatalf("renderer = %+v", st)
	}
	var rect struct{ X, Y, W, H float64 }
	b.evalJSON(`JSON.stringify((r => ({X: r.left, Y: r.top, W: r.width, H: r.height}))(document.getElementById('ov').getBoundingClientRect()))`, &rect)
	shot := filepath.Join(t.TempDir(), "scene.png")
	b.run("screenshot", shot)
	f2, err := os.Open(shot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f2.Close() }()
	img, err := png.Decode(f2)
	if err != nil {
		t.Fatal(err)
	}
	if lit := litPixels(img, rect.X, rect.Y, rect.W, rect.H); lit < 40 {
		t.Fatalf("the canvas is blank: %d pixels differ from the background", lit)
	}
	b.run("wait", "1500")
	b.expectAnimation(false, "overview")
	b.eval(`(() => { setTab('feed'); return 1; })()`)
	b.run("wait", "400")
	if r := b.idle(1500); r.Raf != 0 || r.Frames != 0 {
		t.Fatalf("the graph kept animating behind another tab: %d animation frames requested, %d drawn", r.Raf, r.Frames)
	}
	b.eval(`(() => { setTab('graph'); return 1; })()`)
	b.run("wait", "400")
	b.expectAnimation(false, "graph tab shown again")
}

func litPixels(img image.Image, x, y, w, h float64) int {
	bg := color.RGBA{0x15, 0x15, 0x14, 0xff}
	b := img.Bounds()
	n := 0
	for py := int(y); py < int(y+h) && py < b.Max.Y; py++ {
		for px := int(x); px < int(x+w) && px < b.Max.X; px++ {
			r, g, bl, _ := img.At(px, py).RGBA()
			dr, dg, db := int(r>>8)-int(bg.R), int(g>>8)-int(bg.G), int(bl>>8)-int(bg.B)
			if dr*dr+dg*dg+db*db > 24*24*3 {
				n++
			}
		}
	}
	return n
}

func TestTerritoriesNameThemselvesAndSettle(t *testing.T) {
	b := newBrowser(t, "terr")
	f := newFixture(t)
	srv := httptest.NewServer(f.h)
	t.Cleanup(func() { b.close(); srv.Close() })
	b.run("open", srv.URL+"/#cam=0,0,1.5")
	b.run("set", "viewport", "1600", "900")
	b.waitTrue(`!!(window.__gfx && window.__gfx.stats().frames > 0)`, 20*time.Second)
	b.run("wait", "1500")
	var got struct {
		Backend string  `json:"backend"`
		Topics  int     `json:"topics"`
		Named   int     `json:"named"`
		Labels  int     `json:"labels"`
		K       float64 `json:"k"`
		Pan     bool    `json:"pan"`
	}
	b.evalJSON(`JSON.stringify({backend: __gfx.stats().backend, topics: __gfx.stats().topics, named: TOPICS.filter(t => t.members.length).length, labels: __gfx.labelInfo().count, k: cam.k})`, &got)
	if got.Backend != "webgl2" || got.Topics == 0 || got.Named == 0 {
		t.Fatalf("territories: %+v", got)
	}
	if got.K != 1.5 {
		t.Fatalf("#cam= did not set the camera: k = %v", got.K)
	}
	b.run("eval", "location.hash = '#cam=10,20,.4'")
	b.waitTrue(`cam.k === .4`, 5*time.Second)
	b.run("wait", "1500")
	b.evalJSON(`JSON.stringify({labels: __gfx.labelInfo().count, named: TOPICS.filter(t => t.members.length).length, topics: 0, backend: '', k: 0})`, &got)
	t.Logf("overview names %d of %d territories", got.Labels, got.Named)
	if got.Labels < got.Named {
		t.Fatalf("overview names %d of %d territories", got.Labels, got.Named)
	}
	var seen struct{ Min, Max, Frames int }
	b.evalJSON(`(async () => {
		const f0 = window.__gfx.stats().frames;
		let lo = 1e9, hi = 0;
		for (let i = 0; i < 20; i++) {
			const c = window.__gfx.labelInfo().count;
			lo = Math.min(lo, c);
			hi = Math.max(hi, c);
			await new Promise(r => setTimeout(r, 100));
		}
		return JSON.stringify({Min: lo, Max: hi, Frames: window.__gfx.stats().frames - f0});
	})()`, &seen)
	if seen.Frames < 20 {
		t.Fatalf("the overview drew %d frames in 2 s: the map is not alive", seen.Frames)
	}
	if seen.Min < got.Named {
		t.Fatalf("territory names flickered while the map moved: %d..%d of %d", seen.Min, seen.Max, got.Named)
	}
}

func TestResetAndResizeRefitTheMap(t *testing.T) {
	b := newBrowser(t, "refit")
	f := newFixture(t)
	srv := httptest.NewServer(f.h)
	t.Cleanup(func() { b.close(); srv.Close() })
	b.run("set", "viewport", "1600", "900")
	b.run("open", srv.URL+"/")
	b.waitTrue(`!!(window.__gfx && window.__gfx.stats().frames > 0)`, 20*time.Second)
	b.run("wait", "800")
	var fit struct {
		K      float64 `json:"k"`
		Labels int     `json:"labels"`
		Named  int     `json:"named"`
	}
	probe := `JSON.stringify({k: cam.k, labels: __gfx.labelInfo().count, named: TOPICS.filter(t => t.members.length).length})`
	b.evalJSON(probe, &fit)
	k0 := fit.K
	b.run("eval", "location.hash = '#cam=0,0,2.5'")
	b.waitTrue(`cam.k === 2.5`, 5*time.Second)
	b.run("eval", "location.hash = ''")
	b.run("wait", "800")
	b.evalJSON(probe, &fit)
	if math.Abs(fit.K/k0-1) > .03 {
		t.Fatalf("an empty hash left the camera at k=%v, the overview is k=%v", fit.K, k0)
	}
	for _, size := range [][2]string{{"1300", "800"}, {"700", "700"}} {
		b.run("set", "viewport", size[0], size[1])
		b.run("wait", "1200")
		b.evalJSON(probe, &fit)
		if fit.Labels < fit.Named {
			t.Fatalf("after a resize to %sx%s the map shows %d names of %d territories", size[0], size[1], fit.Labels, fit.Named)
		}
	}
}

func orangePixels(img image.Image, x, y, w, h float64) int {
	b := img.Bounds()
	n := 0
	for py := int(y); py < int(y+h) && py < b.Max.Y; py++ {
		for px := int(x); px < int(x+w) && px < b.Max.X; px++ {
			r, g, bl, _ := img.At(px, py).RGBA()
			if r>>8 > 190 && g>>8 > 80 && g>>8 < 190 && bl>>8 < 130 {
				n++
			}
		}
	}
	return n
}

func (b *browser) shotImage() image.Image {
	b.t.Helper()
	path := filepath.Join(b.t.TempDir(), "shot.png")
	b.run("screenshot", path)
	f, err := os.Open(path)
	if err != nil {
		b.t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		b.t.Fatal(err)
	}
	return img
}

const fireWave = `(() => {
	const seed = byDeg[0];
	__gfx.pin(null);
	makeTurn({event: 'tool', id: 'wave-' + Date.now(), session: 'abcd1234-0000', tool: 'Edit', time: new Date().toISOString(), keys: [seed.key]}, true, 0);
	return JSON.stringify(__gfx.recallInfo());
})()`

func TestRecallWavefrontOnTheRealRenderer(t *testing.T) {
	b := newBrowser(t, "wave")
	f := newFixture(t)
	srv := httptest.NewServer(f.h)
	t.Cleanup(func() { b.close(); srv.Close() })
	b.run("set", "viewport", "1600", "900")
	b.run("open", srv.URL+"/")
	b.waitTrue(`!!(window.__gfx && window.__gfx.stats().frames > 0)`, 20*time.Second)
	b.run("wait", "1200")
	var rect struct{ X, Y, W, H float64 }
	b.evalJSON(`JSON.stringify((r => ({X: r.left, Y: r.top, W: r.width, H: r.height}))(document.getElementById('ov').getBoundingClientRect()))`, &rect)
	before := b.shotImage()
	base := orangePixels(before, rect.X, rect.Y, rect.W, rect.H)
	var info struct {
		Seeds []int   `json:"seeds"`
		Hop1  []int   `json:"hop1"`
		T0    float64 `json:"t0"`
	}
	b.evalJSON(fireWave, &info)
	if len(info.Seeds) != 1 || info.Hop1[0] == 0 {
		t.Fatalf("the busiest record lit %v seeds and %v neighbours", info.Seeds, info.Hop1)
	}
	b.run("eval", `__gfx.pin(__gfx.recallInfo().t0 + 160)`)
	b.run("wait", "300")
	var mid struct{ Comets, Lit int }
	b.evalJSON(`JSON.stringify(__gfx.recallInfo())`, &mid)
	if mid.Comets == 0 {
		t.Fatalf("no comet in flight 160 ms after ignition: %+v", mid)
	}
	img := b.shotImage()
	if got := orangePixels(img, rect.X, rect.Y, rect.W, rect.H); got < base+25 {
		t.Fatalf("the wavefront left %d orange pixels over a baseline of %d", got, base)
	}
	b.run("eval", `__gfx.pin(null)`)
	b.waitTrue(`__gfx.recallInfo().waves === 0 && arcs.length === 0`, 40*time.Second)
	b.run("wait", "300")
	var idle struct{ Waves, Arcs int }
	b.evalJSON(`JSON.stringify({Waves: __gfx.recallInfo().waves, Arcs: arcs.length})`, &idle)
	if idle.Waves != 0 || idle.Arcs != 0 {
		t.Fatalf("after the window the recall still held state: %+v", idle)
	}
	b.expectAnimation(false, "after the recall window")
}

func TestRecallReducedMotionOnTheRealRenderer(t *testing.T) {
	b := newBrowser(t, "wave-reduced")
	f := newFixture(t)
	srv := httptest.NewServer(f.h)
	t.Cleanup(func() { b.close(); srv.Close() })
	b.run("set", "viewport", "1600", "900")
	b.run("set", "media", "dark", "reduced-motion")
	b.run("open", srv.URL+"/")
	b.waitTrue(`!!(window.__gfx && window.__gfx.stats().frames > 0) && REDUCED === true`, 20*time.Second)
	var info struct {
		Seeds []int `json:"seeds"`
		Hop1  []int `json:"hop1"`
		Hop2  []int `json:"hop2"`
	}
	b.evalJSON(fireWave, &info)
	if len(info.Seeds) != 1 || info.Hop1[0] != 0 || info.Hop2[0] != 0 {
		t.Fatalf("reduced motion lit a wave: %+v", info)
	}
	b.run("wait", "500")
	var live struct{ Comets, Lit int }
	b.evalJSON(`JSON.stringify(__gfx.recallInfo())`, &live)
	if live.Comets != 0 || live.Lit != 0 {
		t.Fatalf("reduced motion drew %+v", live)
	}
}

func TestNavigationInTheRealBrowser(t *testing.T) {
	b := newBrowser(t, "nav")
	f := newFixture(t)
	srv := httptest.NewServer(f.h)
	t.Cleanup(func() { b.close(); srv.Close() })
	b.run("set", "viewport", "1600", "900")
	b.run("open", srv.URL+"/")
	ready := `(() => { try { return booted && NET.mode() === 'sse' && !!window.__nav; } catch (e) { return false; } })()`
	b.waitTrue(ready, 20*time.Second)
	b.eval(`(() => { focusTopic(TOPICS.findIndex(t => t.id === 'kernel')); return 1; })()`)
	b.waitTrue(`!cam.anim && S.topicFocus !== null`, 10*time.Second)
	b.eval(`(() => { select(BYKEY.get('feedback_alpha'), { reveal: true }); return 1; })()`)
	b.waitTrue(`!cam.anim && location.hash.includes('cam=') && location.hash.includes('n=feedback_alpha')`, 10*time.Second)
	var before struct {
		Hash string
		Pose [3]float64
		LH   string
	}
	b.evalJSON(`JSON.stringify({Hash: location.hash, Pose: [__nav.pose().x, __nav.pose().y, __nav.pose().k], LH: __nav.layoutHash()})`, &before)
	if !strings.Contains(before.Hash, "t=kernel") || !strings.Contains(before.Hash, "l="+before.LH) {
		t.Fatalf("hash %q lacks the topic or the layout hash %s", before.Hash, before.LH)
	}
	b.eval(`(() => { history.back(); return 1; })()`)
	b.waitTrue(`S.selected === null && S.topicFocus !== null && TOPICS[S.topicFocus].id === 'kernel'`, 10*time.Second)
	b.eval(`(() => { history.forward(); return 1; })()`)
	b.waitTrue(`S.selected !== null && S.selected.key === 'feedback_alpha'`, 10*time.Second)
	b.waitTrue(`!cam.anim`, 10*time.Second)
	b.eval(`(() => { location.reload(); return 1; })()`)
	b.run("wait", "800")
	b.waitTrue(ready, 20*time.Second)
	b.waitTrue(`S.selected !== null && S.selected.key === 'feedback_alpha' && !cam.anim`, 10*time.Second)
	var after struct {
		Pose  [3]float64
		LH    string
		Topic string
	}
	b.evalJSON(`JSON.stringify({Pose: [__nav.pose().x, __nav.pose().y, __nav.pose().k], LH: __nav.layoutHash(), Topic: S.topicFocus === null ? '' : TOPICS[S.topicFocus].id})`, &after)
	if after.LH != before.LH || after.Topic != "kernel" {
		t.Fatalf("reload changed layout hash %s -> %s or topic %q", before.LH, after.LH, after.Topic)
	}
	if math.Abs(after.Pose[0]-before.Pose[0]) > .2 || math.Abs(after.Pose[1]-before.Pose[1]) > .2 || math.Abs(after.Pose[2]/before.Pose[2]-1) > .002 {
		t.Fatalf("reload moved the camera %v -> %v", before.Pose, after.Pose)
	}
	b.eval(`(() => { clearFocus(); document.getElementById('ov').focus(); return 1; })()`)
	b.run("press", "ArrowRight")
	b.waitTrue(`S.selected !== null`, 5*time.Second)
	b.run("press", "Enter")
	b.waitTrue(`document.activeElement.id === 'detail'`, 5*time.Second)
	b.run("press", "Escape")
	b.waitTrue(`document.activeElement.id === 'ov' && S.selected !== null`, 5*time.Second)
	b.run("press", "Escape")
	b.waitTrue(`S.selected === null && S.topicFocus !== null`, 5*time.Second)
	b.run("press", "Escape")
	b.waitTrue(`S.selected === null && S.topicFocus === null`, 5*time.Second)
	b.run("press", "/")
	b.run("type", "#q", "alpha")
	b.waitTrue(`NAV.palette.isOpen() && document.querySelectorAll('#gsearch .gsr').length > 1 && __gfx.sonarOn()`, 10*time.Second)
	b.run("press", "Enter")
	b.waitTrue(`S.selected !== null && S.selected.key === 'feedback_alpha' && !NAV.palette.isOpen() && !__gfx.sonarOn()`, 10*time.Second)
	b.waitTrue(`!cam.anim && document.getElementById('sr').textContent.startsWith('feedback_alpha')`, 10*time.Second)
}

type idleResult struct{ Raf, Frames int }

func (b *browser) idle(ms int) idleResult {
	b.t.Helper()
	var r idleResult
	b.evalJSON(fmt.Sprintf(`(async () => {
		let raf = 0;
		const orig = window.requestAnimationFrame.bind(window);
		window.requestAnimationFrame = f => { raf++; return orig(f); };
		const f0 = window.__gfx.stats().frames;
		await new Promise(r => setTimeout(r, %d));
		window.requestAnimationFrame = orig;
		return JSON.stringify({Raf: raf, Frames: window.__gfx.stats().frames - f0});
	})()`, ms), &r)
	return r
}

type benchStats struct {
	Nodes, Edges int
	Interval     struct{ P50, P95, P99 float64 }
	JS           struct{ P50, P95, Mean float64 }
	Error        string
}

const (
	budgetIntervalP95 = 20.0
	budgetFrameJSP95  = 6.0
	budgetJSMean      = 2.0
	budgetLabelPassMs = 3.0
	budgetLabels      = 400
	budgetBenchNodes  = 15000
	budgetBenchTile   = 42
)

func TestRendererBudgets(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		name := "full-motion"
		if reduced {
			name = "reduced-motion"
		}
		t.Run(name, func(t *testing.T) { checkBudgets(t, reduced) })
	}
}

func checkBudgets(t *testing.T, reduced bool) {
	b := newBrowser(t, fmt.Sprintf("budget-%v", reduced))
	f := newFixture(t)
	srv := httptest.NewServer(f.h)
	t.Cleanup(func() { b.close(); srv.Close() })
	b.run("set", "viewport", "1600", "900")
	if reduced {
		b.run("set", "media", "dark", "reduced-motion")
	}
	b.run("open", srv.URL+"/")
	b.waitTrue(fmt.Sprintf(`!!(window.__gfx && window.__gfx.stats().frames > 0) && REDUCED === %v`, reduced), 20*time.Second)
	b.run("wait", "1500")

	var st struct {
		Backend string `json:"backend"`
		Lost    bool   `json:"lost"`
	}
	b.evalJSON(`JSON.stringify(__gfx.stats())`, &st)
	if st.Backend != "webgl2" || st.Lost {
		t.Fatalf("renderer = %+v", st)
	}

	var rect struct{ X, Y, W, H float64 }
	b.evalJSON(`JSON.stringify((r => ({X: r.left, Y: r.top, W: r.width, H: r.height}))(document.getElementById('ov').getBoundingClientRect()))`, &rect)
	if lit := litPixels(b.shotImage(), rect.X, rect.Y, rect.W, rect.H); lit < 40 {
		t.Errorf("the canvas is blank: %d lit pixels", lit)
	}

	b.expectAnimation(reduced, "overview")
	if !reduced {
		b.frameBudget(t, "overview", 2500)
	}

	var names struct{ Labels, Named int }
	b.evalJSON(`JSON.stringify({Labels: __gfx.labelInfo().count, Named: TOPICS.filter(t => t.members.length).length})`, &names)
	if names.Named == 0 || names.Labels < names.Named {
		t.Errorf("overview names %d of %d territories", names.Labels, names.Named)
	}

	var lbl struct {
		Count int
		Full  struct{ P50, P95 float64 }
	}
	b.evalJSON(fmt.Sprintf(`JSON.stringify(__gfx.labelBench(80, %d))`, budgetLabels), &lbl)
	t.Logf("label pass at %d items: p50 %.3f ms, p95 %.3f ms", budgetLabels, lbl.Full.P50, lbl.Full.P95)
	if lbl.Full.P95 > budgetLabelPassMs {
		t.Errorf("label pass p95 %.3f ms exceeds %.1f ms", lbl.Full.P95, budgetLabelPassMs)
	}

	b.eval(`(() => { select(BYKEY.get('feedback_alpha'), { reveal: true }); return 1; })()`)
	b.waitTrue(`!cam.anim && S.selected !== null`, 10*time.Second)
	b.run("wait", "6000")
	b.expectAnimation(reduced, "selection")
	b.eval(`(() => { clearFocus(); return 1; })()`)
	b.run("wait", "600")

	var wave struct{ Seeds, Hop1 []int }
	b.evalJSON(fireWave, &wave)
	if len(wave.Seeds) != 1 {
		t.Fatalf("the wave lit %v seeds", wave.Seeds)
	}
	var pace struct {
		N             int
		P50, P95, Max float64
		Frames        int
		Comets, Lit   int
	}
	b.evalJSON(`(async () => {
		const iv = [];
		let last = 0;
		const f0 = __gfx.stats().frames;
		let comets = 0, lit = 0;
		await new Promise(done => {
			const t0 = performance.now();
			const loop = now => {
				if (last) iv.push(now - last);
				last = now;
				const i = __gfx.recallInfo();
				comets = Math.max(comets, i.comets);
				lit = Math.max(lit, i.lit);
				if (now - t0 < 1500) requestAnimationFrame(loop); else done();
			};
			requestAnimationFrame(loop);
		});
		const s = iv.slice(2).sort((a, b) => a - b), q = p => +s[Math.floor(p * (s.length - 1))].toFixed(2);
		return JSON.stringify({N: s.length, P50: q(.5), P95: q(.95), Max: s[s.length - 1], Frames: __gfx.stats().frames - f0, Comets: comets, Lit: lit});
	})()`, &pace)
	t.Logf("during the wave: %+v", pace)
	if reduced {
		if pace.Comets != 0 || pace.Lit != 0 {
			t.Errorf("reduced motion animated a wave: %+v", pace)
		}
	} else {
		if pace.Frames < 20 {
			t.Errorf("the wave drew only %d frames in 1.5 s", pace.Frames)
		}
		if pace.P95 > budgetIntervalP95 {
			t.Errorf("rAF interval p95 during a wave %.2f ms exceeds %.0f ms", pace.P95, budgetIntervalP95)
		}
	}
	b.waitTrue(`__gfx.recallInfo().waves === 0 && arcs.length === 0`, 40*time.Second)
	b.run("wait", "300")

	var bench benchStats
	b.evalJSON(fmt.Sprintf(`(async () => JSON.stringify(await __memBench({tile: %d, frames: 240})))()`, budgetBenchTile), &bench)
	t.Logf("x%d scene: %+v", budgetBenchTile, bench)
	if bench.Error != "" {
		t.Fatalf("memBench: %s", bench.Error)
	}
	if bench.Nodes < budgetBenchNodes {
		t.Errorf("the synthetic scene has %d nodes, the budget scene needs %d", bench.Nodes, budgetBenchNodes)
	}
	if bench.Interval.P95 > budgetIntervalP95 {
		t.Errorf("frame interval p95 %.2f ms at %d nodes exceeds %.0f ms", bench.Interval.P95, bench.Nodes, budgetIntervalP95)
	}
	if bench.JS.Mean > budgetJSMean {
		t.Errorf("JS per animated frame %.3f ms at %d nodes exceeds %.1f ms", bench.JS.Mean, bench.Nodes, budgetJSMean)
	}
	b.run("wait", "500")
	b.expectAnimation(reduced, "after the bench")
	if !reduced {
		dragBudget(t, b)
	}
}

func dragBudget(t *testing.T, b *browser) {
	t.Helper()
	b.eval(`(() => { clearFocus(); camTo(BYKEY.get('feedback_alpha').x, BYKEY.get('feedback_alpha').y, 1.6, 0); return 1; })()`)
	b.waitTrue(`!cam.anim`, 5*time.Second)
	x, y := b.nodeScreen("feedback_alpha")
	b.mouseMove(x, y)
	b.run("mouse", "down")
	b.eval(`(() => { __gfx.frameReport(true); __frames.on = true; return 1; })()`)
	for i := 1; i <= 14; i++ {
		b.mouseMove(x+float64(i)*9, y+float64(i)*5)
	}
	var rep frameStats
	b.evalJSON(`JSON.stringify(__gfx.frameReport(true))`, &rep)
	b.eval(`(() => { __frames.on = false; return 1; })()`)
	b.run("mouse", "up")
	t.Logf("while dragging: %+v", rep)
	if rep.N < 10 {
		t.Fatalf("a drag drew only %d frames", rep.N)
	}
	if rep.JS.P95 > budgetFrameJSP95 {
		t.Errorf("JS per frame p95 %.2f ms while dragging exceeds %.1f ms", rep.JS.P95, budgetFrameJSP95)
	}
	if rep.Interval.P95 > budgetIntervalP95 {
		t.Errorf("rAF interval p95 %.2f ms while dragging exceeds %.0f ms", rep.Interval.P95, budgetIntervalP95)
	}
}

type frameStats struct {
	N        int
	JS       struct{ P50, P95, Max float64 }
	Interval struct {
		P50, P95 float64
		N        int
	}
}

func (b *browser) frameBudget(t *testing.T, what string, ms int) {
	t.Helper()
	b.eval(`(() => { __gfx.frameReport(true); __frames.on = true; return 1; })()`)
	b.run("wait", fmt.Sprint(ms))
	var rep frameStats
	b.evalJSON(`JSON.stringify(__gfx.frameReport(true))`, &rep)
	b.eval(`(() => { __frames.on = false; return 1; })()`)
	t.Logf("%s: %+v", what, rep)
	if rep.N < 20 {
		t.Errorf("%s drew only %d frames in %d ms", what, rep.N, ms)
	}
	if rep.JS.P95 > budgetFrameJSP95 {
		t.Errorf("%s: JS per frame p95 %.2f ms exceeds %.1f ms", what, rep.JS.P95, budgetFrameJSP95)
	}
	if rep.Interval.P95 > budgetIntervalP95 {
		t.Errorf("%s: rAF interval p95 %.2f ms exceeds %.0f ms", what, rep.Interval.P95, budgetIntervalP95)
	}
}

func (b *browser) expectAnimation(reduced bool, what string) {
	b.t.Helper()
	r := b.idle(2000)
	if reduced {
		if r.Raf != 0 || r.Frames != 0 {
			b.t.Errorf("%s under reduced motion: %d animation frames requested, %d drawn", what, r.Raf, r.Frames)
		}
		return
	}
	if r.Frames < 40 {
		b.t.Errorf("%s: the map drew only %d frames in 2 s, it should be alive", what, r.Frames)
	}
}

func (b *browser) nodeScreen(key string) (float64, float64) {
	b.t.Helper()
	var p struct{ X, Y float64 }
	b.evalJSON(`JSON.stringify((n => { const r = ov.getBoundingClientRect(); return { X: n.x * cam.k + cam.x + r.left, Y: n.y * cam.k + cam.y + r.top }; })(BYKEY.get('`+key+`')))`, &p)
	return p.X, p.Y
}

type dragProbe struct {
	Pinned   string
	Self     float64
	Near     float64
	Far      float64
	Selected string
	CamX     float64
	Moved    int
}

const dragProbeJS = `JSON.stringify((() => {
	const n = BYKEY.get('feedback_alpha'), d = o => Math.hypot(o.x - o.hx, o.y - o.hy);
	const p = __gfx.live.pinned();
	const nb = [...n.nbr].filter(o => o !== n), far = NODES.filter(o => o !== n && !n.nbr.has(o));
	return { Pinned: p ? p.key : '', Self: d(n), Near: nb.length ? Math.max(...nb.map(d)) : 0, Far: far.length ? Math.max(...far.map(d)) : 0, Selected: S.selected ? S.selected.key : '', CamX: cam.x, Moved: NODES.filter(o => d(o) > 8).length };
})())`

func TestDragMovesNodesOnTheRealRenderer(t *testing.T) {
	b := newBrowser(t, "drag")
	f := newFixture(t)
	srv := httptest.NewServer(f.h)
	t.Cleanup(func() { b.close(); srv.Close() })
	b.run("set", "viewport", "1600", "900")
	b.run("open", srv.URL+"/")
	b.waitTrue(`!!(window.__gfx && window.__gfx.stats().frames > 0)`, 20*time.Second)
	b.eval(`(() => { const n = BYKEY.get('feedback_alpha'); camTo(n.x, n.y, 1.8, 0); return 1; })()`)
	b.waitTrue(`!cam.anim`, 5*time.Second)
	b.run("wait", "500")
	x, y := b.nodeScreen("feedback_alpha")

	b.mouseMove(x, y)
	b.run("mouse", "down")
	b.run("mouse", "up")
	b.waitTrue(`S.selected !== null && S.selected.key === 'feedback_alpha'`, 5*time.Second)
	b.eval(`(() => { clearFocus(); return 1; })()`)

	b.mouseMove(x, y)
	b.run("mouse", "down")
	for i := 1; i <= 10; i++ {
		b.mouseMove(x+float64(i)*8, y+float64(i)*6)
	}
	b.run("wait", "500")
	var held dragProbe
	b.evalJSON(dragProbeJS, &held)
	t.Logf("held: %+v", held)
	if held.Pinned != "feedback_alpha" {
		t.Fatalf("pressing a node and moving did not pin it: %+v", held)
	}
	if held.Self < 40 {
		t.Errorf("the node followed the cursor by only %.1f px", held.Self)
	}
	if held.Near < 8 {
		t.Errorf("linked neighbours followed by only %.1f px", held.Near)
	}
	if held.Selected != "" {
		t.Errorf("a drag selected %q", held.Selected)
	}
	b.run("mouse", "up")
	b.waitTrue(`(() => { const d = o => Math.hypot(o.x - o.hx, o.y - o.hy); return !__gfx.live.pinned() && NODES.every(o => d(o) < 6); })()`, 6*time.Second)
	var after dragProbe
	b.evalJSON(dragProbeJS, &after)
	if after.Selected != "" || after.Pinned != "" {
		t.Errorf("after release: %+v", after)
	}

	var pan struct{ Before, After float64 }
	b.evalJSON(`JSON.stringify({Before: cam.x, After: 0})`, &pan)
	b.run("mouse", "move", "700", "130")
	b.run("mouse", "down")
	for i := 1; i <= 6; i++ {
		b.run("mouse", "move", fmt.Sprint(700+i*15), "130")
	}
	b.run("mouse", "up")
	var moved dragProbe
	b.evalJSON(dragProbeJS, &moved)
	if math.Abs(moved.CamX-pan.Before) < 40 {
		t.Errorf("dragging empty space moved the camera by %.1f px", moved.CamX-pan.Before)
	}
	if moved.Pinned != "" {
		t.Errorf("dragging empty space pinned %q", moved.Pinned)
	}
}

func TestReducedMotionKeepsTheMapStillUntilDragged(t *testing.T) {
	b := newBrowser(t, "still")
	f := newFixture(t)
	srv := httptest.NewServer(f.h)
	t.Cleanup(func() { b.close(); srv.Close() })
	b.run("set", "viewport", "1600", "900")
	b.run("set", "media", "dark", "reduced-motion")
	b.run("open", srv.URL+"/")
	b.waitTrue(`!!(window.__gfx && window.__gfx.stats().frames > 0) && REDUCED === true`, 20*time.Second)
	b.run("wait", "1200")
	if r := b.idle(2500); r.Raf != 0 || r.Frames != 0 {
		t.Fatalf("reduced motion: %d animation frames requested, %d drawn on an untouched map", r.Raf, r.Frames)
	}
	var still struct{ Off int }
	b.evalJSON(`JSON.stringify({Off: NODES.filter(o => o.x !== o.hx || o.y !== o.hy).length})`, &still)
	if still.Off != 0 {
		t.Fatalf("reduced motion left %d nodes off their home", still.Off)
	}
	b.eval(`(() => { const n = BYKEY.get('feedback_alpha'); camTo(n.x, n.y, 1.8, 0); return 1; })()`)
	b.waitTrue(`!cam.anim`, 5*time.Second)
	b.run("wait", "500")
	x, y := b.nodeScreen("feedback_alpha")
	b.mouseMove(x, y)
	b.run("mouse", "down")
	for i := 1; i <= 8; i++ {
		b.mouseMove(x+float64(i)*10, y+float64(i)*4)
	}
	b.run("wait", "300")
	var held dragProbe
	b.evalJSON(dragProbeJS, &held)
	if held.Pinned != "feedback_alpha" || held.Self < 40 {
		t.Fatalf("a drag under reduced motion: %+v", held)
	}
	b.run("mouse", "up")
	b.waitTrue(`!__gfx.awake()`, 8*time.Second)
	b.evalJSON(`JSON.stringify({Off: NODES.filter(o => o.x !== o.hx || o.y !== o.hy).length})`, &still)
	if still.Off != 0 {
		t.Fatalf("after release %d nodes stayed off home", still.Off)
	}
	if r := b.idle(1500); r.Raf != 0 || r.Frames != 0 {
		t.Fatalf("the map kept drawing after the nodes settled: %d requested, %d drawn", r.Raf, r.Frames)
	}
}

func (b *browser) mouseMove(x, y float64) {
	b.t.Helper()
	b.run("mouse", "move", strconv.Itoa(int(math.Round(x))), strconv.Itoa(int(math.Round(y))))
}
