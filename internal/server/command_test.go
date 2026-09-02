package server

import (
	"strings"
	"testing"
	"time"

	"github.com/MauricioJC3/ng_mux/internal/layout"
)

// --- state inspectors (take the session lock) ---

func windowCount(s *session) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.windows)
}

func paneCount(s *session) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.windows) == 0 {
		return 0
	}
	return len(s.windows[s.cur].panes)
}

func currentIdx(s *session) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

func currentName(s *session) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.windows[s.cur].name
}

func activePane(s *session) layout.PaneID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.windows[s.cur].active
}

func activeRect(s *session) layout.Rect {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.windows[s.cur]
	return layout.Compute(w.tree, w.outer(s.cols, s.contentRows()))[w.active]
}

func setupSession(t testing.TB) (*Server, *fakeFleet, *session) {
	t.Helper()
	srv, ff := newTestServer(t)
	sess, err := srv.getOrCreateSession("0")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return srv, ff, sess
}

func exec(t testing.TB, srv *Server, line string) string {
	t.Helper()
	out, err := srv.execCommand(nil, line)
	if err != nil {
		t.Fatalf("execCommand(%q): %v", line, err)
	}
	return out
}

func TestExecUnknownCommandErrors(t *testing.T) {
	srv, _, _ := setupSession(t)
	if _, err := srv.execCommand(nil, "does-not-exist"); err == nil ||
		!strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("err = %v, want an 'unknown command' error", err)
	}
}

func TestSplitWindowAddsPane(t *testing.T) {
	for _, line := range []string{"split-window -h", "split-window -v", "splitw -h"} {
		t.Run(line, func(t *testing.T) {
			srv, _, sess := setupSession(t)
			exec(t, srv, line)
			if got := paneCount(sess); got != 2 {
				t.Fatalf("pane count = %d, want 2", got)
			}
		})
	}
}

func TestNewWindowBecomesCurrent(t *testing.T) {
	for _, line := range []string{"new-window", "neww"} {
		t.Run(line, func(t *testing.T) {
			srv, _, sess := setupSession(t)
			exec(t, srv, line)
			if got := windowCount(sess); got != 2 {
				t.Fatalf("window count = %d, want 2", got)
			}
			if got := currentIdx(sess); got != 1 {
				t.Fatalf("current window = %d, want 1", got)
			}
		})
	}
}

func TestWindowNavigation(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "new-window")
	exec(t, srv, "new-window") // three windows, current = 2

	exec(t, srv, "select-window 0")
	if got := currentIdx(sess); got != 0 {
		t.Fatalf("after select-window 0: current = %d, want 0", got)
	}
	exec(t, srv, "next-window")
	if got := currentIdx(sess); got != 1 {
		t.Fatalf("after next-window: current = %d, want 1", got)
	}
	exec(t, srv, "prev") // alias of previous-window
	if got := currentIdx(sess); got != 0 {
		t.Fatalf("after prev: current = %d, want 0", got)
	}
	exec(t, srv, "previous-window") // wraps to the last
	if got := currentIdx(sess); got != 2 {
		t.Fatalf("after wrap: current = %d, want 2", got)
	}
	exec(t, srv, "select-window 99") // out of range: no change
	if got := currentIdx(sess); got != 2 {
		t.Fatalf("after out-of-range select: current = %d, want 2", got)
	}
}

func TestSelectWindowNeedsIndex(t *testing.T) {
	srv, _, _ := setupSession(t)
	if _, err := srv.execCommand(nil, "select-window"); err == nil {
		t.Fatal("select-window with no index should error")
	}
}

func TestRenameWindow(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "rename-window build-x")
	if got := currentName(sess); got != "build-x" {
		t.Fatalf("window name = %q, want %q", got, "build-x")
	}
	exec(t, srv, "renamew multi word name")
	if got := currentName(sess); got != "multi word name" {
		t.Fatalf("window name = %q, want %q", got, "multi word name")
	}
}

func TestSelectLayoutKeepsPaneCount(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "split-window -h")
	exec(t, srv, "split-window -v")
	if got := paneCount(sess); got != 3 {
		t.Fatalf("setup: pane count = %d, want 3", got)
	}
	exec(t, srv, "select-layout even-horizontal")
	if got := paneCount(sess); got != 3 {
		t.Fatalf("after select-layout: pane count = %d, want 3", got)
	}
}

func TestResizePaneWidensActivePane(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "split-window -h") // active is now the right-hand pane
	before := activeRect(sess).W
	exec(t, srv, "resize-pane -R 6")
	after := activeRect(sess).W
	if after <= before {
		t.Fatalf("active pane width did not grow: before=%d after=%d", before, after)
	}
}

func TestResizePaneNeedsDirection(t *testing.T) {
	srv, _, _ := setupSession(t)
	if _, err := srv.execCommand(nil, "resize-pane"); err == nil {
		t.Fatal("resize-pane with no direction should error")
	}
}

func TestKillWindowReducesCount(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "new-window")
	exec(t, srv, "kill-window")
	if got := windowCount(sess); got != 1 {
		t.Fatalf("window count after kill = %d, want 1", got)
	}
}

func TestSendKeysReachesActivePane(t *testing.T) {
	srv, ff, sess := setupSession(t)
	exec(t, srv, `send-keys "printf hi" Enter`)
	fp := ff.byID(activePane(sess))
	if fp == nil {
		t.Fatal("no fake pane for the active pane")
	}
	waitFor(t, func() bool { return fp.pt.sentToChild() == "printf hi\r" }, time.Second)
}

func TestClientOnlyCommandsRejectCLI(t *testing.T) {
	srv, _, _ := setupSession(t)
	for _, line := range []string{"detach-client", "next-session", "previous-session"} {
		if _, err := srv.execCommand(nil, line); err == nil ||
			!strings.Contains(err.Error(), "attached client") {
			t.Fatalf("%s from CLI: err = %v, want an 'attached client' error", line, err)
		}
	}
}

func TestExecMarksSessionDirty(t *testing.T) {
	srv, _, sess := setupSession(t)
	sess.frame() // clears needsRepaint
	exec(t, srv, "rename-window x")
	sess.mu.Lock()
	dirty := sess.needsRepaint
	sess.mu.Unlock()
	if !dirty {
		t.Fatal("a command should mark the session for repaint")
	}
}

func TestRegistryHasEveryNameAndAlias(t *testing.T) {
	for _, c := range commandList() {
		if registry[c.name] == nil {
			t.Errorf("command %q not registered", c.name)
		}
		for _, a := range c.aliases {
			if registry[a] != registry[c.name] {
				t.Errorf("alias %q does not resolve to command %q", a, c.name)
			}
		}
	}
}

func TestBuildRegistryRejectsDuplicates(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("buildRegistry should panic on a duplicate key")
		}
	}()
	buildRegistry([]command{
		{name: "dup", run: cmdDisplayMessage},
		{name: "dup", run: cmdDisplayMessage},
	})
}
