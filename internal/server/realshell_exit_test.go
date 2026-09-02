//go:build !windows

package server

import (
	"testing"
	"time"
)

// TestRealShellExitReapsPane drives the production pane (a real shell on a real
// pty) rather than the in-memory fake: when the shell exits on its own, its
// pane must be reaped and, since it was the only one, the session must empty.
// This is the regression test for a dead pane staying on screen after `exit`.
func TestRealShellExitReapsPane(t *testing.T) {
	emptied := make(chan string, 1)
	sess, err := newSession("work", 80, 24,
		sessionOpts{historyLimit: 100, defaultShell: "/bin/sh"}, // nil newPane => real startPane
		func(name string) { emptied <- name })
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}
	t.Cleanup(sess.shutdown)

	// Type `exit` into the shell exactly as a user would.
	sess.input([]byte("exit\n"))

	select {
	case name := <-emptied:
		if name != "work" {
			t.Fatalf("onEmpty called with %q, want %q", name, "work")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("session never emptied after the shell exited: the pane was not reaped")
	}
}
