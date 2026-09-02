//go:build windows

package ipc

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
)

// pipeName builds a per-user named pipe path. Windows named pipes are already
// namespaced per machine; including the username keeps two users on the same
// box from colliding.
func pipeName(e Endpoint) string {
	user := os.Getenv("USERNAME")
	if user == "" {
		user = "user"
	}
	user = strings.NewReplacer("\\", "_", "/", "_", " ", "_").Replace(user)
	return fmt.Sprintf(`\\.\pipe\tmux2-%s-%s`, user, e.Name)
}

// Listen creates the server pipe.
func Listen(e Endpoint) (net.Listener, error) {
	name := pipeName(e)
	// A successful dial means another server owns this endpoint.
	if c, err := winio.DialPipe(name, durPtr(200*time.Millisecond)); err == nil {
		c.Close()
		return nil, fmt.Errorf("ipc: server already running at %s", name)
	}
	cfg := &winio.PipeConfig{
		// Default security descriptor already restricts the pipe to the
		// creating user and SYSTEM/administrators.
		MessageMode:      false,
		InputBufferSize:  65536,
		OutputBufferSize: 65536,
	}
	l, err := winio.ListenPipe(name, cfg)
	if err != nil {
		return nil, fmt.Errorf("ipc: listen %s: %w", name, err)
	}
	return l, nil
}

// Dial connects to a running server, returning ErrNoServer when the pipe does
// not exist yet.
func Dial(e Endpoint) (net.Conn, error) {
	name := pipeName(e)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, err := winio.DialPipeContext(ctx, name)
	if err != nil {
		if os.IsNotExist(err) || err == winio.ErrTimeout {
			return nil, ErrNoServer
		}
		return nil, ErrNoServer
	}
	return c, nil
}

// Remove is a no-op on Windows: a named pipe disappears when its last handle
// closes, so there is no file to clean up.
func Remove(e Endpoint) {}

func durPtr(d time.Duration) *time.Duration { return &d }
