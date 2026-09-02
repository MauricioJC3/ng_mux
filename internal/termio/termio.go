// Package termio handles the *client's* real terminal: putting it into raw mode
// so keystrokes arrive unbuffered, enabling ANSI/VT processing, reading its
// size, and reporting size changes. The server never uses this package.
package termio

import "os"

// Size is a terminal size in character cells.
type Size struct {
	Cols, Rows int
}

// Session is an entered raw-mode terminal. Restore puts it back exactly as it
// was; it is safe to call more than once.
type Session struct {
	in, out  *os.File
	restore  func() error
	restored bool
}

// Restore reverts every change Enter made.
func (s *Session) Restore() {
	if s == nil || s.restored || s.restore == nil {
		return
	}
	_ = s.restore()
	s.restored = true
}

// Enter switches in/out into raw mode with VT processing enabled and returns a
// Session that can undo it.
func Enter(in, out *os.File) (*Session, error) {
	return enter(in, out)
}

// GetSize reports the current size of the terminal backing f.
func GetSize(f *os.File) (Size, error) {
	return getSize(f)
}

// WatchResize calls fn once for every size change until stop is closed. The
// first call to fn is the current size, delivered immediately.
func WatchResize(f *os.File, fn func(Size), stop <-chan struct{}) {
	watchResize(f, fn, stop)
}
