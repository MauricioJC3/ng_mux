// Package winpty is ngmux's fallback pseudo-terminal for Windows builds without
// ConPTY (Windows Server 2016 and earlier). It embeds winpty's native helpers
// (winpty.dll + winpty-agent.exe, from https://github.com/rprichard/winpty),
// unpacks them next to the ngmux data directory on first use, and drives
// winpty.dll through a small syscall binding — no cgo.
//
// Everything here is Windows-only; on other platforms the package is empty
// except for this file so `go build ./...` still succeeds.
package winpty
