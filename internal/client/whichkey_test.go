package client

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWhichKeyBoxShape(t *testing.T) {
	box := whichKeyBox("Ctrl-b", builtinWhichKey, 60, 40)
	if len(box) != len(builtinWhichKey)+2 {
		t.Fatalf("line count = %d, want %d (rows + border)", len(box), len(builtinWhichKey)+2)
	}
	w := utf8.RuneCountInString(box[0])
	if w > 60 {
		t.Fatalf("box width %d exceeds maxWidth 60", w)
	}
	for i, line := range box {
		if got := utf8.RuneCountInString(line); got != w {
			t.Fatalf("line %d width = %d, want %d (%q)", i, got, w, line)
		}
	}
	if !strings.Contains(box[0], "Ctrl-b") {
		t.Errorf("header missing prefix label: %q", box[0])
	}
	if !strings.Contains(box[len(box)-1], "Esc") {
		t.Errorf("footer missing the cancel hint: %q", box[len(box)-1])
	}
	joined := strings.Join(box, "\n")
	for _, want := range []string{"split pane", "close the focused pane", "detach"} {
		if !strings.Contains(joined, want) {
			t.Errorf("cheat-sheet missing %q", want)
		}
	}
}

func TestWhichKeyBoxCapsRows(t *testing.T) {
	box := whichKeyBox("Ctrl-b", builtinWhichKey, 60, 8)
	if len(box) != 8 {
		t.Fatalf("line count = %d, want 8 (the maxRows cap)", len(box))
	}
	if !strings.Contains(box[len(box)-2], "more") {
		t.Errorf("expected a '+N more' line before the footer, got %q", box[len(box)-2])
	}
}

func TestWhichKeyBoxTooSmall(t *testing.T) {
	if box := whichKeyBox("Ctrl-b", builtinWhichKey, 10, 40); box != nil {
		t.Fatalf("want nil for a too-narrow panel, got %d lines", len(box))
	}
	if box := whichKeyBox("Ctrl-b", builtinWhichKey, 60, 2); box != nil {
		t.Fatalf("want nil for a too-short panel, got %d lines", len(box))
	}
}

func TestConfigWhichKeyListsBinds(t *testing.T) {
	km := keymap{prefix: DefaultPrefix, binds: map[string]string{"S": "split-window -v"}}
	rows := configWhichKey(km)
	if len(rows) != 1 || rows[0].keys != "S" || !strings.Contains(rows[0].desc, "split-window -v") {
		t.Fatalf("configWhichKey = %+v, want the S binding", rows)
	}
	if !strings.Contains(rows[0].desc, "(config)") {
		t.Errorf("config rows should be tagged: %q", rows[0].desc)
	}
}

func TestPrefixLabel(t *testing.T) {
	cases := map[byte]string{0x02: "Ctrl-b", 0x01: "Ctrl-a", ' ': " ", '~': "~"}
	for in, want := range cases {
		if got := prefixLabel(in); got != want {
			t.Errorf("prefixLabel(%#x) = %q, want %q", in, got, want)
		}
	}
}
