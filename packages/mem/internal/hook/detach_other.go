//go:build !linux

package hook

import "io"

func detach(w io.Writer) {
	if c, ok := w.(io.Closer); ok {
		_ = c.Close()
	}
}
