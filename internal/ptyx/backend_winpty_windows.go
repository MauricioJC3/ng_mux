//go:build windows

package ptyx

import "github.com/MauricioJC3/ng_mux/internal/ptyx/winpty"

// winptyBackend runs the child through winpty, the fallback for Windows builds
// with no ConPTY.
type winptyBackend struct{ p *winpty.PTY }

func startWinpty(prog string, args []string, dir string, env []string, cols, rows int) (backend, error) {
	p, err := winpty.Open(prog, args, dir, env, cols, rows)
	if err != nil {
		return nil, err
	}
	return &winptyBackend{p: p}, nil
}

func (w *winptyBackend) Read(b []byte) (int, error)  { return w.p.Read(b) }
func (w *winptyBackend) Write(b []byte) (int, error) { return w.p.Write(b) }
func (w *winptyBackend) Resize(cols, rows int) error { return w.p.SetSize(cols, rows) }
func (w *winptyBackend) Close() error                { return w.p.Close() }
func (w *winptyBackend) wait() error                 { return w.p.Wait() }
func (w *winptyBackend) kill()                       { w.p.Kill() }
