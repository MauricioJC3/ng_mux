package server

import (
	"sync"

	"github.com/MauricioJC3/ng_mux/internal/protocol"
	"github.com/MauricioJC3/ng_mux/internal/render"
)

// client is the server's view of one attached client: an outbound message
// queue drained by a single writer goroutine, plus the last frame this client
// was shown (so the broadcaster can send a minimal diff).
type client struct {
	pc *protocol.Conn

	out       chan protocol.Message
	closeOnce sync.Once
	closed    chan struct{}

	mu   sync.Mutex
	prev *render.Frame
	sess string // name of the session this client is currently viewing
}

func newClient(pc *protocol.Conn, session string) *client {
	return &client{
		pc:     pc,
		out:    make(chan protocol.Message, 8),
		closed: make(chan struct{}),
		sess:   session,
	}
}

// session returns the name of the session this client is viewing.
func (c *client) session() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sess
}

// setSession points the client at a different session and forces a repaint.
func (c *client) setSession(name string) {
	c.mu.Lock()
	c.sess = name
	c.prev = nil
	c.mu.Unlock()
}

// send queues a message and reports whether it was accepted. A full queue (a
// slow or stuck client) drops the message rather than blocking the broadcaster;
// the caller repaints that client from scratch on the next tick so a dropped
// frame still converges.
func (c *client) send(m protocol.Message) bool {
	select {
	case c.out <- m:
		return true
	case <-c.closed:
		return false
	default:
		return false
	}
}

// writeLoop is the only goroutine that writes to the wire for this client.
func (c *client) writeLoop() {
	for {
		select {
		case <-c.closed:
			return
		case m := <-c.out:
			if err := c.pc.Write(m); err != nil {
				c.close()
				return
			}
		}
	}
}

func (c *client) close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.pc.Close()
	})
}

// reset forces the next broadcast to this client to be a full repaint.
func (c *client) reset() {
	c.mu.Lock()
	c.prev = nil
	c.mu.Unlock()
}

func (c *client) takePrev() *render.Frame {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.prev
}

func (c *client) setPrev(f *render.Frame) {
	c.mu.Lock()
	c.prev = f
	c.mu.Unlock()
}
