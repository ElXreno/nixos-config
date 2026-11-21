package power

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const keep = 14 * 24 * time.Hour

type Memory struct {
	Dir string
	Now func() time.Time
}

type record struct {
	AC       bool      `json:"ac"`
	Capacity int       `json:"capacity"`
	Time     time.Time `json:"time"`
}

func (m Memory) now() time.Time {
	if m.Now == nil {
		return time.Now()
	}
	return m.Now()
}

func fileName(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
		if b.Len() >= 160 {
			break
		}
	}
	return b.String() + ".json"
}

func (m Memory) Swap(key string, s State) (State, bool, error) {
	if m.Dir == "" || key == "" {
		return State{}, false, errors.New("no state dir or session")
	}
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		return State{}, false, err
	}
	path := filepath.Join(m.Dir, fileName(key))
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return State{}, false, err
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return State{}, false, err
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return State{}, false, err
	}
	var prev record
	seen := len(raw) > 0 && json.Unmarshal(raw, &prev) == nil
	now := m.now()
	data, err := json.Marshal(record{AC: s.OnAC, Capacity: s.Capacity, Time: now})
	if err != nil {
		return State{}, false, err
	}
	if err := f.Truncate(0); err != nil {
		return State{}, false, err
	}
	if _, err := f.WriteAt(append(data, '\n'), 0); err != nil {
		return State{}, false, err
	}
	if !seen {
		m.prune(path, now)
	}
	return State{Battery: true, OnAC: prev.AC, Capacity: prev.Capacity}, seen, nil
}

func (m Memory) prune(current string, now time.Time) {
	entries, err := os.ReadDir(m.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		p := filepath.Join(m.Dir, e.Name())
		if p == current || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if info, err := e.Info(); err == nil && info.Mode().IsRegular() && now.Sub(info.ModTime()) > keep {
			_ = os.Remove(p)
		}
	}
}
