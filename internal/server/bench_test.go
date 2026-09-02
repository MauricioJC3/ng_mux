package server

import "testing"

// BenchmarkSessionFrameActive measures one full compose of a session that has
// fresh pane output every iteration: SnapshotInto + ComposeInto into the
// ping-ponged frame buffer. This is the cost paid per tick for a session the
// user is actively working in.
func BenchmarkSessionFrameActive(b *testing.B) {
	_, ff, sess := setupSession(b)
	pane := ff.byID(1)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pane.scr.Write([]byte("some shell output line\r\n"))
		_ = sess.frame()
	}
}

// BenchmarkBroadcastGateIdle measures the check the broadcast loop runs every
// tick to decide whether a session needs repainting at all. A quiet session
// must fall through here allocating nothing, so an idle daemon does no work.
func BenchmarkBroadcastGateIdle(b *testing.B) {
	_, _, sess := setupSession(b)
	sess.frame() // clean baseline

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if sess.dirty() {
			b.Fatal("session unexpectedly dirty")
		}
	}
}
