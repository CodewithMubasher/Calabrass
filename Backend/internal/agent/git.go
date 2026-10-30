package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GitSnapshot says, in a few lines, where the project stands in git: the
// branch, whether there is unfinished work, and the last commit. It is for the
// model, so that "fix the failing test" and "commit this" start from the real
// state of the tree instead of a guess, and so it does not mistake the user's
// uncommitted work for something it made.
//
// It returns "" when the folder is not in a git repository, or git is not
// installed, or git does not answer quickly: the snapshot is a convenience and
// must never hold a turn up.
func GitSnapshot(ctx context.Context, root string) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	run := func(args ...string) (string, bool) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
		// Reading state must not take the index lock the user's own git needs.
		cmd.Env = append(cmd.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
		out, err := cmd.Output()
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(string(out)), true
	}

	if inside, ok := run("rev-parse", "--is-inside-work-tree"); !ok || inside != "true" {
		return ""
	}
	branch, ok := run("rev-parse", "--abbrev-ref", "HEAD")
	if !ok {
		// A repository with no commits yet has no HEAD.
		branch = "(no commits yet)"
	}
	if branch == "HEAD" {
		branch = "(detached HEAD)"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Git: on branch %s.", branch)

	if status, ok := run("status", "--porcelain", "--untracked-files=normal"); ok {
		lines := nonEmptyLines(status)
		if len(lines) == 0 {
			b.WriteString(" The working tree is clean.")
		} else {
			modified, untracked := 0, 0
			for _, l := range lines {
				if strings.HasPrefix(l, "??") {
					untracked++
				} else {
					modified++
				}
			}
			fmt.Fprintf(&b, " The working tree has uncommitted work that is the user's, not yours (%d changed, %d untracked); do not discard or commit it unless asked.", modified, untracked)
		}
	}
	if last, ok := run("log", "-1", "--pretty=%h %s"); ok && last != "" {
		fmt.Fprintf(&b, " Last commit: %s.", clip(last, 100))
	}
	return b.String()
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
