package render

import (
	"testing"

	"github.com/inre/tmux2/internal/layout"
	"github.com/inre/tmux2/internal/vterm"
)

// A single inherit-everything segment must render exactly like the plain-string
// path so existing callers and golden output are unaffected.
func TestComposeStyledMatchesPlainStatus(t *testing.T) {
	pane := PaneView{ID: 1, Rect: layout.Rect{X: 0, Y: 0, W: 12, H: 3}, Snap: snapWith(12, 3, "hi"), Active: true}

	plain := Compose(12, 4, []PaneView{pane}, "0:sh* [+]", DefaultStatusStyle)
	styled := ComposeStyledInto(nil, 12, 4, []PaneView{pane},
		[]StatusSegment{{Text: "0:sh* [+]", FG: InheritColour, BG: InheritColour}}, DefaultStatusStyle)

	if !framesEqual(plain, styled) {
		t.Fatal("ComposeStyledInto with one inherit segment diverged from the string path")
	}
}

func TestStatusSegmentAttrAndColour(t *testing.T) {
	f := ComposeStyledInto(nil, 12, 2, nil, []StatusSegment{
		{Text: "AB", FG: InheritColour, BG: InheritColour, Attr: vterm.AttrBold},
		{Text: "cd", FG: 2, BG: InheritColour},
	}, DefaultStatusStyle)

	y := f.Rows - 1
	if a := f.at(0, y); a.Attr&vterm.AttrBold == 0 || a.Attr&vterm.AttrReverse == 0 {
		t.Errorf("bold inherit cell = %+v, want bold + reverse kept", a)
	}
	// A segment that sets its own FG opts out of the reverse-video base.
	c := f.at(2, y)
	if c.FG != 2 || c.Attr&vterm.AttrReverse != 0 {
		t.Errorf("coloured cell = %+v, want FG 2 and no reverse", c)
	}
	// Tail past the last segment falls back to the default style.
	if tail := f.at(10, y); tail.Ch != ' ' || tail.Attr&vterm.AttrReverse == 0 {
		t.Errorf("padding cell = %+v, want a reverse-video space", tail)
	}
}
