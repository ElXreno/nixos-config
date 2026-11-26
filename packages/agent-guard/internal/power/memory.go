package power

import (
	"encoding/json"
	"io"
	"time"

	"agent-guard/internal/statefile"
)

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

func (m Memory) Swap(key string, s State) (State, bool, error) {
	f, err := statefile.Lock(m.Dir, key)
	if err != nil {
		return State{}, false, err
	}
	defer func() { _ = f.Close() }()
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
	if err := statefile.Replace(f, append(data, '\n')); err != nil {
		return State{}, false, err
	}
	if !seen {
		statefile.Prune(m.Dir, f.Name(), now)
	}
	return State{Battery: true, OnAC: prev.AC, Capacity: prev.Capacity}, seen, nil
}
