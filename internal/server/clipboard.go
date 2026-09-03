package server

import "encoding/base64"

// osc52MaxInput caps how much text we will try to push to the OS clipboard. The
// OSC 52 sequence is base64 and many terminals reject very long ones, so a
// larger yank is simply not mirrored (it still lands in the paste buffer).
const osc52MaxInput = 74994 / 4 * 3

// osc52 wraps text in an OSC 52 "set clipboard" sequence: ESC ] 52 ; c ; <b64> BEL.
// It returns nil when text is empty or too large to encode safely.
func osc52(text string) []byte {
	if text == "" || len(text) > osc52MaxInput {
		return nil
	}
	enc := base64.StdEncoding.EncodeToString([]byte(text))
	out := make([]byte, 0, len(enc)+8)
	out = append(out, "\x1b]52;c;"...)
	out = append(out, enc...)
	out = append(out, '\a')
	return out
}
