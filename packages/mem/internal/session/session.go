package session

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	SubDir      = "sessions"
	RecallClaim = "recall"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type Seen struct {
	path string
	keys map[string]bool
}

func FileName(sessionID string) string {
	if safeID.MatchString(sessionID) {
		return sessionID + ".keys"
	}
	sum := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(sum[:16]) + ".keys"
}

func Open(stateDir, sessionID string) (*Seen, error) {
	if sessionID == "" {
		return nil, errors.New("empty session id")
	}
	s := &Seen{path: filepath.Join(stateDir, SubDir, FileName(sessionID)), keys: map[string]bool{}}
	f, err := os.Open(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k := strings.TrimSpace(sc.Text()); k != "" {
			s.keys[k] = true
		}
	}
	return s, sc.Err()
}

func (s *Seen) Path() string { return s.path }

func (s *Seen) Has(key string) bool { return s.keys[key] }

func (s *Seen) Keys() []string {
	out := make([]string, 0, len(s.keys))
	for k := range s.keys {
		out = append(out, k)
	}
	return out
}

func (s *Seen) Filter(keys []string) []string {
	var out []string
	for _, k := range keys {
		if !s.keys[k] {
			out = append(out, k)
		}
	}
	return out
}

func (s *Seen) Add(keys ...string) error {
	var b strings.Builder
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" || s.keys[k] || strings.ContainsAny(k, "\n\r") {
			continue
		}
		s.keys[k] = true
		b.WriteString(k + "\n")
	}
	if b.Len() == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (s *Seen) Reset() error {
	s.keys = map[string]bool{}
	err := os.Remove(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func Prune(stateDir string, olderThan time.Duration, now time.Time) (int, error) {
	dir := filepath.Join(stateDir, SubDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".keys") && !strings.HasSuffix(e.Name(), "."+RecallClaim) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > olderThan {
			if os.Remove(filepath.Join(dir, e.Name())) == nil {
				n++
			}
		}
	}
	return n, nil
}

func claimPath(stateDir, id, tag string) string {
	return filepath.Join(stateDir, SubDir, strings.TrimSuffix(FileName(id), ".keys")+"."+tag)
}

func Claimed(stateDir, id, tag string) bool {
	_, err := os.Stat(claimPath(stateDir, id, tag))
	return err == nil
}

func Claim(stateDir, id, tag string) (bool, error) {
	path := claimPath(stateDir, id, tag)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, f.Close()
}

func Unclaim(stateDir, id, tag string) error {
	err := os.Remove(claimPath(stateDir, id, tag))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
