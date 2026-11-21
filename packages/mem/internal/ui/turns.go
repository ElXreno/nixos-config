package ui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"mem/internal/statefile"
)

const (
	defaultTurns = 200
	maxTurns     = 1000
	maxSession   = 256
	maxField     = 128
	maxKeyLen    = 200
	maxPath      = 256
	maxKeys      = 100
	sourceTurns  = "turns"
	sourceLog    = "hook.log"
	shadowBefore = 2 * time.Second
	shadowAfter  = 60 * time.Second
	logPrompt    = "user-prompt-submit"
	logTool      = "pre-tool-use"
	logSession   = "session-start"
	logError     = "error"
	logInject    = "inject"
	logNone      = "none"
)

type shadowOut struct {
	Pool     string          `json:"pool"`
	Embed    string          `json:"embed"`
	EmbedMs  int64           `json:"embed_ms"`
	Keys     []statefile.Key `json:"keys"`
	Pointers []statefile.Key `json:"pointers"`
	Only     []statefile.Key `json:"only"`
	cands    []statefile.Key
}

type turnOut struct {
	statefile.Turn
	ID     string     `json:"id"`
	Origin string     `json:"origin"`
	Shadow *shadowOut `json:"shadow"`
	at     time.Time
}

type turnsOut struct {
	Turns   []turnOut `json:"turns"`
	Cursor  string    `json:"cursor"`
	Skipped int       `json:"skipped"`
}

type sessionOut struct {
	Session string `json:"session"`
	Prompts int    `json:"prompts"`
	Tools   int    `json:"tools"`
	Starts  int    `json:"starts"`
	First   string `json:"first"`
	Last    string `json:"last"`
	Project string `json:"project,omitempty"`
	Host    string `json:"host,omitempty"`
	Harness string `json:"harness,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Test    bool   `json:"test,omitempty"`
}

func (s *Server) handleTurns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := intParam(q, "limit", defaultTurns, maxTurns)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	session := q.Get("session")
	if len(session) > maxSession {
		writeError(w, http.StatusBadRequest, "session must be at most "+strconv.Itoa(maxSession)+" bytes")
		return
	}
	var since *statefile.Cursor
	if v := q.Get("since"); v != "" {
		c, err := statefile.ParseCursor(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be a cursor returned by /api/turns")
			return
		}
		since = &c
	}
	out, err := s.turns(since, session, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "state files unreadable")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSessions(w http.ResponseWriter, _ *http.Request) {
	all, err := s.snapshot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "state files unreadable")
		return
	}
	by := map[string]*sessionOut{}
	var at = map[string][2]time.Time{}
	for _, t := range all {
		so := by[t.Session]
		if so == nil {
			so = &sessionOut{Session: t.Session}
			by[t.Session] = so
		}
		switch t.Event {
		case statefile.EventTool:
			so.Tools++
		case statefile.EventSession:
			so.Starts++
		default:
			so.Prompts++
		}
		if t.Project != "" || t.Host != "" {
			so.Project, so.Host, so.Harness, so.Cwd = t.Project, t.Host, t.Harness, t.Cwd
		}
		span, seen := at[t.Session]
		if !seen || t.at.Before(span[0]) {
			span[0], so.First = t.at, t.Time
		}
		if !seen || !t.at.Before(span[1]) {
			span[1], so.Last = t.at, t.Time
		}
		at[t.Session] = span
	}
	out := make([]sessionOut, 0, len(by))
	for _, so := range by {
		so.Test = !uuidLike.MatchString(so.Session)
		out = append(out, *so)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := at[out[i].Session][1], at[out[j].Session][1]
		if !a.Equal(b) {
			return a.After(b)
		}
		return out[i].Session < out[j].Session
	})
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func (s *Server) readTurns(since *statefile.Cursor) ([]turnOut, statefile.Cursor, error) {
	if s.opt.StateDir == "" {
		return nil, statefile.Cursor{}, nil
	}
	lines, next, err := statefile.Read(s.opt.StateDir, statefile.TurnsName, since)
	if err != nil {
		return nil, next, err
	}
	out := make([]turnOut, 0, len(lines))
	for _, l := range lines {
		if t, ok := decodeTurn(l); ok {
			out = append(out, t)
		}
	}
	return out, next, nil
}

func (s *Server) turns(since *statefile.Cursor, session string, limit int) (turnsOut, error) {
	out := turnsOut{Turns: []turnOut{}}
	turns, next, err := s.readTurns(since)
	if err != nil {
		return out, err
	}
	out.Cursor = next.String()
	if since == nil {
		legacy, err := s.legacyTurns(turns)
		if err != nil {
			return out, err
		}
		turns = append(legacy, turns...)
	}
	picked := []turnOut{}
	for _, t := range turns {
		if session == "" || t.Session == session {
			picked = append(picked, t)
		}
	}
	if len(picked) > limit {
		out.Skipped = len(picked) - limit
		picked = picked[len(picked)-limit:]
	}
	s.joinShadow(picked)
	out.Turns = picked
	return out, nil
}

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func clipKeys(ks []string) []string {
	out := make([]string, 0, min(len(ks), maxKeys))
	for _, k := range ks {
		if len(out) == maxKeys {
			break
		}
		if k != "" {
			out = append(out, clipRunes(k, maxKeyLen))
		}
	}
	return out
}

func clipCos(ks []statefile.Key) []statefile.Key {
	out := make([]statefile.Key, 0, min(len(ks), maxKeys))
	for _, k := range ks {
		if len(out) == maxKeys {
			break
		}
		if k.Key != "" {
			out = append(out, statefile.Key{Key: clipRunes(k.Key, maxKeyLen), Cos: k.Cos, Rerank: k.Rerank})
		}
	}
	return out
}

func clipFired(fs []statefile.Fired) []statefile.Fired {
	out := make([]statefile.Fired, 0, min(len(fs), maxKeys))
	for _, f := range fs {
		if len(out) == maxKeys {
			break
		}
		if f.Key != "" {
			out = append(out, statefile.Fired{Key: clipRunes(f.Key, maxKeyLen), Tool: clipRunes(f.Tool, maxField), Regex: clipRunes(f.Regex, maxField)})
		}
	}
	return out
}

func decodeTurn(l statefile.Line) (turnOut, bool) {
	var t statefile.Turn
	if json.Unmarshal(l.Data, &t) != nil || (t.Event != statefile.EventPrompt && t.Event != statefile.EventTool && t.Event != statefile.EventSession) {
		return turnOut{}, false
	}
	when, err := time.Parse(time.RFC3339, t.Time)
	if err != nil {
		return turnOut{}, false
	}
	t.Session = clipRunes(t.Session, maxField)
	t.Agent = clipRunes(t.Agent, maxField)
	t.Tool = clipRunes(t.Tool, maxField)
	t.Mode = clipRunes(t.Mode, maxField)
	t.Pool = clipRunes(t.Pool, maxField)
	t.Status = clipRunes(t.Status, maxField)
	t.Embed = clipRunes(t.Embed, maxField)
	t.PromptSHA = clipRunes(t.PromptSHA, maxField)
	t.Prompt = clipRunes(t.Prompt, statefile.PreviewRunes)
	t.AgentType, t.Harness, t.Host = clipRunes(t.AgentType, maxField), clipRunes(t.Harness, maxField), clipRunes(t.Host, maxField)
	t.Project, t.Cwd, t.Rerank = clipRunes(t.Project, maxPath), clipRunes(t.Cwd, maxPath), clipRunes(t.Rerank, maxField)
	t.Input = clipRunes(t.Input, statefile.InputRunes)
	t.Fired = clipFired(t.Fired)
	t.Strong, t.Pointers, t.Reranked = clipCos(t.Strong), clipCos(t.Pointers), clipCos(t.Reranked)
	t.Lexical, t.Keys = clipKeys(t.Lexical), clipKeys(t.Keys)
	return turnOut{Turn: t, ID: l.File + ":" + strconv.FormatInt(l.End, 10), Origin: sourceTurns, at: when}, true
}

type shadowHitIn struct {
	Key string  `json:"key"`
	Cos float64 `json:"cos"`
}

type shadowIn struct {
	Time     string        `json:"time"`
	Session  string        `json:"session"`
	Agent    string        `json:"agent"`
	Prompt   string        `json:"prompt_sha"`
	Pool     string        `json:"pool"`
	Hits     []shadowHitIn `json:"shadow_hits"`
	Keys     []string      `json:"shadow_keys"`
	Pointers []string      `json:"shadow_pointers"`
	Embed    string        `json:"embed"`
	EmbedMs  int64         `json:"embed_ms"`
	at       time.Time
	used     bool
}

var uuidLike = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-`)

var shaField = []byte(`"prompt_sha":"`)

func lineSHA(data []byte) string {
	i := bytes.Index(data, shaField)
	if i < 0 {
		return ""
	}
	rest := data[i+len(shaField):]
	j := bytes.IndexByte(rest, '"')
	if j < 0 {
		return ""
	}
	return string(rest[:j])
}

func joinKey(session, agent, sha string) string { return session + "\x00" + agent + "\x00" + sha }

func (s *Server) joinShadow(turns []turnOut) {
	need := map[string]bool{}
	for _, t := range turns {
		if t.Event == statefile.EventPrompt && t.PromptSHA != "" {
			need[t.PromptSHA] = true
		}
	}
	if len(need) == 0 || s.opt.StateDir == "" {
		return
	}
	lines, _, err := statefile.Read(s.opt.StateDir, statefile.ShadowName, nil)
	if err != nil {
		return
	}
	cands := map[string][]*shadowIn{}
	for _, l := range lines {
		if !need[lineSHA(l.Data)] {
			continue
		}
		var sl shadowIn
		if json.Unmarshal(l.Data, &sl) != nil || !need[sl.Prompt] {
			continue
		}
		when, err := time.Parse(time.RFC3339, sl.Time)
		if err != nil {
			continue
		}
		sl.at = when
		k := joinKey(sl.Session, sl.Agent, sl.Prompt)
		cands[k] = append(cands[k], &sl)
	}
	for i := range turns {
		t := &turns[i]
		if t.Event != statefile.EventPrompt || t.PromptSHA == "" {
			continue
		}
		var best *shadowIn
		var bestGap time.Duration
		for _, c := range cands[joinKey(t.Session, t.Agent, t.PromptSHA)] {
			d := c.at.Sub(t.at)
			if c.used || d < -shadowBefore || d > shadowAfter {
				continue
			}
			gap := max(d, -d)
			if best == nil || gap < bestGap {
				best, bestGap = c, gap
			}
		}
		if best != nil {
			best.used = true
			t.Shadow = shadowOf(best, t.Turn)
		}
	}
}

func shadowOf(sl *shadowIn, live statefile.Turn) *shadowOut {
	cos := map[string]float64{}
	for _, h := range sl.Hits {
		if _, ok := cos[h.Key]; !ok {
			cos[h.Key] = h.Cos
		}
	}
	injected := map[string]bool{}
	for _, k := range live.Strong {
		injected[k.Key] = true
	}
	for _, k := range live.Pointers {
		injected[k.Key] = true
	}
	for _, k := range live.Lexical {
		injected[k] = true
	}
	out := &shadowOut{Pool: clipRunes(sl.Pool, maxField), Embed: clipRunes(sl.Embed, maxField), EmbedMs: sl.EmbedMs,
		Keys: []statefile.Key{}, Pointers: []statefile.Key{}, Only: []statefile.Key{}}
	for _, h := range sl.Hits {
		out.cands = append(out.cands, statefile.Key{Key: clipRunes(h.Key, maxKeyLen), Cos: h.Cos})
	}
	seen := map[string]bool{}
	add := func(list *[]statefile.Key, k string) {
		key := statefile.Key{Key: clipRunes(k, maxKeyLen), Cos: cos[k]}
		*list = append(*list, key)
		if !injected[k] && !seen[k] {
			seen[k] = true
			out.Only = append(out.Only, key)
		}
	}
	for _, k := range clipKeys(sl.Keys) {
		add(&out.Keys, k)
	}
	for _, k := range clipKeys(sl.Pointers) {
		add(&out.Pointers, k)
	}
	return out
}

func (s *Server) legacyTurns(turns []turnOut) ([]turnOut, error) {
	if s.opt.StateDir == "" {
		return nil, nil
	}
	var cut time.Time
	for _, t := range turns {
		if cut.IsZero() || t.at.Before(cut) {
			cut = t.at
		}
	}
	lines, _, err := statefile.Read(s.opt.StateDir, statefile.HookLogName, nil)
	if err != nil {
		return nil, err
	}
	var out []turnOut
	for _, l := range lines {
		t, ok := parseLogLine(string(l.Data))
		if !ok || (!cut.IsZero() && !t.at.Before(cut)) {
			continue
		}
		t.ID, t.Origin = "log:"+l.File+":"+strconv.FormatInt(l.End, 10), sourceLog
		out = append(out, t)
	}
	return out, nil
}

type labels struct {
	vec  []statefile.Key
	lex  []string
	seen map[string]bool
}

func (ls *labels) add(label string) {
	k, v, ok := strings.Cut(label, "=")
	if !ok || k == "" || v == "" || ls.seen[k] {
		return
	}
	ls.seen[k] = true
	if c, err := strconv.ParseFloat(v, 64); err == nil && !strings.Contains(v, "/") {
		ls.vec = append(ls.vec, statefile.Key{Key: k, Cos: c})
		return
	}
	ls.lex = append(ls.lex, k)
}

func parseLabels(list string, partial bool) labels {
	ls := labels{seen: map[string]bool{}}
	items := strings.Split(list, ",")
	if partial {
		items = items[:len(items)-1]
	}
	for _, it := range items {
		ls.add(it)
	}
	return ls
}

func parseLogLine(line string) (turnOut, bool) {
	f := strings.SplitN(line, " ", 6)
	if len(f) < 5 {
		return turnOut{}, false
	}
	when, err := time.Parse(time.RFC3339, f[0])
	if err != nil {
		return turnOut{}, false
	}
	sess, ok := strings.CutPrefix(f[2], "session=")
	if !ok {
		return turnOut{}, false
	}
	if sess == "-" {
		sess = ""
	}
	msText, ok := strings.CutSuffix(f[4], "ms")
	ms, err := strconv.ParseInt(msText, 10, 64)
	if !ok || err != nil {
		return turnOut{}, false
	}
	detail := ""
	if len(f) == 6 {
		detail = f[5]
	}
	cut := utf8.RuneCountInString(detail) > statefile.HookLogDetail && strings.HasSuffix(detail, "...")
	if cut {
		detail = strings.TrimSuffix(detail, "...")
	}
	t := turnOut{Turn: statefile.Turn{Time: f[0], Session: clipRunes(sess, maxField), Status: f[3], TotalMs: ms}, at: when}
	notes := strings.Split(detail, "; ")
	if f[3] == logError && (f[1] == logPrompt || f[1] == logTool || f[1] == logSession) {
		t.Event = map[string]string{logPrompt: statefile.EventPrompt, logTool: statefile.EventTool, logSession: statefile.EventSession}[f[1]]
		return t, true
	}
	switch f[1] {
	case logSession:
		t.Event = statefile.EventSession
		t.fromSessionNotes(notes, cut)
		return t, true
	case logPrompt:
		if f[3] != logInject && f[3] != logNone {
			return turnOut{}, false
		}
		t.Event = statefile.EventPrompt
		return t, t.fromPromptNotes(notes, cut)
	case logTool:
		if f[3] != logInject {
			return turnOut{}, false
		}
		t.Event = statefile.EventTool
		for i, n := range notes {
			if strings.HasPrefix(n, "tool ") {
				return t, t.fromToolNote(n, cut && i == len(notes)-1)
			}
		}
	}
	return turnOut{}, false
}

func (t *turnOut) fromPromptNotes(notes []string, cut bool) bool {
	reached := false
	left := map[string]bool{}
	var strong labels
	var ptrs labels
	t.Mode, t.Pool = "lexical", "lexical"
	for i, n := range notes {
		partial := cut && i == len(notes)-1
		switch {
		case strings.HasPrefix(n, "hits "):
			reached = true
		case strings.HasPrefix(n, "embed="):
			t.Mode, t.Pool = "vector", ""
			w := strings.Fields(n)
			t.Embed = clipRunes(strings.TrimPrefix(w[0], "embed="), maxField)
			if len(w) > 1 {
				if ms, err := strconv.ParseInt(strings.TrimSuffix(w[1], "ms"), 10, 64); err == nil {
					t.EmbedMs = ms
				}
			}
			if len(w) > 3 && w[2] == "pool" {
				t.Pool = clipRunes(w[3], maxField)
			}
		case strings.HasPrefix(n, "keys "):
			list, chars, done := strings.Cut(strings.TrimPrefix(n, "keys "), " chars ")
			strong = parseLabels(list, partial && !done)
			if done && !partial {
				t.Chars, _ = strconv.Atoi(chars)
			}
		case strings.HasPrefix(n, "pointers ") && !strings.HasPrefix(n, "pointers over "):
			ptrs = parseLabels(strings.TrimPrefix(n, "pointers "), partial)
		case strings.HasPrefix(n, "over "):
			if _, list, ok := strings.Cut(n, ", left out "); ok {
				for _, k := range parseList(list, partial) {
					left[k] = true
				}
			}
		}
	}
	for _, k := range strong.vec {
		if !left[k.Key] {
			t.Strong = append(t.Strong, statefile.Key{Key: clipRunes(k.Key, maxKeyLen), Cos: k.Cos})
		}
	}
	for _, k := range strong.lex {
		if !left[k] {
			t.Lexical = append(t.Lexical, clipRunes(k, maxKeyLen))
		}
	}
	t.Pointers = clipCos(ptrs.vec)
	return reached
}

func parseList(list string, partial bool) []string {
	items := strings.Split(list, ",")
	if partial {
		items = items[:len(items)-1]
	}
	return clipKeys(items)
}

func (t *turnOut) fromToolNote(n string, cut bool) bool {
	w := strings.Fields(n)
	if len(w) < 4 || w[0] != "tool" || w[2] != "keys" {
		return false
	}
	t.Tool = clipRunes(w[1], maxField)
	t.Keys = parseList(w[3], cut && len(w) == 4)
	if len(w) == 6 && w[4] == "chars" && !cut {
		t.Chars, _ = strconv.Atoi(w[5])
	}
	return len(t.Keys) > 0
}

func (t *turnOut) fromSessionNotes(notes []string, cut bool) {
	for i, n := range notes {
		partial := cut && i == len(notes)-1
		switch {
		case strings.HasPrefix(n, "keys "):
			list, chars, done := strings.Cut(strings.TrimPrefix(n, "keys "), " chars ")
			for _, k := range parseList(list, partial && !done) {
				if !strings.HasPrefix(k, "~") {
					t.Keys = append(t.Keys, k)
				}
			}
			if done && !partial {
				t.Chars, _ = strconv.Atoi(chars)
			}
		case strings.HasPrefix(n, "map "):
			if w := strings.Fields(n); len(w) >= 3 {
				t.Topics, _ = strconv.Atoi(w[1])
			}
		}
	}
}
