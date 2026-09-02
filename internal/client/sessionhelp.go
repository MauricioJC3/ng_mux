package client

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/MauricioJC3/ng_mux/internal/termio"
)

// sessionHelpRow is one line of the Ctrl-b m panel: a short label and the exact
// command that does it. NAME is a placeholder the reader fills in.
type sessionHelpRow struct{ label, cmd string }

// sessionHelp is the cheat-sheet shown by Ctrl-b m. It answers the questions a
// new user hits first: how do I start another session, how do I get back into
// one, how do I see what is still running.
var sessionHelp = []sessionHelpRow{
	{"new", "ngmux new -s NAME"},
	{"new (attached)", "Ctrl-b : new-session -s NAME"},
	{"attach", "ngmux attach -t NAME"},
	{"list", "ngmux ls"},
	{"switch", "Ctrl-b (   Ctrl-b )"},
	{"rename", "Ctrl-b : rename-session NAME"},
	{"detach", "Ctrl-b d   (keeps it running)"},
	{"kill", "ngmux kill-session -t NAME"},
}

const sessionHelpTitle = "sessions"

// showSessionHelp draws the session cheat-sheet centred on the terminal and
// returns a function that erases it. Like showWhichKey it is a transient
// overlay: a concurrent server frame may repaint behind it, and the caller
// sends a Refresh once it is dismissed.
func showSessionHelp(out *lockedWriter, term *os.File) func() {
	size, err := termio.GetSize(term)
	if err != nil || size.Cols < whichKeyMinCols || size.Rows < whichKeyMinRows {
		return func() {}
	}

	box := sessionHelpBox(sessionHelp, size.Cols-2, size.Rows-2)
	if len(box) == 0 {
		return func() {}
	}
	boxW := utf8.RuneCountInString(box[0])
	boxH := len(box)

	startRow := (size.Rows-boxH)/2 + 1
	if startRow < 1 {
		startRow = 1
	}
	startCol := (size.Cols-boxW)/2 + 1
	if startCol < 1 {
		startCol = 1
	}

	const (
		panelOn  = "\x1b[0m\x1b[48;5;236m\x1b[38;5;253m"
		panelOff = "\x1b[0m"
	)
	var b strings.Builder
	b.WriteString("\x1b[?25l") // hide the cursor while the panel is up
	for i, line := range box {
		fmt.Fprintf(&b, "\x1b[%d;%dH%s%s%s", startRow+i, startCol, panelOn, line, panelOff)
	}
	out.WriteString(b.String())

	return func() {
		var c strings.Builder
		blank := strings.Repeat(" ", boxW)
		for i := 0; i < boxH; i++ {
			fmt.Fprintf(&c, "\x1b[%d;%dH%s", startRow+i, startCol, blank)
		}
		c.WriteString("\x1b[?25h")
		out.WriteString(c.String())
	}
}

// sessionHelpBox renders the cheat-sheet as a bordered panel, one "label
// command" row per entry, no wider than maxWidth and no taller than maxRows. It
// returns equal-width plain-text lines (no ANSI); the caller adds colour and
// position. It returns nil when the panel cannot fit.
func sessionHelpBox(rows []sessionHelpRow, maxWidth, maxRows int) []string {
	if maxWidth < 20 || maxRows < len(rows)+2 || len(rows) == 0 {
		return nil
	}

	labelW := 0
	cmdW := 0
	for _, r := range rows {
		if n := utf8.RuneCountInString(r.label); n > labelW {
			labelW = n
		}
		if n := utf8.RuneCountInString(r.cmd); n > cmdW {
			cmdW = n
		}
	}

	width := 2 + labelW + 2 + cmdW + 2 // "│ " + label + "  " + cmd + " │"
	if width > maxWidth {
		width = maxWidth
		cmdW = width - 2 - labelW - 2 - 2
	}
	if cmdW < 8 {
		return nil
	}
	inner := width - 2

	center := func(s string) string {
		if utf8.RuneCountInString(s) > inner {
			s = string([]rune(s)[:inner])
		}
		pad := inner - utf8.RuneCountInString(s)
		l := pad / 2
		return strings.Repeat("─", l) + s + strings.Repeat("─", pad-l)
	}

	out := make([]string, 0, len(rows)+2)
	out = append(out, "┌"+center(" "+sessionHelpTitle+" ")+"┐")
	for _, r := range rows {
		out = append(out, "│ "+fitRunes(r.label, labelW)+"  "+fitRunes(r.cmd, cmdW)+" │")
	}
	out = append(out, "└"+center(" press any key ")+"┘")
	return out
}
