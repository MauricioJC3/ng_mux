package vterm

import (
	"strings"
	"testing"
)

// rowText reads row y of a snapshot as a trimmed string.
func rowText(s Snapshot, y int) string {
	var b strings.Builder
	for x := 0; x < s.Cols; x++ {
		b.WriteRune(s.At(x, y).Ch)
	}
	return strings.TrimRight(b.String(), " ")
}

func TestSnapshotReflectsWrites(t *testing.T) {
	term := New(20, 5, nil)
	term.Write([]byte("hello world"))
	if got := rowText(term.Snapshot(), 0); got != "hello world" {
		t.Fatalf("row 0 = %q, want %q", got, "hello world")
	}
}

func TestScrollbackCapturesEvictedLines(t *testing.T) {
	term := New(20, 4, nil) // 4 visible rows
	term.SetHistoryLimit(100)

	// Print 10 numbered lines through a 4-row screen: 6 must scroll off.
	for i := 1; i <= 10; i++ {
		term.Write([]byte("line" + itoa(i) + "\r\n"))
	}
	if got := term.HistoryLen(); got < 6 {
		t.Fatalf("history len = %d, want >= 6", got)
	}

	// Live screen shows the tail.
	live := term.Snapshot()
	if !strings.Contains(rowText(live, 0), "line8") && !strings.Contains(rowText(live, 1), "line8") {
		t.Fatalf("live screen missing recent lines:\n%s", dump(live))
	}

	// Scrolled all the way back, the very first line is visible again.
	back := term.ScrollbackView(term.HistoryLen(), 4)
	if !containsRow(back, "line1") {
		t.Fatalf("scrollback did not retain line1:\n%s", dump(back))
	}
}

func TestScrollbackDisabledWhenLimitZero(t *testing.T) {
	term := New(20, 3, nil)
	term.SetHistoryLimit(0)
	for i := 1; i <= 20; i++ {
		term.Write([]byte("x" + itoa(i) + "\r\n"))
	}
	if term.HistoryLen() != 0 {
		t.Fatalf("history len = %d, want 0 (capture disabled)", term.HistoryLen())
	}
}

func TestScrollbackViewOffsetZeroIsLiveScreen(t *testing.T) {
	term := New(20, 4, nil)
	term.SetHistoryLimit(100)
	for i := 1; i <= 12; i++ {
		term.Write([]byte("row" + itoa(i) + "\r\n"))
	}
	live := term.Snapshot()
	view := term.ScrollbackView(0, 4)
	for y := 0; y < 4; y++ {
		if rowText(live, y) != rowText(view, y) {
			t.Fatalf("offset 0 row %d differs: live %q vs view %q", y, rowText(live, y), rowText(view, y))
		}
	}
}

func containsRow(s Snapshot, want string) bool {
	for y := 0; y < s.Rows; y++ {
		if strings.Contains(rowText(s, y), want) {
			return true
		}
	}
	return false
}

func dump(s Snapshot) string {
	var b strings.Builder
	for y := 0; y < s.Rows; y++ {
		b.WriteString(rowText(s, y))
		b.WriteByte('\n')
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
