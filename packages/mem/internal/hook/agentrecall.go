package hook

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"mem/internal/env"
	"mem/internal/recall"
	"mem/internal/session"
	"mem/internal/statefile"
)

const (
	agentRecallTool = "AgentRecall:"
	harnessPrefix   = "[Workflow harness"
	harnessRelay    = "user request]"
	taskLines       = 4
	maxTaskBytes    = 8 << 20
)

var agentIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func agentTranscript(in Input) (string, bool) {
	id := strings.TrimPrefix(in.AgentID, "agent-")
	if in.TranscriptPath == "" || !agentIDRe.MatchString(id) || !agentIDRe.MatchString(in.SessionID) {
		return "", false
	}
	dir := filepath.Join(filepath.Dir(in.TranscriptPath), in.SessionID, "subagents")
	name := "agent-" + id + ".jsonl"
	if fi, err := os.Stat(filepath.Join(dir, name)); err == nil && fi.Mode().IsRegular() {
		return filepath.Join(dir, name), false
	}
	runs, err := os.ReadDir(filepath.Join(dir, "workflows"))
	if err != nil {
		return "", false
	}
	for _, run := range runs {
		path := filepath.Join(dir, "workflows", run.Name(), name)
		if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
			return path, true
		}
	}
	return "", false
}

func taskText(s string) string {
	if !strings.HasPrefix(s, harnessPrefix) {
		return strings.TrimSpace(s)
	}
	head, body, ok := strings.Cut(s, "\n")
	end := strings.IndexByte(head, ']')
	if !ok || end < 0 || strings.HasSuffix(head[:end+1], harnessRelay) {
		return ""
	}
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(l, "  ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func userText(raw []byte) (string, bool) {
	var l struct {
		Type    string `json:"type"`
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &l) != nil || l.Type != "user" {
		return "", false
	}
	var s string
	if json.Unmarshal(l.Message.Content, &s) == nil {
		return s, true
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(l.Message.Content, &blocks) != nil {
		return "", false
	}
	var parts []string
	for _, b := range blocks {
		if b.Type != "text" {
			return "", false
		}
		parts = append(parts, b.Text)
	}
	return strings.Join(parts, "\n"), true
}

func agentTask(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	r := bufio.NewReaderSize(io.LimitReader(f, maxTaskBytes), 64<<10)
	var parts []string
	for read := 0; read < taskLines; {
		line, err := r.ReadBytes('\n')
		if err != nil {
			break
		}
		var head struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line, &head) == nil && head.Type != "user" && head.Type != "assistant" {
			continue
		}
		read++
		s, ok := userText(line)
		if !ok {
			break
		}
		if t := taskText(s); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

func seenID(in Input) string {
	if in.AgentID == "" {
		return in.SessionID
	}
	return in.SessionID + ":" + in.AgentID
}

func agentRecall(ctx context.Context, cfg config, e env.Env, in Input, r *result) {
	id := seenID(in)
	if in.SessionID == "" || in.AgentID == "" || session.Claimed(e.StateDir(), id, session.RecallClaim) {
		return
	}
	path, workflow := agentTranscript(in)
	if path == "" {
		return
	}
	task := ""
	if workflow {
		if task = agentTask(path); task == "" {
			return
		}
	}
	if won, err := session.Claim(e.StateDir(), id, session.RecallClaim); err != nil || !won || !workflow {
		return
	}
	failed := true
	defer func() {
		if !failed {
			r.claim = id
		} else if err := session.Unclaim(e.StateDir(), id, session.RecallClaim); err != nil {
			r.note("agent recall: %v", err)
		}
	}()
	agent := agentKey(in.AgentType)
	query := clip(task, maxQueryRunes)
	var emb *pendingEmbed
	if cfg.vec.active() {
		emb = startEmbed(ctx, cfg.vec, query)
	}
	seen, err := openSeen(e, in)
	if err != nil || seen == nil {
		r.note("agent recall: session state unavailable")
		return
	}
	s, ok, err := openStore(ctx, e, false)
	if err != nil || !ok {
		r.note("agent recall: no database")
		return
	}
	defer func() { _ = s.Close() }()
	exclude, pointerSeen := seenSplit(seen)
	exclude = append(exclude, r.keys...)
	slices.Sort(exclude)
	turn := &statefile.Turn{Time: e.Now().UTC().Format(time.RFC3339), Event: statefile.EventTool, Session: in.SessionID, Agent: in.AgentID,
		Tool: clip(agentRecallTool+agent, maxToolName), Mode: liveModeOf(cfg), Pool: cfg.vec.pool}
	scratch := result{turn: turn}
	inj, err := pickRecords(ctx, cfg, e, s, query, here(e, in, cfg), exclude, pointerSeen, seen, emb, &scratch)
	r.detail = append(r.detail, scratch.detail...)
	if err != nil {
		r.note("agent recall: %v", err)
		return
	}
	failed = false
	if inj.n == 0 || inj.Text == "" {
		r.note("agent recall: nothing for %s", agent)
		return
	}
	text := strings.Replace(inj.Text, recall.PromptHeader, subagentHeader, 1)
	if r.out == nil {
		r.out = &Output{HookEventName: eventNames[PreToolUse]}
	}
	r.out.AdditionalContext = strings.Join(nonEmpty(text, r.out.AdditionalContext), "\n")
	r.seen = seen
	r.keys = append(r.keys, inj.Keys...)
	for _, k := range inj.pointers {
		r.keys = append(r.keys, pointerMark+k)
	}
	r.status = "inject"
	r.note("agent recall for %s keys %s chars %d", agent, strings.Join(append(slices.Clone(inj.Keys), inj.pointers...), ","), utf8.RuneCountInString(text))
	inj.fillTurn(turn)
	turn.Keys, turn.Chars = slices.Clone(inj.Keys), utf8.RuneCountInString(text)
	if r.turn == nil {
		r.turn = turn
		return
	}
	t := *turn
	stampTurn(&t, e, cfg, in)
	t.Status = "inject"
	r.extra = append(r.extra, t)
}
