package client

import (
	"bytes"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MauricioJC3/ng_mux/internal/protocol"
)

func TestInReaderReadsAndReportsEnd(t *testing.T) {
	pr, pw := io.Pipe()
	ir := newInReader(pr)
	go func() { _, _ = pw.Write([]byte("ab")); pw.Close() }()

	for _, want := range []byte("ab") {
		got, err := ir.ReadByte()
		if err != nil || got != want {
			t.Fatalf("ReadByte = %q,%v; want %q", got, err, want)
		}
	}
	if _, err := ir.ReadByte(); err == nil {
		t.Fatal("ReadByte after the stream ends should error")
	}
}

func TestInReaderPushBack(t *testing.T) {
	pr, pw := io.Pipe()
	ir := newInReader(pr)
	go func() { _, _ = pw.Write([]byte("x")); pw.Close() }()

	b, _ := ir.ReadByte()
	ir.pushBack(b)
	if got, _ := ir.ReadByte(); got != 'x' {
		t.Fatalf("after pushBack('x'), ReadByte = %q, want 'x'", got)
	}
}

// close() must release a ReadByte that is parked waiting for input, so the
// input goroutine unwinds on teardown instead of leaking until stdin closes.
func TestInReaderCloseUnblocksReadByte(t *testing.T) {
	pr, _ := io.Pipe() // nothing is ever written, nothing closes it
	ir := newInReader(pr)

	got := make(chan error, 1)
	go func() { _, err := ir.ReadByte(); got <- err }()

	select {
	case <-got:
		t.Fatal("ReadByte returned before close()")
	case <-time.After(20 * time.Millisecond):
	}

	ir.close()
	ir.close() // idempotent

	select {
	case err := <-got:
		if err == nil {
			t.Fatal("ReadByte after close() should report an error")
		}
	case <-time.After(time.Second):
		t.Fatal("ReadByte did not return after close()")
	}
}

func TestReadByteTimeoutFiresWithNoInput(t *testing.T) {
	pr, _ := io.Pipe()
	ir := newInReader(pr)

	start := time.Now()
	if _, ok := ir.ReadByteTimeout(20 * time.Millisecond); ok {
		t.Fatal("ReadByteTimeout should have timed out")
	}
	if d := time.Since(start); d < 15*time.Millisecond {
		t.Fatalf("returned after %s, expected to wait ~20ms", d)
	}
}

func TestReadByteTimeoutReturnsAByteThatArrives(t *testing.T) {
	pr, pw := io.Pipe()
	ir := newInReader(pr)
	go func() { _, _ = pw.Write([]byte("[")) }()

	if b, ok := ir.ReadByteTimeout(time.Second); !ok || b != '[' {
		t.Fatalf("ReadByteTimeout = %q,%v; want '[',true", b, ok)
	}
}

// A bare Esc with nothing behind it must reach the pane on its own within
// escape-time, not sit buffered until the next keystroke.
func TestHandleEscapeForwardsLoneEsc(t *testing.T) {
	pr, _ := io.Pipe() // nothing is ever written
	ir := newInReader(pr)

	cli, srv := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- handleEscape(ir, protocol.NewConn(cli), 15*time.Millisecond) }()

	msg, err := protocol.NewConn(srv).Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if msg.Type != protocol.TypeInput || !bytes.Equal(msg.Data, []byte{0x1b}) {
		t.Fatalf("got %+v, want a lone Esc TypeInput", msg)
	}
	if err := <-done; err != nil {
		t.Fatalf("handleEscape: %v", err)
	}
}

// A frame that arrives while a local popup owns the screen must be dropped, and
// frames after the overlay clears must flow again.
func TestReadFramesHoldsFramesWhileOverlayIsUp(t *testing.T) {
	srvSide, cliSide := net.Pipe()
	srv := protocol.NewConn(srvSide)

	var buf bytes.Buffer
	var overlay atomic.Bool
	overlay.Store(true)

	done := make(chan error, 1)
	go func() { done <- readFrames(protocol.NewConn(cliSide), &buf, &overlay) }()

	// Sent while the overlay is up: must not reach the terminal.
	if err := srv.Write(protocol.Message{Type: protocol.TypeFrame, Data: []byte("HIDDEN")}); err != nil {
		t.Fatalf("write HIDDEN: %v", err)
	}
	// This second write only unblocks once readFrames has looped back to Read,
	// i.e. it has finished processing (and dropping) the HIDDEN frame.
	if err := srv.Write(protocol.Message{Type: protocol.TypeExecReply}); err != nil {
		t.Fatalf("write sync: %v", err)
	}
	overlay.Store(false)
	if err := srv.Write(protocol.Message{Type: protocol.TypeFrame, Data: []byte("SHOWN")}); err != nil {
		t.Fatalf("write SHOWN: %v", err)
	}
	if err := srv.Write(protocol.Message{Type: protocol.TypeBye}); err != nil {
		t.Fatalf("write bye: %v", err)
	}

	if err := <-done; !errors.Is(err, errDetached) {
		t.Fatalf("readFrames ended with %v, want errDetached", err)
	}
	got := buf.String()
	if strings.Contains(got, "HIDDEN") {
		t.Errorf("a frame was written while the overlay was up: %q", got)
	}
	if !strings.Contains(got, "SHOWN") {
		t.Errorf("a frame after the overlay cleared was not written: %q", got)
	}
}

func TestHandleEscapeForwardsArrowSequence(t *testing.T) {
	pr, pw := io.Pipe()
	ir := newInReader(pr)
	go func() { _, _ = pw.Write([]byte("[A")) }()

	cli, srv := net.Pipe()
	go func() { _ = handleEscape(ir, protocol.NewConn(cli), 200*time.Millisecond) }()

	msg, err := protocol.NewConn(srv).Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if msg.Type != protocol.TypeInput || string(msg.Data) != "\x1b[A" {
		t.Fatalf("got %+v, want ESC[A forwarded verbatim", msg)
	}
}
