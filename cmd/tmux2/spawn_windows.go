//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/sys/windows"

	"github.com/MauricioJC3/ng_mux/internal/ipc"
)

// spawnDaemon starts `tmux2 __server <name>` as a detached, windowless
// background process that outlives this client.
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
	cmd.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP |
			windows.DETACHED_PROCESS |
			windows.CREATE_NO_WINDOW,
		HideWindow: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn daemon: %w", err)
	}
	return cmd.Process.Release()
}
