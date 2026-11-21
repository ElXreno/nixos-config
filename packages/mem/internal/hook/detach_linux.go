package hook

import (
	"io"
	"os"
	"syscall"
)

func detach(w io.Writer) {
	f, ok := w.(*os.File)
	if !ok {
		if c, ok := w.(io.Closer); ok {
			_ = c.Close()
		}
		return
	}
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer func() { _ = null.Close() }()
	_ = syscall.Dup3(int(null.Fd()), int(f.Fd()), 0)
}
