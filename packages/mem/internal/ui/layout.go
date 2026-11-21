package ui

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"runtime"
	"sort"
	"sync"
)

const (
	layoutVersion  = 2
	layoutSeed     = 20261108
	hullBins       = 48
	topicGap       = 46.0
	capHeadroom    = 1.1
	capGrowth      = 1.25
	capStart       = 8.0
	radiusPerSqrt  = 8.2
	relaxIters     = 300
	polishIters    = 40
	mapAspect      = 1.5
	pullX          = 0.997
	pullY          = 0.985
	polishRatio    = 0.9
	topicIters     = 900
	springRest     = 12.0
	placeIters     = 40
	placeJitter    = 8.0
	placeFreeRatio = 0.85
	staleSteps     = 2

	maxLayoutCoord = 1e6
	maxLayoutCap   = 1 << 24
)

type layoutTopic struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Cap  int     `json:"cap"`
	Cap0 int     `json:"c0"`
}

func (t *layoutTopic) UnmarshalJSON(b []byte) error {
	var raw struct {
		X    *float64 `json:"x"`
		Y    *float64 `json:"y"`
		Cap  *int     `json:"cap"`
		Cap0 int      `json:"c0"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if raw.X == nil || raw.Y == nil || raw.Cap == nil {
		return fmt.Errorf("layout topic misses a field")
	}
	*t = layoutTopic{X: *raw.X, Y: *raw.Y, Cap: *raw.Cap, Cap0: raw.Cap0}
	return nil
}

type layoutNode struct {
	X, Y  float64
	Topic string
}

func (n layoutNode) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{n.X, n.Y, n.Topic})
}

func (n *layoutNode) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) != 3 {
		return fmt.Errorf("layout node has %d fields, want 3", len(raw))
	}
	for _, r := range raw {
		if string(r) == "null" {
			return fmt.Errorf("layout node has a null field")
		}
	}
	if err := json.Unmarshal(raw[0], &n.X); err != nil {
		return err
	}
	if err := json.Unmarshal(raw[1], &n.Y); err != nil {
		return err
	}
	return json.Unmarshal(raw[2], &n.Topic)
}

type layoutData struct {
	V      int                     `json:"v"`
	Seed   int64                   `json:"seed"`
	Topics map[string]*layoutTopic `json:"topics"`
	Nodes  map[string]layoutNode   `json:"nodes"`
}

func newLayoutData() *layoutData {
	return &layoutData{V: layoutVersion, Seed: layoutSeed, Topics: map[string]*layoutTopic{}, Nodes: map[string]layoutNode{}}
}

func (d *layoutData) valid() bool {
	if d == nil || d.V != layoutVersion || d.Topics == nil || d.Nodes == nil {
		return false
	}
	for _, t := range d.Topics {
		if t == nil || t.Cap < 1 || t.Cap > maxLayoutCap || t.Cap0 < 0 || t.Cap0 > maxLayoutCap || !sane(t.X) || !sane(t.Y) {
			return false
		}
	}
	for _, n := range d.Nodes {
		if !sane(n.X) || !sane(n.Y) {
			return false
		}
		t, ok := d.Topics[n.Topic]
		if !ok || math.Hypot(n.X-t.X, n.Y-t.Y) > topicRadius(t.Cap)+1 {
			return false
		}
	}
	return true
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func sane(v float64) bool { return finite(v) && math.Abs(v) <= maxLayoutCoord }

func round1(v float64) float64 { return math.Round(v*10) / 10 }

type lnode struct {
	key   string
	topic string
	r     float64
	adj   []int
	vec   []float32
}

type layoutInput struct {
	nodes  []lnode
	topics []string
	size   map[string]int
	index  map[string]int
}

func newLayoutInput(g *Graph, vecs map[string][]float32) *layoutInput {
	in := &layoutInput{size: map[string]int{}, index: make(map[string]int, len(g.Records))}
	for _, t := range g.Topics {
		in.topics = append(in.topics, t.ID)
		in.size[t.ID] = 0
	}
	order := make([]int, len(g.Records))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return g.Records[order[a]].Key < g.Records[order[b]].Key })
	in.nodes = make([]lnode, len(order))
	for i, ri := range order {
		n := g.Records[ri]
		id := g.Topics[n.Topic].ID
		in.index[n.Key] = i
		in.nodes[i] = lnode{key: n.Key, topic: id, vec: vecs[n.Key]}
		in.size[id]++
	}
	directed := make([]int, len(order))
	seen := map[[2]int]bool{}
	for i, ri := range order {
		for _, o := range g.Records[ri].Out {
			j, ok := in.index[o]
			if !ok || j == i {
				continue
			}
			directed[i]++
			directed[j]++
			a, b := i, j
			if b < a {
				a, b = b, a
			}
			if seen[[2]int{a, b}] {
				continue
			}
			seen[[2]int{a, b}] = true
			in.nodes[i].adj = append(in.nodes[i].adj, j)
			in.nodes[j].adj = append(in.nodes[j].adj, i)
		}
	}
	for i, ri := range order {
		bonus := 0.0
		if g.Records[ri].Core {
			bonus = 1.2
		}
		in.nodes[i].r = math.Max(2.6, math.Min(12, 2.6+math.Sqrt(float64(directed[i]))*1.35+bonus))
		sort.Ints(in.nodes[i].adj)
	}
	return in
}

func hash01(s, salt string) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(salt))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(s))
	return float64(h.Sum64()>>11) / float64(1<<53)
}

func capFor(n int) int {
	need := int(math.Ceil(float64(n) * capHeadroom))
	c := capStart
	for int(math.Ceil(c)) < need {
		c *= capGrowth
	}
	return int(math.Ceil(c))
}

func capSteps(from, to int) int {
	if to <= from {
		return 0
	}
	steps := 0
	c := capStart
	for int(math.Ceil(c)) < from {
		c *= capGrowth
	}
	for int(math.Ceil(c)) < to {
		c *= capGrowth
		steps++
	}
	return steps
}

func topicRadius(c int) float64 { return radiusPerSqrt * math.Sqrt(float64(c)) }

type wtopic struct {
	id      string
	n       int
	cap     int
	R       float64
	res     float64
	x, y    float64
	members []int
}

type layoutWork struct {
	in       *layoutInput
	topics   []*wtopic
	tIdx     map[string]int
	topicOf  []int
	x, y     []float64
	placed   []bool
	cellGrid map[[2]int32][]int32
}

func newLayoutWork(in *layoutInput) *layoutWork {
	w := &layoutWork{in: in, tIdx: map[string]int{}}
	for _, id := range in.topics {
		w.tIdx[id] = len(w.topics)
		w.topics = append(w.topics, &wtopic{id: id, n: in.size[id]})
	}
	n := len(in.nodes)
	w.topicOf = make([]int, n)
	w.x = make([]float64, n)
	w.y = make([]float64, n)
	w.placed = make([]bool, n)
	for i, nd := range in.nodes {
		ti := w.tIdx[nd.topic]
		w.topicOf[i] = ti
		w.topics[ti].members = append(w.topics[ti].members, i)
	}
	return w
}

func computeFullLayout(in *layoutInput) *layoutData {
	w := newLayoutWork(in)
	for _, t := range w.topics {
		t.cap = capFor(t.n)
		t.R = topicRadius(t.cap)
		t.res = topicRadius(capNext(t.cap))
	}
	w.placeTopics()
	var isolated []int
	conns := make([][]int, len(w.topics))
	for ti, t := range w.topics {
		for _, i := range t.members {
			if len(in.nodes[i].adj) == 0 {
				isolated = append(isolated, i)
			} else {
				conns[ti] = append(conns[ti], i)
			}
		}
	}
	w.relaxAll(conns)
	w.initGrid()
	for i := range in.nodes {
		if w.placed[i] {
			w.gridAdd(i)
		}
	}
	for _, i := range isolated {
		w.placeOne(i)
	}
	d := newLayoutData()
	for _, t := range w.topics {
		d.Topics[t.id] = &layoutTopic{X: round1(t.x), Y: round1(t.y), Cap: t.cap, Cap0: t.cap}
	}
	for i, nd := range in.nodes {
		d.Nodes[nd.key] = layoutNode{X: round1(w.x[i]), Y: round1(w.y[i]), Topic: nd.topic}
	}
	return d
}

func (w *layoutWork) placeTopics() {
	T := w.topics
	W := make([][]float64, len(T))
	for i := range W {
		W[i] = make([]float64, len(T))
	}
	for i, n := range w.in.nodes {
		for _, j := range n.adj {
			if a, b := w.topicOf[i], w.topicOf[j]; a != b {
				W[a][b] += .5
			}
		}
	}
	ord := make([]int, len(T))
	for i := range ord {
		ord[i] = i
	}
	sort.SliceStable(ord, func(a, b int) bool {
		if T[ord[a]].n != T[ord[b]].n {
			return T[ord[a]].n > T[ord[b]].n
		}
		return T[ord[a]].id < T[ord[b]].id
	})
	for r, i := range ord {
		ang := float64(r) * 2.39996
		rad := 90 * math.Sqrt(float64(r))
		T[i].x, T[i].y = math.Cos(ang)*rad*mapAspect, math.Sin(ang)*rad
	}
	for it := 0; it < topicIters; it++ {
		for i := range T {
			for j := i + 1; j < len(T); j++ {
				A, B := T[i], T[j]
				dx, dy := B.x-A.x, B.y-A.y
				d := math.Hypot(dx, dy)
				if d < .1 {
					d = .1
				}
				min := A.res + B.res + topicGap
				ma, mb := A.R*A.R, B.R*B.R
				wa := mb / (ma + mb)
				wb := 1 - wa
				if d < min {
					f := (min - d) / d
					A.x -= dx * f * wa
					A.y -= dy * f * wa
					B.x += dx * f * wb
					B.y += dy * f * wb
				} else if W[i][j] > 0 {
					f := math.Min(.02, .0009*W[i][j]) * (d - min) / d
					A.x += dx * f * wa
					A.y += dy * f * wa
					B.x -= dx * f * wb
					B.y -= dy * f * wb
				}
			}
		}
		for _, t := range T {
			t.x *= pullX
			t.y *= pullY
		}
	}
	for pass := 0; pass < 400; pass++ {
		worst := 0.0
		for i := range T {
			for j := i + 1; j < len(T); j++ {
				A, B := T[i], T[j]
				dx, dy := B.x-A.x, B.y-A.y
				d := math.Max(math.Hypot(dx, dy), .1)
				need := A.res + B.res + topicGap + .3
				if d >= need {
					continue
				}
				worst = math.Max(worst, need-d)
				ma, mb := A.R*A.R, B.R*B.R
				f := (need - d) / d
				A.x -= dx * f * mb / (ma + mb)
				A.y -= dy * f * mb / (ma + mb)
				B.x += dx * f * ma / (ma + mb)
				B.y += dy * f * ma / (ma + mb)
			}
		}
		if worst < .01 {
			break
		}
	}
	for _, t := range T {
		t.x, t.y = round1(t.x), round1(t.y)
	}
}

func (w *layoutWork) relaxAll(conns [][]int) {
	order := make([]int, len(conns))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return len(conns[order[a]]) > len(conns[order[b]]) })
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), len(order)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ti := range jobs {
				w.relaxTopic(ti, conns[ti])
			}
		}()
	}
	for _, ti := range order {
		jobs <- ti
	}
	close(jobs)
	wg.Wait()
}

func equalizeRadii(pos [][2]float64, rmax float64) {
	n := len(pos)
	order := make([]int, n)
	rad := make([]float64, n)
	for k := range pos {
		order[k] = k
		rad[k] = math.Hypot(pos[k][0], pos[k][1])
	}
	sort.SliceStable(order, func(a, b int) bool { return rad[order[a]] < rad[order[b]] })
	for q, k := range order {
		if rad[k] < 1e-9 {
			continue
		}
		want := rmax * math.Sqrt((float64(q)+.5)/float64(n))
		pos[k][0] *= want / rad[k]
		pos[k][1] *= want / rad[k]
	}
}

func (w *layoutWork) relaxTopic(ti int, ids []int) {
	t := w.topics[ti]
	n := len(ids)
	if n == 0 {
		return
	}
	nodes := w.in.nodes
	pos := make([][2]float64, n)
	loc := make(map[int]int, n)
	for k, i := range ids {
		loc[i] = k
		a := hash01(nodes[i].key, "a") * 2 * math.Pi
		r := math.Sqrt(hash01(nodes[i].key, "r")) * .8 * t.R
		pos[k] = [2]float64{math.Cos(a) * r, math.Sin(a) * r}
	}
	type ghost struct {
		k      int
		dx, dy float64
		w      float64
	}
	var ghosts []ghost
	for k, i := range ids {
		var gx, gy, cnt float64
		for _, j := range nodes[i].adj {
			o := w.topics[w.topicOf[j]]
			if o != t {
				dx, dy := o.x-t.x, o.y-t.y
				d := math.Hypot(dx, dy) + 1e-9
				gx += dx / d
				gy += dy / d
				cnt++
			}
		}
		if cnt > 0 {
			d := math.Hypot(gx, gy) + 1e-9
			ghosts = append(ghosts, ghost{k, gx / d * .85 * t.R, gy / d * .85 * t.R, math.Min(cnt, 4)})
		}
	}
	const cell = 60.0
	gw := int(math.Ceil(2*t.R/cell)) + 3
	head := make([]int32, gw*gw)
	next := make([]int32, n)
	cellOf := make([]int, n)
	cx := make([]int, n)
	cy := make([]int, n)
	disp := make([][2]float64, n)
	hardR := .99 * t.R
	softR := .82 * t.R
	for it := 0; it < relaxIters+polishIters; it++ {
		polish := it >= relaxIters
		if it == relaxIters {
			equalizeRadii(pos, .94*t.R)
		}
		alpha := math.Max(0, 1-float64(it)/relaxIters)
		for k := range disp {
			disp[k] = [2]float64{}
		}
		for g := range head {
			head[g] = -1
		}
		for k := range pos {
			x := int(math.Floor((pos[k][0]+t.R)/cell)) + 1
			y := int(math.Floor((pos[k][1]+t.R)/cell)) + 1
			x = max(0, min(gw-1, x))
			y = max(0, min(gw-1, y))
			cx[k], cy[k] = x, y
			cellOf[k] = y*gw + x
			next[k] = head[cellOf[k]]
			head[cellOf[k]] = int32(k)
		}
		for a := 0; a < n; a++ {
			na := nodes[ids[a]]
			for oy := -1; oy <= 1; oy++ {
				yy := cy[a] + oy
				if yy < 0 || yy >= gw {
					continue
				}
				for ox := -1; ox <= 1; ox++ {
					xx := cx[a] + ox
					if xx < 0 || xx >= gw {
						continue
					}
					for b := head[yy*gw+xx]; b >= 0; b = next[b] {
						if int(b) <= a {
							continue
						}
						nb := nodes[ids[b]]
						dx, dy := pos[b][0]-pos[a][0], pos[b][1]-pos[a][1]
						d2 := dx*dx + dy*dy
						if d2 > cell*cell {
							continue
						}
						if d2 < 1e-6 {
							dx, dy, d2 = 1e-3*(hash01(na.key, "j")-.5), 1e-3*(hash01(nb.key, "j")-.5), 1
						}
						d := math.Sqrt(d2)
						var f float64
						if polish {
							mn := polishRatio * (na.r + nb.r)
							if d >= mn {
								continue
							}
							f = (mn - d) * .5 / d
						} else {
							f = 14 / (d2 + 16)
							if mn := na.r + nb.r + 2; d < mn {
								f += (mn - d) * .5 / d
							}
						}
						disp[a][0] -= dx * f
						disp[a][1] -= dy * f
						disp[b][0] += dx * f
						disp[b][1] += dy * f
					}
				}
			}
		}
		for k, i := range ids {
			if polish {
				break
			}
			for _, j := range nodes[i].adj {
				l, ok := loc[j]
				if !ok || l < k {
					continue
				}
				dx, dy := pos[l][0]-pos[k][0], pos[l][1]-pos[k][1]
				d := math.Hypot(dx, dy) + 1e-9
				s := .06 * (d - springRest) / d / math.Sqrt(float64(min(len(nodes[i].adj), len(nodes[j].adj))))
				disp[k][0] += dx * s
				disp[k][1] += dy * s
				disp[l][0] -= dx * s
				disp[l][1] -= dy * s
			}
		}
		for k := range pos {
			if polish {
				break
			}
			disp[k][0] -= pos[k][0] * .002
			disp[k][1] -= pos[k][1] * .002
			if r := math.Hypot(pos[k][0], pos[k][1]); r > softR {
				pull := .25 * (r - softR) / r
				disp[k][0] -= pos[k][0] * pull
				disp[k][1] -= pos[k][1] * pull
			}
		}
		for _, g := range ghosts {
			if polish {
				break
			}
			disp[g.k][0] += (g.dx - pos[g.k][0]) * .008 * g.w
			disp[g.k][1] += (g.dy - pos[g.k][1]) * .008 * g.w
		}
		lim := 3*alpha + .15
		if polish {
			lim = 1
		}
		for k := range pos {
			dx, dy := disp[k][0], disp[k][1]
			if d := math.Hypot(dx, dy); d > lim {
				dx, dy = dx/d*lim, dy/d*lim
			}
			pos[k][0] += dx
			pos[k][1] += dy
			if r := math.Hypot(pos[k][0], pos[k][1]); r > hardR {
				pos[k][0] *= hardR / r
				pos[k][1] *= hardR / r
			}
		}
	}
	for k, i := range ids {
		w.x[i], w.y[i] = round1(t.x+pos[k][0]), round1(t.y+pos[k][1])
		w.placed[i] = true
	}
}

const gridCell = 32.0

func (w *layoutWork) initGrid() { w.cellGrid = map[[2]int32][]int32{} }

func gridKey(x, y float64) [2]int32 {
	return [2]int32{int32(math.Floor(x / gridCell)), int32(math.Floor(y / gridCell))}
}

func (w *layoutWork) gridAdd(i int) {
	k := gridKey(w.x[i], w.y[i])
	w.cellGrid[k] = append(w.cellGrid[k], int32(i))
}

func (w *layoutWork) near(i int, x, y float64, fn func(j int)) {
	k := gridKey(x, y)
	for dx := int32(-1); dx <= 1; dx++ {
		for dy := int32(-1); dy <= 1; dy++ {
			for _, j := range w.cellGrid[[2]int32{k[0] + dx, k[1] + dy}] {
				if int(j) != i {
					fn(int(j))
				}
			}
		}
	}
}

func (w *layoutWork) free(i int, x, y float64) bool {
	ri := w.in.nodes[i].r
	ok := true
	w.near(i, x, y, func(j int) {
		if !ok {
			return
		}
		if math.Hypot(x-w.x[j], y-w.y[j]) < placeFreeRatio*(ri+w.in.nodes[j].r) {
			ok = false
		}
	})
	return ok
}

func (w *layoutWork) placeOne(i int) {
	nd := w.in.nodes[i]
	ti := w.topicOf[i]
	t := w.topics[ti]
	var sx, sy, sw float64
	for _, j := range nd.adj {
		if w.placed[j] && w.topicOf[j] == ti {
			sx += w.x[j] * 2
			sy += w.y[j] * 2
			sw += 2
		}
	}
	if nd.vec != nil {
		type cand struct {
			c float64
			j int
		}
		var top [5]cand
		cnt := 0
		for _, j := range t.members {
			o := w.in.nodes[j]
			if !w.placed[j] || o.vec == nil || j == i || len(o.vec) != len(nd.vec) {
				continue
			}
			var s float32
			for q := range nd.vec {
				s += nd.vec[q] * o.vec[q]
			}
			c := cand{float64(s), j}
			pos := cnt
			if cnt == len(top) {
				if c.c <= top[cnt-1].c {
					continue
				}
				pos = cnt - 1
			} else {
				cnt++
			}
			for pos > 0 && top[pos-1].c < c.c {
				top[pos] = top[pos-1]
				pos--
			}
			top[pos] = c
		}
		for q := 0; q < cnt; q++ {
			wt := math.Max(.05, top[q].c)
			sx += w.x[top[q].j] * wt
			sy += w.y[top[q].j] * wt
			sw += wt
		}
	}
	if sw == 0 {
		a := hash01(nd.key, "a") * 2 * math.Pi
		r := math.Sqrt(hash01(nd.key, "r")) * .8 * t.R
		sx, sy, sw = t.x+math.Cos(a)*r, t.y+math.Sin(a)*r, 1
	}
	ja := hash01(nd.key, "ja") * 2 * math.Pi
	jr := hash01(nd.key, "jr") * placeJitter
	x, y := sx/sw+math.Cos(ja)*jr, sy/sw+math.Sin(ja)*jr
	limit := .97 * t.R
	clampTo := func() {
		dx, dy := x-t.x, y-t.y
		if r := math.Hypot(dx, dy); r > limit {
			x, y = t.x+dx*limit/r, t.y+dy*limit/r
		}
	}
	clampTo()
	for it := 0; it < placeIters; it++ {
		var fx, fy float64
		w.near(i, x, y, func(j int) {
			dx, dy := x-w.x[j], y-w.y[j]
			d := math.Hypot(dx, dy) + 1e-6
			if mn := nd.r + w.in.nodes[j].r + 2; d < mn {
				fx += dx / d * (mn - d) * .5
				fy += dy / d * (mn - d) * .5
			}
		})
		x += fx
		y += fy
		clampTo()
	}
	if !w.free(i, x, y) {
		bx, by := x, y
		phase := hash01(nd.key, "sp") * 2 * math.Pi
		for s := 1; s <= 4000; s++ {
			a := phase + float64(s)*2.39996
			rad := 3.2 * math.Sqrt(float64(s))
			cx, cy := bx+math.Cos(a)*rad, by+math.Sin(a)*rad
			if math.Hypot(cx-t.x, cy-t.y) > limit {
				continue
			}
			if w.free(i, cx, cy) {
				x, y = cx, cy
				break
			}
		}
	}
	w.x[i], w.y[i] = round1(x), round1(y)
	w.placed[i] = true
	w.gridAdd(i)
}

func (d *layoutData) topicOrder() []string {
	ids := make([]string, 0, len(d.Topics))
	for id := range d.Topics {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

type tpt struct{ x, y float64 }

func spreadTopics(cur []tpt, rad, mass []float64, active []bool, power float64, passes int) bool {
	n := len(cur)
	weight := make([]float64, n)
	for i := range weight {
		weight[i] = math.Pow(mass[i], power)
	}
	for pass := 0; pass < passes; pass++ {
		worst := 0.0
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				if !active[i] && !active[j] {
					continue
				}
				dx, dy := cur[j].x-cur[i].x, cur[j].y-cur[i].y
				dist := math.Hypot(dx, dy)
				if dist < .1 {
					dx, dy, dist = 1e-3*float64(j-i), 1e-3, .1
				}
				need := rad[i] + rad[j] + topicGap
				if dist >= need {
					continue
				}
				worst = math.Max(worst, need-dist)
				f := (need - dist) / dist * 1.02
				mi, mj := weight[i], weight[j]
				wa, wb := mj/(mi+mj), mi/(mi+mj)
				if power == 0 {
					wa, wb = 0, 1
					if mass[i] < mass[j] || (mass[i] == mass[j] && i < j) {
						wa, wb = 1, 0
					}
				}
				if wa > 0 {
					active[i] = true
				}
				if wb > 0 {
					active[j] = true
				}
				cur[i].x -= dx * f * wa
				cur[i].y -= dy * f * wa
				cur[j].x += dx * f * wb
				cur[j].y += dy * f * wb
			}
		}
		if worst < .05 {
			return true
		}
	}
	return false
}

func (d *layoutData) planTopicPush(bumped []string) (map[string][2]float64, float64) {
	ids := d.topicOrder()
	start := make(map[string]tpt, len(ids))
	cur := make([]tpt, len(ids))
	rad := make([]float64, len(ids))
	for i, id := range ids {
		t := d.Topics[id]
		cur[i] = tpt{t.X, t.Y}
		start[id] = cur[i]
		rad[i] = topicRadius(t.Cap)
	}
	collide := false
	for i := range ids {
		for j := i + 1; j < len(ids); j++ {
			if math.Hypot(cur[j].x-cur[i].x, cur[j].y-cur[i].y) < rad[i]+rad[j]+topicGap-.5 {
				collide = true
			}
		}
	}
	if !collide {
		return map[string][2]float64{}, 0
	}
	isBumped := make(map[string]bool, len(bumped))
	for _, id := range bumped {
		isBumped[id] = true
	}
	mass := make([]float64, len(ids))
	slot := make(map[string]int, len(ids))
	for i, id := range ids {
		slot[id] = i
		mass[i] = 1
	}
	for _, n := range d.Nodes {
		if i, ok := slot[n.Topic]; ok {
			mass[i]++
		}
	}
	reserve := make([]float64, len(ids))
	for i, id := range ids {
		reserve[i] = topicRadius(capNext(d.Topics[id].Cap))
	}
	attempt := func(power float64, keep bool) bool {
		active := make([]bool, len(ids))
		for i, id := range ids {
			cur[i] = start[id]
			rad[i] = topicRadius(d.Topics[id].Cap)
			if isBumped[id] {
				active[i] = true
				if keep {
					rad[i] = reserve[i]
				}
			}
		}
		return spreadTopics(cur, rad, mass, active, power, 3000)
	}
	cost := func() (nodes, drift float64) {
		for i, id := range ids {
			s := start[id]
			if dx, dy := round1(cur[i].x-s.x), round1(cur[i].y-s.y); dx != 0 || dy != 0 {
				nodes += mass[i]
				drift += mass[i] * math.Hypot(dx, dy)
			}
		}
		return nodes, drift
	}
	pick := func(keep bool) ([]tpt, float64, float64) {
		var best []tpt
		var bestNodes, bestDrift float64
		for _, power := range []float64{0, 1, 2, 3, 4} {
			if !attempt(power, keep) {
				continue
			}
			nodes, drift := cost()
			if best == nil || nodes < bestNodes || (nodes == bestNodes && drift < bestDrift) {
				best, bestNodes, bestDrift = append([]tpt(nil), cur...), nodes, drift
			}
		}
		return best, bestNodes, bestDrift
	}
	best, bestNodes, _ := pick(false)
	if roomy, roomyNodes, _ := pick(true); roomy != nil && (best == nil || roomyNodes <= 2*bestNodes) {
		best = roomy
	}
	if best != nil {
		copy(cur, best)
	} else {
		active := make([]bool, len(ids))
		for i, id := range ids {
			cur[i] = start[id]
			active[i] = true
		}
		spreadTopics(cur, reserve, mass, active, 1, 600)
	}
	moved := map[string][2]float64{}
	var pushCost float64
	for i, id := range ids {
		s := start[id]
		dx, dy := round1(cur[i].x-s.x), round1(cur[i].y-s.y)
		if dx != 0 || dy != 0 {
			moved[id] = [2]float64{dx, dy}
			pushCost += mass[i] - 1
		}
	}
	return moved, pushCost
}

func (d *layoutData) translateTopics(moved map[string][2]float64) {
	for id, dv := range moved {
		t := d.Topics[id]
		t.X, t.Y = round1(t.X+dv[0]), round1(t.Y+dv[1])
	}
	for k, n := range d.Nodes {
		if dv, ok := moved[n.Topic]; ok {
			d.Nodes[k] = layoutNode{X: round1(n.X + dv[0]), Y: round1(n.Y + dv[1]), Topic: n.Topic}
		}
	}
}

func (d *layoutData) nodeCount(id string) int {
	n := 0
	for _, p := range d.Nodes {
		if p.Topic == id {
			n++
		}
	}
	return n
}

func (d *layoutData) clearance(id string, x, y, own float64, reserve bool) float64 {
	worst := math.Inf(1)
	for _, o := range d.topicOrder() {
		if o == id {
			continue
		}
		t := d.Topics[o]
		r := topicRadius(t.Cap)
		if reserve {
			r = topicRadius(capNext(t.Cap))
		}
		worst = math.Min(worst, math.Hypot(x-t.X, y-t.Y)-own-r-topicGap)
	}
	return worst
}

func (d *layoutData) collides(id string) bool {
	t := d.Topics[id]
	return d.clearance(id, t.X, t.Y, topicRadius(t.Cap), false) < -.5
}

func (d *layoutData) shiftTopic(id string, oldCap int) bool {
	t := d.Topics[id]
	room := topicRadius(t.Cap) - topicRadius(oldCap)
	if room <= 0 {
		return false
	}
	R := topicRadius(t.Cap)
	own := topicRadius(capNext(t.Cap))
	bx, by := t.X, t.Y
	bestScore := math.Inf(-1)
	found := false
	for _, frac := range []float64{.25, .5, .75, 1} {
		for a := 0; a < 72; a++ {
			ang := float64(a) * math.Pi / 36
			x, y := round1(t.X+math.Cos(ang)*room*frac), round1(t.Y+math.Sin(ang)*room*frac)
			if d.clearance(id, x, y, R, false) < 0 {
				continue
			}
			score := math.Min(0, d.clearance(id, x, y, own, true))
			if !found || score > bestScore {
				bestScore, bx, by, found = score, x, y, true
			}
		}
	}
	if !found {
		return false
	}
	t.X, t.Y = bx, by
	return true
}

func (d *layoutData) relocationSpot(id string) (float64, float64, bool) {
	t := d.Topics[id]
	own := topicRadius(capNext(t.Cap))
	phase := hash01(id, "topic") * 2 * math.Pi
	for s := 0; s < 20000; s++ {
		a := phase + float64(s)*2.39996
		rad := 14 * math.Sqrt(float64(s))
		x, y := round1(t.X+math.Cos(a)*rad), round1(t.Y+math.Sin(a)*rad)
		if d.clearance(id, x, y, own, true) >= 0 {
			return x, y, true
		}
	}
	return 0, 0, false
}

func (d *layoutData) accommodate(bumped []string, oldCap map[string]int) (res syncResult) {
	var left []string
	for _, id := range bumped {
		if !d.collides(id) {
			continue
		}
		if d.shiftTopic(id, oldCap[id]) {
			res.shifted++
			continue
		}
		left = append(left, id)
	}
	for _, id := range left {
		if !d.collides(id) {
			continue
		}
		x, y, ok := d.relocationSpot(id)
		push, pushCost := d.planTopicPush([]string{id})
		relCost := float64(d.nodeCount(id))
		if ok && (len(push) == 0 || relCost <= pushCost) {
			t := d.Topics[id]
			d.translateTopics(map[string][2]float64{id: {round1(x - t.X), round1(y - t.Y)}})
			res.relocated++
			continue
		}
		d.translateTopics(push)
		res.moved += len(push)
	}
	return res
}

func (d *layoutData) freeTopicSpot(id string, R, sx, sy float64) (float64, float64) {
	ids := d.topicOrder()
	phase := hash01(id, "topic") * 2 * math.Pi
	for s := 0; s < 20000; s++ {
		a := phase + float64(s)*2.39996
		rad := 14 * math.Sqrt(float64(s))
		x, y := sx+math.Cos(a)*rad, sy+math.Sin(a)*rad
		ok := true
		for _, o := range ids {
			t := d.Topics[o]
			if math.Hypot(x-t.X, y-t.Y) < R+topicRadius(capNext(t.Cap))+topicGap {
				ok = false
				break
			}
		}
		if ok {
			return round1(x), round1(y)
		}
	}
	return round1(sx), round1(sy)
}

type syncResult struct {
	changed   bool
	placed    int
	bumped    []string
	moved     int
	shifted   int
	relocated int
	removed   int
}

func syncLayout(d *layoutData, in *layoutInput) syncResult {
	var res syncResult
	byKey := in.index
	for k, n := range d.Nodes {
		i, ok := byKey[k]
		if !ok || in.nodes[i].topic != n.Topic {
			delete(d.Nodes, k)
			res.removed++
		}
	}
	for id := range d.Topics {
		if _, ok := in.size[id]; !ok {
			delete(d.Topics, id)
		}
	}
	for k, n := range d.Nodes {
		if _, ok := d.Topics[n.Topic]; !ok {
			delete(d.Nodes, k)
			res.removed++
		}
	}
	var fresh []int
	for i, nd := range in.nodes {
		if _, ok := d.Nodes[nd.key]; !ok {
			fresh = append(fresh, i)
		}
	}
	var newTopics []string
	for _, id := range in.topics {
		if _, ok := d.Topics[id]; !ok {
			newTopics = append(newTopics, id)
		}
	}
	sort.SliceStable(newTopics, func(a, b int) bool {
		if in.size[newTopics[a]] != in.size[newTopics[b]] {
			return in.size[newTopics[a]] > in.size[newTopics[b]]
		}
		return newTopics[a] < newTopics[b]
	})
	for _, id := range newTopics {
		c := capFor(in.size[id])
		var sx, sy, cnt float64
		for _, nd := range in.nodes {
			if nd.topic != id {
				continue
			}
			for _, j := range nd.adj {
				if o, ok := d.Nodes[in.nodes[j].key]; ok && o.Topic != id {
					t := d.Topics[o.Topic]
					sx += t.X
					sy += t.Y
					cnt++
				}
			}
		}
		if cnt > 0 {
			sx, sy = sx/cnt, sy/cnt
		}
		x, y := d.freeTopicSpot(id, topicRadius(capNext(c)), sx, sy)
		d.Topics[id] = &layoutTopic{X: x, Y: y, Cap: c, Cap0: c}
		res.changed = true
	}
	oldCap := map[string]int{}
	for _, id := range d.topicOrder() {
		t := d.Topics[id]
		if need := capFor(in.size[id]); need > t.Cap {
			oldCap[id] = t.Cap
			t.Cap = need
			res.bumped = append(res.bumped, id)
			res.changed = true
		}
	}
	if len(res.bumped) > 0 {
		acc := d.accommodate(res.bumped, oldCap)
		res.moved, res.shifted, res.relocated = acc.moved, acc.shifted, acc.relocated
	}
	if len(fresh) > 0 {
		w := newLayoutWork(in)
		for _, t := range w.topics {
			dt := d.Topics[t.id]
			t.cap, t.R, t.x, t.y = dt.Cap, topicRadius(dt.Cap), dt.X, dt.Y
		}
		w.initGrid()
		for i, nd := range in.nodes {
			if p, ok := d.Nodes[nd.key]; ok {
				w.x[i], w.y[i], w.placed[i] = p.X, p.Y, true
				w.gridAdd(i)
			}
		}
		for _, i := range fresh {
			w.placeOne(i)
			d.Nodes[in.nodes[i].key] = layoutNode{X: w.x[i], Y: w.y[i], Topic: in.nodes[i].topic}
		}
		res.placed = len(fresh)
		res.changed = true
	}
	if res.removed > 0 {
		res.changed = true
	}
	return res
}

func (d *layoutData) staleTopics() int {
	n := 0
	for _, t := range d.Topics {
		if capSteps(t.Cap0, t.Cap) >= staleSteps {
			n++
		}
	}
	return n
}

type topicGeom struct {
	X, Y, R float64
	Cap     int
	Hull    []float64
	LX, LY  float64
}

func (d *layoutData) geometry(in *layoutInput) map[string]*topicGeom {
	members := map[string][]memberExt{}
	for _, nd := range in.nodes {
		p, ok := d.Nodes[nd.key]
		if !ok {
			continue
		}
		t := d.Topics[p.Topic]
		members[p.Topic] = append(members[p.Topic], memberExt{p.X - t.X, p.Y - t.Y, nd.r})
	}
	out := make(map[string]*topicGeom, len(d.Topics))
	for id, t := range d.Topics {
		R := topicRadius(t.Cap)
		g := &topicGeom{X: t.X, Y: t.Y, R: round1(R), Cap: t.Cap, Hull: make([]float64, hullBins), LX: t.X, LY: t.Y}
		ms := members[id]
		raw := make([]float64, hullBins)
		for _, m := range ms {
			dist := math.Hypot(m.dx, m.dy)
			ang := math.Atan2(m.dy, m.dx)
			b := int(math.Floor((ang+math.Pi)/(2*math.Pi)*hullBins)) % hullBins
			e := dist + m.r
			raw[b] = math.Max(raw[b], e)
			for _, nb := range []int{(b + 1) % hullBins, (b + hullBins - 1) % hullBins} {
				raw[nb] = math.Max(raw[nb], e*.9)
			}
		}
		for pass := 0; pass < 2; pass++ {
			sm := make([]float64, hullBins)
			for b := range raw {
				sm[b] = (raw[(b+hullBins-1)%hullBins] + 2*raw[b] + raw[(b+1)%hullBins]) / 4
			}
			raw = sm
		}
		for b := range raw {
			g.Hull[b] = round1(math.Max(.7*R, math.Min(R, raw[b]+12)))
		}
		if len(ms) > 0 {
			g.LX, g.LY = trimmedCentroid(ms, t.X, t.Y)
			g.LX, g.LY = round1(g.LX), round1(g.LY)
		}
		out[id] = g
	}
	return out
}

type memberExt struct{ dx, dy, r float64 }

func trimmedCentroid(ms []memberExt, cx, cy float64) (float64, float64) {
	pts := make([][2]float64, len(ms))
	xs := make([]float64, len(ms))
	ys := make([]float64, len(ms))
	for i, m := range ms {
		pts[i] = [2]float64{cx + m.dx, cy + m.dy}
		xs[i], ys[i] = pts[i][0], pts[i][1]
	}
	sort.Float64s(xs)
	sort.Float64s(ys)
	mx, my := xs[len(xs)/2], ys[len(ys)/2]
	type pd struct {
		d float64
		i int
	}
	ds := make([]pd, len(pts))
	for i, p := range pts {
		ds[i] = pd{math.Hypot(p[0]-mx, p[1]-my), i}
	}
	sort.Slice(ds, func(a, b int) bool {
		if ds[a].d != ds[b].d {
			return ds[a].d < ds[b].d
		}
		return ds[a].i < ds[b].i
	})
	keep := max(1, len(ds)*9/10)
	var sx, sy float64
	for _, e := range ds[:keep] {
		sx += pts[e.i][0]
		sy += pts[e.i][1]
	}
	return sx / float64(keep), sy / float64(keep)
}

func capNext(c int) int {
	v := capStart
	for int(math.Ceil(v)) <= c {
		v *= capGrowth
	}
	return int(math.Ceil(v))
}
