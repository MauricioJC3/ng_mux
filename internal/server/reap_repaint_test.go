package server

import (
	"testing"
	"time"

	"github.com/MauricioJC3/ng_mux/internal/layout"
)

// dirtyNow reports the session's dirty state without consuming it.
func dirtyNow(s *session) bool { return s.dirty() }

// paneScreenSize returns the emulator size the fake pane currently believes it
// has, i.e. what the last applyLayout pushed to it.
func paneScreenSize(ff *fakeFleet, id layout.PaneID) (int, int) {
	if fp := ff.byID(id); fp != nil {
		return fp.scr.size()
	}
	return 0, 0
}

// After a pane's shell exits, the tree collapses on the async reap path. The
// client must be told to repaint even though the survivor produced no output —
// otherwise the dead pane stays on screen until the user runs `clear`.
func TestReapMarksSessionDirty(t *testing.T) {
	srv, ff, sess := setupSession(t)
	exec(t, srv, "split-window -h")

	// Take a frame so the session starts clean (execCommand set needsRepaint).
	sess.frame()
	if dirtyNow(sess) {
		t.Fatal("session should be clean after a frame with no pending output")
	}

	victim := ff.byID(2) // the pane split off; pane 1 stays and is idle
	victim.pt.Close()

	waitFor(t, func() bool { return paneCount(sess) == 1 }, 2*time.Second)
	if !dirtyNow(sess) {
		t.Fatal("reap did not mark the session dirty: the client would keep showing the closed pane")
	}
}

// After a pane closes, the survivors must be resized to the rectangles they now
// occupy. Without that, their pty/emulator keep the old geometry and multi-pane
// layouts render overlapped or with stale gaps.
func TestReapReflowsSurvivingPanes(t *testing.T) {
	srv, ff, sess := setupSession(t)
	exec(t, srv, "split-window -h") // pane 1 | pane 2, ~half width each

	beforeW, _ := paneScreenSize(ff, 1)
	fullW := sess.cols
	if beforeW >= fullW {
		t.Fatalf("pane 1 started at width %d; expected roughly half of %d", beforeW, fullW)
	}

	ff.byID(2).pt.Close()
	waitFor(t, func() bool { return paneCount(sess) == 1 }, 2*time.Second)

	waitFor(t, func() bool {
		w, h := paneScreenSize(ff, 1)
		return w == fullW && h == sess.contentRows()
	}, 2*time.Second)

	fp := ff.byID(1)
	if c, r := fp.pt.size(); c != fullW || r != sess.contentRows() {
		t.Fatalf("survivor pty size = %dx%d, want %dx%d", c, r, fullW, sess.contentRows())
	}
}

// Three panes, drop the middle one: the remaining two must still tile the whole
// width with a single divider, no overlap, no widened gap.
func TestReapWithThreePanesKeepsLayoutTight(t *testing.T) {
	srv, ff, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	exec(t, srv, "split-window -h") // now 3 panes across

	if got := paneCount(sess); got != 3 {
		t.Fatalf("pane count = %d, want 3", got)
	}

	// Kill whichever pane is not currently active so focus handling is exercised
	// alongside the reflow.
	active := activePane(sess)
	var victim layout.PaneID
	for _, id := range []layout.PaneID{1, 2, 3} {
		if id != active {
			victim = id
			break
		}
	}
	ff.byID(victim).pt.Close()
	waitFor(t, func() bool { return paneCount(sess) == 2 }, 2*time.Second)

	sess.mu.Lock()
	w := sess.windows[sess.cur]
	rects := layout.Compute(w.tree, w.outer(sess.cols, sess.contentRows()))
	sess.mu.Unlock()

	assertTilesOuter(t, rects, sess.cols, sess.contentRows())
}

// assertTilesOuter is the server-side mirror of the layout package's geometry
// contract: no overlap, inside the outer, no 2x2 uncovered block.
func assertTilesOuter(t *testing.T, rects map[layout.PaneID]layout.Rect, w, h int) {
	t.Helper()
	cov := make([]int, w*h)
	for id, r := range rects {
		if r.X < 0 || r.Y < 0 || r.X+r.W > w || r.Y+r.H > h || r.W < 1 || r.H < 1 {
			t.Fatalf("pane %d rect %+v is out of bounds for %dx%d", id, r, w, h)
		}
		for y := r.Y; y < r.Y+r.H; y++ {
			for x := r.X; x < r.X+r.W; x++ {
				cov[y*w+x]++
				if cov[y*w+x] > 1 {
					t.Fatalf("cell (%d,%d) covered by more than one pane", x, y)
				}
			}
		}
	}
	at := func(x, y int) bool { return x >= 0 && y >= 0 && x < w && y < h && cov[y*w+x] > 0 }
	for y := 0; y < h-1; y++ {
		for x := 0; x < w-1; x++ {
			if !at(x, y) && !at(x+1, y) && !at(x, y+1) && !at(x+1, y+1) {
				t.Fatalf("2x2 gap at (%d,%d): surviving panes did not close up", x, y)
			}
		}
	}
}
