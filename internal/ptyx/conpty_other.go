//go:build !windows

package ptyx

// CheckAvailable is a no-op off Windows: every supported Unix has a real pty.
func CheckAvailable() error { return nil }
