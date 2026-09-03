package layout

import (
	"fmt"
	"testing"
)

// These tests treat Compute as a black box and assert the geometric contract a
// renderer relies on: panes never overlap, never leave the outer rectangle, and
// the only uncovered cells are single-cell dividers (no "fat" gaps where two
// panes drifted apart). They sweep pane counts, outer sizes that force ratio
// truncation, every preset layout, and the state left behind by Remove.

type grid struct {
	w, h  int
	count []int // count[y*w+x] == number of panes covering that cell
}

func newGrid(w, h int) *grid { return &grid{w: w, h: h, count: make([]int, w*h)} }

func (g *grid) fill(r Rect) {
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			if x >= 0 && y >= 0 && x < g.w && y < g.h {
				g.count[y*g.w+x]++
			}
		}
	}
}

func (g *grid) covered(x, y int) bool {
	if x < 0 || y < 0 || x >= g.w || y >= g.h {
		return false
	}
	return g.count[y*g.w+x] > 0
}

// assertInvariants checks the full geometric contract for rects laid out inside
// an outer of size w x h.
func assertInvariants(t *testing.T, rects map[PaneID]Rect, w, h int) {
	t.Helper()

	g := newGrid(w, h)
	for id, r := range rects {
		if r.X < 0 || r.Y < 0 || r.X+r.W > w || r.Y+r.H > h {
			t.Fatalf("pane %d rect %+v escapes the %dx%d outer", id, r, w, h)
		}
		if r.W < 1 || r.H < 1 {
			t.Fatalf("pane %d collapsed to %+v (zero-area pane)", id, r)
		}
		g.fill(r)
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if g.count[y*g.w+x] > 1 {
				t.Fatalf("cell (%d,%d) is covered by %d panes; panes must not overlap",
					x, y, g.count[y*g.w+x])
			}
		}
	}

	// No 2x2 block may be entirely uncovered: a correct divider is exactly one
	// cell thick, so a 2x2 hole means two panes failed to meet.
	for y := 0; y < h-1; y++ {
		for x := 0; x < w-1; x++ {
			if !g.covered(x, y) && !g.covered(x+1, y) &&
				!g.covered(x, y+1) && !g.covered(x+1, y+1) {
				t.Fatalf("2x2 gap at (%d,%d): panes are not adapting to fill the space", x, y)
			}
		}
	}
}

var presetNames = []string{
	LayoutEvenHorizontal,
	LayoutEvenVertical,
	LayoutTiled,
	LayoutMainVertical,
	LayoutMainHorizontal,
}

func TestComputeGeometryInvariants(t *testing.T) {
	// Odd and prime sizes make float(avail)*ratio truncate on most splits.
	// Every size here can still hold 8 panes along either axis (2*8-1 cells),
	// so a well-behaved layout must never collapse a pane to zero area.
	sizes := []struct{ w, h int }{
		{80, 24}, {132, 43}, {100, 40},
		{81, 25}, {97, 31}, {113, 37},
		{48, 24}, {51, 29},
	}

	for count := 2; count <= 8; count++ {
		ids := make([]PaneID, count)
		for i := range ids {
			ids[i] = PaneID(i + 1)
		}
		for _, name := range presetNames {
			tree := Arrange(ids, name)
			for _, sz := range sizes {
				t.Run(fmt.Sprintf("%s/%dpanes/%dx%d", name, count, sz.w, sz.h), func(t *testing.T) {
					rects := Compute(tree, Rect{W: sz.w, H: sz.h})
					if len(rects) != count {
						t.Fatalf("got %d rects, want %d", len(rects), count)
					}
					assertInvariants(t, rects, sz.w, sz.h)
				})
			}
		}
	}
}

// When more panes are asked for than the outer can physically hold (each pane
// needs at least one cell plus a one-cell divider), some panes unavoidably get
// zero area. That case must still never corrupt the layout: no pane may escape
// the outer and no two panes may overlap. This guards the "open a bunch of
// panes in a small window" path from rendering garbage.
func TestComputeGeometryTooManyPanesStaysConsistent(t *testing.T) {
	tiny := []struct{ w, h int }{{20, 8}, {12, 6}, {30, 10}, {9, 9}}
	for count := 6; count <= 12; count++ {
		ids := make([]PaneID, count)
		for i := range ids {
			ids[i] = PaneID(i + 1)
		}
		for _, name := range presetNames {
			tree := Arrange(ids, name)
			for _, sz := range tiny {
				t.Run(fmt.Sprintf("%s/%dpanes/%dx%d", name, count, sz.w, sz.h), func(t *testing.T) {
					g := newGrid(sz.w, sz.h)
					for id, r := range Compute(tree, Rect{W: sz.w, H: sz.h}) {
						if r.X < 0 || r.Y < 0 || r.X+r.W > sz.w || r.Y+r.H > sz.h {
							t.Fatalf("pane %d rect %+v escapes the %dx%d outer", id, r, sz.w, sz.h)
						}
						if r.W < 0 || r.H < 0 {
							t.Fatalf("pane %d has a negative dimension: %+v", id, r)
						}
						g.fill(r)
					}
					for i, n := range g.count {
						if n > 1 {
							t.Fatalf("cell %d covered by %d panes; overlap even when space is tight", i, n)
						}
					}
				})
			}
		}
	}
}

func TestComputeGeometryAfterInteractiveSplits(t *testing.T) {
	// Build the tree the way a user does: repeated Split on the active pane,
	// alternating direction. This is the shape bug reports come from.
	outer := Rect{W: 120, H: 40}
	root := NewLeaf(1)
	active := PaneID(1)
	for next := PaneID(2); next <= 7; next++ {
		dir := Horizontal
		if next%2 == 0 {
			dir = Vertical
		}
		newRoot, err := Split(root, active, next, dir, outer)
		if err != nil {
			t.Fatalf("Split #%d: %v", next, err)
		}
		root = newRoot
		active = next
		assertInvariants(t, Compute(root, outer), outer.W, outer.H)
	}
}

func TestComputeGeometryAfterRemove(t *testing.T) {
	outer := Rect{W: 100, H: 30}
	ids := []PaneID{1, 2, 3, 4, 5}

	for _, name := range presetNames {
		for _, victim := range ids {
			t.Run(fmt.Sprintf("%s/remove%d", name, victim), func(t *testing.T) {
				root := Arrange(ids, name) // fresh tree per case
				newRoot, _, err := Remove(root, victim)
				if err != nil {
					t.Fatalf("Remove(%d): %v", victim, err)
				}
				rects := Compute(newRoot, outer)
				if len(rects) != len(ids)-1 {
					t.Fatalf("after removing %d: %d rects, want %d", victim, len(rects), len(ids)-1)
				}
				if _, stillThere := rects[victim]; stillThere {
					t.Fatalf("removed pane %d is still in the layout", victim)
				}
				assertInvariants(t, rects, outer.W, outer.H)
			})
		}
	}
}
