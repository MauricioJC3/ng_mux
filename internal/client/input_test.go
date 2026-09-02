package client

import (
	"bytes"
	"io"
	"net"
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
