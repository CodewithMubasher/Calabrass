//go:build windows

package desktop

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// Open hands something to the shell: an application, a document, a folder, or a
// Windows URI like ms-settings:.
//
// ShellExecute is used rather than starting a process: it is the shell's own
// answer to "open this", so it finds the default handler, respects the person's
// choices, and needs no command line to be quoted — which is the part that goes
// wrong when a URL carries an ampersand.
func Open(target string) error {
	if target == "" {
		return errors.New("nothing to open")
	}
	if DryRun() {
		return nil
	}

	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return fmt.Errorf("could not read %q: %w", target, err)
	}
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return fmt.Errorf("could not read the verb: %w", err)
	}

	if err := windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("could not open %s: %w", target, err)
	}
	return nil
}
