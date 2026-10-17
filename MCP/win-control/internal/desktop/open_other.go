//go:build !windows

package desktop

import "errors"

// The errors both calls report where there is no desktop to act on. They are
// here so that the server still builds, its protocol can still be tested, and
// the tools above it can still be reasoned about on another machine.
var errNoDesktop = errors.New("acting on the desktop is wired up for Windows only")

// Open has nothing to open with anywhere but Windows.
func Open(string) error { return errNoDesktop }

// StartMenuSearch has no Start menu to search anywhere but Windows.
func StartMenuSearch(string) error { return errNoDesktop }
