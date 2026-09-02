// Package ptyx is a thin wrapper over github.com/aymanbagabas/go-pty. It opens
// a pseudo-terminal, starts a child process attached to it, and exposes the
// master end as an io.ReadWriteCloser plus a Resize method. The point of the
// wrapper is to keep the rest of tmux2 free of any direct pty/ConPTY concern
// and to centralise the "what shell do we launch" decision.
package ptyx

import (
	"fmt"
	"io"
	"os"

	"github.com/aymanbagabas/go-pty"
)

// Pane is a running child process on its own pseudo-terminal.
type Pane struct {
	pty pty.Pty
	cmd *pty.Cmd
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

	return &Pane{pty: pt, cmd: cmd}, nil
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

// Wait blocks until the child process exits.
func (p *Pane) Wait() error { return p.cmd.Wait() }

// Close terminates the child (best effort) and releases the pty.
func (p *Pane) Close() error {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return p.pty.Close()
}

var _ io.ReadWriteCloser = (*Pane)(nil)
