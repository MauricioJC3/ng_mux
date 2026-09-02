//go:build !windows

package ptyx

import "errors"

// CheckAvailable is a no-op off Windows: every supported Unix has a real pty.
func CheckAvailable() error { return nil }

// conPTYAvailable is always true off Windows (a real pty is the "conpty" path).
func conPTYAvailable() bool { return true }

// startFallback is unreachable off Windows because conPTYAvailable is always
// true; it exists only so ptyx.go compiles on every platform.
func startFallback(prog string, args []string, dir string, env []string, cols, rows int) (backend, error) {
	return nil, errors.New("ptyx: no pseudo-terminal backend for this platform")
}
