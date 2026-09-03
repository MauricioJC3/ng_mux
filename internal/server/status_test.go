package server

import (
	"strings"
	"testing"

	"github.com/MauricioJC3/ng_mux/internal/render"
	"github.com/MauricioJC3/ng_mux/internal/vterm"
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

// A window name with a double-width character must be measured in columns, not
// runes: the [+] hit region, the padding and the clock all key off that width.
func TestBuildStatusAccountsForWideWindowName(t *testing.T) {
	srv, _, sess := setupSession(t)
	exec(t, srv, "rename-window 名前") // 4 display columns, 2 runes

	sess.mu.Lock()
	segs := sess.buildStatus(80)
	hits := append([]statusHit(nil), sess.statusHits...)
	name := sess.windows[sess.cur].name
	sess.mu.Unlock()

	if name != "名前" {
		t.Fatalf("window name = %q, want 名前", name)
	}

	var plusX0 int
	for _, h := range hits {
		if h.action == "new-window" {
			plusX0 = h.x0
		}
	}
	// " [0] " (5) + "0:" (2) + name (4 cols) + "* " (2) = 13 -> [+] starts at 13.
	if plusX0 != 13 {
		t.Fatalf("[+] hit x0 = %d, want 13 (wide name counted as 4 columns)", plusX0)
	}

	// The bar is still exactly one screen wide in display columns.
	if w := vterm.StringWidth(statusText(segs)); w != 80 {
		t.Fatalf("status bar width = %d columns, want 80", w)
	}
}
