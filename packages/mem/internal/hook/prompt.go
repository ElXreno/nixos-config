package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"mem/internal/env"
	"mem/internal/recall"
	"mem/internal/session"
	"mem/internal/statefile"
	"mem/internal/store"
)

const (
	degradedMark   = "~degraded:"
	recoveredMark  = "~recovered:"
	noticeDegraded = "degraded"
	degradedText   = "Agent memory: recall for this prompt was lexical only (embedder %s), so it misses paraphrases and prompts that are not in English. Search yourself with English keywords: `mem search WORDS`."
	pointerMark    = "~ptr:"
	maxQueryRunes  = 8000
	transcriptScan = 8 << 20
	transcriptStep = 256 << 10
)

var slashCommand = regexp.MustCompile(`^/[A-Za-z0-9][A-Za-z0-9:._-]*(\s|$)`)

var quoted = regexp.MustCompile(`"[^"]*"`)

var nonUserMarkers = []string{"<agent-message", "<task-notification", "<system-reminder", "<local-command-"}

func nonUserSource(prompt string, extra []string) string {
	for _, m := range append(slices.Clone(nonUserMarkers), extra...) {
		if m != "" && strings.HasPrefix(prompt, m) {
			return m
		}
	}
	return ""
}

func isSlashCommand(prompt string) bool {
	return slashCommand.MatchString(strings.TrimSpace(prompt))
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func tailRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}

func userPromptSubmit(ctx context.Context, cfg config, e env.Env, in Input) result {
	var r result
	now := e.Now()
	prompt := strings.TrimSpace(in.Prompt)
	switch {
	case prompt == "":
		r.status = "skip-empty"
		return r
	case isSlashCommand(prompt):
		r.status = "skip-command"
		return r
	}
	if src := nonUserSource(prompt, cfg.skipPrefixes); src != "" {
		preview, cut := statefile.PreviewOf(prompt)
		r.turn = &statefile.Turn{
			Time: now.UTC().Format(time.RFC3339), Event: statefile.EventPrompt, Session: in.SessionID, Agent: in.AgentID,
			PromptSHA: promptSHA(prompt), Prompt: preview, PromptCut: cut, PromptChars: utf8.RuneCountInString(prompt), Source: src,
		}
		r.status = statusSkipped
		r.note("source %s", src)
		return r
	}
	query := clip(prompt, maxQueryRunes)
	if utf8.RuneCountInString(prompt) < cfg.short && in.TranscriptPath != "" {
		if tail := lastAssistantTail(in.TranscriptPath, cfg.tail); tail != "" {
			query += "\n" + tail
			r.note("query +%d chars of the last answer", utf8.RuneCountInString(tail))
		}
	}
	var emb *pendingEmbed
	if cfg.vec.embeds() {
		emb = startEmbed(ctx, cfg.vec, query)
	}
	seen, err := openSeen(e, in)
	if err != nil {
		r.err = err
		return r
	}
	s, ok, err := openStore(ctx, e, false)
	if err != nil {
		r.err = err
		return r
	}
	if !ok {
		r.status = "no-db"
		return r
	}
	defer func() { _ = s.Close() }()
	loc := here(e, in, cfg)
	exclude, pointerSeen := seenSplit(seen)
	preview, cut := statefile.PreviewOf(prompt)
	r.turn = &statefile.Turn{
		Time: now.UTC().Format(time.RFC3339), Event: statefile.EventPrompt, Session: in.SessionID, Agent: in.AgentID,
		PromptSHA: promptSHA(prompt), Prompt: preview, PromptCut: cut, PromptChars: utf8.RuneCountInString(prompt),
		Mode: liveModeOf(cfg), Pool: cfg.vec.pool,
	}
	var liveKeys, livePointers []string
	if cfg.vec.shadowActive() {
		job := shadowJob{cfg: cfg, e: e, in: in, prompt: prompt, query: query, loc: loc, exclude: slices.Clone(exclude), pointerSeen: maps.Clone(pointerSeen), emb: emb, liveMode: liveModeOf(cfg), liveKeys: &liveKeys, livePointers: &livePointers, liveTurn: r.turn}
		r.after = job.run
	}
	inj, err := pickRecords(ctx, cfg, e, s, query, loc, exclude, pointerSeen, seen, emb, &r)
	if err != nil {
		r.err = err
		return r
	}
	notice, mark := degradedNotice(cfg, seen, r.turn.Embed)
	if notice != "" {
		r.seen, r.turn.Notice = seen, noticeDegraded
		if mark != "" {
			r.keys = append(r.keys, mark)
		}
		r.note("notice embed=%s", r.turn.Embed)
	}
	if inj.n > 0 && len(inj.Dropped) > 0 {
		r.note("over %d chars, left out %s", cfg.recall.MaxChars, strings.Join(inj.Dropped, ","))
	}
	if inj.n == 0 || inj.Text == "" {
		r.status = "none"
		if notice != "" {
			r.out = &Output{HookEventName: eventNames[UserPromptSubmit], AdditionalContext: notice}
		}
		return r
	}
	r.out = &Output{HookEventName: eventNames[UserPromptSubmit], AdditionalContext: strings.Join(nonEmpty(inj.Text, notice), "\n")}
	r.seen = seen
	r.keys = append(r.keys, inj.Keys...)
	for _, k := range inj.pointers {
		r.keys = append(r.keys, pointerMark+k)
	}
	liveKeys, livePointers = inj.Keys, inj.pointers
	inj.fillTurn(r.turn)
	r.status = "inject"
	r.note("keys %s chars %d", joinLabels(inj.labels), utf8.RuneCountInString(inj.Text))
	if len(inj.pointers) > 0 {
		r.note("pointers %s", joinLabels(inj.pointerLabels))
	}
	if l := inj.rerankLabels(); len(l) > 0 {
		r.note("rerank %s", joinLabels(l))
	}
	return r
}

func pickRecords(ctx context.Context, cfg config, e env.Env, s *store.Store, query string, loc store.Here, exclude []string, pointerSeen map[string]bool, seen *session.Seen, emb *pendingEmbed, r *result) (chosen, error) {
	hits, err := s.Search(ctx, cfg.searchQuery(query, loc, exclude))
	if err != nil {
		return chosen{}, fmt.Errorf("search: %s", quoted.ReplaceAllString(err.Error(), `"..."`))
	}
	var fresh []store.Hit
	for _, h := range hits {
		if seen == nil || !seen.Has(h.Record.Key) {
			fresh = append(fresh, h)
		}
	}
	cands := recall.Candidates(query, fresh)
	lexInj := cfg.recall.Inject(query, cands)
	inj := chosen{Packed: lexInj.Packed, n: len(lexInj.Picks)}
	for _, p := range lexInj.Picks {
		inj.labels = append(inj.labels, fmt.Sprintf("%s=%.3f/%d/%d", p.Hit.Record.Key, p.Relevance.Cover, p.Relevance.Shared, p.Relevance.Topic))
	}
	r.note("hits %d", len(hits))
	if emb != nil && cfg.vec.active() {
		if vi, ok := vectorInject(ctx, cfg, e, s, query, loc, exclude, pointerSeen, emb, cfg.recall.Picks(cands), r); ok {
			return vi, nil
		}
	}
	return lexicalExpired(ctx, cfg, s, query, loc, exclude, pointerSeen, inj), nil
}

func lexicalExpired(ctx context.Context, cfg config, s *store.Store, query string, loc store.Here, exclude []string, pointerSeen map[string]bool, inj chosen) chosen {
	if cfg.vec.settings.PointerTake <= 0 {
		return inj
	}
	q := cfg.searchQuery(query, loc, exclude)
	q.Statuses = []store.Status{store.Expired}
	hits, err := s.Search(ctx, q)
	if err != nil {
		return inj
	}
	var recs []store.Record
	for _, c := range cfg.recall.Picks(recall.Candidates(query, hits)) {
		if !pointerSeen[c.Hit.Record.Key] && len(recs) < recall.ExpiredCandidates {
			recs = append(recs, c.Hit.Record)
		}
	}
	if len(recs) == 0 {
		return inj
	}
	room := cfg.recall.MaxChars - utf8.RuneCountInString(inj.Text)
	if inj.Text != "" {
		room--
	}
	packed := recall.PackPointers(recs, room)
	if packed.Text == "" {
		return inj
	}
	inj.Text = strings.Join(nonEmpty(inj.Text, packed.Text), "\n")
	inj.pointers = packed.Keys
	for _, k := range packed.Keys {
		inj.pointerLabels = append(inj.pointerLabels, k+"=lex-expired")
	}
	inj.n += len(packed.Keys)
	return inj
}

type transcriptLine struct {
	Type              string `json:"type"`
	IsSidechain       bool   `json:"isSidechain"`
	IsAPIErrorMessage bool   `json:"isApiErrorMessage"`
	Message           struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

func assistantText(raw []byte) string {
	if !bytes.Contains(raw, []byte(`"assistant"`)) {
		return ""
	}
	var l transcriptLine
	if json.Unmarshal(raw, &l) != nil || l.Type != "assistant" || l.IsSidechain || l.IsAPIErrorMessage {
		return ""
	}
	var s string
	if json.Unmarshal(l.Message.Content, &s) == nil {
		return strings.TrimSpace(s)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(l.Message.Content, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			parts = append(parts, strings.TrimSpace(b.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func lastAssistantTail(path string, n int) string {
	if n <= 0 {
		return ""
	}
	var found string
	backward(path, transcriptScan, func(line []byte) bool {
		found = assistantText(line)
		return found != ""
	})
	return tailRunes(found, n)
}

func backward(path string, limit int64, visit func([]byte) bool) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return
	}
	floor := max(st.Size()-limit, 0)
	var carry []byte
	for end := st.Size(); end > floor; {
		start := max(end-transcriptStep, floor)
		buf := make([]byte, end-start, int(end-start)+len(carry))
		if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
			return
		}
		buf = append(buf, carry...)
		lines := bytes.Split(buf, []byte("\n"))
		first := 0
		if start > floor {
			carry, first = lines[0], 1
		} else if start > 0 {
			first = 1
		}
		for i := len(lines) - 1; i >= first; i-- {
			if line := bytes.TrimSpace(lines[i]); len(line) > 0 && visit(line) {
				return
			}
		}
		end = start
	}
}

func outages(seen *session.Seen) (down, up int) {
	if seen == nil {
		return 0, 0
	}
	for _, k := range seen.Keys() {
		switch {
		case strings.HasPrefix(k, degradedMark):
			down++
		case strings.HasPrefix(k, recoveredMark):
			up++
		}
	}
	return down, up
}

func degradedNotice(cfg config, seen *session.Seen, outcome string) (string, string) {
	if !cfg.vec.active() || outcome == "" {
		return "", ""
	}
	down, up := outages(seen)
	if outcome == outcomeOK {
		if down > up {
			_ = seen.Add(recoveredMark + strconv.Itoa(up+1))
		}
		return "", ""
	}
	if down > up {
		return "", ""
	}
	return fmt.Sprintf(degradedText, outcome), degradedMark + strconv.Itoa(down+1)
}
