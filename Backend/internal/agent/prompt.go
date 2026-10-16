package agent

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

// SystemPrompt builds the harness instructions: which project the model may
// work on, what the tools are for, and the rules that keep its answers and its
// trace readable. base is the application's own prompt, kept in front.
func SystemPrompt(ws *Workspace, base string) string {
	var b strings.Builder

	if trimmed := strings.TrimSpace(base); trimmed != "" {
		b.WriteString(trimmed)
		b.WriteString("\n\n")
	}

	b.WriteString("You are working in a real project on the user's machine and can inspect and change it with tools.\n\n")

	if ws != nil {
		fmt.Fprintf(&b, "Project root: %s\n", ws.Root())
		b.WriteString("All tool paths are resolved inside that directory; anything outside it is refused.\n")
	}
	fmt.Fprintf(&b, "Operating system: %s. Today: %s.\n\n", runtime.GOOS, time.Now().Format("2006-01-02"))

	b.WriteString(strings.Join([]string{
		"Tool use:",
		"- Use tools when the answer depends on the project: read before you claim, and check before you change.",
		"- Pass paths relative to the project root, e.g. \"src/app.tsx\". Never invent file contents.",
		"- Locate things with glob and grep instead of reading whole directories, and read a file before you change it.",
		"- Change an existing file with edit_file, quoting the exact text to replace. Use write_file for a new file, or when the whole file is being rewritten.",
		"- Use create_dir when a directory is missing; write_file creates missing parents on its own.",
		"- Gather what you need in as few calls as possible. Do not repeat a call that already answered the question.",
		"- If a tool fails, read the error and adjust; do not retry the same call unchanged.",
		"",
		"Answering:",
		"- Say what you found or changed, and where. Keep it short; the user can see the full trace of every tool call, so do not narrate each step twice.",
		"- Quote only the lines that matter instead of pasting whole files.",
		"- If you cannot finish, say plainly what is missing rather than guessing.",
	}, "\n"))

	return b.String()
}
