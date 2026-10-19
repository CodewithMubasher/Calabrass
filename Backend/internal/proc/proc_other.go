//go:build !windows

package proc

import "os/exec"

// hide does nothing away from Windows: a process that has no terminal attached
// simply has none, and there is no window to suppress.
func hide(cmd *exec.Cmd) {}
