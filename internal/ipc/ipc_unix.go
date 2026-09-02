//go:build !windows

package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// socketDir is the per-user directory that holds server sockets. It prefers
// XDG_RUNTIME_DIR (already user-private and tmpfs) and falls back to /tmp.
func socketDir() (string, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "tmux2-"+strconv.Itoa(os.Getuid()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func socketPath(e Endpoint) (string, error) {
	dir, err := socketDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, e.Name), nil
}

// Listen creates the server socket, removing a stale one left by a crash.
func Listen(e Endpoint) (net.Listener, error) {
	path, err := socketPath(e)
	if err != nil {
		return nil, err
	}
	// If something is already listening, this endpoint is taken.
	if c, err := net.DialTimeout("unix", path, 200*time.Millisecond); err == nil {
		c.Close()
		return nil, fmt.Errorf("ipc: server already running at %s", path)
	}
	_ = os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("ipc: listen %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// Dial connects to a running server. It returns ErrNoServer if nothing is
// listening at the endpoint.
func Dial(e Endpoint) (net.Conn, error) {
	path, err := socketPath(e)
	if err != nil {
		return nil, err
	}
	c, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			return nil, ErrNoServer
		}
		// Socket file exists but nobody answered: stale.
		return nil, ErrNoServer
	}
	return c, nil
}

// Remove deletes the socket file for an endpoint (used on clean shutdown).
func Remove(e Endpoint) {
	if path, err := socketPath(e); err == nil {
		_ = os.Remove(path)
	}
}
