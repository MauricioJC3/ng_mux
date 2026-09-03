package server

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/MauricioJC3/ng_mux/internal/protocol"
)

func TestOSC52Encoding(t *testing.T) {
	got := osc52("hi")
	if want := "\x1b]52;c;aGk=\a"; string(got) != want {
		t.Fatalf("osc52(%q) = %q, want %q", "hi", got, want)
	}
	if osc52("") != nil {
		t.Fatal("osc52 of empty text should be nil")
	}
	if osc52(strings.Repeat("a", osc52MaxInput+1)) != nil {
		t.Fatal("osc52 of oversized text should be nil")
	}
}

// yankOneCell puts the current window's active pane into copy-mode and performs
// a one-cell selection + yank, returning what session.input reported.
func yankOneCell(t *testing.T, sess *session, ff *fakeFleet) (repaint bool, yank string) {
	t.Helper()
	sess.mu.Lock()
	ap := ff.byID(sess.windows[sess.cur].active)
	ap.scr.fillCh = 'x'
	sess.windows[sess.cur].enterCopy(sess.cols, sess.contentRows())
	sess.mu.Unlock()

	if _, y := sess.input([]byte(" ")); y != "" {
		t.Fatalf("starting a selection yielded a yank %q", y)
	}
	return sess.input([]byte("y"))
}

func TestSessionInputReturnsYankedText(t *testing.T) {
	_, ff, sess := setupSession(t)

	repaint, yank := yankOneCell(t, sess, ff)
	if !repaint || yank == "" {
		t.Fatalf("yank key: repaint=%v yank=%q, want true and non-empty", repaint, yank)
	}
	sess.mu.Lock()
	buf := sess.pasteBuf
	sess.mu.Unlock()
	if yank != buf {
		t.Fatalf("returned yank %q != paste buffer %q", yank, buf)
	}
}

func TestCopyYankSendsSetClipboardToClient(t *testing.T) {
	srv, ff, _ := setupSession(t)
	ff.byID(1).scr.fillCh = 'x'

	srvConn, cliConn := net.Pipe()
	t.Cleanup(func() { srvConn.Close(); cliConn.Close() })
	go srv.serveClient(protocol.NewConn(srvConn),
		protocol.Message{Type: protocol.TypeAttach, Name: "0", Cols: 80, Rows: 24})

	cli := protocol.NewConn(cliConn)
	writeMsg(t, cli, protocol.Message{Type: protocol.TypeExec, Name: "copy-mode"})
	writeMsg(t, cli, protocol.Message{Type: protocol.TypeInput, Data: []byte(" ")})
	writeMsg(t, cli, protocol.Message{Type: protocol.TypeInput, Data: []byte("y")})

	msg := readUntil(t, cli, protocol.TypeSetClipboard, 2*time.Second)
	if !bytes.HasPrefix(msg.Data, []byte("\x1b]52;c;")) {
		t.Fatalf("clipboard message data = %q, want an OSC 52 sequence", msg.Data)
	}
}

func TestCopyYankSuppressedWhenSetClipboardOff(t *testing.T) {
	srv, ff, _ := setupSession(t)
	srv.opts.setClipboard = false

	srvConn, cliConn := net.Pipe()
	t.Cleanup(func() { srvConn.Close(); cliConn.Close() })
	go srv.serveClient(protocol.NewConn(srvConn),
		protocol.Message{Type: protocol.TypeAttach, Name: "0", Cols: 80, Rows: 24})

	cli := protocol.NewConn(cliConn)
	ff.byID(1).scr.fillCh = 'x'
	writeMsg(t, cli, protocol.Message{Type: protocol.TypeExec, Name: "copy-mode"})
	writeMsg(t, cli, protocol.Message{Type: protocol.TypeInput, Data: []byte(" ")})
	writeMsg(t, cli, protocol.Message{Type: protocol.TypeInput, Data: []byte("y")})

	// Give the server room to process, then confirm nothing clipboard-shaped
	// arrived. Poll for a short window rather than blocking forever.
	deadline := time.Now().Add(300 * time.Millisecond)
	got := make(chan protocol.Message, 4)
	go func() {
		for {
			m, err := cli.Read()
			if err != nil {
				return
			}
			got <- m
		}
	}()
	for time.Now().Before(deadline) {
		select {
		case m := <-got:
			if m.Type == protocol.TypeSetClipboard {
				t.Fatal("received a clipboard message while set-clipboard is off")
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func writeMsg(t *testing.T, c *protocol.Conn, m protocol.Message) {
	t.Helper()
	if err := c.Write(m); err != nil {
		t.Fatalf("write %v: %v", m.Type, err)
	}
}

func readUntil(t *testing.T, c *protocol.Conn, want protocol.Type, timeout time.Duration) protocol.Message {
	t.Helper()
	done := make(chan protocol.Message, 1)
	go func() {
		for {
			m, err := c.Read()
			if err != nil {
				return
			}
			if m.Type == want {
				done <- m
				return
			}
		}
	}()
	select {
	case m := <-done:
		return m
	case <-time.After(timeout):
		t.Fatalf("did not receive a %s message within %s", want, timeout)
		return protocol.Message{}
	}
}
