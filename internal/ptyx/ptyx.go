// Package ptyx opens a pseudo-terminal, starts a child process on it, and
// exposes the controlling end as an io.ReadWriteCloser plus Resize/Wait. It
// keeps the rest of ngmux free of any pty/ConPTY/winpty concern and centralises
// the "what shell do we launch" decision.
//
// Two backends sit behind the same Pane:
//
//   - go-pty: a real pty on Unix, ConPTY on Windows 10 1809+ / Server 2019+.
//   - winpty: the fallback on older Windows that has no ConPTY. Its native
//     helper binaries are embedded in the ngmux executable and unpacked on
//     first use (see internal/ptyx/winpty).
//
// Start picks the backend; everything above ptyx sees only *Pane.
package ptyx

import (
	"io"
	"os"
	"sync"
)

// Config controls how a Pane is started.
type Config struct {
	// Prog is the executable to run. Empty means DefaultShell().
	Prog string
	// Args are passed after Prog. Ignored when Prog is empty.
	Args []string
	// Dir is the working directory. Empty means inherit.
	Dir string
	// Env is the child environment. Nil means inherit the current process env.
	Env []string
	// Cols and Rows are the initial terminal size. Zero falls back to 80x24.
	Cols, Rows int
}

// backend is one running child on one pseudo-terminal. go-pty and winpty each
// provide an implementation; Pane wraps whichever Start chose.
type backend interface {
	io.ReadWriteCloser
	// Resize changes the terminal window size.
	Resize(cols, rows int) error
	// wait blocks until the child process exits and returns its exit error
	// (nil on a clean exit).
	wait() error
	// kill best-effort terminates the child. Close then releases the pty.
	kill()
}

// Pane is a running child process on its own pseudo-terminal.
type Pane struct {
	b backend

	closeOnce sync.Once
	exited    chan struct{} // closed once the child has been reaped
	waitErr   error         // result of b.wait; read only after exited is closed
}

// Start opens a pty and launches the configured program on it.
func Start(cfg Config) (*Pane, error) {
	if err := CheckAvailable(); err != nil {
		return nil, err
	}

	cols, rows := cfg.Cols, cfg.Rows
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	prog := cfg.Prog
	var args []string
	if prog == "" {
		prog = DefaultShell()
		args = defaultShellArgs(prog)
	} else {
		args = cfg.Args
	}

	env := cfg.Env
	if env == nil {
		env = append(os.Environ(), "TERM=xterm-256color")
	}

	var (
		b   backend
		err error
	)
	if conPTYAvailable() {
		b, err = startGoPty(prog, args, cfg.Dir, env, cols, rows)
	} else {
		b, err = startFallback(prog, args, cfg.Dir, env, cols, rows)
	}
	if err != nil {
		return nil, err
	}

	p := &Pane{b: b, exited: make(chan struct{})}
	// Reap the child ourselves: without a Wait the process lingers as a zombie
	// and a Read on the controlling end never unblocks when the shell exits.
	// Closing the backend once the child is gone wakes any pending Read.
	go func() {
		p.waitErr = p.b.wait()
		close(p.exited)
		p.closeBackend()
	}()
	return p, nil
}

// Read implements io.Reader over the pty's controlling end.
func (p *Pane) Read(b []byte) (int, error) { return p.b.Read(b) }

// Write implements io.Writer over the pty's controlling end.
func (p *Pane) Write(b []byte) (int, error) { return p.b.Write(b) }

// Resize changes the pty window size, which delivers SIGWINCH to the child on
// Unix and the equivalent resize event on Windows.
func (p *Pane) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	return p.b.Resize(cols, rows)
}

// Wait blocks until the child process has exited and been reaped, returning the
// child's exit error (nil on a clean exit).
func (p *Pane) Wait() error {
	<-p.exited
	return p.waitErr
}

// Close terminates the child (best effort) and releases the pty. The internal
// reaper goroutine also closes the backend once the child exits on its own;
// closeBackend makes the release happen exactly once.
func (p *Pane) Close() error {
	p.b.kill()
	return p.closeBackend()
}

func (p *Pane) closeBackend() error {
	var err error
	p.closeOnce.Do(func() { err = p.b.Close() })
	return err
}

var _ io.ReadWriteCloser = (*Pane)(nil)
