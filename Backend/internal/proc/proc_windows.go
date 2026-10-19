//go:build windows

package proc

import (
	"os/exec"
	"syscall"
)

// createNoWindow keeps Windows from giving a console program a console of its
// own. A GUI parent — which this app is — has none to lend it, so without this
// flag every git call and every shell command flashes a terminal over the app.
const createNoWindow = 0x08000000

func hide(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// HideWindow covers a window the program opens for itself; the creation
	// flag covers the console Windows attaches before the program even starts.
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
