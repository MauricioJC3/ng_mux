package render

import (
	"testing"

	"github.com/inre/tmux2/internal/layout"
)

func framesEqual(a, b *Frame) bool {
	if a.Cols != b.Cols || a.Rows != b.Rows || len(a.Cells) != len(b.Cells) {
		return false
	}
	if a.CurX != b.CurX || a.CurY != b.CurY || a.CurVisible != b.CurVisible {
		return false
	}
	for i := range a.Cells {
		if a.Cells[i] != b.Cells[i] {
			return false
		}
	}
	return true
}

func sampleScene() ([]PaneView, string, StatusStyle) {
	left := PaneView{ID: 1, Rect: layout.Rect{X: 0, Y: 0, W: 10, H: 5}, Snap: snapWith(10, 5, "hello"), Active: true}
	right := PaneView{ID: 2, Rect: layout.Rect{X: 11, Y: 0, W: 10, H: 5}, Snap: snapWith(10, 5, "world")}
	return []PaneView{left, right}, "STATUS", DefaultStatusStyle
}

func TestComposeIntoMatchesCompose(t *testing.T) {
	panes, status, style := sampleScene()
	want := Compose(21, 6, panes, status, style)

	got := ComposeInto(nil, 21, 6, panes, status, style)
	if !framesEqual(want, got) {
		t.Fatal("ComposeInto(nil) diverged from Compose")
	}
}

func TestComposeIntoReusesBufferWithoutCorruption(t *testing.T) {
	panes, status, style := sampleScene()
	want := Compose(21, 6, panes, status, style)

	f := NewFrame(3, 3) // deliberately the wrong size
	f = ComposeInto(f, 21, 6, panes, status, style)
	if !framesEqual(want, f) {
		t.Fatal("first ComposeInto into a wrong-sized frame diverged")
	}

	cellsBefore := &f.Cells[0]

	// Paint an unrelated scene into the same frame, then the original again.
	ComposeInto(f, 21, 6, nil, "other status entirely", style)
	f = ComposeInto(f, 21, 6, panes, status, style)
	if !framesEqual(want, f) {
		t.Fatal("reused frame diverged from a fresh Compose")
	}
	if &f.Cells[0] != cellsBefore {
		t.Error("same-size ComposeInto reallocated Cells instead of reusing them")
	}
}
