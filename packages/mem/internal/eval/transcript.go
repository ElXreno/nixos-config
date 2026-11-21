package eval

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const HookTranscriptScan = 8 << 20

type Prompt struct {
	Session      string
	Turn         int
	Text         string
	Query        string
	Queued       bool
	ShortTail    bool
	Gold         []string
	Written      []string
	IndexReads   int
	MemoryUse    bool
	Start        time.Time
	BashGold     []string
	SubGold      []string
	WrittenAll   []string
	ScriptWrites []string
	TailTouched  bool
}

type Counts struct {
	Files         int `json:"files"`
	Lines         int `json:"lines"`
	BadLines      int `json:"bad_lines"`
	Boundaries    int `json:"turns"`
	Genuine       int `json:"genuine_prompts"`
	Queued        int `json:"genuine_queued_mid_turn"`
	NotHuman      int `json:"excluded_not_human"`
	Tagged        int `json:"excluded_tagged"`
	Slash         int `json:"excluded_slash"`
	Interrupted   int `json:"excluded_interrupted"`
	Empty         int `json:"excluded_empty"`
	Sidechain     int `json:"sidechain_entries"`
	MemoryReads   int `json:"memory_reads"`
	IndexReads    int `json:"index_reads"`
	OtherMemReads int `json:"other_memory_reads"`
	OrphanReads   int `json:"reads_outside_genuine_turns"`
	PromptsRead   int `json:"prompts_with_memory_reads"`
	Late          int `json:"entries_after_until"`
	OtherUses     int `json:"other_memory_uses"`
	SubagentFiles int `json:"subagent_files"`
	SubagentUses  int `json:"subagent_memory_uses"`
	PromptsUsing  int `json:"prompts_with_other_memory_use"`
}

func (c *Counts) add(o Counts) {
	c.Files += o.Files
	c.Lines += o.Lines
	c.BadLines += o.BadLines
	c.Boundaries += o.Boundaries
	c.Genuine += o.Genuine
	c.Queued += o.Queued
	c.NotHuman += o.NotHuman
	c.Tagged += o.Tagged
	c.Slash += o.Slash
	c.Interrupted += o.Interrupted
	c.Empty += o.Empty
	c.Sidechain += o.Sidechain
	c.MemoryReads += o.MemoryReads
	c.IndexReads += o.IndexReads
	c.OtherMemReads += o.OtherMemReads
	c.OrphanReads += o.OrphanReads
	c.PromptsRead += o.PromptsRead
	c.Late += o.Late
	c.OtherUses += o.OtherUses
	c.SubagentFiles += o.SubagentFiles
	c.SubagentUses += o.SubagentUses
	c.PromptsUsing += o.PromptsUsing
}

type Legacy struct {
	Dirs    []string
	Indexes map[string]bool
	Slugs   map[string]bool
}

func (l Legacy) slug(path string) (string, bool) {
	if !strings.HasSuffix(path, ".md") || !filepath.IsAbs(path) {
		return "", false
	}
	path = filepath.Clean(path)
	if !slices.Contains(l.Dirs, filepath.Dir(path)) {
		return "", false
	}
	return strings.TrimSuffix(filepath.Base(path), ".md"), true
}

type ParseOptions struct {
	MinChars     int
	Tail         int
	MaxQuery     int
	Home         string
	Until        time.Time
	SubagentUses []time.Time
	SubAccess    []Access
}

type Parsed struct {
	Prompts []Prompt
	Counts  Counts
	Gold    GoldCounts
	Writes  []Access
}

var (
	userType      = []byte(`"type":"user"`)
	assistantType = []byte(`"type":"assistant"`)
	queuedType    = []byte(`"type":"queued_command"`)
	toolResultKey = []byte(`"toolUseResult":`)
	tagRe         = regexp.MustCompile(`^\s*<[a-z][a-z0-9_-]*(\s[^>]*)?>`)
	slashRe       = regexp.MustCompile(`^/[A-Za-z0-9][A-Za-z0-9:._-]*(\s|$)`)
	otherMemRe    = regexp.MustCompile(`/\.claude/projects/[^/]+/memory/`)
)

type entry struct {
	Type             string                 `json:"type"`
	Timestamp        string                 `json:"timestamp"`
	IsSidechain      bool                   `json:"isSidechain"`
	IsMeta           bool                   `json:"isMeta"`
	IsCompactSummary bool                   `json:"isCompactSummary"`
	IsAPIError       bool                   `json:"isApiErrorMessage"`
	Origin           *struct{ Kind string } `json:"origin"`
	PromptSource     string                 `json:"promptSource"`
	ToolUseResult    json.RawMessage        `json:"toolUseResult"`
	Message          struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	Attachment *struct {
		Type        string                 `json:"type"`
		CommandMode string                 `json:"commandMode"`
		Prompt      json.RawMessage        `json:"prompt"`
		IsMeta      bool                   `json:"isMeta"`
		Origin      *struct{ Kind string } `json:"origin"`
	} `json:"attachment"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func (b contentBlock) path() string {
	var in struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	}
	if json.Unmarshal(b.Input, &in) != nil {
		return ""
	}
	if in.FilePath != "" {
		return in.FilePath
	}
	return in.NotebookPath
}

var memoryUseRe = regexp.MustCompile(`\.claude/projects/[^/"\s]+/memory\b|MEMORY\.md|\bmem (get|search|list)\b`)

var (
	scriptWriteRe = regexp.MustCompile(`\.write_text\(|\.write_bytes\(|\.write\(|\bopen\([^)]*,\s*(mode\s*=\s*)?["'][^"']*[wax+][^"']*["']|\bos\.(rename|replace|remove|unlink)\(|\bshutil\.(copy|copy2|copyfile|move|rmtree)\(|\.(rename|unlink|touch)\(|writeFileSync|appendFileSync|renameSync|unlinkSync|\bFile\.(write|open)\b|\bsed\s+-[a-zA-Z]*i|\bperl\s+-[a-zA-Z]*i|\bmv\b|\brm\b|\btee\b|>>?\s*(["$~]|/[^d])`)
	slugTokenRe   = regexp.MustCompile(`[A-Za-z0-9_][A-Za-z0-9_.-]*`)
)

func isOpaqueRunner(name string) bool {
	return isInterpreter(name) || name == "xargs" || name == "parallel" || name == "find" || name == "fd"
}

func (l Legacy) mentioned(cmd string) []string {
	var out []string
	for _, tok := range slugTokenRe.FindAllString(cmd, -1) {
		if s := strings.TrimSuffix(tok, ".md"); l.Slugs[s] {
			out = addSlug(out, s)
		}
	}
	return out
}

func (l Legacy) own(input []byte) bool {
	for _, d := range l.Dirs {
		if bytes.Contains(input, []byte(".claude/projects/"+filepath.Base(filepath.Dir(d))+"/memory")) || bytes.Contains(input, []byte(d)) {
			return true
		}
	}
	return false
}

func (l Legacy) uses(input []byte) bool {
	if memoryUseRe.Match(input) {
		return true
	}
	for _, d := range l.Dirs {
		if bytes.Contains(input, []byte(d)) {
			return true
		}
	}
	return false
}

func blocksOf(raw json.RawMessage) (string, []contentBlock, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return "", nil, false
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return "", nil, false
		}
		return s, nil, true
	}
	var bs []contentBlock
	if json.Unmarshal(raw, &bs) != nil {
		return "", nil, false
	}
	var texts []string
	for _, b := range bs {
		if b.Type == "text" {
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, "\n"), bs, true
}

type boundary struct {
	queued       bool
	text         string
	tail         string
	hasOrigin    bool
	human        bool
	heuristic    bool
	start        time.Time
	gold         []string
	written      []string
	indexReads   int
	memoryUse    bool
	bashGold     []string
	subGold      []string
	writtenAll   []string
	scriptWrites []string
}

func visibleTail(text string, at, boundaryAt int64) string {
	if boundaryAt > HookTranscriptScan && at <= boundaryAt-HookTranscriptScan {
		return ""
	}
	return text
}

func readLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if len(line) > 0 && errors.Is(err, io.EOF) {
		err = nil
	}
	return line, err
}

func addSlug(list []string, s string) []string {
	if slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

func clip(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func lastRunes(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}

func expandHome(p, home string) string {
	if home != "" && (p == "~" || strings.HasPrefix(p, "~/")) {
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

func ParseTranscript(r io.Reader, session string, legacy Legacy, opt ParseOptions) ([]Prompt, Counts, error) {
	p, err := ParseTranscriptFull(r, session, legacy, opt)
	return p.Prompts, p.Counts, err
}

func ownerIndex(turns []*boundary, t time.Time) int {
	owner := -1
	for i, b := range turns {
		if !b.start.IsZero() && !b.start.After(t) && (owner < 0 || b.start.After(turns[owner].start)) {
			owner = i
		}
	}
	return owner
}

func ParseTranscriptFull(r io.Reader, session string, legacy Legacy, opt ParseOptions) (Parsed, error) {
	var c Counts
	var gc GoldCounts
	var writes []Access
	c.Files = 1
	br := bufio.NewReaderSize(r, 1<<20)
	var turns []*boundary
	var cur *boundary
	lastText := ""
	var offset, lastAt int64
	fileHasOrigin := false
	for {
		line, err := readLine(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Parsed{Counts: c}, err
		}
		c.Lines++
		lineAt := offset
		offset += int64(len(line))
		isUser := bytes.Contains(line, userType)
		isQueued := !isUser && bytes.Contains(line, queuedType)
		if !isUser && !isQueued && !bytes.Contains(line, assistantType) {
			continue
		}
		if isUser && bytes.Contains(line, toolResultKey) {
			continue
		}
		var e entry
		if json.Unmarshal(line, &e) != nil {
			c.BadLines++
			continue
		}
		if e.Type != "user" && e.Type != "assistant" && e.Type != "attachment" {
			continue
		}
		if late(e.Timestamp, opt.Until) {
			c.Late++
			continue
		}
		if e.IsSidechain {
			c.Sidechain++
			continue
		}
		if e.Type == "attachment" {
			a := e.Attachment
			if a == nil || a.Type != "queued_command" || a.CommandMode != "prompt" {
				continue
			}
			text, _, ok := blocksOf(a.Prompt)
			if !ok {
				continue
			}
			b := &boundary{queued: true, text: strings.TrimSpace(text), tail: visibleTail(lastText, lastAt, lineAt), hasOrigin: a.Origin != nil, start: stampTime(e.Timestamp)}
			if a.Origin != nil {
				fileHasOrigin = true
				b.human = a.Origin.Kind == "human" && !a.IsMeta
			}
			turns = append(turns, b)
			cur = b
			continue
		}
		text, blocks, ok := blocksOf(e.Message.Content)
		if !ok {
			continue
		}
		if e.Type == "user" {
			if len(e.ToolUseResult) > 0 || e.IsMeta || e.IsCompactSummary {
				continue
			}
			if slices.ContainsFunc(blocks, func(b contentBlock) bool { return b.Type == "tool_result" }) {
				continue
			}
			b := &boundary{text: strings.TrimSpace(text), tail: visibleTail(lastText, lastAt, lineAt), hasOrigin: e.Origin != nil, start: stampTime(e.Timestamp)}
			if e.Origin != nil {
				fileHasOrigin = true
				b.human = e.Origin.Kind == "human"
			}
			b.heuristic = e.PromptSource == "" || e.PromptSource == "typed" || e.PromptSource == "queued"
			turns = append(turns, b)
			cur = b
			continue
		}
		for _, bl := range blocks {
			switch bl.Type {
			case "text":
				if strings.TrimSpace(bl.Text) != "" && !e.IsAPIError {
					lastText = bl.Text
					lastAt = lineAt
				}
			case "tool_use":
				path := expandHome(bl.path(), opt.Home)
				slug, inLegacy := legacy.slug(path)
				use := legacy.uses(bl.Input)
				switch bl.Name {
				case "Read":
					switch {
					case inLegacy && legacy.Indexes[slug]:
						c.IndexReads++
						use = false
						if cur != nil {
							cur.indexReads++
						}
					case inLegacy:
						c.MemoryReads++
						use = false
						if cur == nil {
							c.OrphanReads++
							continue
						}
						cur.gold = addSlug(cur.gold, slug)
					case otherMemRe.MatchString(path):
						c.OtherMemReads++
						use = true
					}
				case "Write", "Edit", "MultiEdit", "NotebookEdit":
					if inLegacy {
						use = true
						if cur != nil {
							cur.written = addSlug(cur.written, slug)
						}
					}
				}
				for _, a := range accessesOf(bl, legacy, opt.Home, &gc) {
					if a.Kind == AccessRead {
						continue
					}
					a.At, a.Session = stampTime(e.Timestamp), session
					if a.Kind == AccessWrite || a.Kind == AccessScript {
						writes = append(writes, a)
						if cur != nil {
							cur.addWrite(a)
						}
						continue
					}
					switch {
					case a.Index:
						gc.BashIndexReads++
					case cur == nil:
						gc.BashOrphan++
					default:
						cur.bashGold = addSlug(cur.bashGold, a.Slug)
					}
				}
				if use {
					c.OtherUses++
					if cur != nil {
						cur.memoryUse = true
					}
				}
			}
		}
	}
	for _, t := range opt.SubagentUses {
		if i := ownerIndex(turns, t); i >= 0 {
			turns[i].memoryUse = true
		}
	}
	for _, a := range opt.SubAccess {
		owner := -1
		if !a.Spawn.IsZero() {
			owner = ownerIndex(turns, a.Spawn)
		}
		if owner < 0 {
			owner = ownerIndex(turns, a.At)
		}
		if owner >= 0 && ownerIndex(turns, a.At) != owner {
			gc.SubCrossTurn++
		}
		if a.Kind == AccessWrite || a.Kind == AccessScript {
			gc.SubWrites++
			writes = append(writes, a)
			if owner >= 0 {
				turns[owner].addWrite(a)
			}
			continue
		}
		switch {
		case a.Index:
			gc.SubIndexReads++
		case owner < 0:
			gc.SubUnattributed++
		case a.Kind == AccessBashRead:
			gc.SubBashReads++
			turns[owner].bashGold = addSlug(turns[owner].bashGold, a.Slug)
		default:
			gc.SubReads++
			turns[owner].subGold = addSlug(turns[owner].subGold, a.Slug)
		}
	}
	kinds := make([]turnKind, len(turns))
	for i, b := range turns {
		kinds[i] = classify(b, fileHasOrigin)
	}
	runWrites := make([][]string, len(turns))
	runScripts := make([][]string, len(turns))
	tailTouched := make([]bool, len(turns))
	var acc, accScripts []string
	tail := false
	for i := len(turns) - 1; i >= 0; i-- {
		b := turns[i]
		acc = appendSlugs(acc, b.writtenAll)
		accScripts = appendSlugs(accScripts, b.scriptWrites)
		runWrites[i] = slices.Clone(acc)
		runScripts[i] = slices.Clone(accScripts)
		if kinds[i] == kindNotHuman {
			tail = tail || b.touches()
			continue
		}
		if kinds[i] == kindGenuine {
			tailTouched[i] = tail
		}
		tail = false
		if !b.queued {
			acc, accScripts = nil, nil
		}
	}
	var out []Prompt
	for i, b := range turns {
		c.Boundaries++
		switch kinds[i] {
		case kindNotHuman:
			c.NotHuman++
		case kindEmpty:
			c.Empty++
		case kindInterrupted:
			c.Interrupted++
		case kindTagged:
			c.Tagged++
		case kindSlash:
			c.Slash++
		default:
			c.Genuine++
			if b.queued {
				c.Queued++
			}
			if len(b.gold) > 0 {
				c.PromptsRead++
			}
			if b.memoryUse && len(b.gold) == 0 && b.indexReads == 0 {
				c.PromptsUsing++
			}
			p := Prompt{Session: session, Turn: i + 1, Text: b.text, Query: clip(b.text, opt.MaxQuery), Queued: b.queued, Gold: b.gold, Written: b.written,
				IndexReads: b.indexReads, MemoryUse: b.memoryUse, Start: b.start, BashGold: b.bashGold, SubGold: b.subGold, WrittenAll: runWrites[i], ScriptWrites: runScripts[i], TailTouched: tailTouched[i]}
			if utf8.RuneCountInString(b.text) < opt.MinChars {
				if tail := lastRunes(b.tail, opt.Tail); tail != "" {
					p.Query += "\n" + tail
					p.ShortTail = true
				}
			}
			out = append(out, p)
			continue
		}
		if len(b.gold) > 0 {
			c.OrphanReads += len(b.gold)
		}
	}
	return Parsed{Prompts: out, Counts: c, Gold: gc, Writes: writes}, nil
}

type turnKind uint8

const (
	kindGenuine turnKind = iota
	kindNotHuman
	kindEmpty
	kindInterrupted
	kindTagged
	kindSlash
)

func classify(b *boundary, fileHasOrigin bool) turnKind {
	genuine := b.human
	if !b.hasOrigin {
		genuine = !fileHasOrigin && b.heuristic
	}
	switch {
	case !genuine:
		return kindNotHuman
	case b.text == "":
		return kindEmpty
	case strings.HasPrefix(b.text, "[Request interrupted"), strings.HasPrefix(b.text, "This session is being continued"):
		return kindInterrupted
	case tagRe.MatchString(b.text):
		return kindTagged
	case slashRe.MatchString(b.text):
		return kindSlash
	}
	return kindGenuine
}

func (b *boundary) touches() bool {
	return b.memoryUse || b.indexReads > 0 || len(b.gold) > 0 || len(b.bashGold) > 0 || len(b.subGold) > 0 || len(b.writtenAll) > 0 || len(b.scriptWrites) > 0
}

func (b *boundary) addWrite(a Access) {
	if a.Kind == AccessScript {
		b.scriptWrites = addSlug(b.scriptWrites, a.Slug)
		return
	}
	b.writtenAll = addSlug(b.writtenAll, a.Slug)
}

func appendSlugs(list, more []string) []string {
	for _, s := range more {
		list = addSlug(list, s)
	}
	return list
}

func late(stamp string, until time.Time) bool {
	if until.IsZero() || stamp == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, stamp)
	return err == nil && t.After(until)
}

func stampTime(stamp string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return time.Time{}
	}
	return t
}

var toolUseType = []byte(`"type":"tool_use"`)

type bashInput struct {
	Command string `json:"command"`
}

func accessesOf(bl contentBlock, lg Legacy, home string, gc *GoldCounts) []Access {
	switch bl.Name {
	case "Read", "Write", "Edit", "MultiEdit", "NotebookEdit":
		slug, ok := lg.slug(expandHome(bl.path(), home))
		if !ok {
			return nil
		}
		kind := AccessWrite
		if bl.Name == "Read" {
			kind = AccessRead
		}
		return []Access{{Slug: slug, Kind: kind, Tool: bl.Name, Index: lg.Indexes[slug]}}
	case "Bash":
		if !lg.uses(bl.Input) {
			return nil
		}
		var in bashInput
		if json.Unmarshal(bl.Input, &in) != nil {
			return nil
		}
		if !lg.own(bl.Input) {
			gc.BashOther++
			return nil
		}
		gc.BashCalls++
		scan := ScanShell(in.Command, home, func(p string) bool { _, ok := lg.slug(p); return ok })
		var out []Access
		seen := map[Access]bool{}
		for _, ef := range scan.Effects {
			slug, _ := lg.slug(ef.Path)
			a := Access{Slug: slug, Kind: AccessBashRead, Tool: "Bash:" + ef.Op, Index: lg.Indexes[slug]}
			if ef.Write {
				a.Kind = AccessWrite
				gc.BashWrites++
			} else {
				gc.BashReads++
			}
			if !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
		switch {
		case len(out) > 0:
			gc.BashWithEffect++
		default:
			gc.BashNoEffect++
			if slices.ContainsFunc(scan.Commands, isInterpreter) {
				gc.BashScript++
			}
		}
		if slices.ContainsFunc(scan.Commands, isOpaqueRunner) && scriptWriteRe.MatchString(in.Command) {
			for _, slug := range lg.mentioned(in.Command) {
				a := Access{Slug: slug, Kind: AccessScript, Tool: "Bash:script", Index: lg.Indexes[slug]}
				if !seen[a] {
					seen[a] = true
					out = append(out, a)
					gc.BashScriptWrites++
				}
			}
		}
		return out
	}
	return nil
}

type SubScan struct {
	Uses   []time.Time
	Access []Access
	Files  int
	Gold   GoldCounts
}

func scanSubagent(path, session string, legacy Legacy, home string, until time.Time, out *SubScan) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	br := bufio.NewReaderSize(f, 1<<20)
	var spawn time.Time
	for {
		line, err := readLine(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if spawn.IsZero() {
			var first struct{ Timestamp string }
			if json.Unmarshal(line, &first) == nil {
				spawn = stampTime(first.Timestamp)
			}
		}
		if !bytes.Contains(line, toolUseType) || !legacy.uses(line) {
			continue
		}
		var e entry
		if json.Unmarshal(line, &e) != nil || e.Type != "assistant" || late(e.Timestamp, until) {
			continue
		}
		t := stampTime(e.Timestamp)
		if t.IsZero() {
			continue
		}
		_, blocks, ok := blocksOf(e.Message.Content)
		if !ok {
			continue
		}
		if slices.ContainsFunc(blocks, func(b contentBlock) bool { return b.Type == "tool_use" && legacy.uses(b.Input) }) {
			out.Uses = append(out.Uses, t)
		}
		for _, b := range blocks {
			if b.Type != "tool_use" {
				continue
			}
			for _, a := range accessesOf(b, legacy, home, &out.Gold) {
				a.At, a.Spawn, a.Session, a.Sub = t, spawn, session, true
				out.Access = append(out.Access, a)
			}
		}
	}
}

func SubagentScan(dir, session string, legacy Legacy, home string, until time.Time) (SubScan, error) {
	var out SubScan
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path == dir {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		out.Files++
		return scanSubagent(path, session, legacy, home, until, &out)
	})
	return out, err
}
