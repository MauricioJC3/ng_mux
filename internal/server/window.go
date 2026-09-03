package server

import (
	"fmt"

	"github.com/MauricioJC3/ng_mux/internal/layout"
	"github.com/MauricioJC3/ng_mux/internal/render"
	"github.com/MauricioJC3/ng_mux/internal/vterm"
)

// window is one window of a session: a binary split tree of panes with exactly
// one focused pane. Every method assumes the owning session's mutex is held.
type window struct {
	id     int
	name   string
	tree   *layout.Node
	panes  map[layout.PaneID]*pane
	active layout.PaneID

	shell     string
	histLimit int
	newPane   paneFactory
}

// newWindow creates a window with a single pane sized to the given content area.
func newWindow(id int, name string, paneID layout.PaneID, cols, rows int, shell string, histLimit int, newPane paneFactory) (*window, error) {
	if newPane == nil {
		newPane = startPane
	}
	p, err := newPane(paneID, cols, rows, shell, histLimit)
	if err != nil {
		return nil, err
	}
	w := &window{
		id:        id,
		name:      name,
		tree:      layout.NewLeaf(paneID),
		panes:     map[layout.PaneID]*pane{paneID: p},
		active:    paneID,
		shell:     shell,
		histLimit: histLimit,
		newPane:   newPane,
	}
	p.win = w
	return w, nil
}

// resizeStep is how many cells one resize keystroke moves a boundary.
const resizeStep = 2

func (w *window) outer(cols, rows int) layout.Rect {
	return layout.Rect{X: 0, Y: 0, W: cols, H: rows}
}

// applyLayout pushes each pane's computed rectangle to its pty and emulator.
func (w *window) applyLayout(cols, rows int) {
	for id, r := range layout.Compute(w.tree, w.outer(cols, rows)) {
		if p := w.panes[id]; p != nil {
			p.resize(max1(r.W), max1(r.H))
		}
	}
}

// split adds a pane beside the active one, sizing it to the new layout.
func (w *window) split(sess *session, dir layout.Orientation, cols, rows int) error {
	newID := sess.nextPane()
	newTree, err := layout.Split(w.tree, w.active, newID, dir, w.outer(cols, rows))
	if err != nil {
		return err
	}
	r := layout.Compute(newTree, w.outer(cols, rows))[newID]
	p, err := w.newPane(newID, max1(r.W), max1(r.H), w.shell, w.histLimit)
	if err != nil {
		return err
	}
	p.win = w
	w.tree = newTree
	w.panes[newID] = p
	w.active = newID
	go p.pump(sess.reportExit)
	w.applyLayout(cols, rows)
	return nil
}

func (w *window) focus(delta int) {
	w.active = layout.Neighbor(w.tree, w.active, delta)
}

func (w *window) resizeActive(dir layout.Orientation, delta, cols, rows int) {
	if err := layout.Resize(w.tree, w.active, dir, delta, w.outer(cols, rows)); err == nil {
		w.applyLayout(cols, rows)
	}
}

// enterCopy puts the active pane into copy-mode sized to its current rectangle.
func (w *window) enterCopy(cols, rows int) {
	p := w.panes[w.active]
	if p == nil || p.copy != nil {
		return
	}
	r := layout.Compute(w.tree, w.outer(cols, rows))[w.active]
	p.copy = newCopyState(max1(r.W), max1(r.H))
}

// removePane drops a pane, collapses the tree, moves focus to its sibling, and
// re-flows the surviving panes into the freed space (cols/rows is the current
// content area). It reports whether the window is now empty.
func (w *window) removePane(id layout.PaneID, cols, rows int) (empty bool) {
	p, ok := w.panes[id]
	if !ok {
		return len(w.panes) == 0
	}
	p.close()
	delete(w.panes, id)
	if len(w.panes) == 0 {
		w.tree = nil
		return true
	}
	if newTree, focus, err := layout.Remove(w.tree, id); err == nil {
		w.tree = newTree
		if w.active == id {
			w.active = focus
		}
	}
	w.applyLayout(cols, rows)
	return false
}

func (w *window) closeAll() {
	for id, p := range w.panes {
		p.close()
		delete(w.panes, id)
	}
	w.tree = nil
}

// views builds the per-pane render input for the given content area, appending
// into the caller-owned views slice and using snaps as reusable snapshot
// storage (one entry per pane). Both are returned so the session can keep the
// grown backing arrays for the next frame. A pane in copy-mode is drawn from
// its scrollback view with the selection highlighted.
func (w *window) views(cols, rows int, views []render.PaneView, snaps []vterm.Snapshot) ([]render.PaneView, []vterm.Snapshot) {
	rects := layout.Compute(w.tree, w.outer(cols, rows))

	// Size snaps to the pane count up front: taking &snaps[i] below is only
	// safe if the backing array cannot move mid-loop.
	n := len(w.panes)
	if cap(snaps) < n {
		snaps = make([]vterm.Snapshot, n)
	} else {
		snaps = snaps[:n]
	}

	i := 0
	for id, p := range w.panes {
		sn := &snaps[i]
		i++
		pv := render.PaneView{ID: id, Rect: rects[id], Active: id == w.active}
		if p.copy != nil {
			*sn = p.vt.ScrollbackView(p.copy.offset, max1(rects[id].H))
			pv.Sel = p.copy.selection()
			pv.CopyCur = p.copy.cursor()
			total := p.vt.HistoryLen()
			pv.Overlay = fmt.Sprintf(" COPY %d/%d ", total-p.copy.offset, total)
		} else {
			p.vt.SnapshotInto(sn)
		}
		pv.Snap = sn
		views = append(views, pv)
	}
	return views, snaps
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
