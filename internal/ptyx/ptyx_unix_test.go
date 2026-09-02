//go:build !windows

package ptyx

import (
	"testing"
	"time"
)

// TestReadUnblocksWhenChildExits is the core contract the server's output pump
// depends on: once the shell on the pty exits, a Read on the master end must
// return an error so pump() can stop and the pane can be reaped. Before ptyx
// reaped the child itself, the parent kept a slave fd open and this Read blocked
// forever, leaving a dead pane on screen that accepted input but ran nothing.
func TestReadUnblocksWhenChildExits(t *testing.T) {
	p, err := Start(Config{Prog: "/bin/sh", Args: []string{"-c", "exit 0"}, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	readReturned := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, rerr := p.Read(buf); rerr != nil {
				close(readReturned)
				return
			}
		}
	}()

	select {
	case <-readReturned:
	case <-time.After(3 * time.Second):
		t.Fatal("Read on the pty master never returned after the child exited")
	}
}

// TestWaitReturnsAfterChildExits checks the child is reaped (no lingering
// zombie) and that Wait reports a clean exit.
func TestWaitReturnsAfterChildExits(t *testing.T) {
	p, err := Start(Config{Prog: "/bin/sh", Args: []string{"-c", "exit 0"}, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	done := make(chan error, 1)
	go func() { done <- p.Wait() }()

	select {
	case werr := <-done:
		if werr != nil {
			t.Fatalf("Wait returned %v, want nil for a clean exit", werr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Wait did not return within 3s after the child exited")
	}
}

// TestCloseIsIdempotentWithReaper closes the pane explicitly while the internal
// reaper may also be closing the pty; both paths must be safe (run with -race).
func TestCloseIsIdempotentWithReaper(t *testing.T) {
	p, err := Start(Config{Prog: "/bin/sh", Args: []string{"-c", "exit 0"}, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := p.Close(); err != nil {
			t.Logf("Close #%d returned %v", i, err)
		}
	}
	_ = p.Wait()
}
