//go:build windows

package hostctl

import (
	"os/exec"
	"syscall"
)

func detach(cmd *exec.Cmd) {
	// No console, no window, and not in this process's group, so the host
	// outlives this terminal.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x00000008 | 0x00000200 | 0x08000000, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW
	}
}
