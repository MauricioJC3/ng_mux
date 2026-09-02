//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/MauricioJC3/ng_mux/internal/ipc"
)

// spawnDaemon starts `tmux2 __server <name>` in its own session, fully
// detached from this process's controlling terminal and process group so it
// survives the client exiting.
func spawnDaemon(ep ipc.Endpoint) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn daemon: %w", err)
	}
	// We never wait for it; release our handle so no zombie is tracked.
	return cmd.Process.Release()
}
