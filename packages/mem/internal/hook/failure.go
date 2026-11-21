package hook

import (
	"context"
	"slices"
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
	failureTool         = "ToolFailure:"
	failureMark         = "~fail:"
	failureHeader       = "Agent memory: possibly related to this failure, not loaded (`mem get KEY` prints one).\n"
	failureExit         = "Exit code "
	failureTake         = 3
	failureCommandRunes = 300
	failureErrorRunes   = 1200
	failureMinRunes     = 24
)

func failureQuery(in Input) string {
	text := strings.TrimSpace(in.Error)
	if first, rest, _ := strings.Cut(text, "\n"); strings.HasPrefix(first, failureExit) {
		text = strings.TrimSpace(rest)
	}
	if utf8.RuneCountInString(text) < failureMinRunes {
		return ""
	}
	return strings.TrimSpace(clip(subject(in), failureCommandRunes) + "\n" + tailRunes(text, failureErrorRunes))
}

func seenSplit(seen *session.Seen) ([]string, map[string]bool) {
	var exclude []string
	pointerSeen := map[string]bool{}
	if seen == nil {
		return nil, pointerSeen
	}
	for _, k := range seen.Keys() {
		if p, ok := strings.CutPrefix(k, pointerMark); ok {
			pointerSeen[p] = true
		} else if !strings.HasPrefix(k, "~") {
			exclude = append(exclude, k)
		}
	}
	slices.Sort(exclude)
	return exclude, pointerSeen
}

func toolFailure(ctx context.Context, cfg config, e env.Env, in Input) result {
	var r result
	if in.IsInterrupt {
		r.status = "skip-interrupt"
		return r
	}
	query := failureQuery(in)
	if query == "" {
		r.status = "skip-short"
		return r
	}
	seen, err := openSeen(e, in)
	if err != nil {
		r.err = err
		return r
	}
	mark := failureMark + promptSHA(query)
	if seen != nil && seen.Has(mark) {
		r.status = "skip-repeat"
		return r
	}
	var emb *pendingEmbed
	if cfg.vec.active() {
		emb = startEmbed(ctx, cfg.vec, query)
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
	if seen != nil {
		if err := seen.Add(mark); err != nil {
			r.note("session state: %v", err)
		}
	}
	exclude, pointerSeen := seenSplit(seen)
	r.turn = &statefile.Turn{
		Time: e.Now().UTC().Format(time.RFC3339), Event: statefile.EventTool, Session: in.SessionID, Agent: in.AgentID,
		Tool: clip(failureTool+in.ToolName, maxToolName), Input: toolPreview(in), Mode: liveModeOf(cfg), Pool: cfg.vec.pool,
	}
	inj, err := pickRecords(ctx, cfg, e, s, query, here(e, in, cfg), exclude, pointerSeen, seen, emb, &r)
	if err != nil {
		r.err = err
		return r
	}
	var recs []store.Record
	for _, k := range append(slices.Clone(inj.Keys), inj.pointers...) {
		if len(recs) == failureTake {
			break
		}
		if pointerSeen[k] || slices.ContainsFunc(recs, func(x store.Record) bool { return x.Key == k }) {
			continue
		}
		rec, err := s.Get(ctx, k)
		if err != nil {
			continue
		}
		recs = append(recs, rec)
	}
	packed := recall.PackPointers(recs, cfg.recall.MaxChars)
	if packed.Text == "" {
		r.status = "none"
		return r
	}
	text := strings.Replace(packed.Text, recall.PointerHeader, failureHeader, 1)
	r.out = &Output{HookEventName: eventNames[ToolFailure], AdditionalContext: text}
	r.seen = seen
	for _, k := range packed.Keys {
		r.keys = append(r.keys, pointerMark+k)
		cos, ok := inj.cos[k]
		if !ok {
			cos = inj.pointerCos[k]
		}
		r.turn.Pointers = append(r.turn.Pointers, inj.key(k, cos))
	}
	r.turn.Keys, r.turn.Chars = slices.Clone(packed.Keys), utf8.RuneCountInString(text)
	r.status = "inject"
	r.note("tool %s pointers %s chars %d", in.ToolName, strings.Join(packed.Keys, ","), utf8.RuneCountInString(text))
	return r
}
