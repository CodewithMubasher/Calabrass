package agent

import "strings"

// The two operating modes a conversation can run in.
//
// Chat is the ordinary one: the assistant talks, and it works — files,
// commands, connectors, all of it.
//
// Crew is for long and complex work. It is the same assistant with a longer
// budget and a longer leash, and it lives under its own address so the two
// kinds of conversation are told apart at a glance.
const (
	ModeChat = "chat"
	ModeCrew = "crew"
)

// NormalMode returns the mode a value stands for, or "" when it is neither of
// the two. An empty string is what the callers use to mean "not a mode this
// build knows about".
//
// The two names this build replaced — "build" and "plan" — are still read, and
// both land on "chat": plan mode is gone, and build mode is the ordinary chat
// under its new name. Nothing stale is ever stored.
func NormalMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ModeChat, "build", "plan":
		return ModeChat
	case ModeCrew:
		return ModeCrew
	default:
		return ""
	}
}

// ValidMode reports whether a value names a mode this build knows.
func ValidMode(mode string) bool {
	return NormalMode(mode) != ""
}
