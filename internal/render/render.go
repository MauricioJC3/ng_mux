// Package render composites the individual pane screens into one frame the
// size of the client terminal, draws pane borders and a status bar, and turns
// consecutive frames into a minimal stream of ANSI escape sequences.
//
// The server renders; the client just writes the bytes it receives to stdout.
// Keeping all emulation and diffing server-side makes the client trivial and
// identical on every platform.
package render

import (
	"bytes"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/MauricioJC3/ng_mux/internal/layout"
	"github.com/MauricioJC3/ng_mux/internal/vterm"
)

// Cell is one composited character position.
type Cell struct {
	Ch   rune
	FG   uint32
	BG   uint32
	Attr uint16
}

var blank = Cell{Ch: ' ', FG: vterm.ColorDefault, BG: vterm.ColorDefault}

// Frame is a full client-sized screen plus the cursor state to show.
type Frame struct {
	Cols, Rows int
	Cells      []Cell
	CurX, CurY int
	CurVisible bool

	// covered is border-detection scratch, reused across ComposeInto calls on
	// the same Frame so a per-tick repaint allocates nothing.
	covered []bool
}

// NewFrame returns a blank frame of the given size.
func NewFrame(cols, rows int) *Frame {
	f := &Frame{Cols: cols, Rows: rows, Cells: make([]Cell, cols*rows)}
	for i := range f.Cells {
		f.Cells[i] = blank
	}
	return f
}

// reset makes f a blank cols x rows frame, reusing its Cells buffer when it is
// already the right length.
func (f *Frame) reset(cols, rows int) {
	f.Cols, f.Rows = cols, rows
	f.CurX, f.CurY, f.CurVisible = 0, 0, false
	if len(f.Cells) != cols*rows {
		f.Cells = make([]Cell, cols*rows)
	}
	for i := range f.Cells {
		f.Cells[i] = blank
	}
}

// scratchCovered returns an all-false []bool of length n backed by f.covered.
func (f *Frame) scratchCovered(n int) []bool {
	if cap(f.covered) < n {
		f.covered = make([]bool, n)
	}
	f.covered = f.covered[:n]
	for i := range f.covered {
		f.covered[i] = false
	}
	return f.covered
}

func (f *Frame) set(x, y int, c Cell) {
	if x < 0 || y < 0 || x >= f.Cols || y >= f.Rows {
		return
	}
	f.Cells[y*f.Cols+x] = c
}

func (f *Frame) at(x, y int) Cell {
	if x < 0 || y < 0 || x >= f.Cols || y >= f.Rows {
		return blank
	}
	return f.Cells[y*f.Cols+x]
}

// PaneView is the input for one pane: where it sits and what it shows.
type PaneView struct {
	ID     layout.PaneID
	Rect   layout.Rect
	Snap   *vterm.Snapshot
	Active bool

	// Overlay, when non-empty, is printed in reverse video at the pane's
	// top-right corner (used for the "-- COPY 12/340 --" indicator).
	Overlay string
	// Sel, when non-nil, highlights an inclusive cell range in the pane's own
	// coordinates (used by copy-mode selection).
	Sel *Selection
	// CopyCur, when non-nil, places the terminal cursor here (pane coords)
	// instead of at the emulator cursor (used by copy-mode).
	CopyCur *[2]int
}

// Selection is an inclusive rectangle-free range: from (X0,Y0) to (X1,Y1) in
// reading order. It need not be normalised; the renderer orders the endpoints.
type Selection struct {
	X0, Y0, X1, Y1 int
}

func (s Selection) contains(x, y, cols int) bool {
	a := s.Y0*cols + s.X0
	b := s.Y1*cols + s.X1
	if a > b {
		a, b = b, a
	}
	p := y*cols + x
	return p >= a && p <= b
}

// StatusStyle holds the status bar's palette (xterm colour indices).
type StatusStyle struct {
	FG, BG int
}

// DefaultStatusStyle is black on light-grey.
var DefaultStatusStyle = StatusStyle{FG: 0, BG: 7}

// StatusSegment is a run of status-bar text with its own emphasis. FG and BG
// are xterm colour indices; -1 means "inherit the bar's default style" (which
// is what a plain-string status uses for every cell). Attr adds attribute bits
// (bold, underline, …) on top of the bar's reverse-video base.
type StatusSegment struct {
	Text   string
	FG, BG int
	Attr   uint16
}

// InheritColour, used for StatusSegment.FG/BG, keeps the bar's default colour.
const InheritColour = -1

// Compose builds a fresh frame of size cols x rows. The last row is the status
// bar; panes are laid out in rows [0, rows-1). status is the text shown in the
// bar.
func Compose(cols, rows int, panes []PaneView, status string, style StatusStyle) *Frame {
	return ComposeInto(nil, cols, rows, panes, status, style)
}

// ComposeInto is Compose that writes into dst, reusing dst's Cells buffer when
// it is already the right size. A nil or wrong-sized dst is allocated fresh.
// The returned frame (dst when reused) should be kept for the next call so the
// reuse actually happens.
func ComposeInto(dst *Frame, cols, rows int, panes []PaneView, status string, style StatusStyle) *Frame {
	return ComposeStyledInto(dst, cols, rows, panes,
		[]StatusSegment{{Text: status, FG: InheritColour, BG: InheritColour}}, style)
}

// ComposeStyledInto is ComposeInto with a segmented status bar: each segment
// carries its own emphasis (see StatusSegment). Segments are laid out left to
// right and the bar is padded to full width with the default style.
func ComposeStyledInto(dst *Frame, cols, rows int, panes []PaneView, status []StatusSegment, style StatusStyle) *Frame {
	f := dst
	if f == nil {
		f = &Frame{}
	}
	f.reset(cols, rows)

	contentRows := rows - 1
	if contentRows < 1 {
		contentRows = rows
	}

	covered := f.scratchCovered(cols * contentRows)
	markCovered := func(x, y int) {
		if x >= 0 && y >= 0 && x < cols && y < contentRows {
			covered[y*cols+x] = true
		}
	}

	var active *PaneView
	for i := range panes {
		p := &panes[i]
		if p.Active {
			active = p
		}
		r := p.Rect
		for row := 0; row < r.H; row++ {
			for col := 0; col < r.W; col++ {
				sx, sy := r.X+col, r.Y+row
				markCovered(sx, sy)
				var sc vterm.Cell
				if p.Snap != nil {
					sc = p.Snap.At(col, row)
				} else {
					sc = vterm.Cell{Ch: ' ', FG: vterm.ColorDefault, BG: vterm.ColorDefault}
				}
				cell := Cell{Ch: sc.Ch, FG: sc.FG, BG: sc.BG, Attr: sc.Attr}
				if p.Sel != nil && p.Sel.contains(col, row, r.W) {
					cell.Attr ^= vterm.AttrReverse
				}
				f.set(sx, sy, cell)
			}
		}
		if p.Overlay != "" {
			ox := r.X + r.W - len([]rune(p.Overlay))
			if ox < r.X {
				ox = r.X
			}
			for i, ch := range p.Overlay {
				f.set(ox+i, r.Y, Cell{Ch: ch, FG: 0, BG: 3, Attr: vterm.AttrReverse})
			}
		}
	}

	drawBorders(f, covered, cols, contentRows, active)
	drawStatusSegments(f, status, style)

	switch {
	case active != nil && active.CopyCur != nil:
		f.CurX = active.Rect.X + active.CopyCur[0]
		f.CurY = active.Rect.Y + active.CopyCur[1]
		f.CurVisible = true
	case active != nil && active.Snap != nil && active.Snap.CurVisible:
		f.CurX = active.Rect.X + active.Snap.CurX
		f.CurY = active.Rect.Y + active.Snap.CurY
		f.CurVisible = true
	}
	return f
}

// borderColor is a dim grey for inactive dividers; the active pane's dividers
// are drawn in green so the focused pane is obvious.
const (
	borderDim    = 8 // xterm bright-black
	borderActive = 2 // xterm green
)

func drawBorders(f *Frame, covered []bool, cols, contentRows int, active *PaneView) {
	activeAdj := func(x, y int) bool {
		if active == nil {
			return false
		}
		r := active.Rect
		return (x >= r.X-1 && x <= r.X+r.W && y >= r.Y-1 && y <= r.Y+r.H)
	}
	for y := 0; y < contentRows; y++ {
		for x := 0; x < cols; x++ {
			if covered[y*cols+x] {
				continue
			}
			left := x > 0 && covered[y*cols+x-1]
			right := x < cols-1 && covered[y*cols+x+1]
			up := y > 0 && covered[(y-1)*cols+x]
			down := y < contentRows-1 && covered[(y+1)*cols+x]

			var ch rune = ' '
			switch {
			case (up || down) && (left || right):
				ch = '┼'
			case left || right:
				ch = '│'
			case up || down:
				ch = '─'
			}
			if ch == ' ' {
				continue
			}
			color := uint32(borderDim)
			if activeAdj(x, y) {
				color = borderActive
			}
			f.set(x, y, Cell{Ch: ch, FG: color, BG: vterm.ColorDefault})
		}
	}
}

// drawStatusSegments paints the bar's bottom row from left to right. Every cell
// keeps the bar's reverse-video base; a segment that overrides FG or BG opts out
// of reverse so its colour reads literally. Any tail past the last segment is
// filled with the default style.
func drawStatusSegments(f *Frame, segs []StatusSegment, style StatusStyle) {
	y := f.Rows - 1
	if y < 0 {
		return
	}
	defFG, defBG := uint32(style.FG), uint32(style.BG)
	x := 0
	put := func(c Cell) {
		if x >= f.Cols {
			return
		}
		f.set(x, y, c)
		x++
	}
	for _, seg := range segs {
		fg, bg := defFG, defBG
		attr := uint16(vterm.AttrReverse)
		if seg.FG != InheritColour {
			fg, attr = uint32(seg.FG), 0
		}
		if seg.BG != InheritColour {
			bg, attr = uint32(seg.BG), 0
		}
		attr |= seg.Attr
		for _, r := range seg.Text {
			put(Cell{Ch: r, FG: fg, BG: bg, Attr: attr})
		}
	}
	for x < f.Cols {
		put(Cell{Ch: ' ', FG: defFG, BG: defBG, Attr: vterm.AttrReverse})
	}
}

// Paint returns the ANSI byte stream that turns a terminal currently showing
// prev into one showing next. If prev is nil or differently sized, it does a
// full repaint.
func Paint(prev, next *Frame) []byte {
	var b bytes.Buffer
	full := prev == nil || prev.Cols != next.Cols || prev.Rows != next.Rows

	b.WriteString("\x1b[?25l") // hide cursor while we draw

	if full {
		b.WriteString("\x1b[2J")
		prev = nil
	}

	var (
		curX, curY = -1, -1
		lastFG     = uint32(0xDEADBEEF)
		lastBG     = uint32(0xDEADBEEF)
		lastAttr   = uint16(0xFFFF)
	)
	for y := 0; y < next.Rows; y++ {
		for x := 0; x < next.Cols; x++ {
			nc := next.at(x, y)
			if prev != nil && nc == prev.at(x, y) {
				continue
			}
			if curX != x || curY != y {
				b.WriteString("\x1b[")
				b.WriteString(strconv.Itoa(y + 1))
				b.WriteByte(';')
				b.WriteString(strconv.Itoa(x + 1))
				b.WriteByte('H')
				curX, curY = x, y
			}
			if nc.FG != lastFG || nc.BG != lastBG || nc.Attr != lastAttr {
				writeSGR(&b, nc)
				lastFG, lastBG, lastAttr = nc.FG, nc.BG, nc.Attr
			}
			ch := nc.Ch
			if ch == 0 {
				ch = ' '
			}
			var tmp [4]byte
			n := utf8.EncodeRune(tmp[:], ch)
			b.Write(tmp[:n])
			curX = x + 1
		}
	}

	b.WriteString("\x1b[0m")
	if next.CurVisible {
		b.WriteString("\x1b[")
		b.WriteString(strconv.Itoa(next.CurY + 1))
		b.WriteByte(';')
		b.WriteString(strconv.Itoa(next.CurX + 1))
		b.WriteByte('H')
		b.WriteString("\x1b[?25h")
	}
	return b.Bytes()
}

// writeSGR emits a full "reset then set" SGR sequence for cell c.
func writeSGR(b *bytes.Buffer, c Cell) {
	b.WriteString("\x1b[0")
	if c.Attr&vterm.AttrBold != 0 {
		b.WriteString(";1")
	}
	if c.Attr&vterm.AttrItalic != 0 {
		b.WriteString(";3")
	}
	if c.Attr&vterm.AttrUnderline != 0 {
		b.WriteString(";4")
	}
	if c.Attr&vterm.AttrBlink != 0 {
		b.WriteString(";5")
	}
	if c.Attr&vterm.AttrReverse != 0 {
		b.WriteString(";7")
	}
	writeColor(b, c.FG, true)
	writeColor(b, c.BG, false)
	b.WriteByte('m')
}

func writeColor(b *bytes.Buffer, color uint32, fg bool) {
	if color == vterm.ColorDefault {
		if fg {
			b.WriteString(";39")
		} else {
			b.WriteString(";49")
		}
		return
	}
	switch {
	case color < 8:
		base := 30
		if !fg {
			base = 40
		}
		fmt.Fprintf(b, ";%d", base+int(color))
	case color < 16:
		base := 90
		if !fg {
			base = 100
		}
		fmt.Fprintf(b, ";%d", base+int(color-8))
	case color < 256:
		if fg {
			fmt.Fprintf(b, ";38;5;%d", color)
		} else {
			fmt.Fprintf(b, ";48;5;%d", color)
		}
	default: // packed 0xRRGGBB
		r, g, bl := (color>>16)&0xff, (color>>8)&0xff, color&0xff
		if fg {
			fmt.Fprintf(b, ";38;2;%d;%d;%d", r, g, bl)
		} else {
			fmt.Fprintf(b, ";48;2;%d;%d;%d", r, g, bl)
		}
	}
}
