//go:build windows

package ptyx

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// CheckAvailable fails early, with an actionable message, when this Windows
// build has no pseudo-terminal API ngmux can use. Today that means ConPTY
// (kernel32!CreatePseudoConsole), added in Windows 10 1809 / Windows Server
// 2019. Older builds — Windows Server 2016, for one — land here; without this
// guard the go-pty call panics on the missing kernel32 export instead of
// reporting something the user can act on.
func CheckAvailable() error {
	if err := windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole").Find(); err != nil {
		major, minor, build := windows.RtlGetNtVersionNumbers()
		return fmt.Errorf(
			"ngmux needs ConPTY, which this Windows build (%d.%d.%d) does not have. "+
				"ConPTY requires Windows 10 1809+ or Windows Server 2019+. "+
				"Run ngmux on a newer Windows for now",
			major, minor, build&0xffff,
		)
	}
	return nil
}
