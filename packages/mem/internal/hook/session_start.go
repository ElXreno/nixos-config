package hook

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"mem/internal/env"
	"mem/internal/pending"
	"mem/internal/recall"
	"mem/internal/session"
	"mem/internal/statefile"
	"mem/internal/store"
)

const (
	mapMarker     = "~map"
	standingChars = 4000
)

func sessionStart(ctx context.Context, cfg config, e env.Env, in Input) result {
	var r result
	r.turn = &statefile.Turn{Time: e.Now().UTC().Format(time.RFC3339), Event: statefile.EventSession, Session: in.SessionID, Agent: in.AgentID, Source: in.Source}
	for _, w := range prewarmAll(ctx, cfg.vec.url, cfg.vec.rerank.url) {
		r.note("%s", w)
	}
	seen, err := openSeen(e, in)
	if err != nil {
		r.err = err
		return r
	}
	if seen != nil && (in.Source == "clear" || in.Source == "compact") {
		if err := seen.Reset(); err != nil {
			r.note("session reset: %v", err)
		}
	}
	if n, err := session.Prune(e.StateDir(), pruneAge, time.Now()); err != nil {
		r.note("prune: %v", err)
	} else if n > 0 {
		r.note("pruned %d session files", n)
	}
	if n, err := pending.Prune(e.StateDir(), pruneAge, time.Now()); err != nil {
		r.note("prune pending: %v", err)
	} else if n > 0 {
		r.note("pruned %d pending queues", n)
	}
	writable := true
	s, ok, err := openStore(ctx, e, true)
	if err != nil {
		r.note("open writable: %v", err)
		writable = false
		s, ok, err = openStore(ctx, e, false)
	}
	if err != nil {
		r.err = err
		return r
	}
	if !ok {
		r.status = "no-db"
		return r
	}
	defer func() { _ = s.Close() }()
	if !writable {
		r.note("expire skipped")
	} else if expired, err := s.Expire(ctx); err != nil {
		r.note("expire: %v", err)
	} else if len(expired) > 0 {
		keys := make([]string, len(expired))
		for i, x := range expired {
			keys[i] = x.Key
		}
		r.note("expired %s", strings.Join(keys, ","))
		if _, err := s.WriteCore(ctx, e.CorePath()); err != nil {
			r.note("core: %v", err)
		}
	}
	fresh := seen == nil || !seen.Has(mapMarker)
	var parts []string
	var keys []string
	if fresh && !cfg.noUsage {
		parts = append(parts, store.UsageHeader+"\n")
	}
	if scopes := localScopes(e, in); len(scopes) > 0 {
		recs, err := s.CoreAt(ctx, here(e, in, cfg))
		if err != nil {
			r.err = err
			return r
		}
		var pick []store.Record
		for _, x := range recs {
			if x.HasScope(store.GlobalScope) || (seen != nil && seen.Has(x.Key)) {
				continue
			}
			pick = append(pick, x)
		}
		header := "Agent memory: core records for " + strings.Join(scopes, " and ") +
			" (current versions; `mem get KEY --history` shows earlier ones).\n"
		p := recall.Pack(header, pick, "", cfg.maxChars)
		if len(p.Dropped) > 0 {
			r.note("over %d chars, left out %s", cfg.maxChars, strings.Join(p.Dropped, ","))
		}
		if p.Text != "" {
			parts = append(parts, p.Text)
			keys = append(keys, p.Keys...)
		}
	}
	if fresh {
		recs, err := s.StandingAt(ctx, here(e, in, cfg))
		if err != nil {
			r.note("standing: %v", err)
		} else if text := store.RenderStanding(recs, standingChars); text != "" {
			parts = append(parts, text)
			r.note("standing %d rules %d chars", len(recs), utf8.RuneCountInString(text))
		}
	}
	if fresh && cfg.mapChars > 0 {
		entries, err := s.TopicMap(ctx, cfg.mapKeywords)
		if err != nil {
			r.note("topic map: %v", err)
		} else if text := store.RenderTopicMap(entries, cfg.mapChars); text != "" {
			parts = append(parts, text)
			r.turn.Topics = len(entries)
			r.note("map %d topics %d chars", len(entries), utf8.RuneCountInString(text))
		}
	}
	if len(parts) == 0 {
		return r
	}
	if fresh && (!cfg.noUsage || cfg.mapChars > 0) {
		keys = append(keys, mapMarker)
	}
	ctxText := strings.Join(parts, "\n")
	r.out = &Output{HookEventName: eventNames[SessionStart], AdditionalContext: ctxText}
	r.seen, r.keys = seen, keys
	r.turn.Keys, r.turn.Chars = slices.DeleteFunc(slices.Clone(keys), func(k string) bool { return k == mapMarker }), utf8.RuneCountInString(ctxText)
	r.status = "inject"
	r.note("keys %s chars %d", strings.Join(keys, ","), utf8.RuneCountInString(ctxText))
	return r
}
