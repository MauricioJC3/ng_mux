package server

import (
	"testing"
	"time"
)

func TestNewSessionHasOneWindowOnePane(t *testing.T) {
	_, ff, sess := setupSession(t)
	if got := windowCount(sess); got != 1 {
		t.Fatalf("window count = %d, want 1", got)
	}
	if got := paneCount(sess); got != 1 {
		t.Fatalf("pane count = %d, want 1", got)
	}
	if got := ff.count(); got != 1 {
		t.Fatalf("fake panes created = %d, want 1", got)
	}
}

func TestReapDropsAnExitedPane(t *testing.T) {
	srv, ff, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	if got := paneCount(sess); got != 2 {
		t.Fatalf("pane count after split = %d, want 2", got)
	}

	// Pane 1 is the original (left); its process ends when its pty closes.
	victim := ff.byID(1)
	if victim == nil {
		t.Fatal("no fake pane with id 1")
	}
	victim.pt.Close()

	waitFor(t, func() bool { return paneCount(sess) == 1 }, 2*time.Second)
	if got := activePane(sess); got != 2 {
		t.Fatalf("surviving pane = %d, want 2", got)
	}
}

// TestKillPaneClosesTheActivePane covers the kill-pane command: closing the pty
// makes the output pump end and the pane is reaped through the normal path,
// same as a shell that exits on its own.
func TestKillPaneClosesTheActivePane(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	if got := paneCount(sess); got != 2 {
		t.Fatalf("pane count after split = %d, want 2", got)
	}

	exec(t, srv, "kill-pane")

	waitFor(t, func() bool { return paneCount(sess) == 1 }, 2*time.Second)
}

func TestLastPaneExitEmptiesSession(t *testing.T) {
	ff := &fakeFleet{}
	emptied := make(chan string, 1)
	sess, err := newSession("work", 80, 24,
		sessionOpts{historyLimit: 100, defaultShell: "/bin/fakesh", newPane: ff.factory()},
		func(name string) { emptied <- name })
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}
	t.Cleanup(sess.shutdown)

	ff.byID(1).pt.Close() // the only pane's process ends

	select {
	case name := <-emptied:
		if name != "work" {
			t.Fatalf("onEmpty called with %q, want %q", name, "work")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onEmpty never fired after the last pane exited")
	}
}

func TestResizePropagatesToPanes(t *testing.T) {
	_, ff, sess := setupSession(t)
	sess.resize(100, 40)

	fp := ff.byID(1)
	waitFor(t, func() bool {
		c, r := fp.scr.size()
		return c == 100 && r == 39 // one row reserved for the status bar
	}, time.Second)
}
