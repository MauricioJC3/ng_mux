package server

import (
	"strings"
	"testing"

	"github.com/MauricioJC3/ng_mux/internal/layout"
)

func paneIDsAt(s *session, winIdx int) []layout.PaneID {
	s.mu.Lock()
	defer s.mu.Unlock()
	if winIdx < 0 || winIdx >= len(s.windows) {
		return nil
	}
	return layout.Panes(s.windows[winIdx].tree)
}

func paneCountAt(s *session, winIdx int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if winIdx < 0 || winIdx >= len(s.windows) {
		return 0
	}
	return len(s.windows[winIdx].panes)
}

func TestSwapPaneReordersButKeepsFocus(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	exec(t, srv, "split-window -h") // panes 1,2,3 in order; active is 3

	exec(t, srv, "swap-pane -U") // swap 3 with its previous neighbour, 2

	if got := paneIDsAt(sess, 0); len(got) != 3 || got[1] != 3 || got[2] != 2 {
		t.Fatalf("pane order after swap -U = %v, want [1 3 2]", got)
	}
	if got := activePane(sess); got != 3 {
		t.Fatalf("focus moved off the swapped pane: active = %d, want 3", got)
	}
}

func TestBreakPaneMovesLivePaneToNewWindow(t *testing.T) {
	srv, ff, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	moved := activePane(sess)
	madeBefore := ff.count()

	exec(t, srv, "break-pane")

	if got := windowCount(sess); got != 2 {
		t.Fatalf("window count after break-pane = %d, want 2", got)
	}
	if got := ff.count(); got != madeBefore {
		t.Fatalf("break-pane created %d new pane(s); it must reuse the running one", got-madeBefore)
	}
	if got := paneCountAt(sess, 0); got != 1 {
		t.Fatalf("source window kept %d panes, want 1", got)
	}
	if got := paneIDsAt(sess, 1); len(got) != 1 || got[0] != moved {
		t.Fatalf("new window panes = %v, want [%d]", got, moved)
	}
	if currentIdx(sess) != 1 {
		t.Fatalf("break-pane did not focus the new window (cur = %d)", currentIdx(sess))
	}
}

func TestBreakPaneRefusedForLonePane(t *testing.T) {
	srv, _, _ := setupSession(t)
	if _, err := srv.execCommand(nil, "break-pane"); err == nil ||
		!strings.Contains(err.Error(), "only one pane") {
		t.Fatalf("break-pane on a lone pane: err = %v, want an 'only one pane' error", err)
	}
}

func TestJoinPanePullsPaneInAndDropsEmptyWindow(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "new-window") // window 1 is now current, window 0 has one pane
	srcPane := paneIDsAt(sess, 0)[0]

	exec(t, srv, "join-pane -s 0")

	if got := windowCount(sess); got != 1 {
		t.Fatalf("window count after join-pane = %d, want 1 (source emptied)", got)
	}
	ids := paneIDsAt(sess, 0)
	if len(ids) != 2 {
		t.Fatalf("target window panes = %v, want 2", ids)
	}
	var found bool
	for _, id := range ids {
		if id == srcPane {
			found = true
		}
	}
	if !found {
		t.Fatalf("joined pane %d not present in %v", srcPane, ids)
	}
	if got := activePane(sess); got != srcPane {
		t.Fatalf("join-pane should focus the pulled-in pane; active = %d, want %d", got, srcPane)
	}
}

func TestJoinPaneKeepsSourceWindowWhenItHasMore(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "split-window -h") // window 0 has two panes
	exec(t, srv, "new-window")      // window 1 current

	exec(t, srv, "join-pane -s 0")

	if got := windowCount(sess); got != 2 {
		t.Fatalf("window count = %d, want 2 (source still has a pane)", got)
	}
	if got := paneCountAt(sess, 0); got != 1 {
		t.Fatalf("source window kept %d panes, want 1", got)
	}
}

func TestJoinPaneSameWindowErrors(t *testing.T) {
	srv, _, _ := setupSession(t)
	if _, err := srv.execCommand(nil, "join-pane -s 0"); err == nil ||
		!strings.Contains(err.Error(), "same window") {
		t.Fatalf("join-pane -s 0 (current): err = %v, want a 'same window' error", err)
	}
}

func TestJoinPaneNoRoomLeavesBothWindowsIntact(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "new-window")
	sess.resize(4, 6) // too small to split the target

	_, err := srv.execCommand(nil, "join-pane -s 0 -h")
	if err == nil {
		t.Fatal("join-pane into a 4-column window should fail for lack of room")
	}
	if got := windowCount(sess); got != 2 {
		t.Fatalf("failed join-pane changed the window count to %d, want 2", got)
	}
	if got := paneCountAt(sess, 0); got != 1 {
		t.Fatalf("failed join-pane still moved a pane out of the source (%d left)", got)
	}
}
