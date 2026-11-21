package statefile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

const (
	TurnsName     = "turns.jsonl"
	ShadowName    = "shadow.jsonl"
	HookLogName   = "hook.log"
	HookLogDetail = 400
	MaxTurnsSize  = 16 << 20
	OldSuffix     = ".1"
	PreviewRunes  = 100
	EventPrompt   = "prompt"
	EventTool     = "tool"
	EventSession  = "session"
	InputRunes    = 160

	idBytes   = 8
	maxIDScan = 1 << 20
	maxRead   = 64 << 20
	cursorTag = "c1"
)

var ErrCursor = errors.New("not a cursor from mem ui")

type Key struct {
	Key    string   `json:"key"`
	Cos    float64  `json:"cos"`
	Rerank *float64 `json:"rerank,omitempty"`
}

type Fired struct {
	Key   string `json:"key"`
	Tool  string `json:"tool"`
	Regex string `json:"regex"`
}

type Turn struct {
	Time        string   `json:"time"`
	Event       string   `json:"event"`
	Session     string   `json:"session"`
	Agent       string   `json:"agent,omitempty"`
	AgentType   string   `json:"agent_type,omitempty"`
	Harness     string   `json:"harness,omitempty"`
	Host        string   `json:"host,omitempty"`
	Project     string   `json:"project,omitempty"`
	Cwd         string   `json:"cwd,omitempty"`
	Input       string   `json:"input,omitempty"`
	Fired       []Fired  `json:"fired,omitempty"`
	Topics      int      `json:"topics,omitempty"`
	PromptSHA   string   `json:"prompt_sha,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
	PromptCut   bool     `json:"prompt_cut,omitempty"`
	PromptChars int      `json:"prompt_chars,omitempty"`
	Tool        string   `json:"tool,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	Pool        string   `json:"pool,omitempty"`
	Status      string   `json:"status"`
	Source      string   `json:"source,omitempty"`
	Notice      string   `json:"notice,omitempty"`
	Strong      []Key    `json:"strong,omitempty"`
	Pointers    []Key    `json:"pointers,omitempty"`
	Lexical     []string `json:"lexical,omitempty"`
	Keys        []string `json:"keys,omitempty"`
	Embed       string   `json:"embed,omitempty"`
	EmbedMs     int64    `json:"embed_ms"`
	Rerank      string   `json:"rerank,omitempty"`
	RerankMs    int64    `json:"rerank_ms,omitempty"`
	Reranked    []Key    `json:"reranked,omitempty"`
	TotalMs     int64    `json:"total_ms"`
	Chars       int      `json:"chars"`
}

func Preview(s string) string {
	p, _ := PreviewOf(s)
	return p
}

func PreviewOf(s string) (string, bool) {
	return PreviewN(s, PreviewRunes)
}

func PreviewN(s string, n int) (string, bool) {
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.Is(unicode.Bidi_Control, r):
			return -1
		case unicode.IsControl(r):
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return strings.TrimRightFunc(string(r[:n]), unicode.IsSpace), true
	}
	return s, false
}

func Append(dir, name string, maxSize int64, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxSize {
		_ = os.Rename(path, path+OldSuffix)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if fi, err := f.Stat(); err == nil && fi.Mode().IsRegular() && fi.Size() > 0 {
		var last [1]byte
		if _, err := f.ReadAt(last[:], fi.Size()-1); err == nil && last[0] != '\n' {
			data = append([]byte{'\n'}, data...)
		}
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write: %w", err)
	}
	return f.Close()
}

type Cursor struct {
	ID  string
	Off int64
}

func (c Cursor) String() string {
	return cursorTag + "-" + c.ID + "-" + strconv.FormatInt(c.Off, 10)
}

func ParseCursor(s string) (Cursor, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 3 || parts[0] != cursorTag {
		return Cursor{}, ErrCursor
	}
	id := parts[1]
	if id != "" {
		if len(id) != 2*idBytes || strings.ToLower(id) != id {
			return Cursor{}, ErrCursor
		}
		if _, err := hex.DecodeString(id); err != nil {
			return Cursor{}, ErrCursor
		}
	}
	off, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || off < 0 || parts[2] != strconv.FormatInt(off, 10) || (id == "" && off != 0) {
		return Cursor{}, ErrCursor
	}
	return Cursor{ID: id, Off: off}, nil
}

type Line struct {
	Data []byte
	File string
	End  int64
}

type part struct {
	f  *os.File
	id string
}

func openPart(path string) (part, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return part{}, nil
	}
	if err != nil {
		return part{}, err
	}
	if !fi.Mode().IsRegular() {
		return part{}, nil
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return part{}, nil
	}
	if err != nil {
		return part{}, err
	}
	id, err := firstLineID(f)
	if err != nil {
		_ = f.Close()
		return part{}, err
	}
	return part{f: f, id: id}, nil
}

func (p part) close() {
	if p.f != nil {
		_ = p.f.Close()
	}
}

func firstLineID(f *os.File) (string, error) {
	h := sha256.New()
	buf := make([]byte, 32<<10)
	for off := int64(0); off < maxIDScan; {
		n, err := f.ReadAt(buf, off)
		if i := bytes.IndexByte(buf[:n], '\n'); i >= 0 {
			h.Write(buf[:i+1])
			return hex.EncodeToString(h.Sum(nil)[:idBytes]), nil
		}
		h.Write(buf[:n])
		off += int64(n)
		if errors.Is(err, io.EOF) || n == 0 {
			return "", nil
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:idBytes]), nil
}

func (p part) from(off int64) ([]Line, int64, error) {
	if p.f == nil || p.id == "" {
		return nil, 0, nil
	}
	fi, err := p.f.Stat()
	if err != nil {
		return nil, 0, err
	}
	size := fi.Size()
	if off > size {
		off = 0
	}
	start := max(off, size-maxRead)
	buf := make([]byte, size-start)
	n, err := p.f.ReadAt(buf, start)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, off, err
	}
	buf = buf[:n]
	if start > off {
		i := bytes.IndexByte(buf, '\n')
		if i < 0 {
			return nil, off, nil
		}
		buf, start = buf[i+1:], start+int64(i+1)
	}
	end := bytes.LastIndexByte(buf, '\n')
	if end < 0 {
		return nil, start, nil
	}
	buf = buf[:end+1]
	var out []Line
	pos := start
	for len(buf) > 0 {
		i := bytes.IndexByte(buf, '\n')
		pos += int64(i + 1)
		if line := bytes.TrimSpace(buf[:i]); len(line) > 0 {
			out = append(out, Line{Data: line, File: p.id, End: pos})
		}
		buf = buf[i+1:]
	}
	return out, pos, nil
}

func Read(dir, name string, since *Cursor) ([]Line, Cursor, error) {
	path := filepath.Join(dir, name)
	cur, err := openPart(path)
	if err != nil {
		return nil, Cursor{}, err
	}
	defer cur.close()
	old, err := openPart(path + OldSuffix)
	if err != nil {
		return nil, Cursor{}, err
	}
	defer func() { old.close() }()
	if old.id != "" && old.id == cur.id {
		old.close()
		old = part{}
	}
	var lines []Line
	readOld, oldFrom, curFrom := true, int64(0), int64(0)
	if since != nil && since.ID != "" {
		switch since.ID {
		case cur.id:
			readOld, curFrom = false, since.Off
		case old.id:
			oldFrom = since.Off
		}
	}
	var oldEnd int64
	if readOld {
		ls, end, err := old.from(oldFrom)
		if err != nil {
			return nil, Cursor{}, err
		}
		lines, oldEnd = append(lines, ls...), end
	}
	ls, curEnd, err := cur.from(curFrom)
	if err != nil {
		return nil, Cursor{}, err
	}
	lines = append(lines, ls...)
	switch {
	case cur.id != "":
		return lines, Cursor{ID: cur.id, Off: curEnd}, nil
	case old.id != "":
		return lines, Cursor{ID: old.id, Off: oldEnd}, nil
	}
	return lines, Cursor{}, nil
}
