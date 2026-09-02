package layout

import (
	"reflect"
	"testing"
)

func TestComputeSingleLeaf(t *testing.T) {
	root := NewLeaf(1)
	got := Compute(root, Rect{X: 0, Y: 0, W: 80, H: 24})
	want := map[PaneID]Rect{1: {X: 0, Y: 0, W: 80, H: 24}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestComputeHorizontalSplitAccountsForDivider(t *testing.T) {
	root := &Node{
		Orientation: Horizontal,
		Ratio:       0.5,
		A:           NewLeaf(1),
		B:           NewLeaf(2),
	}
	got := Compute(root, Rect{X: 0, Y: 0, W: 81, H: 24})
	// 81 - 1 divider = 80 usable, split evenly -> 40 and 40.
	if got[1] != (Rect{X: 0, Y: 0, W: 40, H: 24}) {
		t.Errorf("pane 1: got %+v", got[1])
	}
	if got[2] != (Rect{X: 41, Y: 0, W: 40, H: 24}) {
		t.Errorf("pane 2: got %+v", got[2])
	}
}

func TestComputeVerticalSplitAccountsForDivider(t *testing.T) {
	root := &Node{
		Orientation: Vertical,
		Ratio:       0.5,
		A:           NewLeaf(1),
		B:           NewLeaf(2),
	}
	got := Compute(root, Rect{X: 0, Y: 0, W: 80, H: 25})
	if got[1] != (Rect{X: 0, Y: 0, W: 80, H: 12}) {
		t.Errorf("pane 1: got %+v", got[1])
	}
	if got[2] != (Rect{X: 0, Y: 13, W: 80, H: 12}) {
		t.Errorf("pane 2: got %+v", got[2])
	}
}

func TestComputeNestedSplit(t *testing.T) {
	// Left pane, and a right column itself split top/bottom.
	root := &Node{
		Orientation: Horizontal,
		Ratio:       0.5,
		A:           NewLeaf(1),
		B: &Node{
			Orientation: Vertical,
			Ratio:       0.5,
			A:           NewLeaf(2),
			B:           NewLeaf(3),
		},
	}
	got := Compute(root, Rect{X: 0, Y: 0, W: 81, H: 25})
	if len(got) != 3 {
		t.Fatalf("expected 3 panes, got %d: %v", len(got), got)
	}
	if got[1] != (Rect{X: 0, Y: 0, W: 40, H: 25}) {
		t.Errorf("pane 1: got %+v", got[1])
	}
	if got[2] != (Rect{X: 41, Y: 0, W: 40, H: 12}) {
		t.Errorf("pane 2: got %+v", got[2])
	}
	if got[3] != (Rect{X: 41, Y: 13, W: 40, H: 12}) {
		t.Errorf("pane 3: got %+v", got[3])
	}
}

func TestSplitCreatesInteriorNode(t *testing.T) {
	root := NewLeaf(1)
	outer := Rect{X: 0, Y: 0, W: 80, H: 24}
	root, err := Split(root, 1, 2, Horizontal, outer)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if root.Orientation != Horizontal {
		t.Fatalf("root orientation = %v, want Horizontal", root.Orientation)
	}
	if got := Panes(root); !reflect.DeepEqual(got, []PaneID{1, 2}) {
		t.Fatalf("panes after split = %v, want [1 2]", got)
	}
}

func TestSplitRejectsWhenNoRoom(t *testing.T) {
	root := NewLeaf(1)
	outer := Rect{X: 0, Y: 0, W: 4, H: 24} // too narrow to split horizontally
	if _, err := Split(root, 1, 2, Horizontal, outer); err != ErrNoRoom {
		t.Fatalf("err = %v, want ErrNoRoom", err)
	}
}

func TestSplitUnknownPane(t *testing.T) {
	root := NewLeaf(1)
	outer := Rect{X: 0, Y: 0, W: 80, H: 24}
	if _, err := Split(root, 99, 2, Vertical, outer); err != ErrPaneNotFound {
		t.Fatalf("err = %v, want ErrPaneNotFound", err)
	}
}

func TestRemovePromotesSibling(t *testing.T) {
	root := NewLeaf(1)
	outer := Rect{X: 0, Y: 0, W: 80, H: 24}
	root, _ = Split(root, 1, 2, Horizontal, outer)
	root, _ = Split(root, 2, 3, Vertical, outer)
	// Tree: H(1, V(2,3)). Removing 1 should promote V(2,3) to root.
	root, focus, err := Remove(root, 1)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if root.Orientation != Vertical {
		t.Fatalf("root orientation = %v, want Vertical", root.Orientation)
	}
	if focus != 2 {
		t.Errorf("focus = %d, want 2", focus)
	}
	if got := Panes(root); !reflect.DeepEqual(got, []PaneID{2, 3}) {
		t.Errorf("panes = %v, want [2 3]", got)
	}
}

func TestRemoveLastPaneFails(t *testing.T) {
	root := NewLeaf(1)
	if _, _, err := Remove(root, 1); err != ErrLastPane {
		t.Fatalf("err = %v, want ErrLastPane", err)
	}
}

func TestResizeGrowsPaneAndShrinksSibling(t *testing.T) {
	root := &Node{Orientation: Horizontal, Ratio: 0.5, A: NewLeaf(1), B: NewLeaf(2)}
	outer := Rect{X: 0, Y: 0, W: 101, H: 24} // 100 usable
	if err := Resize(root, 1, Horizontal, 10, outer); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	got := Compute(root, outer)
	if got[1].W != 60 {
		t.Errorf("pane 1 width = %d, want 60", got[1].W)
	}
	if got[2].W != 40 {
		t.Errorf("pane 2 width = %d, want 40", got[2].W)
	}
}

func TestResizeFromSiblingSideFlipsSign(t *testing.T) {
	root := &Node{Orientation: Horizontal, Ratio: 0.5, A: NewLeaf(1), B: NewLeaf(2)}
	outer := Rect{X: 0, Y: 0, W: 101, H: 24}
	// Growing pane 2 (the B side) by 10 should also give it 60.
	if err := Resize(root, 2, Horizontal, 10, outer); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	got := Compute(root, outer)
	if got[2].W != 60 || got[1].W != 40 {
		t.Errorf("got pane1=%d pane2=%d, want 40/60", got[1].W, got[2].W)
	}
}

func TestResizeClampsToMinimum(t *testing.T) {
	root := &Node{Orientation: Vertical, Ratio: 0.5, A: NewLeaf(1), B: NewLeaf(2)}
	outer := Rect{X: 0, Y: 0, W: 80, H: 21} // 20 usable rows
	// Ask for an absurd shrink of pane 1; it must not collapse below the min.
	if err := Resize(root, 1, Vertical, -1000, outer); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	got := Compute(root, outer)
	if got[1].H < minPaneH {
		t.Errorf("pane 1 height = %d, below min %d", got[1].H, minPaneH)
	}
}

func TestResizeNoMatchingSplit(t *testing.T) {
	root := &Node{Orientation: Horizontal, Ratio: 0.5, A: NewLeaf(1), B: NewLeaf(2)}
	outer := Rect{X: 0, Y: 0, W: 80, H: 24}
	// No vertical (top/bottom) split anywhere: resizing vertically must report it.
	if err := Resize(root, 1, Vertical, 5, outer); err != ErrNoSplit {
		t.Fatalf("err = %v, want ErrNoSplit", err)
	}
}

func TestArrangeEvenHorizontalGivesEqualWidths(t *testing.T) {
	root := Arrange([]PaneID{1, 2, 3, 4}, LayoutEvenHorizontal)
	got := Compute(root, Rect{X: 0, Y: 0, W: 120, H: 30})
	if len(got) != 4 {
		t.Fatalf("want 4 panes, got %d", len(got))
	}
	for id, r := range got {
		if r.W < 27 || r.W > 30 { // ~ (120-3 dividers)/4
			t.Errorf("pane %d width %d not close to equal", id, r.W)
		}
		if r.H != 30 {
			t.Errorf("pane %d height %d, want 30", id, r.H)
		}
	}
}

func TestArrangeTiledIsGrid(t *testing.T) {
	root := Arrange([]PaneID{1, 2, 3, 4}, LayoutTiled)
	got := Compute(root, Rect{X: 0, Y: 0, W: 100, H: 40})
	if len(got) != 4 {
		t.Fatalf("want 4 panes, got %d", len(got))
	}
	// 2x2 grid: two distinct X positions and two distinct Y positions.
	xs := map[int]bool{}
	ys := map[int]bool{}
	for _, r := range got {
		xs[r.X] = true
		ys[r.Y] = true
	}
	if len(xs) != 2 || len(ys) != 2 {
		t.Errorf("tiled 4 panes: got %d columns, %d rows; want 2 and 2", len(xs), len(ys))
	}
}

func TestArrangePreservesPaneOrderAndCount(t *testing.T) {
	ids := []PaneID{5, 9, 2, 7, 1}
	for _, name := range []string{LayoutEvenHorizontal, LayoutEvenVertical, LayoutTiled, LayoutMainVertical, LayoutMainHorizontal} {
		root := Arrange(ids, name)
		got := Panes(root)
		if len(got) != len(ids) {
			t.Fatalf("%s: got %d panes, want %d", name, len(got), len(ids))
		}
		for i := range ids {
			if got[i] != ids[i] {
				t.Fatalf("%s: pane order changed: got %v want %v", name, got, ids)
			}
		}
	}
}

func TestNeighborWraps(t *testing.T) {
	root := NewLeaf(1)
	outer := Rect{X: 0, Y: 0, W: 120, H: 40}
	root, _ = Split(root, 1, 2, Horizontal, outer)
	root, _ = Split(root, 2, 3, Horizontal, outer)
	if got := Neighbor(root, 3, +1); got != 1 {
		t.Errorf("next after 3 = %d, want 1 (wrap)", got)
	}
	if got := Neighbor(root, 1, -1); got != 3 {
		t.Errorf("prev before 1 = %d, want 3 (wrap)", got)
	}
	if got := Neighbor(root, 1, +1); got != 2 {
		t.Errorf("next after 1 = %d, want 2", got)
	}
}
