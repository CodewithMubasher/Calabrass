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
