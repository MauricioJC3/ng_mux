package server

import (
	"strings"
	"testing"

	"github.com/inre/tmux2/internal/render"
	"github.com/inre/tmux2/internal/vterm"
)

func statusText(segs []render.StatusSegment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Text)
	}
	return b.String()
}

func TestBuildStatusLayoutAndHints(t *testing.T) {
	_, _, sess := setupSession(t)
	sess.mu.Lock()
	segs := sess.buildStatus(80)
	hits := append([]statusHit(nil), sess.statusHits...)
	sess.mu.Unlock()

	text := statusText(segs)

	// Left side is unchanged from the original string layout, so every
	// mouse-hit column still lines up.
	if !strings.HasPrefix(text, " [0] 0:fakesh* [+] ") {
		t.Fatalf("status left side = %q, want the classic layout prefix", text)
	}
	// New: the close/exit reminder and a clock on the right.
	if !strings.Contains(text, "close pane") || !strings.Contains(text, "exit") {
		t.Errorf("status bar is missing the close/exit hint: %q", text)
	}
	if len(text) < 80 || strings.Count(text, ":") < 2 { // window sep + clock
		t.Errorf("status bar not padded to width or missing clock: %q", text)
	}

	// The [+] hit region must cover the actual "[+]" runes in the text.
	var plus *statusHit
	for i := range hits {
		if hits[i].action == "new-window" {
			plus = &hits[i]
		}
	}
	if plus == nil {
		t.Fatal("no new-window hit region recorded")
	}
	if got := text[plus.x0 : plus.x1+1]; got != "[+]" {
		t.Errorf("new-window hit covers %q at [%d,%d], want \"[+]\"", got, plus.x0, plus.x1)
	}
}

func TestBuildStatusActiveWindowIsBold(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "new-window") // two windows, second is current

	sess.mu.Lock()
	segs := sess.buildStatus(80)
	sess.mu.Unlock()

	var sawBoldActive, sawPlainInactive bool
	for _, s := range segs {
		switch strings.TrimSpace(s.Text) {
		case "1:fakesh*":
			sawBoldActive = s.Attr&vterm.AttrBold != 0
		case "0:fakesh":
			sawPlainInactive = s.Attr&vterm.AttrBold == 0
		}
	}
	if !sawBoldActive {
		t.Error("active window entry should be bold")
	}
	if !sawPlainInactive {
		t.Error("inactive window entry should not be bold")
	}
}

func TestBuildStatusNarrowTerminalDropsHint(t *testing.T) {
	_, _, sess := setupSession(t)
	sess.mu.Lock()
	segs := sess.buildStatus(30)
	sess.mu.Unlock()

	text := statusText(segs)
	if strings.Contains(text, "close pane") {
		t.Errorf("hint should be dropped when it will not fit: %q", text)
	}
}
