//go:build !windows

package hostctl

import (
	"os/exec"
	"syscall"
)

func detach(cmd *exec.Cmd) {
	// A session of its own: closing this terminal does not hang the host up.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
