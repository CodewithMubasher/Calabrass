// Package permission decides what the agent may do without asking.
//
// Every action a tool can take falls in one of five tiers. The tier, not the
// tool's name, is what the policy talks about, so a new tool only has to say
// which tier it belongs in.
//
//	Tier 0  Observe          read, list and search inside the workspace
//	Tier 1  Change           create or edit files inside the workspace
//	Tier 2  Execute          run a command, start a connector action, book a task
//	Tier 3  Reach outside    touch paths or settings beyond the workspace, or do
//	                         something that is hard to undo
//	Tier 4  External effect  send, publish, push, delete remotely, spend money
//
// Defaults (see DefaultPolicy):
//
//   - Interactive chat: tiers 0 and 1 run freely. Tier 2 asks, and the person
//     may remember the answer for a command pattern in this workspace, or for
//     the rest of a crew run. Tiers 3 and 4 ask every time and are never
//     remembered.
//   - Unattended runs (schedules, voice, anything with nobody to ask): tiers 0
//     and 1 run. Anything above is refused unless the task carries a scope the
//     person wrote down when they created it. Tiers 3 and 4 are never allowed
//     unattended.
package permission

import "fmt"

// Tier is how much trust an action needs.
type Tier int

const (
	TierObserve  Tier = 0
	TierChange   Tier = 1
	TierExecute  Tier = 2
	TierReach    Tier = 3
	TierExternal Tier = 4
)

// String is the short name shown in an approval and in the run ledger.
func (t Tier) String() string {
	switch t {
	case TierObserve:
		return "observe"
	case TierChange:
		return "change files"
	case TierExecute:
		return "run a command"
	case TierReach:
		return "reach outside the workspace"
	case TierExternal:
		return "act on the outside world"
	default:
		return fmt.Sprintf("tier %d", int(t))
	}
}

// Valid reports whether t is one of the five tiers.
func (t Tier) Valid() bool { return t >= TierObserve && t <= TierExternal }

// Remembered reports whether an answer at this tier may be remembered. Tiers 3
// and 4 are asked every time, by design.
func (t Tier) Remembered() bool { return t <= TierExecute }
