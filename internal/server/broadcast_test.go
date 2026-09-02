package server

import "testing"

func TestSessionDirtyLifecycle(t *testing.T) {
	_, ff, sess := setupSession(t)

	if !sess.dirty() {
		t.Fatal("a new session should be dirty until its first frame")
	}
	sess.frame()
	if sess.dirty() {
		t.Fatal("session should be clean right after frame()")
	}

	ff.byID(1).scr.Write([]byte("x")) // pane produced output
	if !sess.dirty() {
		t.Fatal("pane output should mark the session dirty")
	}
	sess.frame()
	if sess.dirty() {
		t.Fatal("frame() should clear pane dirtiness")
	}

	sess.mu.Lock()
	sess.needsRepaint = true
	sess.mu.Unlock()
	if !sess.dirty() {
		t.Fatal("needsRepaint alone should count as dirty")
	}
}

func TestFrameBuffersPingPongBetweenTwo(t *testing.T) {
	_, _, sess := setupSession(t)
	f0 := sess.frame()
	f1 := sess.frame()
	f2 := sess.frame()
	f3 := sess.frame()

	if f0 == nil || f1 == nil {
		t.Fatal("frame() returned nil")
	}
	if f0 == f1 {
		t.Fatal("consecutive frames must use different buffers")
	}
	if f0 != f2 || f1 != f3 {
		t.Fatalf("frame buffers should cycle between exactly two (got %p,%p,%p,%p)", f0, f1, f2, f3)
	}
}
