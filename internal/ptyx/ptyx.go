// Package ptyx is a thin wrapper over github.com/aymanbagabas/go-pty. It opens
// a pseudo-terminal, starts a child process attached to it, and exposes the
// master end as an io.ReadWriteCloser plus a Resize method. The point of the
// wrapper is to keep the rest of ngmux free of any direct pty/ConPTY concern
// and to centralise the "what shell do we launch" decision.
package ptyx

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/aymanbagabas/go-pty"
)

// Pane is a running child process on its own pseudo-terminal.
type Pane struct {
	pty pty.Pty
	cmd *pty.Cmd

	closeOnce sync.Once
	exited    chan struct{} // closed once the child has been reaped
	waitErr   error         // result of cmd.Wait; read only after exited is closed
}

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

// Start opens a pty and launches the configured program on it.
func Start(cfg Config) (*Pane, error) {
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

	pt, err := pty.New()
	if err != nil {
		return nil, fmt.Errorf("ptyx: open pty: %w", err)
	}
	if err := pt.Resize(cols, rows); err != nil {
		pt.Close()
		return nil, fmt.Errorf("ptyx: initial resize: %w", err)
	}

	cmd := pt.Command(prog, args...)
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}
	if cfg.Env != nil {
		cmd.Env = cfg.Env
	} else {
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	}
	if err := cmd.Start(); err != nil {
		pt.Close()
		return nil, fmt.Errorf("ptyx: start %q: %w", prog, err)
	}

	p := &Pane{pty: pt, cmd: cmd, exited: make(chan struct{})}
	// Reap the child ourselves. Without this the process lingers as a zombie
	// and, more importantly, a Read on the master never unblocks when the shell
	// exits (the parent still holds a slave fd open), so the pane is never
	// reaped. Closing the pty once the child is gone wakes any pending Read.
	go func() {
		p.waitErr = p.cmd.Wait()
		close(p.exited)
		p.closePTY()
	}()
	return p, nil
}

// Read implements io.Reader over the pty master end.
func (p *Pane) Read(b []byte) (int, error) { return p.pty.Read(b) }

// Write implements io.Writer over the pty master end.
func (p *Pane) Write(b []byte) (int, error) { return p.pty.Write(b) }

// Resize changes the pty window size, which delivers SIGWINCH to the child on
// Unix and the equivalent resize event on Windows.
func (p *Pane) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	return p.pty.Resize(cols, rows)
}

// Wait blocks until the child process has exited and been reaped, returning the
// child's exit error (nil on a clean exit).
func (p *Pane) Wait() error {
	<-p.exited
	return p.waitErr
}

// Close terminates the child (best effort) and releases the pty. The internal
// reaper goroutine also closes the pty once the child exits on its own; both
// paths are safe to run and closePTY makes the pty close happen exactly once.
func (p *Pane) Close() error {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return p.closePTY()
}

func (p *Pane) closePTY() error {
	var err error
	p.closeOnce.Do(func() { err = p.pty.Close() })
	return err
}

var _ io.ReadWriteCloser = (*Pane)(nil)
