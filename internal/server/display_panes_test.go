package server

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDisplayPanesBadgesEveryPane(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	exec(t, srv, "split-window -v") // 3 panes

	exec(t, srv, "display-panes")

	if !dirtyNow(sess) {
		t.Fatal("display-panes should make the session dirty")
	}
	for _, v := range windowViews(sess) {
		if strings.TrimSpace(v.Badge) == "" {
			t.Fatalf("pane %d has no index badge while display-panes is active", v.ID)
		}
	}
}

func TestDisplayPanesBadgeIndicesMatchSelectPane(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	exec(t, srv, "split-window -v")
	exec(t, srv, "display-panes")

	for _, v := range windowViews(sess) {
		n, err := strconv.Atoi(strings.TrimSpace(v.Badge))
		if err != nil {
			t.Fatalf("badge %q is not a number: %v", v.Badge, err)
		}
		exec(t, srv, "select-pane "+strconv.Itoa(n))
		if got := activePane(sess); got != v.ID {
			t.Fatalf("badge %d pointed at pane %d, but select-pane %d focused pane %d",
				n, v.ID, n, got)
		}
	}
}

func TestDisplayPanesExpiresAfterOneRepaint(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	exec(t, srv, "display-panes")

	sess.frame() // draws the badges
	sess.mu.Lock()
	if !sess.panesShown {
		sess.mu.Unlock()
		t.Fatal("frame did not record that badges were shown")
	}
	sess.displayPanesUntil = time.Now().Add(-time.Millisecond) // force expiry
	sess.mu.Unlock()

	if !dirtyNow(sess) {
		t.Fatal("an expired display-panes still owes one repaint to wipe the badges")
	}
	sess.frame()
	for _, v := range windowViews(sess) {
		if v.Badge != "" {
			t.Fatalf("pane %d still badged after display-panes expired", v.ID)
		}
	}
	if dirtyNow(sess) {
		t.Fatal("session should be clean once the badges are wiped")
	}
}
