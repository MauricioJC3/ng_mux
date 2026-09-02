package server

import (
	"github.com/inre/tmux2/internal/layout"
	"github.com/inre/tmux2/internal/protocol"
)

// wheelStep is how many scrollback lines one wheel notch moves.
const wheelStep = 3

// dragState tracks an in-progress pane-border drag.
type dragState struct {
	active bool
	pane   layout.PaneID
	dir    layout.Orientation
	lastX  int
	lastY  int
}

// mouse handles one mouse event (coordinates are 0-based cells). It returns
// true when the client needs a repaint.
func (s *session) mouse(kind string, x, y, button int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	w := s.current()
	if w == nil {
		return false
	}
	cols, content := s.cols, s.contentRows()
	if x < 0 || y < 0 || x >= cols {
		return false
	}
	// The status row: only a press acts, and only on a clickable region.
	if y >= content {
		if y == content && kind == protocol.MousePress {
			return s.statusClick(x)
		}
		return false
	}
	rects := layout.Compute(w.tree, w.outer(cols, content))

	switch kind {
	case protocol.MouseWheelUp:
		pid := paneAt(rects, x, y)
		if pid == 0 {
			pid = w.active
		}
		w.active = pid
		if p := w.panes[pid]; p != nil {
			if p.copy == nil {
				w.enterCopy(cols, content)
				p = w.panes[w.active]
			}
			if p != nil && p.copy != nil {
				p.copy.offset += wheelStep
				if max := p.vt.HistoryLen(); p.copy.offset > max {
					p.copy.offset = max
				}
			}
		}
		return true

	case protocol.MouseWheelDown:
		if p := w.panes[w.active]; p != nil && p.copy != nil {
			p.copy.offset -= wheelStep
			if p.copy.offset <= 0 {
				p.copy = nil // scrolled back to the live screen: leave copy-mode
			}
		}
		return true

	case protocol.MousePress:
		s.drag = dragState{}
		if pid := paneAt(rects, x, y); pid != 0 {
			if pid != w.active {
				w.active = pid
				return true
			}
			return false
		}
		s.drag = dragFrom(rects, x, y)
		return false

	case protocol.MouseDrag:
		if !s.drag.active {
			return false
		}
		outer := w.outer(cols, content)
		if s.drag.dir == layout.Horizontal {
			if d := x - s.drag.lastX; d != 0 {
				_ = layout.Resize(w.tree, s.drag.pane, layout.Horizontal, d, outer)
				s.drag.lastX = x
			}
		} else {
			if d := y - s.drag.lastY; d != 0 {
				_ = layout.Resize(w.tree, s.drag.pane, layout.Vertical, d, outer)
				s.drag.lastY = y
			}
		}
		w.applyLayout(cols, content)
		return true

	case protocol.MouseRelease:
		s.drag = dragState{}
		return false
	}
	return false
}

// paneAt returns the id of the pane whose rectangle contains (x,y), or 0.
func paneAt(rects map[layout.PaneID]layout.Rect, x, y int) layout.PaneID {
	for id, r := range rects {
		if x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H {
			return id
		}
	}
	return 0
}

// dragFrom decides whether (x,y) is on a divider between two panes and, if so,
// returns a dragState anchored to the pane whose boundary should move.
func dragFrom(rects map[layout.PaneID]layout.Rect, x, y int) dragState {
	left := paneAt(rects, x-1, y)
	right := paneAt(rects, x+1, y)
	if left != 0 && right != 0 && left != right {
		return dragState{active: true, pane: left, dir: layout.Horizontal, lastX: x, lastY: y}
	}
	up := paneAt(rects, x, y-1)
	down := paneAt(rects, x, y+1)
	if up != 0 && down != 0 && up != down {
		return dragState{active: true, pane: up, dir: layout.Vertical, lastX: x, lastY: y}
	}
	return dragState{}
}
