package eval

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const failureExit = "Exit code "

var (
	toolUseMark   = []byte(`"tool_use"`)
	isErrorMark   = []byte(`"is_error"`)
	bashMark      = []byte(`"Bash"`)
	memKeyRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:#-]*$`)
	fdRedirectRe  = regexp.MustCompile(`^[0-9]+$`)
	globMetaChars = "*?["
)

type failure struct {
	Time    time.Time
	Session string
	Agent   string
	Cwd     string
	File    string
	Command string
	Error   string
	call    int
}

type failureScan struct {
	calls  []string
	msgs   []string
	events []failure
}

type txLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"sessionId"`
	AgentID   string `json:"agentId"`
	Cwd       string `json:"cwd"`
	Message   struct {
		ID      string          `json:"id"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type txBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
	Text      string          `json:"text"`
}

type pendingCall struct {
	idx                 int
	command             string
	session, agent, cwd string
}

func txBlocks(raw json.RawMessage) []txBlock {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	var bs []txBlock
	if json.Unmarshal(raw, &bs) != nil {
		return nil
	}
	return bs
}

func resultText(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", false
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return "", false
		}
		return s, true
	}
	var bs []txBlock
	if json.Unmarshal(raw, &bs) != nil {
		return "", false
	}
	var texts []string
	for _, b := range bs {
		if b.Type == "text" {
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, "\n"), true
}

func scanFailures(r io.Reader, file string) (failureScan, error) {
	var out failureScan
	br := bufio.NewReaderSize(r, 1<<20)
	calls := map[string]pendingCall{}
	for {
		line, err := readLine(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return out, err
		}
		isUse := bytes.Contains(line, toolUseMark) && bytes.Contains(line, bashMark)
		isResult := bytes.Contains(line, isErrorMark)
		if !isUse && !isResult {
			continue
		}
		var l txLine
		if json.Unmarshal(line, &l) != nil {
			continue
		}
		switch l.Type {
		case "assistant":
			for _, b := range txBlocks(l.Message.Content) {
				if b.Type != "tool_use" || b.Name != "Bash" {
					continue
				}
				var in bashInput
				if json.Unmarshal(b.Input, &in) != nil {
					continue
				}
				calls[b.ID] = pendingCall{idx: len(out.calls), command: in.Command, session: l.SessionID, agent: l.AgentID, cwd: l.Cwd}
				out.calls = append(out.calls, in.Command)
				out.msgs = append(out.msgs, l.Message.ID)
			}
		case "user":
			at := stampTime(l.Timestamp)
			for _, b := range txBlocks(l.Message.Content) {
				if b.Type != "tool_result" || !b.IsError {
					continue
				}
				c, ok := calls[b.ToolUseID]
				if !ok {
					continue
				}
				text, ok := resultText(b.Content)
				if !ok || !strings.HasPrefix(text, failureExit) || at.IsZero() {
					continue
				}
				f := failure{Time: at, Session: l.SessionID, Agent: l.AgentID, Cwd: l.Cwd, File: file, Command: c.command, Error: text, call: c.idx}
				if f.Session == "" {
					f.Session = c.session
				}
				if f.Agent == "" {
					f.Agent = c.agent
				}
				if f.Cwd == "" {
					f.Cwd = c.cwd
				}
				out.events = append(out.events, f)
			}
		}
	}
}

func (s failureScan) lookups(ev failure, horizon int) []string {
	var keys []string
	msg := s.msgs[ev.call]
	n := 0
	for j := ev.call + 1; j < len(s.calls) && n < horizon; j++ {
		if msg != "" && s.msgs[j] == msg {
			continue
		}
		n++
		for _, k := range memGetKeys(s.calls[j]) {
			keys = addSlug(keys, k)
		}
	}
	return keys
}

type shTok struct {
	s  string
	op bool
}

type shLexer struct {
	src     string
	i       int
	toks    []shTok
	word    strings.Builder
	inWord  bool
	pending []string
}

func (l *shLexer) flush() {
	if l.inWord {
		l.toks = append(l.toks, shTok{s: l.word.String()})
		l.word.Reset()
		l.inWord = false
	}
}

func (l *shLexer) emit(op string) {
	l.flush()
	l.toks = append(l.toks, shTok{s: op, op: true})
}

func (l *shLexer) skipHeredocs() {
	for len(l.pending) > 0 {
		delim := l.pending[0]
		l.pending = l.pending[1:]
		for l.i < len(l.src) {
			end := strings.IndexByte(l.src[l.i:], '\n')
			var lineText string
			if end < 0 {
				lineText, l.i = l.src[l.i:], len(l.src)
			} else {
				lineText, l.i = l.src[l.i:l.i+end], l.i+end+1
			}
			if strings.TrimSpace(lineText) == delim {
				break
			}
		}
	}
}

func (l *shLexer) heredocWord() {
	for l.i < len(l.src) && (l.src[l.i] == ' ' || l.src[l.i] == '\t') {
		l.i++
	}
	var b strings.Builder
	for l.i < len(l.src) {
		c := l.src[l.i]
		switch {
		case c == '\'' || c == '"':
			end := strings.IndexByte(l.src[l.i+1:], c)
			if end < 0 {
				b.WriteString(l.src[l.i+1:])
				l.i = len(l.src)
				continue
			}
			b.WriteString(l.src[l.i+1 : l.i+1+end])
			l.i += end + 2
		case c == '\\' && l.i+1 < len(l.src):
			b.WriteByte(l.src[l.i+1])
			l.i += 2
		case strings.IndexByte(" \t\n;&|()<>", c) >= 0:
			l.pending = append(l.pending, b.String())
			return
		default:
			b.WriteByte(c)
			l.i++
		}
	}
	l.pending = append(l.pending, b.String())
}

func shellTokens(src string) []shTok {
	l := &shLexer{src: src}
	for l.i < len(l.src) {
		c := l.src[l.i]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			l.flush()
			l.i++
		case c == '\n':
			l.emit("\n")
			l.i++
			l.skipHeredocs()
		case c == '#' && !l.inWord:
			for l.i < len(l.src) && l.src[l.i] != '\n' {
				l.i++
			}
		case c == '\\':
			if l.i+1 < len(l.src) && l.src[l.i+1] == '\n' {
				l.i += 2
				continue
			}
			if l.i+1 < len(l.src) {
				l.word.WriteByte(l.src[l.i+1])
			}
			l.inWord = true
			l.i += 2
		case c == '\'':
			end := strings.IndexByte(l.src[l.i+1:], '\'')
			l.inWord = true
			if end < 0 {
				l.word.WriteString(l.src[l.i+1:])
				l.i = len(l.src)
				continue
			}
			l.word.WriteString(l.src[l.i+1 : l.i+1+end])
			l.i += end + 2
		case c == '"':
			l.inWord = true
			l.i++
			for l.i < len(l.src) && l.src[l.i] != '"' {
				if l.src[l.i] == '\\' && l.i+1 < len(l.src) && strings.IndexByte("\"\\$`", l.src[l.i+1]) >= 0 {
					l.word.WriteByte(l.src[l.i+1])
					l.i += 2
					continue
				}
				l.word.WriteByte(l.src[l.i])
				l.i++
			}
			l.i++
		case c == '<' || c == '>':
			if l.inWord && fdRedirectRe.MatchString(l.word.String()) {
				l.word.Reset()
				l.inWord = false
			}
			if strings.HasPrefix(l.src[l.i:], "<<<") {
				l.emit("<<<")
				l.i += 3
				continue
			}
			if strings.HasPrefix(l.src[l.i:], "<<") {
				l.emit("<<")
				l.i += 2
				if l.i < len(l.src) && l.src[l.i] == '-' {
					l.i++
				}
				l.heredocWord()
				continue
			}
			l.emit(string(c))
			l.i++
		case strings.IndexByte(";&|()`", c) >= 0:
			l.emit(string(c))
			l.i++
		default:
			l.word.WriteByte(c)
			l.inWord = true
			l.i++
		}
	}
	l.flush()
	return l.toks
}

func isMemWord(t shTok) bool {
	return !t.op && (t.s == "mem" || strings.HasSuffix(t.s, "/mem"))
}

func memGetKeys(cmd string) []string {
	if !strings.Contains(cmd, "get") {
		return nil
	}
	toks := shellTokens(cmd)
	var keys []string
	for i := 0; i < len(toks); i++ {
		if !isMemWord(toks[i]) {
			continue
		}
		j := i + 1
		for j < len(toks) && !toks[j].op && strings.HasPrefix(toks[j].s, "-") {
			if toks[j].s == "--db" || toks[j].s == "-db" {
				j++
			}
			j++
		}
		if j >= len(toks) || toks[j].op || toks[j].s != "get" {
			continue
		}
		for j++; j < len(toks) && !toks[j].op; j++ {
			if w := toks[j].s; !strings.HasPrefix(w, "-") && memKeyRe.MatchString(w) {
				keys = addSlug(keys, w)
			}
		}
		i = j - 1
	}
	return keys
}

func hasGlobMeta(s string) bool {
	return strings.ContainsAny(s, globMetaChars)
}

func globFiles(pattern, home string) ([]string, error) {
	pattern = filepath.Clean(expandHome(pattern, home))
	root := "."
	if filepath.IsAbs(pattern) {
		root = string(filepath.Separator)
	}
	segs := strings.Split(strings.TrimPrefix(pattern, string(filepath.Separator)), string(filepath.Separator))
	for _, s := range segs {
		if s != "**" {
			if _, err := filepath.Match(s, ""); err != nil {
				return nil, err
			}
		}
	}
	found := map[string]bool{}
	if err := globWalk(root, segs, found); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(found))
	for f := range found {
		out = append(out, f)
	}
	slices.Sort(out)
	return out, nil
}

func globWalk(dir string, segs []string, found map[string]bool) error {
	if len(segs) == 0 {
		return nil
	}
	seg, rest := segs[0], segs[1:]
	if seg == "**" {
		if err := globWalk(dir, rest, found); err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		for _, e := range entries {
			if e.IsDir() {
				if err := globWalk(filepath.Join(dir, e.Name()), segs, found); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if !hasGlobMeta(seg) {
		return globVisit(filepath.Join(dir, seg), rest, found)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if ok, _ := filepath.Match(seg, e.Name()); ok {
			if err := globVisit(filepath.Join(dir, e.Name()), rest, found); err != nil {
				return err
			}
		}
	}
	return nil
}

func globVisit(path string, rest []string, found map[string]bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if len(rest) == 0 {
		if info.Mode().IsRegular() {
			found[path] = true
		}
		return nil
	}
	if !info.IsDir() {
		return nil
	}
	return globWalk(path, rest, found)
}
