package server

import (
	"testing"
	"time"
)

func TestPanePumpForwardsOutputThenSignalsExit(t *testing.T) {
	pt := newFakePty(80, 24)
	sc := newFakeScreen(80, 24)
	p := &pane{id: 1, pt: pt, vt: sc}

	exited := make(chan *pane, 1)
	go p.pump(func(ep *pane) { exited <- ep })

	pt.feed([]byte("hello world"))
	waitFor(t, func() bool { return sc.consumedBytes() == "hello world" }, time.Second)

	pt.Close()
	select {
	case got := <-exited:
		if got != p {
			t.Fatalf("onExit called with %v, want the pane itself", got)
		}
	case <-time.After(time.Second):
		t.Fatal("pump did not call onExit after the pty closed")
	}
}

func TestPaneResizePropagatesToPtyAndScreen(t *testing.T) {
	pt := newFakePty(80, 24)
	sc := newFakeScreen(80, 24)
	p := &pane{id: 1, pt: pt, vt: sc}

	p.resize(40, 10)

	if c, r := pt.size(); c != 40 || r != 10 {
		t.Errorf("pty size = %dx%d, want 40x10", c, r)
	}
	if c, r := sc.size(); c != 40 || r != 10 {
		t.Errorf("screen size = %dx%d, want 40x10", c, r)
	}
}

func TestPaneResizeClampsCopyCursor(t *testing.T) {
	pt := newFakePty(80, 24)
	sc := newFakeScreen(80, 24)
	p := &pane{id: 1, pt: pt, vt: sc, copy: newCopyState(80, 24)}
	p.copy.cy = 23

	p.resize(80, 10)

	if p.copy.cy != 9 {
		t.Errorf("copy cursor y = %d, want it clamped to 9", p.copy.cy)
	}
	if p.copy.rows != 10 || p.copy.cols != 80 {
		t.Errorf("copy viewport = %dx%d, want 80x10", p.copy.cols, p.copy.rows)
	}
}
