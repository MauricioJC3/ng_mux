package ptyx

import (
	"fmt"

	"github.com/aymanbagabas/go-pty"
)

// goPtyBackend runs the child on a real pty (Unix) or ConPTY (Windows 1809+)
// via github.com/aymanbagabas/go-pty.
type goPtyBackend struct {
	pty pty.Pty
	cmd *pty.Cmd
}

func startGoPty(prog string, args []string, dir string, env []string, cols, rows int) (backend, error) {
	pt, err := pty.New()
	if err != nil {
		return nil, fmt.Errorf("ptyx: open pty: %w", err)
	}
	if err := pt.Resize(cols, rows); err != nil {
		pt.Close()
		return nil, fmt.Errorf("ptyx: initial resize: %w", err)
	}

	cmd := pt.Command(prog, args...)
	cmd.Dir = dir
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		pt.Close()
		return nil, fmt.Errorf("ptyx: start %q: %w", prog, err)
	}
	return &goPtyBackend{pty: pt, cmd: cmd}, nil
}

func (g *goPtyBackend) Read(b []byte) (int, error)  { return g.pty.Read(b) }
func (g *goPtyBackend) Write(b []byte) (int, error) { return g.pty.Write(b) }
func (g *goPtyBackend) Resize(cols, rows int) error { return g.pty.Resize(cols, rows) }
func (g *goPtyBackend) Close() error                { return g.pty.Close() }
func (g *goPtyBackend) wait() error                 { return g.cmd.Wait() }

func (g *goPtyBackend) kill() {
	if g.cmd != nil && g.cmd.Process != nil {
		_ = g.cmd.Process.Kill()
	}
}
