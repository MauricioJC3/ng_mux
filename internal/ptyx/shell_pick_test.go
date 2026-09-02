package ptyx

import (
	"strings"
	"testing"
)

// probe builds a shellProbe whose lookups are driven by a small set of flags,
// so each case says exactly what is installed.
func probe(onPath map[string]bool, winPS, comspecCmd, sys32Cmd bool) shellProbe {
	return shellProbe{
		lookPath: func(name string) (string, bool) {
			if onPath[name] {
				return `C:\tools\` + name, true
			}
			return "", false
		},
		getenv: func(k string) string {
			switch k {
			case "SystemRoot":
				return `C:\Windows`
			case "COMSPEC":
				return `C:\Windows\System32\cmd.exe`
			}
			return ""
		},
		isFile: func(p string) bool {
			switch {
			case strings.Contains(p, "WindowsPowerShell"):
				return winPS
			case strings.Contains(p, `System32\cmd.exe`), strings.Contains(p, "System32/cmd.exe"):
				return comspecCmd || sys32Cmd
			}
			return false
		},
	}
}

func TestWindowsDefaultShellPrefersPowerShell7(t *testing.T) {
	got := windowsDefaultShell(probe(map[string]bool{"pwsh.exe": true}, true, true, true))
	if got != `C:\tools\pwsh.exe` {
		t.Fatalf("got %q, want pwsh.exe to win", got)
	}
}

func TestWindowsDefaultShellFallsBackToWindowsPowerShell(t *testing.T) {
	got := windowsDefaultShell(probe(nil, true, true, true))
	if !strings.Contains(got, "WindowsPowerShell") || !strings.HasSuffix(got, "powershell.exe") {
		t.Fatalf("got %q, want Windows PowerShell 5.1", got)
	}
}

func TestWindowsDefaultShellUsesPowerShellOnPath(t *testing.T) {
	// pwsh missing, System32 powershell.exe missing, but powershell.exe on PATH.
	got := windowsDefaultShell(probe(map[string]bool{"powershell.exe": true}, false, true, true))
	if got != `C:\tools\powershell.exe` {
		t.Fatalf("got %q, want the PATH powershell.exe", got)
	}
}

func TestWindowsDefaultShellFallsBackToCmd(t *testing.T) {
	got := windowsDefaultShell(probe(nil, false, true, false))
	if got != `C:\Windows\System32\cmd.exe` {
		t.Fatalf("got %q, want cmd.exe only when no PowerShell exists", got)
	}
}

func TestWindowsDefaultShellLastResort(t *testing.T) {
	got := windowsDefaultShell(probe(nil, false, false, false))
	if got != "cmd.exe" {
		t.Fatalf("got %q, want the bare cmd.exe last resort", got)
	}
}
