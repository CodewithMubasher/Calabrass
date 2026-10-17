// Package wininput puts keystrokes into the desktop: the Windows key, a string
// of text, Enter.
//
// It is the one piece of a desktop tool that has no standard-library answer —
// there is no way to press a key on Windows without calling the keyboard API —
// and it is written small enough to read at a glance: what to type, and what to
// press.
//
// Only 64-bit Windows is wired up. Everywhere else the calls report
// ErrUnsupported, so the tools above still build, and the decisions they make
// can still be tested, on a machine that has no keyboard to type into.
package wininput

import "errors"

// The two keys the tools press, as Windows names them.
const (
	// KeyWin is the Windows key, which opens the Start menu.
	KeyWin = 0x5B
	// KeyReturn is Enter, which opens what the Start menu is showing.
	KeyReturn = 0x0D
)

// ErrUnsupported is what the desktop calls report where there is no keyboard to
// type on.
var ErrUnsupported = errors.New("typing on the desktop is wired up for 64-bit Windows only")
