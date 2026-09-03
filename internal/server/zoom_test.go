package server

import (
	"strings"
	"testing"
	"time"

	"github.com/MauricioJC3/ng_mux/internal/render"
	"github.com/MauricioJC3/ng_mux/internal/vterm"
)

// windowViews returns the render views the current window would produce at the
// session's current size.
func windowViews(s *session) []render.PaneView {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.windows[s.cur]
	views, _ := w.views(s.cols, s.contentRows(), nil, []vterm.Snapshot(nil))
	return views
}

func isZoomed(s *session) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.windows[s.cur].zoom != 0
}

func TestZoomShowsOnlyTheActivePane(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	exec(t, srv, "split-window -v") // 3 panes
	if got := len(windowViews(sess)); got != 3 {
		t.Fatalf("tiled window rendered %d views, want 3", got)
	}

	exec(t, srv, "resize-pane -Z")

	views := windowViews(sess)
	if len(views) != 1 {
		t.Fatalf("zoomed window rendered %d views, want 1", len(views))
	}
	v := views[0]
	if v.ID != activePane(sess) {
		t.Fatalf("zoomed view is pane %d, want the active pane %d", v.ID, activePane(sess))
	}
	if v.Rect.W != sess.cols || v.Rect.H != sess.contentRows() {
		t.Fatalf("zoomed view rect = %dx%d, want the full %dx%d content area",
			v.Rect.W, v.Rect.H, sess.cols, sess.contentRows())
	}
	if !v.Active {
		t.Fatal("zoomed view should be marked active")
	}
}

func TestZoomResizesActivePaneToFullArea(t *testing.T) {
	srv, ff, sess := setupSession(t)
	exec(t, srv, "split-window -h")

	active := activePane(sess)
	beforeW, _ := ff.byID(active).scr.size()
	if beforeW >= sess.cols {
		t.Fatalf("active pane started at width %d; expected less than %d", beforeW, sess.cols)
	}

	exec(t, srv, "resize-pane -Z")

	waitFor(t, func() bool {
		w, h := ff.byID(active).scr.size()
		return w == sess.cols && h == sess.contentRows()
	}, time.Second)
	if c, r := ff.byID(active).pt.size(); c != sess.cols || r != sess.contentRows() {
		t.Fatalf("zoomed pane pty size = %dx%d, want %dx%d", c, r, sess.cols, sess.contentRows())
	}
}

func TestZoomToggleRestoresTiling(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	exec(t, srv, "resize-pane -Z")
	if !isZoomed(sess) {
		t.Fatal("first toggle should have zoomed")
	}
	exec(t, srv, "resize-pane -Z")
	if isZoomed(sess) {
		t.Fatal("second toggle should have un-zoomed")
	}
	if got := len(windowViews(sess)); got != 2 {
		t.Fatalf("after un-zoom the window rendered %d views, want 2", got)
	}
}

func TestZoomClearedBySplit(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	exec(t, srv, "resize-pane -Z")
	exec(t, srv, "split-window -v")
	if isZoomed(sess) {
		t.Fatal("splitting a zoomed window must drop the zoom")
	}
	if got := len(windowViews(sess)); got != 3 {
		t.Fatalf("rendered %d views after split, want 3", got)
	}
}

func TestZoomClearedBySelectPane(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	exec(t, srv, "resize-pane -Z")
	exec(t, srv, "select-pane")
	if isZoomed(sess) {
		t.Fatal("changing focus must drop the zoom")
	}
}

func TestZoomIgnoredWithLonePane(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "resize-pane -Z")
	if isZoomed(sess) {
		t.Fatal("a single-pane window has nothing to zoom")
	}
}

func TestZoomSelfHealsWhenZoomedPaneCloses(t *testing.T) {
	srv, ff, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	zoomed := activePane(sess)
	exec(t, srv, "resize-pane -Z")

	ff.byID(zoomed).pt.Close()
	waitFor(t, func() bool { return paneCount(sess) == 1 }, 2*time.Second)

	if isZoomed(sess) {
		t.Fatal("zoom should clear when the zoomed pane closes")
	}
	if got := len(windowViews(sess)); got != 1 {
		t.Fatalf("rendered %d views after the zoomed pane closed, want 1", got)
	}
}

func TestZoomIndicatorInStatusBar(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	exec(t, srv, "resize-pane -Z")

	sess.mu.Lock()
	segs := sess.buildStatus(sess.cols)
	sess.mu.Unlock()

	var joined string
	for _, s := range segs {
		joined += s.Text
	}
	if !strings.Contains(joined, "ZOOM") {
		t.Fatalf("status bar %q has no ZOOM indicator while zoomed", joined)
	}
}
