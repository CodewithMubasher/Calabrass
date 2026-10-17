package agent

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

// Connector is a running MCP server, as the prompt should describe it: the name
// it goes by, what it says it is for, and the tools it offers.
//
// The model is told about the machine it is sitting on rather than left to infer
// it from tool schemas. A model that does not know it has a desktop reads its
// tools as one more way to work on files, and answers "open the settings app"
// by listing the project.
type Connector struct {
	Name string
	// Instructions are the server's own words about what it is for, from the
	// MCP handshake. They are the connector saying so, not this program guessing.
	Instructions string
	Tools        []string
}

// SystemPrompt builds the harness instructions: which project the model may
// work on, what the tools are for, and the rules that keep its answers and its
// trace readable. base is the application's own prompt, kept in front.
func SystemPrompt(ws *Workspace, base string, connectors []Connector) string {
	var b strings.Builder

	if trimmed := strings.TrimSpace(base); trimmed != "" {
		b.WriteString(trimmed)
		b.WriteString("\n\n")
	}

	if ws != nil {
		b.WriteString("You are working in a real project on the user's machine and can inspect and change it with tools.\n\n")
		fmt.Fprintf(&b, "Project root: %s\n", ws.Root())
		b.WriteString("All tool paths are resolved inside that directory; anything outside it is refused.\n")
	}
	fmt.Fprintf(&b, "Operating system: %s. Today: %s.\n\n", runtime.GOOS, time.Now().Format("2006-01-02"))

	if section := machineSection(connectors); section != "" {
		b.WriteString(section)
		b.WriteString("\n")
	}

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

// machineSection describes the connectors that reach the user's own computer.
// It is empty when there are none, because then there is nothing to say: the
// tools really are all about the project.
func machineSection(connectors []Connector) string {
	if len(connectors) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("The computer you are running on is the user's own, and the connectors below act on it: " +
		"an application that opens, music that plays, a page that appears in front of them. " +
		"A request about the machine rather than the project — open an app, play a song, search the web — " +
		"is a request for these tools, so use them instead of looking through the project for it.\n")

	for _, connector := range connectors {
		fmt.Fprintf(&b, "- %s", strings.TrimSpace(connector.Name))
		if instructions := strings.TrimSpace(connector.Instructions); instructions != "" {
			fmt.Fprintf(&b, ": %s", instructions)
		}
		if len(connector.Tools) > 0 {
			fmt.Fprintf(&b, " Tools: %s.", strings.Join(connector.Tools, ", "))
		}
		b.WriteString("\n")
	}

	b.WriteString("If none of them can do what was asked, say what is missing in a line " +
		"rather than calling a tool that will refuse it.\n")
	return b.String()
}
