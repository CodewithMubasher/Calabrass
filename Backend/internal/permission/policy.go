package permission

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Mode says whether a person is there to answer.
type Mode int

const (
	// Interactive means a person is watching a conversation and can answer.
	Interactive Mode = iota
	// Unattended means nobody is there: a schedule, a voice turn, a request
	// that arrived without a stream to ask on. It runs inside its Scope.
	Unattended
)

// Decision is a person's answer to an approval.
type Decision string

const (
	AllowOnce    Decision = "allow_once"    // this one action
	AllowPattern Decision = "allow_pattern" // this kind of command, in this workspace
	AllowRun     Decision = "allow_run"     // everything up to tier 2, for the rest of this conversation
	Deny         Decision = "deny"
)

// Valid reports whether d is an answer the broker understands.
func (d Decision) Valid() bool {
	switch d {
	case AllowOnce, AllowPattern, AllowRun, Deny:
		return true
	}
	return false
}

// Scope is what a person pre-approved for an unattended run when they created
// it. The model never writes a scope: it is read from the task a human made.
//
// The zero Scope is the default: tiers 0 and 1 only.
type Scope struct {
	// MaxTier is the highest tier that runs without asking. It is capped at
	// Execute: nothing unattended ever reaches outside the workspace.
	MaxTier Tier `json:"maxTier"`
	// Commands are the command patterns allowed at the Execute tier. A trailing
	// " *" matches any sub-command: "npm *" allows "npm test" and "npm run".
	Commands []string `json:"commands,omitempty"`
}

// DefaultScope is what a task gets when its creator said nothing.
func DefaultScope() Scope { return Scope{MaxTier: TierChange} }

// ParseScope reads a stored scope. Anything unreadable falls back to the
// default, which is the cautious answer.
func ParseScope(raw string) Scope {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultScope()
	}
	var s Scope
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return DefaultScope()
	}
	return s.Normalized()
}

// Normalized clamps a scope to what unattended runs may ever hold.
func (s Scope) Normalized() Scope {
	if s.MaxTier < TierObserve {
		s.MaxTier = TierObserve
	}
	if s.MaxTier > TierExecute {
		s.MaxTier = TierExecute
	}
	cleaned := make([]string, 0, len(s.Commands))
	for _, c := range s.Commands {
		if c = strings.ToLower(strings.TrimSpace(c)); c != "" {
			cleaned = append(cleaned, c)
		}
	}
	s.Commands = cleaned
	return s
}

// JSON is the stored form.
func (s Scope) JSON() string {
	data, err := json.Marshal(s.Normalized())
	if err != nil {
		return ""
	}
	return string(data)
}

// Allows reports whether an unattended run may take the action.
func (s Scope) Allows(act Action) bool {
	s = s.Normalized()
	if act.Tier <= TierChange {
		return true
	}
	if act.Tier > TierExecute {
		return false
	}
	// An execution is allowed only when a command pattern names it. A bare
	// MaxTier of 2 is not a blank cheque: it still needs a listed command.
	if act.Pattern == "" {
		return false
	}
	for _, allowed := range s.Commands {
		if matchPattern(allowed, act.Pattern) {
			return true
		}
	}
	return false
}

func matchPattern(allowed, pattern string) bool {
	if allowed == pattern {
		return true
	}
	if strings.HasSuffix(allowed, " *") {
		return pattern == strings.TrimSuffix(allowed, " *") ||
			strings.HasPrefix(pattern, strings.TrimSuffix(allowed, "*"))
	}
	return false
}

// Denied is returned to the model when an action is not allowed. It reads as a
// refusal to respect, and it classifies as a permission error so the harness
// tells the model not to look for a way around it.
type Denied struct {
	Action Action
	Reason string
}

func (d *Denied) Error() string {
	return fmt.Sprintf(
		"permission denied: %q was not run (%s). %s Do not retry it or look for another way to do the same thing; "+
			"tell the user what you needed and why, and carry on with what is allowed.",
		d.Action.Summary, d.Action.Tier, d.Reason)
}

// Is makes a denial look like a permission error to errors.Is.
func (d *Denied) Is(target error) bool { return target == os.ErrPermission }
