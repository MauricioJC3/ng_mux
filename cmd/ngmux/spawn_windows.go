//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/sys/windows"

	"github.com/MauricioJC3/ng_mux/internal/ipc"
)

// baseSpawnFlags detach the daemon from the console and give it its own
// process group, but they do not free it from a job object.
const baseSpawnFlags = windows.CREATE_NEW_PROCESS_GROUP |
	windows.DETACHED_PROCESS |
	windows.CREATE_NO_WINDOW

// spawnDaemon starts `ngmux __server <name>` as a detached, windowless
// background process that outlives this client.
//
// The OpenSSH server on Windows puts every process started inside an SSH
// session into a job object and terminates that job when the connection
// closes. DETACHED_PROCESS only frees a process from the console, not from
// the job, so without CREATE_BREAKAWAY_FROM_JOB the daemon dies the moment
// the user disconnects and detached sessions never survive a logout. We ask
// to break away from the job, and fall back to a plain detached spawn when
// the job forbids it (CreateProcess then fails with ERROR_ACCESS_DENIED).
func spawnDaemon(ep ipc.Endpoint) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}

	err = startDetached(self, ep, baseSpawnFlags|windows.CREATE_BREAKAWAY_FROM_JOB)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return startDetached(self, ep, baseSpawnFlags)
	}
	return err
}

// startDetached launches the daemon with the given creation flags, wiring its
// standard streams to the null device so it holds no handle on the client's
// console.
func startDetached(self string, ep ipc.Endpoint, flags uint32) error {
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devnull.Close()

	cmd := exec.Command(self, "__server", ep.Name)
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: flags,
		HideWindow:    true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn daemon: %w", err)
	}
	return cmd.Process.Release()
}
