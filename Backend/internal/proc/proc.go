// Package proc starts the child processes this program runs so that they do not
// put a console window on the user's screen.
//
// The desktop app is a GUI program: it has no console of its own, so Windows
// gives one to every console program it starts — git for the workspace card,
// PowerShell for the agent's run_command. The window appears over the app and
// stays until the command ends. Hide marks a command so that never happens.
package proc

import "os/exec"

// Hide readies a command to run without a console window of its own. It is a
// no-op on systems where console windows are not a thing.
func Hide(cmd *exec.Cmd) { hide(cmd) }

// Detach makes cmd outlive the process that starts it: no console, no shared
// process group or session. A background host is started this way, so closing
// the window or terminal that launched it does not take it down.
func Detach(cmd *exec.Cmd) { detach(cmd) }

// KillTree makes cancelling cmd end the whole command, not just the shell that
// started it. A `sh -c "sleep 30"` that is killed leaves the sleep running, still
// holding the output pipe, so Wait would block until the sleep ends and a
// timeout would not time anything out. On Unix the command gets a process group
// that is killed as one; everywhere, Wait gives up on a held pipe soon after the
// kill.
func KillTree(cmd *exec.Cmd) { killTree(cmd) }
