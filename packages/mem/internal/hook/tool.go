package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

var writeTools = map[string]bool{
	"Write":        true,
	"Edit":         true,
	"MultiEdit":    true,
	"NotebookEdit": true,
}

var subjectFields = []string{"command", "file_path", "notebook_path", "path", "url"}

func inputFields(raw json.RawMessage) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	return fields
}

func field(fields map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		var s string
		if json.Unmarshal(fields[k], &s) == nil && s != "" {
			return s
		}
	}
	return ""
}

func subject(in Input) string {
	fields := inputFields(in.ToolInput)
	if s := field(fields, subjectFields...); s != "" {
		return s
	}
	var b bytes.Buffer
	if json.Compact(&b, in.ToolInput) == nil {
		return b.String()
	}
	return string(in.ToolInput)
}

func preToolUseTriggers(ctx context.Context, cfg config, e env.Env, in Input) result {
	var r result
	now := e.Now()
	if writeTools[in.ToolName] {
		target := field(inputFields(in.ToolInput), "file_path", "notebook_path")
		if dir, ok := protectedBy(e, cwdOf(e, in), target); ok {
			where := fmt.Sprintf("%s is inside %s, which only the memory store writes.", target, dir)
			if dir == filepath.Clean(expandHome(e, e.CorePath())) {
				where = fmt.Sprintf("%s is generated from the memory store by `mem core`.", target)
			}
			r.out = &Output{
				HookEventName:      eventNames[PreToolUse],
				PermissionDecision: "deny",
				PermissionDecisionReason: where + " Records change through `mem put` " +
					"(`mem put --help` lists the flags; `mem get KEY` and `mem search QUERY` read them).",
			}
			r.status = "deny"
			r.note("tool %s", in.ToolName)
			return r
		}
	}
	s, ok, err := openStore(ctx, e, false)
	if err != nil {
		r.err = err
		return r
	}
	if !ok {
		return r
	}
	defer func() { _ = s.Close() }()
	hits, err := s.MatchTriggersAt(ctx, in.ToolName, subject(in), here(e, in, cfg))
	if err != nil {
		r.err = err
		return r
	}
	if len(hits) == 0 {
		return r
	}
	seen, err := openSeen(e, in)
	if err != nil {
		r.err = err
		return r
	}
	once, err := triggerSet(e, in, cfg, seen)
	if err != nil {
		r.err = err
		return r
	}
	var pick []store.Record
	for _, h := range hits {
		if seen != nil && seen.Has(h.Record.Key) || once != nil && once.Has(triggerMark+h.Record.Key) {
			continue
		}
		pick = append(pick, h.Record)
	}
	if len(pick) == 0 {
		return r
	}
	header := "Agent memory: records whose trigger matches this " + in.ToolName + " call.\n"
	p := recall.Pack(header, pick, subject(in), cfg.maxChars)
	ctxText, keys := p.Text, p.Keys
	if len(p.Dropped) > 0 {
		r.note("over %d chars, left out %s", cfg.maxChars, strings.Join(p.Dropped, ","))
	}
	if ctxText == "" {
		return r
	}
	r.out = &Output{HookEventName: eventNames[PreToolUse], AdditionalContext: ctxText}
	r.seen, r.keys = seen, keys
	if once != nil {
		r.also = &marks{seen: once}
		for _, k := range keys {
			r.also.keys = append(r.also.keys, triggerMark+k)
		}
	}
	r.status = "inject"
	r.note("tool %s keys %s chars %d", in.ToolName, strings.Join(keys, ","), utf8.RuneCountInString(ctxText))
	r.turn = &statefile.Turn{
		Time: now.UTC().Format(time.RFC3339), Event: statefile.EventTool, Session: in.SessionID, Agent: in.AgentID,
		Tool: clip(in.ToolName, maxToolName), Keys: slices.Clone(keys), Chars: utf8.RuneCountInString(ctxText),
		Input: toolPreview(in), Fired: firedOf(hits, keys),
	}
	return r
}

const (
	triggerMark        = "~trig:"
	triggerOnceSession = "session"
	triggerOnceAgent   = "agent"
)

type marks struct {
	seen *session.Seen
	keys []string
}

func triggerSet(e env.Env, in Input, cfg config, seen *session.Seen) (*session.Seen, error) {
	if seen == nil || cfg.triggerOnce == triggerOnceAgent || in.AgentID == "" {
		return seen, nil
	}
	return session.Open(e.StateDir(), in.SessionID)
}

func expandHome(e env.Env, p string) string {
	if p == "~" {
		return e.Home()
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(e.Home(), rest)
	}
	return p
}

func absPath(e env.Env, cwd, p string) string {
	p = expandHome(e, p)
	if !filepath.IsAbs(p) {
		if cwd == "" {
			return ""
		}
		p = filepath.Join(cwd, p)
	}
	return filepath.Clean(p)
}

func hasMeta(p string) bool {
	return strings.ContainsAny(p, "*?[")
}

const maxLinks = 40

func resolveExisting(p string) string {
	return resolveLinks(p, 0)
}

func resolveLinks(p string, depth int) string {
	rest := ""
	for d := p; ; d = filepath.Dir(d) {
		if real, err := filepath.EvalSymlinks(d); err == nil {
			return filepath.Join(real, rest)
		}
		if fi, err := os.Lstat(d); err == nil && fi.Mode()&os.ModeSymlink != 0 && depth < maxLinks {
			if target, err := os.Readlink(d); err == nil {
				if !filepath.IsAbs(target) {
					dir := filepath.Dir(d)
					if real, err := filepath.EvalSymlinks(dir); err == nil {
						dir = real
					}
					target = filepath.Join(dir, target)
				}
				return resolveLinks(filepath.Join(target, rest), depth+1)
			}
		}
		parent := filepath.Dir(d)
		if parent == d {
			return p
		}
		rest = filepath.Join(filepath.Base(d), rest)
	}
}

func patternVariants(pattern string) []string {
	out := []string{pattern}
	parts := strings.Split(pattern, string(filepath.Separator))
	static := len(parts)
	for i, part := range parts {
		if hasMeta(part) {
			static = i
			break
		}
	}
	prefix := strings.Join(parts[:static], string(filepath.Separator))
	if prefix == "" {
		return out
	}
	real, err := filepath.EvalSymlinks(prefix)
	if err != nil || real == prefix {
		return out
	}
	return append(out, filepath.Join(append([]string{real}, parts[static:]...)...))
}

func within(path, pattern string) bool {
	if pattern == string(filepath.Separator) {
		return true
	}
	if !hasMeta(pattern) {
		return path == pattern || strings.HasPrefix(path, pattern+string(filepath.Separator))
	}
	n := strings.Count(pattern, string(filepath.Separator))
	parts := strings.Split(path, string(filepath.Separator))
	if len(parts) <= n {
		return false
	}
	ok, err := filepath.Match(pattern, strings.Join(parts[:n+1], string(filepath.Separator)))
	return err == nil && ok
}

func protectedBy(e env.Env, cwd, target string) (string, bool) {
	if target == "" {
		return "", false
	}
	p := absPath(e, cwd, target)
	if p == "" {
		return "", false
	}
	paths := []string{p}
	if real := resolveExisting(p); real != p {
		paths = append(paths, real)
	}
	for _, dir := range e.ProtectedDirs() {
		pattern := expandHome(e, dir)
		if !filepath.IsAbs(pattern) {
			continue
		}
		pattern = filepath.Clean(pattern)
		for _, v := range patternVariants(pattern) {
			for _, c := range paths {
				if within(c, v) {
					return pattern, true
				}
			}
		}
	}
	return "", false
}
