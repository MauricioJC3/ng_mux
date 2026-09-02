//go:build !windows

package termio

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

func enter(in, out *os.File) (*Session, error) {
	fd := int(in.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	return &Session{
		in:      in,
		out:     out,
		restore: func() error { return term.Restore(fd, old) },
	}, nil
}

func getSize(f *os.File) (Size, error) {
	w, h, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return Size{}, err
	}
	return Size{Cols: w, Rows: h}, nil
}

func watchResize(f *os.File, fn func(Size), stop <-chan struct{}) {
	if sz, err := getSize(f); err == nil {
		fn(sz)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	defer signal.Stop(ch)
	for {
		select {
		case <-stop:
			return
		case <-ch:
			if sz, err := getSize(f); err == nil {
				fn(sz)
			}
		}
	}
}
