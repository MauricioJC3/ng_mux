package vt10x

import "testing"

func TestRuneWidth(t *testing.T) {
	cases := []struct {
		r    rune
		want int
	}{
		{'a', 1},
		{'é', 1},
		{'™', 1},     // U+2122, not in a wide block -> conservative 1
		{0xFF71, 1},  // halfwidth katakana ｱ
		{'名', 2},     // CJK unified
		{'가', 2},     // Hangul syllable
		{'！', 2},     // U+FF01 fullwidth exclamation
		{'あ', 2},     // Hiragana
		{0x1F600, 2}, // 😀
		{0x20000, 2}, // CJK Ext B
	}
	for _, c := range cases {
		if got := RuneWidth(c.r); got != c.want {
			t.Errorf("RuneWidth(%#U) = %d, want %d", c.r, got, c.want)
		}
	}
}

func gridString(term Terminal, y int) []rune {
	cols, _ := term.Size()
	out := make([]rune, 0, cols)
	for x := 0; x < cols; x++ {
		out = append(out, term.Cell(x, y).Char)
	}
	return out
}

func TestWideGlyphOccupiesTwoCells(t *testing.T) {
	term := New(WithSize(10, 3))
	if _, err := term.Write([]byte("名a")); err != nil {
		t.Fatalf("write: %v", err)
	}

	lead := term.Cell(0, 0)
	if lead.Char != '名' || lead.Mode&AttrWide == 0 {
		t.Fatalf("cell(0,0) = %#U mode=%b, want '名' with AttrWide set", lead.Char, lead.Mode)
	}
	tail := term.Cell(1, 0)
	if tail.Mode&AttrWideTail == 0 {
		t.Fatalf("cell(1,0) mode=%b, want AttrWideTail set", tail.Mode)
	}
	if next := term.Cell(2, 0); next.Char != 'a' {
		t.Fatalf("cell(2,0) = %#U, want 'a' (narrow char after a wide one)", next.Char)
	}
	if x := term.Cursor().X; x != 3 {
		t.Fatalf("cursor X = %d, want 3 (2 for the wide glyph + 1 for 'a')", x)
	}
}

func TestWideGlyphWrapsAtRightMargin(t *testing.T) {
	term := New(WithSize(3, 3))
	if _, err := term.Write([]byte("aa名")); err != nil {
		t.Fatalf("write: %v", err)
	}
	// "aa" fills columns 0 and 1; the wide glyph cannot fit in the last
	// column, so it wraps to row 1.
	if got := gridString(term, 0); got[0] != 'a' || got[1] != 'a' {
		t.Fatalf("row 0 = %q, want it to start \"aa\"", string(got))
	}
	if lead := term.Cell(0, 1); lead.Char != '名' || lead.Mode&AttrWide == 0 {
		t.Fatalf("cell(0,1) = %#U, want the wrapped wide glyph", lead.Char)
	}
	if y := term.Cursor().Y; y != 1 {
		t.Fatalf("cursor Y = %d, want 1 after the wrap", y)
	}
}

func TestNarrowRunesUnaffected(t *testing.T) {
	term := New(WithSize(10, 3))
	if _, err := term.Write([]byte("abc")); err != nil {
		t.Fatalf("write: %v", err)
	}
	for x := 0; x < 3; x++ {
		if term.Cell(x, 0).Mode&(AttrWide|AttrWideTail) != 0 {
			t.Fatalf("cell(%d,0) picked up a wide flag on a narrow run", x)
		}
	}
	if x := term.Cursor().X; x != 3 {
		t.Fatalf("cursor X = %d, want 3", x)
	}
}
