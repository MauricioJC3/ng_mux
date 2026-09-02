package server

import (
	"io"

	"github.com/MauricioJC3/ng_mux/internal/layout"
	"github.com/MauricioJC3/ng_mux/internal/ptyx"
	"github.com/MauricioJC3/ng_mux/internal/vterm"
)

// pty is the pseudo-terminal half of a pane: a child process on a real pty.
// *ptyx.Pane is the production implementation; tests substitute an in-memory
// fake so session/window logic can run without spawning a shell.
type pty interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

// screen is the terminal-emulator half of a pane: it consumes pty output and
// answers snapshot/scrollback queries. *vterm.Term is the production
// implementation.
type screen interface {
	io.Writer
	Resize(cols, rows int)
	Snapshot() vterm.Snapshot
	// SnapshotInto is Snapshot writing into a caller-owned value so the hot
	// render path can reuse one buffer per pane instead of allocating.
	SnapshotInto(dst *vterm.Snapshot)
	ScrollbackView(offset, rows int) vterm.Snapshot
	HistoryLen() int
	SetHistoryLimit(n int)
	// Dirty reports whether bytes have arrived (or a resize happened) since the
	// last Snapshot. The broadcaster uses it to skip idle sessions entirely.
	Dirty() bool
}

// paneFactory builds a pane for a window. Injected through sessionOpts so tests
// can supply fakes; production always uses startPane.
type paneFactory func(id layout.PaneID, cols, rows int, shell string, histLimit int) (*pane, error)

// pane couples a child process on a pty with the emulator that interprets its
// output. One pane == one shell. win is the window that currently holds it, so
// an exit event can be routed back to the right split tree. copy is non-nil
// while the pane is in scrollback / selection mode.
type pane struct {
	id   layout.PaneID
	win  *window
	pt   pty
	vt   screen
	copy *copyState
}

// startPane opens a pty running shell (empty = platform default) and wires an
// emulator with the given scrollback limit. It is the production paneFactory.
func startPane(id layout.PaneID, cols, rows int, shell string, histLimit int) (*pane, error) {
	pt, err := ptyx.Start(ptyx.Config{Cols: cols, Rows: rows, Prog: shell})
	if err != nil {
		return nil, err
	}
	vt := vterm.New(cols, rows, pt)
	vt.SetHistoryLimit(histLimit)
	return &pane{id: id, pt: pt, vt: vt}, nil
}

// pump copies pty output into the emulator until the child exits or errors.
// It calls onExit exactly once when the pane's process is finished.
func (p *pane) pump(onExit func(*pane)) {
	buf := make([]byte, 32*1024)
	for {
		n, err := p.pt.Read(buf)
		if n > 0 {
			_, _ = p.vt.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	onExit(p)
}

func (p *pane) resize(cols, rows int) {
	p.vt.Resize(cols, rows)
	_ = p.pt.Resize(cols, rows)
	if p.copy != nil {
		p.copy.rows, p.copy.cols = rows, cols
		if p.copy.cy >= rows {
			p.copy.cy = rows - 1
		}
	}
}

// copyKey feeds one key chunk to copy-mode. It returns the text to store in the
// paste buffer (empty unless the key was a yank) and whether copy-mode ended.
func (p *pane) copyKey(data []byte) (yank string, exited bool) {
	cs := p.copy
	if cs == nil {
		return "", false
	}
	exit, doYank := cs.key(data, p.vt.HistoryLen())
	if doYank {
		snap := p.vt.ScrollbackView(cs.offset, cs.rows)
		yank = extractText(snap, cs.ax, cs.ay, cs.cx, cs.cy)
	}
	if exit {
		p.copy = nil
	}
	return yank, exit
}

func (p *pane) close() {
	_ = p.pt.Close()
}
