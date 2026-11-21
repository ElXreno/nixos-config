package journal

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"agent-guard/internal/harness"
)

type ImportStats struct {
	Read, Appended, Duplicates, Undecodable int
	Kept                                    string
}

type ImportOptions struct {
	Harness harness.ID
	Now     func() time.Time
	Settle  time.Duration
}

const importBatch = 1 << 20

func (e Entry) key() [32]byte {
	return sha256.Sum256([]byte(e.Time.UTC().Format(time.RFC3339Nano) + "|" + e.Session + "|" + e.ToolUseID + "|" + e.Hook + "|" + e.Input + "|" + string(e.Level)))
}

func Import(target, legacy string, opts ImportOptions) (ImportStats, error) {
	var st ImportStats
	if same(target, legacy) {
		return st, errors.New("the target journal is the legacy journal itself")
	}
	importing := legacy + ".importing"
	if _, err := os.Stat(importing); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(legacy, importing); err != nil {
			return st, err
		}
	} else if err != nil {
		return st, err
	}
	known := map[[32]byte]struct{}{}
	if entries, _, err := Read(target); err == nil {
		for _, e := range entries {
			known[e.key()] = struct{}{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return st, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return st, err
	}
	out, err := os.OpenFile(target, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return st, err
	}
	defer func() { _ = out.Close() }()
	in, err := os.Open(importing)
	if err != nil {
		return st, err
	}
	defer func() { _ = in.Close() }()

	var batch bytes.Buffer
	flush := func() error {
		if batch.Len() == 0 {
			return nil
		}
		if _, err := out.Write(batch.Bytes()); err != nil {
			return err
		}
		batch.Reset()
		return out.Sync()
	}
	take := func(line []byte) error {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			return nil
		}
		st.Read++
		var e Entry
		var fields map[string]json.RawMessage
		if json.Unmarshal(line, &e) != nil || json.Unmarshal(line, &fields) != nil {
			st.Undecodable++
			return nil
		}
		if _, ok := fields["harness"]; !ok {
			e.Harness = opts.Harness
			stamp, _ := json.Marshal(opts.Harness)
			line = append([]byte(`{"harness":`+string(stamp)+","), line[1:]...)
		}
		k := e.key()
		if _, dup := known[k]; dup {
			st.Duplicates++
			return nil
		}
		known[k] = struct{}{}
		st.Appended++
		batch.Write(line)
		batch.WriteByte('\n')
		if batch.Len() >= importBatch {
			return flush()
		}
		return nil
	}

	var offset int64
	var pending []byte
	buf := make([]byte, 1<<16)
	for stable := 0; stable < 2; {
		n, err := in.ReadAt(buf, offset)
		if n > 0 {
			offset += int64(n)
			pending = append(pending, buf[:n]...)
			for {
				i := bytes.IndexByte(pending, '\n')
				if i < 0 {
					break
				}
				if err := take(pending[:i]); err != nil {
					return st, err
				}
				pending = pending[i+1:]
			}
			stable = 0
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return st, err
		}
		if err := flush(); err != nil {
			return st, err
		}
		time.Sleep(opts.Settle)
		if fi, err := in.Stat(); err == nil && fi.Size() == offset {
			stable++
		}
	}
	if err := take(pending); err != nil {
		return st, err
	}
	if err := flush(); err != nil {
		return st, err
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	st.Kept = legacy + ".imported-" + now().UTC().Format("20060102T150405Z")
	if err := os.Rename(importing, st.Kept); err != nil {
		return st, err
	}
	return st, nil
}

func same(a, b string) bool {
	x, err1 := filepath.Abs(a)
	y, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && filepath.Clean(x) == filepath.Clean(y)
}

func (s ImportStats) String() string {
	return fmt.Sprintf("read %d, appended %d, duplicates %d, undecodable %d; kept as %s", s.Read, s.Appended, s.Duplicates, s.Undecodable, s.Kept)
}
