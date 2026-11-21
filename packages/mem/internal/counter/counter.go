package counter

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

var safeName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func Bump(dir, name string) error {
	if !safeName.MatchString(name) {
		return errors.New("unsafe counter name " + name)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write([]byte{'.'})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func Counts(dir string) (map[string]int64, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]int64{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, e := range ents {
		fi, err := e.Info()
		if err != nil || !fi.Mode().IsRegular() || !safeName.MatchString(e.Name()) {
			continue
		}
		out[e.Name()] = fi.Size()
	}
	return out, nil
}
