package vt10x

// RuneWidth reports how many terminal columns r occupies: 2 for the East-Asian
// wide / fullwidth blocks and the common wide emoji blocks, 1 otherwise.
//
// It is deliberately conservative. Anything outside a well-known wide range is
// treated as one column, exactly as before, so this can only ever line up
// better with a real terminal, never worse. Zero-width combining marks are also
// treated as one column (unchanged behaviour) — folding them into the preceding
// cell would need multi-rune cells, which the grid does not have.
func RuneWidth(r rune) int {
	if r < 0x1100 {
		return 1
	}
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E,   // CJK Radicals, Kangxi, CJK symbols
		r >= 0x3041 && r <= 0x33FF,   // Hiragana, Katakana, CJK compat
		r >= 0x3400 && r <= 0x4DBF,   // CJK Unified Ext A
		r >= 0x4E00 && r <= 0x9FFF,   // CJK Unified Ideographs
		r >= 0xA000 && r <= 0xA4CF,   // Yi Syllables / Radicals
		r >= 0xAC00 && r <= 0xD7A3,   // Hangul Syllables
		r >= 0xF900 && r <= 0xFAFF,   // CJK Compatibility Ideographs
		r >= 0xFE30 && r <= 0xFE4F,   // CJK Compatibility Forms
		r >= 0xFF00 && r <= 0xFF60,   // Fullwidth Forms
		r >= 0xFFE0 && r <= 0xFFE6,   // Fullwidth currency / signs
		r >= 0x1F300 && r <= 0x1F64F, // Misc Symbols & Pictographs, Emoticons
		r >= 0x1F900 && r <= 0x1F9FF, // Supplemental Symbols & Pictographs
		r >= 0x20000 && r <= 0x3FFFD: // CJK Unified Ext B and beyond
		return 2
	}
	return 1
}
