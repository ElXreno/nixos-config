package cli

import (
	"errors"
	"io"
	"os"
	"syscall"
	"time"
)

var stdinStall = 3 * time.Second

var errNoStdin = errors.New("nothing arrives on stdin; pass the text in the flag or pipe it")

func pollable(r io.Reader) (*os.File, os.FileMode, bool) {
	f, ok := r.(*os.File)
	if !ok {
		return nil, 0, false
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, 0, false
	}
	mode := fi.Mode() & (os.ModeNamedPipe | os.ModeSocket)
	return f, mode, mode != 0
}

func readable(f *os.File, wait time.Duration) bool {
	fd := int(f.Fd())
	var set syscall.FdSet
	set.Bits[fd/64] |= int64(1) << (uint(fd) % 64)
	tv := syscall.NsecToTimeval(wait.Nanoseconds())
	n, err := syscall.Select(fd+1, &set, nil, nil, &tv)
	return err != nil || n > 0
}

func stdinReady(r io.Reader, wait time.Duration) bool {
	f, _, ok := pollable(r)
	return !ok || readable(f, wait)
}

func readStdin(r io.Reader) ([]byte, error) {
	if f, mode, ok := pollable(r); ok && mode&os.ModeSocket != 0 && !readable(f, stdinStall) {
		return nil, errNoStdin
	}
	return io.ReadAll(r)
}
