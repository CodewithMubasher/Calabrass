//go:build windows

package proc

import (
	"os/exec"
	"syscall"
	"time"
)

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

func detach(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// No console, no window, and not part of the parent's process group, so
	// closing the window or terminal that started it does not stop it.
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= detachedProcess | createNewProcessGroup | createNoWindow
}

func killTree(cmd *exec.Cmd) {
	// Windows has no process groups to signal here; stop waiting on a pipe that a
	// child still holds soon after the kill.
	cmd.WaitDelay = 3 * time.Second
}
