package transcript

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"agent-guard/internal/call"
	"agent-guard/internal/harness"
)

type File struct {
	Path string
	Sub  bool
}

type Record struct {
	Harness harness.ID
	Sub     bool
	Call    call.Call
	Key     string
}

type Source interface {
	Files(root string, topOnly bool) ([]File, error)
	Read(f File, emit func(Record)) (skipped int, err error)
}

type Stats struct {
	Files      int
	Unreadable int
	Skipped    int
}

func Walk(src Source, root string, topOnly bool) ([]Record, Stats, error) {
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	files, err := src.Files(root, topOnly)
	if err != nil {
		return nil, Stats{}, err
	}
	per := make([][]Record, len(files))
	skipped := make([]int, len(files))
	failed := make([]bool, len(files))
	Each(len(files), func(i int) {
		var n int
		var err error
		n, err = src.Read(files[i], func(r Record) { per[i] = append(per[i], r) })
		skipped[i], failed[i] = n, err != nil
	})
	st := Stats{Files: len(files)}
	var out []Record
	for i := range files {
		out = append(out, per[i]...)
		st.Skipped += skipped[i]
		if failed[i] {
			st.Unreadable++
		}
	}
	return out, st, nil
}

func EachLine(path string, visit func(line []byte)) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = fh.Close() }()
	r := bufio.NewReaderSize(fh, 1<<16)
	for {
		line, err := r.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			visit(line)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func Each(n int, f func(int)) {
	work := make(chan int)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			for i := range work {
				f(i)
			}
		})
	}
	for i := range n {
		work <- i
	}
	close(work)
	wg.Wait()
}
