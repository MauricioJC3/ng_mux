//go:build windows

package ptyx

import (
	"sync"

	"golang.org/x/sys/windows"

	"github.com/MauricioJC3/ng_mux/internal/ptyx/winpty"
)

var conptyOnce struct {
	sync.Once
	ok bool
}

// conPTYAvailable reports whether this Windows build has the ConPTY API
// (kernel32!CreatePseudoConsole), added in Windows 10 1809 / Server 2019. The
// probe uses LazyProc.Find, which returns an error rather than panicking the
// way Addr/Call do on the missing export.
func conPTYAvailable() bool {
	conptyOnce.Do(func() {
		err := windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole").Find()
		conptyOnce.ok = err == nil
	})
	return conptyOnce.ok
}

// CheckAvailable succeeds when ngmux can open a pty on this machine: ConPTY when
// present, otherwise the embedded winpty fallback. It only fails if neither is
// usable (winpty binaries missing or unextractable).
func CheckAvailable() error {
	if conPTYAvailable() {
		return nil
	}
	return winpty.Available()
}

// startFallback runs the child through winpty, for Windows without ConPTY.
func startFallback(prog string, args []string, dir string, env []string, cols, rows int) (backend, error) {
	return startWinpty(prog, args, dir, env, cols, rows)
}
