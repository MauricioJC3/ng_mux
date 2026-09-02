package server

import (
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/MauricioJC3/ng_mux/internal/protocol"
)

// nopRWC is a do-nothing transport for building a *client in tests that only
// exercise server-side bookkeeping (session reassignment), never the wire.
type nopRWC struct{}

func (nopRWC) Read([]byte) (int, error)    { return 0, io.EOF }
func (nopRWC) Write(p []byte) (int, error) { return len(p), nil }
func (nopRWC) Close() error                { return nil }

func testClient(session string) *client {
	return newClient(protocol.NewConn(nopRWC{}), session)
}

// TestSessionEmptiedMovesClientToSurvivor is the basic case: the session a
// client is viewing empties, and the client is moved to one that still exists.
func TestSessionEmptiedMovesClientToSurvivor(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, n := range []string{"a", "b", "c"} {
		if _, err := srv.getOrCreateSession(n); err != nil {
			t.Fatalf("create %q: %v", n, err)
		}
	}
	cl := testClient("b")
	srv.addClient(cl)

	srv.sessionEmptied("b")

	if got := cl.session(); got == "b" {
		t.Fatalf("client still on the emptied session %q", got)
	}
	if srv.sessionByName(cl.session()) == nil {
		t.Fatalf("client points at %q, which is not a live session", cl.session())
	}
}

// TestSessionEmptiedNeverStrandsAClient is the regression test for the race
// where several sessions emptying at once could leave a client pointed at a
// name already removed from the server. Run with -race.
func TestSessionEmptiedNeverStrandsAClient(t *testing.T) {
	srv, _ := newTestServer(t)
	const n = 8
	names := make([]string, n)
	clients := make([]*client, n)
	for i := range names {
		names[i] = fmt.Sprintf("s%d", i)
		if _, err := srv.getOrCreateSession(names[i]); err != nil {
			t.Fatalf("create %q: %v", names[i], err)
		}
		clients[i] = testClient(names[i])
		srv.addClient(clients[i])
	}

	// Empty every session but the last, all at once.
	var wg sync.WaitGroup
	for i := 0; i < n-1; i++ {
		wg.Add(1)
		go func(name string) { defer wg.Done(); srv.sessionEmptied(name) }(names[i])
	}
	wg.Wait()

	for i, c := range clients {
		if got := srv.sessionByName(c.session()); got == nil {
			t.Fatalf("client %d stranded on %q, which is not a live session", i, c.session())
		}
	}
}
