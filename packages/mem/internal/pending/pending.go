package pending

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"mem/internal/session"
)

const (
	SubDir     = "pending"
	MaxEntries = 32
	Retention  = 10 * time.Minute

	suffix    = ".json"
	tmpPrefix = ".tmp-"

	claimedInfix = ".claimed-"
	stampLen     = 20
	maxEntry     = 1 << 20
)

type Entry struct {
	Agent    string   `json:"agent"`
	Text     string   `json:"text"`
	Keys     []string `json:"keys,omitempty"`
	Pointers []string `json:"pointers,omitempty"`
}

func sessionDir(sessionID string) string {
	return strings.TrimSuffix(session.FileName(sessionID), ".keys")
}

func Fold(agent string) string {
	return strings.Map(func(r rune) rune {
		if r == '_' || unicode.IsSpace(r) || unicode.Is(unicode.Pd, r) {
			return -1
		}
		return unicode.ToLower(r)
	}, agent)
}

func agentTag(agent string) string {
	sum := sha256.Sum256([]byte(Fold(agent)))
	return hex.EncodeToString(sum[:6])
}

func stamp(t time.Time) string {
	n := t.UnixNano()
	if n < 0 {
		n = 0
	}
	s := strconv.FormatInt(n, 10)
	return strings.Repeat("0", max(stampLen-len(s), 0)) + s
}

func parseName(name string) (ts int64, tag string, ok bool) {
	base, found := strings.CutSuffix(name, suffix)
	if !found || strings.HasPrefix(name, tmpPrefix) {
		return 0, "", false
	}
	parts := strings.Split(base, "-")
	if len(parts) != 3 || len(parts[0]) != stampLen {
		return 0, "", false
	}
	n, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", false
	}
	return n, parts[1], true
}

func queueDir(stateDir, sessionID string) (string, error) {
	if sessionID == "" {
		return "", errors.New("empty session id")
	}
	return filepath.Join(stateDir, SubDir, sessionDir(sessionID)), nil
}

func Enqueue(stateDir, sessionID string, e Entry, now time.Time) error {
	dir, err := queueDir(stateDir, sessionID)
	if err != nil {
		return err
	}
	if e.Agent == "" || e.Text == "" {
		return errors.New("pending entry needs an agent type and text")
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	var salt [4]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return err
	}
	name := fmt.Sprintf("%s-%s-%d%s%s", stamp(now), agentTag(e.Agent), os.Getpid(), hex.EncodeToString(salt[:]), suffix)
	tmp := filepath.Join(dir, tmpPrefix+name)
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	trim(dir, now)
	return nil
}

func listing(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range ents {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func strayName(name string) bool {
	return strings.HasPrefix(name, tmpPrefix) || strings.Contains(name, claimedInfix)
}

func trim(dir string, now time.Time) {
	names, err := listing(dir)
	if err != nil {
		return
	}
	var live []string
	for _, n := range names {
		ts, _, ok := parseName(n)
		if !ok {
			if strayName(n) {
				if info, err := os.Stat(filepath.Join(dir, n)); err == nil && time.Since(info.ModTime()) > Retention {
					_ = os.Remove(filepath.Join(dir, n))
				}
			}
			continue
		}
		if now.Sub(time.Unix(0, ts)) > Retention {
			_ = os.Remove(filepath.Join(dir, n))
			continue
		}
		live = append(live, n)
	}
	for len(live) > MaxEntries {
		_ = os.Remove(filepath.Join(dir, live[0]))
		live = live[1:]
	}
}

func Dequeue(stateDir, sessionID, agent string, ttl time.Duration, now time.Time) (Entry, bool, error) {
	dir, err := queueDir(stateDir, sessionID)
	if err != nil {
		return Entry{}, false, err
	}
	names, err := listing(dir)
	if err != nil {
		return Entry{}, false, err
	}
	tag := agentTag(agent)
	for _, n := range names {
		ts, nameTag, ok := parseName(n)
		if !ok {
			continue
		}
		path := filepath.Join(dir, n)
		if now.Sub(time.Unix(0, ts)) > ttl {
			_ = os.Remove(path)
			continue
		}
		if nameTag != tag {
			continue
		}
		claimed := path + claimedInfix + strconv.Itoa(os.Getpid())
		if err := os.Rename(path, claimed); err != nil {
			continue
		}
		data, err := readCapped(claimed)
		_ = os.Remove(claimed)
		if err != nil {
			continue
		}
		var e Entry
		if json.Unmarshal(data, &e) != nil || e.Text == "" || Fold(e.Agent) != Fold(agent) {
			continue
		}
		return e, true, nil
	}
	return Entry{}, false, nil
}

func readCapped(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxEntry+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxEntry {
		return nil, errors.New("pending entry too large")
	}
	return data, nil
}

func Prune(stateDir string, olderThan time.Duration, now time.Time) (int, error) {
	root := filepath.Join(stateDir, SubDir)
	ents, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > olderThan && os.RemoveAll(filepath.Join(root, e.Name())) == nil {
			n++
		}
	}
	return n, nil
}
