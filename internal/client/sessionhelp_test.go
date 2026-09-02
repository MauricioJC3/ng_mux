package client

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSessionHelpBoxShape(t *testing.T) {
	box := sessionHelpBox(sessionHelp, 70, 20)
	if len(box) != len(sessionHelp)+2 {
		t.Fatalf("line count = %d, want %d (rows + border)", len(box), len(sessionHelp)+2)
	}
	w := utf8.RuneCountInString(box[0])
	if w > 70 {
		t.Fatalf("box width %d exceeds maxWidth 70", w)
	}
	for i, line := range box {
		if got := utf8.RuneCountInString(line); got != w {
			t.Fatalf("line %d width = %d, want %d (%q)", i, got, w, line)
		}
	}
	if !strings.Contains(box[0], sessionHelpTitle) {
		t.Errorf("header missing the title: %q", box[0])
	}
	if !strings.Contains(box[len(box)-1], "press any key") {
		t.Errorf("footer missing the dismiss hint: %q", box[len(box)-1])
	}
	joined := strings.Join(box, "\n")
	for _, want := range []string{"ngmux new -s", "ngmux attach -t", "ngmux ls", "new-session"} {
		if !strings.Contains(joined, want) {
			t.Errorf("session help missing %q", want)
		}
	}
}

func TestSessionHelpBoxTooSmall(t *testing.T) {
	if box := sessionHelpBox(sessionHelp, 12, 20); box != nil {
		t.Fatalf("want nil for a too-narrow panel, got %d lines", len(box))
	}
	if box := sessionHelpBox(sessionHelp, 70, 4); box != nil {
		t.Fatalf("want nil when maxRows cannot hold every row, got %d lines", len(box))
	}
}
