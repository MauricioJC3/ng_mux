// Package layout models pane geometry as a binary split tree, the same shape
// tmux uses. Interior nodes split their area either left/right (Horizontal) or
// top/bottom (Vertical); leaves carry a pane ID. Compute walks the tree and
// assigns every leaf a concrete rectangle for a given outer size.
package layout

import "errors"

// Orientation of an interior split.
type Orientation int

const (
	// Leaf is not a split; the node carries a PaneID.
	Leaf Orientation = iota
	// Horizontal splits into a left and a right child (a vertical divider).
	Horizontal
	// Vertical splits into a top and a bottom child (a horizontal divider).
	Vertical
)

// PaneID identifies a pane. Zero is never a valid pane.
type PaneID int

// Rect is a screen rectangle in cell coordinates. X,Y is the top-left corner.
type Rect struct {
	X, Y, W, H int
}

// Node is one node of the split tree. A Leaf has Orientation==Leaf, a non-zero
// Pane, and nil children. An interior node has two non-nil children and a
// Ratio in (0,1) giving the fraction of space assigned to child A.
type Node struct {
	Orientation Orientation
	Pane        PaneID
	Ratio       float64
	A, B        *Node
}

// NewLeaf returns a leaf node for pane id.
func NewLeaf(id PaneID) *Node {
	return &Node{Orientation: Leaf, Pane: id}
}

// dividerSize is the thickness in cells of the border drawn between panes.
const dividerSize = 1

// minPaneW and minPaneH are the smallest interior area a pane may be given.
// Splits that cannot honour these are rejected.
const (
	minPaneW = 3
	minPaneH = 2
)

var (
	// ErrPaneNotFound is returned when a target pane is not in the tree.
	ErrPaneNotFound = errors.New("layout: pane not found")
	// ErrNoRoom is returned when a split would leave a pane below the minimum.
	ErrNoRoom = errors.New("layout: not enough room to split")
	// ErrLastPane is returned when removing the only remaining pane.
	ErrLastPane = errors.New("layout: cannot remove the last pane")
	// ErrNoSplit is returned when a resize finds no ancestor split of the
	// requested orientation (e.g. a lone pane, or only splits the other way).
	ErrNoSplit = errors.New("layout: no split to resize in that direction")
)

// Compute assigns a rectangle to every leaf pane within outer.
func Compute(root *Node, outer Rect) map[PaneID]Rect {
	out := make(map[PaneID]Rect)
	if root != nil {
		compute(root, outer, out)
	}
	return out
}

func compute(n *Node, r Rect, out map[PaneID]Rect) {
	if n.Orientation == Leaf {
		out[n.Pane] = r
		return
	}
	a, b := splitRect(n.Orientation, r, n.Ratio)
	compute(n.A, a, out)
	compute(n.B, b, out)
}

// splitRect divides r into two sub-rectangles separated by a one-cell divider.
// The two children plus the divider always add up to exactly r along the split
// axis, so no child can ever escape r no matter how deeply (or impossibly)
// nested the tree is. Each side is kept to at least one cell whenever there is
// room for the divider, so a crowded tree degrades by shrinking panes evenly
// rather than collapsing some to zero and leaving a phantom band.
func splitRect(o Orientation, r Rect, ratio float64) (a, b Rect) {
	if o == Horizontal {
		aw, dv, bw := splitAxis(r.W, ratio)
		a = Rect{X: r.X, Y: r.Y, W: aw, H: r.H}
		b = Rect{X: r.X + aw + dv, Y: r.Y, W: bw, H: r.H}
		return a, b
	}
	ah, dv, bh := splitAxis(r.H, ratio)
	a = Rect{X: r.X, Y: r.Y, W: r.W, H: ah}
	b = Rect{X: r.X, Y: r.Y + ah + dv, W: r.W, H: bh}
	return a, b
}

// splitAxis partitions total cells into child A, a divider, and child B. It
// guarantees aSpan + divider + bSpan == max(total, 0). When there is no room
// for the divider the divider is dropped; A gets at least one cell and at most
// all of the remainder once the divider fits.
func splitAxis(total int, ratio float64) (aSpan, divider, bSpan int) {
	if total <= 0 {
		return 0, 0, 0
	}
	if total <= dividerSize {
		return total, 0, 0
	}
	avail := total - dividerSize // >= 1
	a := int(float64(avail) * ratio)
	if a < 1 {
		a = 1
	}
	if a > avail-1 {
		a = avail - 1
	}
	if a < 0 {
		a = 0
	}
	return a, dividerSize, avail - a
}

// Split replaces the leaf holding target with an interior node whose A child
// keeps target and whose B child is a new leaf with pane newID. dir selects
// the split orientation. outer is the current outer size, used to check that
// both halves stay above the minimum. It returns ErrNoRoom otherwise.
func Split(root *Node, target, newID PaneID, dir Orientation, outer Rect) (*Node, error) {
	if dir != Horizontal && dir != Vertical {
		return root, errors.New("layout: split direction must be Horizontal or Vertical")
	}
	leaf, _ := find(root, target)
	if leaf == nil {
		return root, ErrPaneNotFound
	}
	rects := Compute(root, outer)
	cur := rects[target]
	if !fits(dir, cur) {
		return root, ErrNoRoom
	}
	*leaf = Node{
		Orientation: dir,
		Ratio:       0.5,
		A:           NewLeaf(target),
		B:           NewLeaf(newID),
	}
	return root, nil
}

func fits(dir Orientation, r Rect) bool {
	if dir == Horizontal {
		return (r.W-dividerSize)/2 >= minPaneW && r.H >= minPaneH
	}
	return (r.H-dividerSize)/2 >= minPaneH && r.W >= minPaneW
}

// FitsMinimum reports whether every pane in root is given at least
// minPaneW x minPaneH cells when the tree is computed inside outer. It is the
// check select-layout / Arrange callers use to refuse a preset that would
// collapse panes, the same floor Split enforces on an interactive split. A nil
// root has no panes and trivially fits.
func FitsMinimum(root *Node, outer Rect) bool {
	for _, r := range Compute(root, outer) {
		if r.W < minPaneW || r.H < minPaneH {
			return false
		}
	}
	return true
}

// Resize nudges the pane's boundary by delta cells along the given axis.
// dir == Horizontal changes the pane's width (adjusts the nearest ancestor
// left/right split); dir == Vertical changes its height. A positive delta
// grows the pane. The change is clamped so neither side of the affected split
// drops below the minimum pane size. It returns ErrNoSplit when the pane has
// no ancestor split of that orientation.
func Resize(root *Node, target PaneID, dir Orientation, delta int, outer Rect) error {
	if dir != Horizontal && dir != Vertical {
		return errors.New("layout: resize direction must be Horizontal or Vertical")
	}
	path := make([]pathStep, 0, 8)
	if !walkPath(root, outer, target, &path) {
		return ErrPaneNotFound
	}
	// Walk up from the leaf to the first split matching dir.
	for i := len(path) - 1; i >= 1; i-- {
		parent := path[i-1]
		if parent.node.Orientation != dir {
			continue
		}
		inA := parent.node.A == path[i].node
		span := parent.rect.W
		if dir == Vertical {
			span = parent.rect.H
		}
		avail := float64(span - dividerSize)
		if avail <= 0 {
			return ErrNoRoom
		}
		d := float64(delta) / avail
		if !inA {
			d = -d
		}
		minFrac := minSideFrac(dir, avail)
		parent.node.Ratio = clamp(parent.node.Ratio+d, minFrac, 1-minFrac)
		return nil
	}
	return ErrNoSplit
}

type pathStep struct {
	node *Node
	rect Rect
}

// walkPath records the node and rect of every ancestor of target plus the leaf
// itself, root first. It returns false if target is not present.
func walkPath(n *Node, r Rect, target PaneID, out *[]pathStep) bool {
	if n == nil {
		return false
	}
	*out = append(*out, pathStep{node: n, rect: r})
	if n.Orientation == Leaf {
		if n.Pane == target {
			return true
		}
		*out = (*out)[:len(*out)-1]
		return false
	}
	a, b := splitRect(n.Orientation, r, n.Ratio)
	if walkPath(n.A, a, target, out) {
		return true
	}
	if walkPath(n.B, b, target, out) {
		return true
	}
	*out = (*out)[:len(*out)-1]
	return false
}

func minSideFrac(dir Orientation, avail float64) float64 {
	min := float64(minPaneW)
	if dir == Vertical {
		min = float64(minPaneH)
	}
	if avail <= 0 {
		return 0.1
	}
	f := min / avail
	if f > 0.45 {
		f = 0.45
	}
	return f
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Remove deletes the leaf holding target and collapses its parent, promoting
// the sibling subtree in its place. It returns the new root and the pane that
// should receive focus next (the first leaf of the promoted sibling).
func Remove(root *Node, target PaneID) (*Node, PaneID, error) {
	if root == nil {
		return nil, 0, ErrPaneNotFound
	}
	if root.Orientation == Leaf {
		if root.Pane == target {
			return root, 0, ErrLastPane
		}
		return root, 0, ErrPaneNotFound
	}
	newRoot, focus, ok := removeIn(root, target)
	if !ok {
		return root, 0, ErrPaneNotFound
	}
	return newRoot, focus, nil
}

// removeIn returns the possibly-replaced subtree, the pane to focus, and
// whether target was found beneath n.
func removeIn(n *Node, target PaneID) (*Node, PaneID, bool) {
	if n.Orientation == Leaf {
		return n, 0, n.Pane == target
	}
	if n.A.Orientation == Leaf && n.A.Pane == target {
		return n.B, firstLeaf(n.B), true
	}
	if n.B.Orientation == Leaf && n.B.Pane == target {
		return n.A, firstLeaf(n.A), true
	}
	if repl, focus, ok := removeIn(n.A, target); ok {
		n.A = repl
		return n, focus, true
	}
	if repl, focus, ok := removeIn(n.B, target); ok {
		n.B = repl
		return n, focus, true
	}
	return n, 0, false
}

// Panes returns every pane ID in left-to-right, depth-first order. This is the
// order focus cycling walks.
func Panes(root *Node) []PaneID {
	var ids []PaneID
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Orientation == Leaf {
			ids = append(ids, n.Pane)
			return
		}
		walk(n.A)
		walk(n.B)
	}
	walk(root)
	return ids
}

// Neighbor returns the pane after (delta=+1) or before (delta=-1) cur in
// traversal order, wrapping around. It returns cur unchanged if cur is absent.
func Neighbor(root *Node, cur PaneID, delta int) PaneID {
	ids := Panes(root)
	for i, id := range ids {
		if id == cur {
			n := (i + delta + len(ids)) % len(ids)
			return ids[n]
		}
	}
	if len(ids) > 0 {
		return ids[0]
	}
	return cur
}

// Preset layout names accepted by Arrange.
const (
	LayoutEvenHorizontal = "even-horizontal"
	LayoutEvenVertical   = "even-vertical"
	LayoutTiled          = "tiled"
	LayoutMainVertical   = "main-vertical"
	LayoutMainHorizontal = "main-horizontal"
)

// Arrange builds a fresh split tree that places ids in the named preset layout.
// The pane order is preserved. An unknown name falls back to even-horizontal.
// A nil/empty id list yields nil.
func Arrange(ids []PaneID, name string) *Node {
	switch len(ids) {
	case 0:
		return nil
	case 1:
		return NewLeaf(ids[0])
	}
	switch name {
	case LayoutEvenVertical:
		return evenChain(ids, Vertical)
	case LayoutTiled:
		return tiled(ids)
	case LayoutMainVertical:
		return mainSplit(ids, Horizontal)
	case LayoutMainHorizontal:
		return mainSplit(ids, Vertical)
	case LayoutEvenHorizontal:
		fallthrough
	default:
		return evenChain(ids, Horizontal)
	}
}

// evenChain nests splits so every pane gets an equal share along one axis.
func evenChain(ids []PaneID, dir Orientation) *Node {
	if len(ids) == 1 {
		return NewLeaf(ids[0])
	}
	return &Node{
		Orientation: dir,
		Ratio:       1.0 / float64(len(ids)),
		A:           NewLeaf(ids[0]),
		B:           evenChain(ids[1:], dir),
	}
}

// mainSplit gives ids[0] half the area and evenly stacks the rest beside it.
func mainSplit(ids []PaneID, dir Orientation) *Node {
	rest := Vertical
	if dir == Vertical {
		rest = Horizontal
	}
	return &Node{
		Orientation: dir,
		Ratio:       0.5,
		A:           NewLeaf(ids[0]),
		B:           evenChain(ids[1:], rest),
	}
}

// tiled lays ids out in a roughly square grid, row-major.
func tiled(ids []PaneID) *Node {
	n := len(ids)
	cols := 1
	for cols*cols < n {
		cols++
	}
	rows := (n + cols - 1) / cols

	rowNodes := make([]*Node, 0, rows)
	for r := 0; r < rows; r++ {
		start := r * cols
		end := start + cols
		if end > n {
			end = n
		}
		rowNodes = append(rowNodes, evenChain(ids[start:end], Horizontal))
	}
	return stack(rowNodes, Vertical)
}

// stack nests a slice of subtrees along one axis with equal shares.
func stack(nodes []*Node, dir Orientation) *Node {
	if len(nodes) == 1 {
		return nodes[0]
	}
	return &Node{
		Orientation: dir,
		Ratio:       1.0 / float64(len(nodes)),
		A:           nodes[0],
		B:           stack(nodes[1:], dir),
	}
}

func find(n *Node, target PaneID) (*Node, *Node) {
	if n == nil {
		return nil, nil
	}
	if n.Orientation == Leaf {
		if n.Pane == target {
			return n, nil
		}
		return nil, nil
	}
	if l, _ := find(n.A, target); l != nil {
		return l, n
	}
	if l, _ := find(n.B, target); l != nil {
		return l, n
	}
	return nil, nil
}

func firstLeaf(n *Node) PaneID {
	for n.Orientation != Leaf {
		n = n.A
	}
	return n.Pane
}
