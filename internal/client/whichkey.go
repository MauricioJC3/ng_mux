package client

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/MauricioJC3/ng_mux/internal/termio"
)

// whichKeyRow is one line of the prefix cheat-sheet: the key(s) pressed after
// the prefix and a short description of what they do.
type whichKeyRow struct {
	keys string
	desc string
}

// builtinWhichKey mirrors defaultKeyCommands (plus the digit and arrow keys
// that resolveKey handles specially) in reading order with human labels.
// resolveKey stays the single source of truth for behaviour; this is display
// only, so a binding change must be reflected here by hand.
var builtinWhichKey = []whichKeyRow{
	{`"`, "split pane — top / bottom"},
	{"%", "split pane — left / right"},
	{"o", "focus next pane"},
	{";", "focus previous pane"},
	{"← ↑ ↓ →", "focus pane by direction"},
	{"H J K L", "resize the focused pane"},
	{"z", "zoom the focused pane — toggle full screen"},
	{"q", "flash each pane's index number"},
	{"x", "close the focused pane"},
	{"c", "new window"},
	{"n / p", "next / previous window"},
	{"0 – 9", "select window by number"},
	{"& ", "close the current window"},
	{"( / )", "previous / next session"},
	{"m", "session help — new / attach / list"},
	{"[", "copy mode — scroll and select"},
	{"]", "paste the copy buffer"},
	{":", "command prompt"},
	{"d", "detach — leave the session running"},
	{"Ctrl-b", "send a literal Ctrl-b to the pane"},
}

// whichKeyMinCols / whichKeyMinRows are the smallest terminal the popup will
// draw itself on; below that it silently does nothing.
const (
	whichKeyMinCols = 24
	whichKeyMinRows = 8
)

// showWhichKey draws the prefix cheat-sheet as a panel in the lower-right
// corner and returns a function that erases it again. It is called right after
// the prefix key is pressed, while the client blocks waiting for the next key,
// so the panel behaves like Neovim's which-key: press the prefix, see the
// choices, press a key. A concurrent server frame can repaint a pane behind the
// panel; that is the same trade-off the ':' prompt makes and it self-heals on
// the Refresh the caller sends after hiding.
func showWhichKey(out *lockedWriter, term *os.File, km keymap) func() {
	size, err := termio.GetSize(term)
	if err != nil || size.Cols < whichKeyMinCols || size.Rows < whichKeyMinRows {
		return func() {}
	}

	rows := append([]whichKeyRow(nil), builtinWhichKey...)
	rows = append(rows, configWhichKey(km)...)

	box := whichKeyBox(prefixLabel(km.prefix), rows, size.Cols-2, size.Rows-3)
	if len(box) == 0 {
		return func() {}
	}
	boxW := utf8.RuneCountInString(box[0])
	boxH := len(box)

	startRow := size.Rows - 1 - boxH // keep the status bar (last row) clear
	if startRow < 1 {
		startRow = 1
	}
	startCol := size.Cols - boxW - 1
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

// configWhichKey lists the user's own `bind` directives so a custom config
// shows up in the cheat-sheet too.
func configWhichKey(km keymap) []whichKeyRow {
	if len(km.binds) == 0 {
		return nil
	}
	keys := make([]string, 0, len(km.binds))
	for k := range km.binds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]whichKeyRow, 0, len(keys))
	for _, k := range keys {
		line, ok := km.resolveKey(k[0])
		if !ok {
			line = km.binds[k]
		}
		out = append(out, whichKeyRow{keys: k, desc: line + "  (config)"})
	}
	return out
}

// whichKeyBox renders the cheat-sheet as a bordered panel no wider than
// maxWidth and no taller than maxRows runes/lines. Rows that do not fit are
// dropped and replaced with a "+N more" line. It returns the panel as equal-
// width plain-text lines (no ANSI); the caller adds colour and position.
func whichKeyBox(title string, rows []whichKeyRow, maxWidth, maxRows int) []string {
	if maxWidth < 12 || maxRows < 3 || len(rows) == 0 {
		return nil
	}

	keyW := 0
	for _, r := range rows {
		if n := utf8.RuneCountInString(r.keys); n > keyW {
			keyW = n
		}
	}
	if keyW > 10 {
		keyW = 10
	}

	// Panel width: "│ " + keys + "  " + desc + " │". Grow to the longest desc,
	// then clamp to maxWidth.
	descW := 0
	for _, r := range rows {
		if n := utf8.RuneCountInString(r.desc); n > descW {
			descW = n
		}
	}
	width := 2 + keyW + 2 + descW + 2
	if width > maxWidth {
		width = maxWidth
		descW = width - 2 - keyW - 2 - 2
	}
	if descW < 4 {
		return nil
	}

	bodyCap := maxRows - 2 // header + footer
	if bodyCap < 1 {
		return nil
	}
	hidden := 0
	if len(rows) > bodyCap {
		keep := bodyCap - 1 // leave a line for the "+N more" note
		if keep < 1 {
			keep = 1
		}
		hidden = len(rows) - keep
		rows = rows[:keep]
	}

	inner := width - 2
	border := func(left, right rune, fill string) string {
		return string(left) + fill + string(right)
	}
	center := func(s string) string {
		if utf8.RuneCountInString(s) > inner {
			s = string([]rune(s)[:inner])
		}
		pad := inner - utf8.RuneCountInString(s)
		l := pad / 2
		return strings.Repeat("─", l) + s + strings.Repeat("─", pad-l)
	}

	out := make([]string, 0, len(rows)+2)
	out = append(out, border('┌', '┐', center(" "+title+" ")))
	for _, r := range rows {
		keys := fitRunes(r.keys, keyW)
		desc := fitRunes(r.desc, descW)
		out = append(out, "│ "+keys+"  "+desc+" │")
	}
	if hidden > 0 {
		out = append(out, "│ "+fitRunes(fmt.Sprintf("+%d more", hidden), inner-2)+" │")
	}
	out = append(out, border('└', '┘', center(" press a key · Esc to cancel ")))
	return out
}

// fitRunes left-justifies s to exactly w runes, truncating with an ellipsis
// when it is too long.
func fitRunes(s string, w int) string {
	n := utf8.RuneCountInString(s)
	if n == w {
		return s
	}
	if n < w {
		return s + strings.Repeat(" ", w-n)
	}
	if w <= 1 {
		return strings.Repeat("…", w)
	}
	return string([]rune(s)[:w-1]) + "…"
}

// prefixLabel renders a prefix byte the way a user would say it: a control byte
// as "Ctrl-x", a printable byte as itself.
func prefixLabel(b byte) string {
	if b >= 1 && b <= 26 {
		return "Ctrl-" + string(rune('a'+b-1))
	}
	if b >= 0x20 && b < 0x7f {
		return string(rune(b))
	}
	return fmt.Sprintf("0x%02x", b)
}
