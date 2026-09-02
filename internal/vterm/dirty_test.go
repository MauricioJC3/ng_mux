package vterm

import "testing"

func TestDirtyStartsTrueAndClearsOnSnapshot(t *testing.T) {
	term := New(20, 5, nil)
	if !term.Dirty() {
		t.Fatal("a fresh Term should be dirty so its first frame is drawn")
	}
	term.Snapshot()
	if term.Dirty() {
		t.Fatal("Snapshot should clear the dirty flag")
	}
}

func TestDirtySetByWriteAndResize(t *testing.T) {
	term := New(20, 5, nil)
	term.Snapshot() // clean

	if _, err := term.Write([]byte("hi")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !term.Dirty() {
		t.Fatal("Write should mark the Term dirty")
	}

	term.Snapshot() // clean again
	term.Resize(30, 8)
	if !term.Dirty() {
		t.Fatal("Resize should mark the Term dirty")
	}
}

func TestSnapshotIntoReusesBufferAndMatchesSnapshot(t *testing.T) {
	term := New(12, 4, nil)
	term.Write([]byte("abcdef"))

	want := term.Snapshot()

	var dst Snapshot
	term.SnapshotInto(&dst)
	backing := &dst.Cells[0]

	if dst.Cols != want.Cols || dst.Rows != want.Rows || len(dst.Cells) != len(want.Cells) {
		t.Fatalf("SnapshotInto shape = %dx%d/%d, want %dx%d/%d",
			dst.Cols, dst.Rows, len(dst.Cells), want.Cols, want.Rows, len(want.Cells))
	}
	for i := range want.Cells {
		if dst.Cells[i] != want.Cells[i] {
			t.Fatalf("cell %d: SnapshotInto=%+v Snapshot=%+v", i, dst.Cells[i], want.Cells[i])
		}
	}

	term.Write([]byte("Z"))
	term.SnapshotInto(&dst)
	if &dst.Cells[0] != backing {
		t.Error("SnapshotInto reallocated its buffer instead of reusing it")
	}
}
