package ptyx

import "path/filepath"

// shellProbe abstracts the OS lookups windowsDefaultShell needs (PATH search,
// environment, "is this a real file") so the preference order can be unit-tested
// without a Windows host.
type shellProbe struct {
	lookPath func(name string) (string, bool) // resolve an executable on PATH
	getenv   func(key string) string
	isFile   func(path string) bool // exists and is not a directory
}

// windowsDefaultShell picks the shell a new pane runs on Windows, most
// preferred first:
//
//  1. PowerShell 7+ (pwsh.exe), if it is on PATH
//  2. Windows PowerShell 5.1 from its fixed System32 location, or a bare
//     powershell.exe found on PATH
//  3. cmd.exe (from %COMSPEC%, then System32) as a last resort
//
// The previous logic consulted %COMSPEC% first, which on every real Windows
// install is cmd.exe, so PowerShell could never win. This reverses that: cmd is
// only chosen when neither PowerShell is present.
func windowsDefaultShell(p shellProbe) string {
	if path, ok := p.lookPath("pwsh.exe"); ok {
		return path
	}
	if root := p.getenv("SystemRoot"); root != "" {
		ps := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		if p.isFile(ps) {
			return ps
		}
	}
	if path, ok := p.lookPath("powershell.exe"); ok {
		return path
	}
	if c := p.getenv("COMSPEC"); c != "" && p.isFile(c) {
		return c
	}
	if root := p.getenv("SystemRoot"); root != "" {
		if cmd := filepath.Join(root, "System32", "cmd.exe"); p.isFile(cmd) {
			return cmd
		}
	}
	return "cmd.exe"
}
