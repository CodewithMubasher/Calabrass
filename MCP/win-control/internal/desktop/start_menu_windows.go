//go:build windows

package desktop

import (
	"time"

	"github.com/CodewithMubasher/Calabrass/mcp/win-control/internal/wininput"
)

// How long Windows is given to keep up. Both waits are generous on purpose: this
// happens once, and a Start menu that has not finished searching opens the wrong
// thing — or, worse, the keystrokes land in whatever was in front.
const (
	startMenuDelay = 400 * time.Millisecond
	searchDelay    = 900 * time.Millisecond
)

// StartMenuSearch opens something by name: the Windows key, the name typed into
// the Start menu's search, then Enter.
//
// It is how a person opens an application they only know the name of —
// "settings", "notepad", "Spotify" — because it does not need a path or an
// installed-name lookup: Windows' own search does the matching.
func StartMenuSearch(name string) error {
	if DryRun() {
		return nil
	}

	if err := wininput.Tap(wininput.KeyWin); err != nil {
		return err
	}
	time.Sleep(startMenuDelay)

	if err := wininput.Type(name); err != nil {
		return err
	}
	time.Sleep(searchDelay)

	return wininput.Tap(wininput.KeyReturn)
}
