//go:build windows

package ptyx

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultShell picks a shell for Windows: PowerShell 7 (pwsh.exe) first, then
// Windows PowerShell 5.1, then cmd.exe. See windowsDefaultShell for the full
// order and why cmd is last. `set default-shell <path>` in the config overrides
// this entirely.
func DefaultShell() string {
	return windowsDefaultShell(shellProbe{
		lookPath: func(name string) (string, bool) {
			p, err := lookPath(name)
			return p, err == nil
		},
		getenv: os.Getenv,
		isFile: func(path string) bool {
			fi, err := os.Stat(path)
			return err == nil && !fi.IsDir()
		},
	})
}

// defaultShellArgs returns startup args appropriate to the chosen shell.
func defaultShellArgs(shell string) []string {
	base := strings.ToLower(filepath.Base(shell))
	switch {
	case strings.Contains(base, "powershell"), strings.Contains(base, "pwsh"):
		return []string{"-NoLogo"}
	default:
		return nil
	}
}

// lookPath is a tiny stand-in for exec.LookPath to avoid importing os/exec
// just for this; it checks each PATH entry for name.
func lookPath(name string) (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}
