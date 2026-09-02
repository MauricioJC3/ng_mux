//go:build !windows

package ptyx

import "os"

// DefaultShell returns the user's login shell, falling back to /bin/sh.
func DefaultShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

// defaultShellArgs starts the shell as an interactive login shell so it reads
// the user's rc files.
func defaultShellArgs(shell string) []string {
	return []string{"-l"}
}
