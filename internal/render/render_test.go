package render

import (
	"strings"
	"testing"

	"github.com/MauricioJC3/ng_mux/internal/layout"
	"github.com/MauricioJC3/ng_mux/internal/vterm"
)

func snapWith(cols, rows int, text string) *vterm.Snapshot {
	s := &vterm.Snapshot{Cols: cols, Rows: rows, Cells: make([]vterm.Cell, cols*rows)}
	for i := range s.Cells {
		s.Cells[i] = vterm.Cell{Ch: ' ', FG: vterm.ColorDefault, BG: vterm.ColorDefault}
	}
	for i, r := range text {
		if i >= cols {
			break
		}
		s.Cells[i] = vterm.Cell{Ch: r, FG: vterm.ColorDefault, BG: vterm.ColorDefault}
	}
	return s
}

func TestComposePlacesPaneContentAndStatus(t *testing.T) {
	pv := PaneView{
		ID:     1,
		Rect:   layout.Rect{X: 0, Y: 0, W: 20, H: 5},
		Snap:   snapWith(20, 5, "hello"),
		Active: true,
	}
	f := Compose(20, 6, []PaneView{pv}, "STATUS", DefaultStatusStyle)

	if got := rowString(f, 0); !strings.HasPrefix(got, "hello") {
		t.Errorf("row 0 = %q, want it to start with 'hello'", got)
	}
	if got := rowString(f, 5); !strings.HasPrefix(got, "STATUS") {
		t.Errorf("status row = %q, want it to start with 'STATUS'", got)
	}
}

func TestComposeDrawsDividerBetweenPanes(t *testing.T) {
	left := PaneView{ID: 1, Rect: layout.Rect{X: 0, Y: 0, W: 10, H: 5}, Snap: snapWith(10, 5, ""), Active: true}
	right := PaneView{ID: 2, Rect: layout.Rect{X: 11, Y: 0, W: 10, H: 5}, Snap: snapWith(10, 5, "")}
	f := Compose(21, 6, []PaneView{left, right}, "", DefaultStatusStyle)

	// Column 10 is the uncovered gap: it must be a vertical divider.
	if got := f.at(10, 2).Ch; got != '│' {
		t.Errorf("divider cell (10,2) = %q, want '│'", got)
	}
}

func TestPaintFullThenNoOpDiff(t *testing.T) {
	f1 := Compose(10, 3, []PaneView{{ID: 1, Rect: layout.Rect{X: 0, Y: 0, W: 10, H: 2}, Snap: snapWith(10, 2, "abc"), Active: true}}, "s", DefaultStatusStyle)
	full := Paint(nil, f1)
	if !strings.Contains(string(full), "\x1b[2J") {
		t.Errorf("first paint should clear the screen")
	}
	if !strings.Contains(string(full), "abc") {
		t.Errorf("first paint should contain the pane text; got %q", full)
	}

	// Re-painting the identical frame should emit no cell writes.
	same := Paint(f1, f1)
	if strings.Contains(string(same), "abc") {
		t.Errorf("no-op diff should not rewrite unchanged cells; got %q", same)
	}
}

func TestPaintDiffOnlyChangedCells(t *testing.T) {
	base := []PaneView{{ID: 1, Rect: layout.Rect{X: 0, Y: 0, W: 10, H: 2}, Snap: snapWith(10, 2, "abc"), Active: true}}
	f1 := Compose(10, 3, base, "s", DefaultStatusStyle)
	f2 := Compose(10, 3, []PaneView{{ID: 1, Rect: layout.Rect{X: 0, Y: 0, W: 10, H: 2}, Snap: snapWith(10, 2, "aXc"), Active: true}}, "s", DefaultStatusStyle)

	out := string(Paint(f1, f2))
	if strings.Contains(out, "\x1b[2J") {
		t.Errorf("same-size diff should not full-clear")
	}
	if !strings.Contains(out, "X") {
		t.Errorf("diff should write the changed rune; got %q", out)
	}
	if strings.Count(out, "a") > 0 {
		t.Errorf("diff should not rewrite the unchanged 'a'; got %q", out)
	}
}

func rowString(f *Frame, y int) string {
	var b strings.Builder
	for x := 0; x < f.Cols; x++ {
		b.WriteRune(f.at(x, y).Ch)
	}
	return b.String()
}

func TestComposeBadgeCentredInPane(t *testing.T) {
	pv := PaneView{
		ID:    1,
		Rect:  layout.Rect{X: 0, Y: 0, W: 21, H: 7},
		Snap:  snapWith(21, 7, ""),
		Badge: " 3 ",
	}
	f := Compose(21, 8, []PaneView{pv}, "", DefaultStatusStyle)

	// Centred: (21-3)/2 = 9 on row 7/2 = 3.
	if got := rowString(f, 3); got[9:12] != " 3 " {
		t.Fatalf("row 3 = %q, want %q at column 9", got, " 3 ")
	}
	c := f.at(10, 3)
	if c.Ch != '3' || c.Attr&vterm.AttrReverse == 0 {
		t.Fatalf("badge digit cell = %+v, want '3' in reverse video", c)
	}
}

func TestComposeNoBadgeWhenEmpty(t *testing.T) {
	pv := PaneView{ID: 1, Rect: layout.Rect{X: 0, Y: 0, W: 21, H: 7}, Snap: snapWith(21, 7, "")}
	f := Compose(21, 8, []PaneView{pv}, "", DefaultStatusStyle)
	if got := strings.TrimSpace(rowString(f, 3)); got != "" {
		t.Fatalf("row 3 = %q, want blank when no badge is set", got)
	}
}
