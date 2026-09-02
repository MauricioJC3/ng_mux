//go:build windows

package ptyx

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultShell picks a shell for Windows. Preference order:
//  1. %COMSPEC% if it points at something real (usually cmd.exe)
//  2. PowerShell (pwsh.exe if on PATH, else Windows PowerShell)
//  3. cmd.exe as a last resort
func DefaultShell() string {
	if c := os.Getenv("COMSPEC"); c != "" {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if p, err := lookPath("pwsh.exe"); err == nil {
		return p
	}
	if sysroot := os.Getenv("SystemRoot"); sysroot != "" {
		ps := filepath.Join(sysroot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		if _, err := os.Stat(ps); err == nil {
			return ps
		}
	}
	return "cmd.exe"
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
