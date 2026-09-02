//go:build windows

package termio

import (
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func enter(in, out *os.File) (*Session, error) {
	inH := windows.Handle(in.Fd())
	outH := windows.Handle(out.Fd())

	var oldIn, oldOut uint32
	if err := windows.GetConsoleMode(inH, &oldIn); err != nil {
		return nil, err
	}
	if err := windows.GetConsoleMode(outH, &oldOut); err != nil {
		return nil, err
	}

	// Input: raw. Drop line-buffering, echo and Ctrl-C processing; enable VT
	// input so keys arrive as escape sequences like on Unix.
	newIn := oldIn &^ (windows.ENABLE_ECHO_INPUT |
		windows.ENABLE_LINE_INPUT |
		windows.ENABLE_PROCESSED_INPUT)
	newIn |= windows.ENABLE_VIRTUAL_TERMINAL_INPUT
	if err := windows.SetConsoleMode(inH, newIn); err != nil {
		return nil, err
	}

	// Output: enable ANSI/VT escape interpretation.
	newOut := oldOut | windows.ENABLE_PROCESSED_OUTPUT |
		windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	if err := windows.SetConsoleMode(outH, newOut); err != nil {
		windows.SetConsoleMode(inH, oldIn)
		return nil, err
	}

	return &Session{
		in:  in,
		out: out,
		restore: func() error {
			windows.SetConsoleMode(inH, oldIn)
			windows.SetConsoleMode(outH, oldOut)
			return nil
		},
	}, nil
}

func getSize(f *os.File) (Size, error) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(f.Fd()), &info); err != nil {
		return Size{}, err
	}
	cols := int(info.Window.Right-info.Window.Left) + 1
	rows := int(info.Window.Bottom-info.Window.Top) + 1
	return Size{Cols: cols, Rows: rows}, nil
}

// watchResize polls, because the Windows console has no SIGWINCH. Polling the
// screen-buffer info is cheap and 250ms latency on a resize is imperceptible.
func watchResize(f *os.File, fn func(Size), stop <-chan struct{}) {
	last, _ := getSize(f)
	fn(last)
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if sz, err := getSize(f); err == nil && sz != last {
				last = sz
				fn(sz)
			}
		}
	}
}
