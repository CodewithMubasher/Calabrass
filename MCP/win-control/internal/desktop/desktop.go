// Package desktop does the things this server promises: it opens a browser on
// a web address. Each promise is a small function here, so the tools above can
// be read as what they do rather than how they do it.
package desktop

import (
	"os"
	"strings"
)

// DryRunEnv turns acting into saying: when it is set, every action is reported
// instead of taken. It is what makes this server safe to test — and safe to
// hand to an agent before you have watched it work.
const DryRunEnv = "CALABRASS_WIN_CONTROL_DRY_RUN"

// DryRun reports whether actions should be described rather than taken.
func DryRun() bool {
	return strings.TrimSpace(os.Getenv(DryRunEnv)) != ""
}
