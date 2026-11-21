package hook

import (
	"context"
	"fmt"
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
	defaultPendingTTL = 2 * time.Minute
	defaultAgentType  = "general-purpose"
	forkAgentType     = "fork"
	subagentHeader    = "Agent memory: records related to your task (`mem get KEY` prints one in full, `mem search QUERY` finds more).\n"
	maxAgentType      = 256
)

func isAgentTool(name string) bool { return name == "Agent" || name == "Task" }

func agentKey(t string) string { return clip(strings.TrimSpace(t), maxAgentType) }

func launchType(in Input) string {
	if t := agentKey(field(inputFields(in.ToolInput), "subagent_type")); t != "" {
		return t
	}
	return defaultAgentType
}

func queueSubagentRecall(ctx context.Context, cfg config, e env.Env, in Input) (notes []string) {
	defer func() {
		if p := recover(); p != nil {
			notes = append(notes, fmt.Sprintf("subagent queue: panic: %v", p))
		}
	}()
	agent := launchType(in)
	prompt := strings.TrimSpace(field(inputFields(in.ToolInput), "prompt"))
	switch {
	case in.SessionID == "":
		return []string{"subagent queue skipped: no session id"}
	case pending.Fold(agent) == forkAgentType:
		return []string{"subagent queue skipped: fork inherits the context"}
	case prompt == "":
		return []string{"subagent queue skipped: no prompt"}
	}
	query := clip(prompt, maxQueryRunes)
	var emb *pendingEmbed
	if cfg.vec.active() {
		emb = startEmbed(ctx, cfg.vec, query)
	}
	s, ok, err := openStore(ctx, e, false)
	if err != nil {
		return []string{fmt.Sprintf("subagent queue: %v", err)}
	}
	if !ok {
		return []string{"subagent queue skipped: no database"}
	}
	defer func() { _ = s.Close() }()
	var r result
	inj, err := pickRecords(ctx, cfg, e, s, query, here(e, in, cfg), nil, map[string]bool{}, nil, emb, &r)
	notes = r.detail
	if err != nil {
		return append(notes, err.Error())
	}
	if inj.n == 0 || inj.Text == "" {
		return append(notes, "subagent queue: nothing to hand over for "+agent)
	}
	text := strings.Replace(inj.Text, recall.PromptHeader, subagentHeader, 1)
	entry := pending.Entry{Agent: agent, Text: text, Keys: slices.Clone(inj.Keys), Pointers: slices.Clone(inj.pointers)}
	if err := pending.Enqueue(e.StateDir(), in.SessionID, entry, e.Now()); err != nil {
		return append(notes, fmt.Sprintf("subagent queue: %v", err))
	}
	return append(notes, fmt.Sprintf("subagent queued for %s keys %s chars %d", agent, strings.Join(entry.Keys, ","), utf8.RuneCountInString(text)))
}

func preToolUse(ctx context.Context, cfg config, e env.Env, in Input) result {
	var notes []string
	if isAgentTool(in.ToolName) {
		notes = queueSubagentRecall(ctx, cfg, e, in)
	}
	r := preToolUseTriggers(ctx, cfg, e, in)
	r.detail = append(notes, r.detail...)
	if cfg.agentRecall && r.err == nil && (r.out == nil || r.out.PermissionDecision == "") {
		agentRecall(ctx, cfg, e, in, &r)
	}
	return r
}

func subagentStart(ctx context.Context, cfg config, e env.Env, in Input) result {
	var r result
	agent := agentKey(in.AgentType)
	if pending.Fold(agent) == forkAgentType {
		r.status = "skip-fork"
		return r
	}
	var seen *session.Seen
	if in.AgentID != "" {
		var err error
		if seen, err = openSeen(e, in); err != nil {
			r.note("session state: %v", err)
			seen = nil
		}
	}
	if seen != nil && seen.Has(mapMarker) {
		r.status = "none"
		r.note("agent already served")
		return r
	}
	var parts, keys []string
	if in.SessionID != "" && agent != "" {
		entry, ok, err := pending.Dequeue(e.StateDir(), in.SessionID, agent, cfg.pendingTTL, e.Now())
		switch {
		case err != nil:
			r.note("queue: %v", err)
		case ok:
			parts = append(parts, entry.Text)
			keys = append(keys, entry.Keys...)
			for _, p := range entry.Pointers {
				keys = append(keys, pointerMark+p)
			}
			r.note("dequeued for %s keys %s", agent, strings.Join(entry.Keys, ","))
		default:
			r.note("queue empty for %q", agent)
		}
	}
	if !cfg.noMap && cfg.mapChars > 0 {
		if text, n, err := topicMapText(ctx, e, cfg); err != nil {
			r.note("topic map: %v", err)
		} else if text != "" {
			parts = append(parts, text)
			r.note("map %d topics %d chars", n, utf8.RuneCountInString(text))
		}
	}
	if len(parts) == 0 {
		r.status = "none"
		return r
	}
	text := strings.Join(parts, "\n")
	r.out = &Output{HookEventName: eventNames[SubagentStart], AdditionalContext: text}
	r.seen, r.keys = seen, append(keys, mapMarker)
	r.status = "inject"
	r.turn = &statefile.Turn{
		Time: e.Now().UTC().Format(time.RFC3339), Event: statefile.EventTool, Session: in.SessionID, Agent: in.AgentID,
		Tool: clip("SubagentStart:"+agent, maxToolName), Keys: slices.Clone(keys), Chars: utf8.RuneCountInString(text),
	}
	return r
}

func topicMapText(ctx context.Context, e env.Env, cfg config) (string, int, error) {
	s, ok, err := openStore(ctx, e, false)
	if err != nil || !ok {
		return "", 0, err
	}
	defer func() { _ = s.Close() }()
	entries, err := s.TopicMap(ctx, cfg.mapKeywords)
	if err != nil {
		return "", 0, err
	}
	return store.RenderTopicMap(entries, cfg.mapChars), len(entries), nil
}
