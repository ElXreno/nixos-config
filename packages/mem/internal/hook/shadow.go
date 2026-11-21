package hook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"time"
	"unicode/utf8"

	"mem/internal/env"
	"mem/internal/recall"
	"mem/internal/statefile"
	"mem/internal/store"
	"mem/internal/vec"
)

const (
	ShadowName    = statefile.ShadowName
	liveLexical   = "lexical"
	liveVector    = "vector"
	promptSHALen  = 8
	maxShadowSize = 16 << 20
)

type shadowHit struct {
	Key  string  `json:"key"`
	Cos  float64 `json:"cos"`
	Log  bool    `json:"log"`
	Gate string  `json:"gate"`
}

type shadowLine struct {
	Time        string      `json:"time"`
	Session     string      `json:"session"`
	Agent       string      `json:"agent,omitempty"`
	Prompt      string      `json:"prompt_sha"`
	PromptChars int         `json:"prompt_chars"`
	Pool        string      `json:"pool"`
	LiveMode    string      `json:"live_mode"`
	LiveKeys    []string    `json:"live_keys"`
	Hits        []shadowHit `json:"shadow_hits"`
	ShadowKeys  []string    `json:"shadow_keys"`
	Embed       string      `json:"embed"`
	EmbedMs     int64       `json:"embed_ms"`
	Error       string      `json:"error,omitempty"`

	LiveRerank   string          `json:"live_rerank,omitempty"`
	LiveRerankMs int64           `json:"live_rerank_ms,omitempty"`
	LiveReranked []statefile.Key `json:"live_reranked,omitempty"`

	*shadowPointers
}

type shadowPointers struct {
	LivePointers   []string `json:"live_pointers"`
	ShadowPointers []string `json:"shadow_pointers"`
}

type shadowJob struct {
	cfg          config
	e            env.Env
	in           Input
	prompt       string
	query        string
	loc          store.Here
	exclude      []string
	pointerSeen  map[string]bool
	emb          *pendingEmbed
	liveMode     string
	liveKeys     *[]string
	livePointers *[]string
	liveTurn     *statefile.Turn
}

func promptSHA(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:promptSHALen])
}

func roundCos(c float64) float64 { return math.Round(c*1e4) / 1e4 }

func (j shadowJob) run(ctx context.Context) {
	line := shadowLine{
		Time:        j.e.Now().UTC().Format(time.RFC3339),
		Session:     j.in.SessionID,
		Agent:       j.in.AgentID,
		Prompt:      promptSHA(j.prompt),
		PromptChars: utf8.RuneCountInString(j.prompt),
		Pool:        string(j.cfg.vec.shadow),
		LiveMode:    j.liveMode,
		LiveKeys:    append([]string{}, *j.liveKeys...),
		Hits:        []shadowHit{},
		ShadowKeys:  []string{},
	}
	if j.cfg.vec.settings.PointersConfigured() {
		line.shadowPointers = &shadowPointers{LivePointers: append([]string{}, *j.livePointers...), ShadowPointers: []string{}}
	}
	j.decide(ctx, &line)
	if t := j.liveTurn; t != nil {
		line.LiveRerank, line.LiveRerankMs, line.LiveReranked = t.Rerank, t.RerankMs, t.Reranked
	}
	if err := appendShadow(j.e, line); err != nil {
		writeLog(j.e, UserPromptSubmit, j.in.SessionID, "error", "shadow: "+err.Error(), 0)
	}
}

func (j shadowJob) decide(ctx context.Context, line *shadowLine) {
	res, ok := j.emb.wait(ctx)
	if !ok {
		line.Embed = vec.FailTimeout
		return
	}
	line.EmbedMs = res.took.Milliseconds()
	if res.err != nil {
		line.Embed = outcomeOf(res.err)
		return
	}
	s, found, err := openStore(ctx, j.e, false)
	if err != nil || !found {
		line.Embed = outcomeError
		line.Error = "database not readable"
		if err != nil {
			line.Error = oneLine(err.Error(), maxLogDetail)
		}
		return
	}
	defer func() { _ = s.Close() }()
	vs := j.cfg.vec.settings
	vs.Pool = j.cfg.vec.shadow
	rk, outcome := rankVectors(ctx, s, j.cfg, vs, j.query, j.loc, j.exclude, res.vec)
	if outcome != "" {
		line.Embed = outcome
		return
	}
	built := rk.built
	line.Embed = outcomeOK
	var reg, logs []recall.VHit
	for _, h := range built {
		if h.Log {
			logs = append(logs, h)
		} else {
			reg = append(reg, h)
		}
	}
	verdict := map[string]string{}
	for i, g := range vs.Reg.Judge(reg) {
		verdict[reg[i].Key] = g
	}
	for i, g := range vs.Log.Judge(logs) {
		verdict[logs[i].Key] = g
	}
	for _, h := range built {
		line.Hits = append(line.Hits, shadowHit{Key: h.Key, Cos: roundCos(h.Cos), Log: h.Log, Gate: verdict[h.Key]})
	}
	sel := vs.Gate(built)
	strong := map[string]bool{}
	for _, h := range append(append([]recall.VHit{}, sel.Reg...), sel.Log...) {
		line.ShadowKeys = append(line.ShadowKeys, h.Key)
		strong[h.Key] = true
	}
	if line.shadowPointers == nil {
		return
	}
	ptrs := cosinePointers(vs, built, rk.expired, func(h recall.VHit) bool { return strong[h.Key] || j.pointerSeen[h.Key] })
	for _, h := range append(append([]recall.VHit{}, ptrs.Reg...), ptrs.Log...) {
		line.ShadowPointers = append(line.ShadowPointers, h.Key)
		for i := range line.Hits {
			if line.Hits[i].Key == h.Key {
				line.Hits[i].Gate = recall.GatePointer
			}
		}
	}
}

func appendShadow(e env.Env, line shadowLine) error {
	return statefile.Append(e.StateDir(), ShadowName, maxShadowSize, line)
}

func liveModeOf(cfg config) string {
	if cfg.vec.active() {
		return liveVector
	}
	return liveLexical
}
