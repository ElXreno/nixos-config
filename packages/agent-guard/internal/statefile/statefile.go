package statefile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const Keep = 14 * 24 * time.Hour

func Name(key string) string {
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

func Lock(dir, key string) (*os.File, error) {
	if dir == "" || key == "" {
		return nil, errors.New("no state dir or session")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, Name(key)), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func Replace(f *os.File, data []byte) error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err := f.WriteAt(data, 0)
	return err
}

func Prune(dir, current string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if p == current || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if info, err := e.Info(); err == nil && info.Mode().IsRegular() && now.Sub(info.ModTime()) > Keep {
			_ = os.Remove(p)
		}
	}
}
