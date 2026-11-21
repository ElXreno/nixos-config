package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"mem/internal/statefile"
)

type turnSnap struct {
	sig   string
	turns []turnOut
}

func fileSig(dir string, names ...string) string {
	sig := ""
	for _, n := range names {
		fi, err := os.Stat(filepath.Join(dir, n))
		if err != nil {
			sig += "-|"
			continue
		}
		sig += fmt.Sprintf("%d.%d|", fi.Size(), fi.ModTime().UnixNano())
	}
	return sig
}

func (s *Server) snapshot() ([]turnOut, error) {
	if s.opt.StateDir == "" {
		return nil, nil
	}
	sig := fileSig(s.opt.StateDir, statefile.TurnsName, statefile.TurnsName+statefile.OldSuffix, statefile.HookLogName, statefile.HookLogName+statefile.OldSuffix, statefile.ShadowName, statefile.ShadowName+statefile.OldSuffix)
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if s.snap != nil && s.snap.sig == sig {
		return s.snap.turns, nil
	}
	turns, _, err := s.readTurns(nil)
	if err != nil {
		return nil, err
	}
	legacy, err := s.legacyTurns(turns)
	if err != nil {
		return nil, err
	}
	all := append(legacy, turns...)
	s.joinShadow(all)
	s.snap = &turnSnap{sig: sig, turns: all}
	return all, nil
}

func (s *Server) surfaced(days int) (map[string]bool, int) {
	all, err := s.snapshot()
	if err != nil || len(all) == 0 {
		return map[string]bool{}, 0
	}
	now := s.opt.now()
	cut := now.AddDate(0, 0, -days)
	out := map[string]bool{}
	var first time.Time
	for _, t := range all {
		if first.IsZero() || t.at.Before(first) {
			first = t.at
		}
		if t.at.Before(cut) {
			continue
		}
		for _, k := range injectedKeys(t.Turn) {
			base, _ := BaseKey(k)
			out[base] = true
		}
	}
	covered := 0
	if !first.IsZero() {
		covered = int(now.Sub(first).Hours() / 24)
	}
	return out, covered
}

func injectedKeys(t statefile.Turn) []string {
	var out []string
	for _, k := range t.Strong {
		out = append(out, k.Key)
	}
	for _, k := range t.Pointers {
		out = append(out, k.Key)
	}
	out = append(out, t.Lexical...)
	out = append(out, t.Keys...)
	return out
}
