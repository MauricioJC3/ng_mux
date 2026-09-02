package protocol

import (
	"bytes"
	"io"
	"testing"
)

// pipe is a minimal in-memory ReadWriteCloser backed by a bytes.Buffer.
type pipe struct{ buf bytes.Buffer }

func (p *pipe) Read(b []byte) (int, error)  { return p.buf.Read(b) }
func (p *pipe) Write(b []byte) (int, error) { return p.buf.Write(b) }
func (p *pipe) Close() error                { return nil }

func TestConnRoundTrip(t *testing.T) {
	p := &pipe{}
	c := NewConn(p)

	in := []Message{
		{Type: TypeAttach, Cols: 120, Rows: 40},
		{Type: TypeInput, Data: []byte("ls -la\r")},
		{Type: TypeCommand, Name: CmdSplitVertical},
		{Type: TypeListReply, Sessions: []SessionInfo{
			{Name: "work", Panes: 3, Attached: true},
			{Name: "scratch", Panes: 1},
		}},
	}
	for _, m := range in {
		if err := c.Write(m); err != nil {
			t.Fatalf("Write(%v): %v", m.Type, err)
		}
	}

	for i, want := range in {
		got, err := c.Read()
		if err != nil {
			t.Fatalf("Read #%d: %v", i, err)
		}
		if got.Type != want.Type || got.Cols != want.Cols || got.Rows != want.Rows ||
			got.Name != want.Name || !bytes.Equal(got.Data, want.Data) ||
			len(got.Sessions) != len(want.Sessions) {
			t.Fatalf("message #%d round-tripped wrong:\n got %+v\nwant %+v", i, got, want)
		}
	}
}

func TestReadRejectsOversizedHeader(t *testing.T) {
	p := &pipe{}
	// 0xFFFFFFFF length header, no body.
	p.buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	c := NewConn(p)
	if _, err := c.Read(); err == nil {
		t.Fatal("expected error on oversized frame header, got nil")
	}
}

func TestReadEOFOnEmptyStream(t *testing.T) {
	c := NewConn(&pipe{})
	if _, err := c.Read(); err != io.EOF {
		t.Fatalf("err = %v, want io.EOF", err)
	}
}
