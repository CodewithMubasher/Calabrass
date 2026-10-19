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
// mode is the chat's operating mode: "build" (full tools) or "plan" (read-only).
func SystemPrompt(ws *Workspace, base string, connectors []Connector, mode string) string {
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

	// The clock is stated rather than left to be inferred: a model that does not
	// know what time it is on this machine cannot set a timer for "in ten
	// minutes" or turn "at nine tonight" into a time, and guesses.
	now := time.Now()
	fmt.Fprintf(&b, "Operating system: %s. It is %s local time (%s), on %s.\n\n",
		runtime.GOOS,
		now.Format("15:04"),
		now.Format("MST"),
		now.Format("Monday 2 January 2006"),
	)

	if section := machineSection(connectors); section != "" {
		b.WriteString(section)
		b.WriteString("\n")
	}

	// Mode-specific instructions
	if mode == "plan" {
		b.WriteString(strings.Join([]string{
			"OPERATING MODE: PLAN (Read-only)",
			"- You are in PLANNING MODE. Your tools are LIMITED TO READ-ONLY operations.",
			"- You CAN: read files, list directories, search with glob/grep, read code.",
			"- You CANNOT: write files, edit files, create directories, run shell commands, use MCP connectors that modify state.",
			"- Your goal is to ANALYZE, RESEARCH, and CREATE A DETAILED PLAN when given a specific task or request.",
			"- If the user provides a greeting, casual chat, or general question, respond conversationally FIRST. DO NOT call any workspace tools (like listing directories or reading files) unless the user asks you to inspect the workspace or gives you a specific problem to analyze.",
			"- When the user wants to execute the plan, they will switch to BUILD mode.",
			"- Always output your plan as a clear, structured Markdown document.",
			"",
		}, "\n"))
	} else {
		b.WriteString(strings.Join([]string{
			"OPERATING MODE: BUILD (Full access)",
			"- You are in BUILD MODE. You have FULL ACCESS to all tools.",
			"- You CAN: read, write, edit files, create directories, run shell commands, use all MCP connectors.",
			"- Your goal is to IMPLEMENT, BUILD, and EXECUTE tasks requested by the user.",
			"- If the user provides a greeting, casual chat, or general question, respond conversationally FIRST. DO NOT call any tools unless the user gives an explicit request, task, or instruction that requires action.",
			"- Use tools freely when executing a task, but never call tools proactively on greetings or simple conversation.",
			"",
		}, "\n"))
	}

	b.WriteString(strings.Join([]string{
		"Tool use:",
		"- Prefer parallel tool calls: when several calls do not depend on each other — reading multiple files, several searches, independent edits or commands — make them all in the same step so they run together. Only run one call at a time when a later call needs what an earlier one returns. This is the first choice, not the fallback.",
		"- Use tools when the answer depends on the project: read before you claim, and check before you change.",
		"- Pass paths relative to the project root, e.g. \"src/app.tsx\". Never invent file contents.",
		"- Locate things with glob and grep instead of reading whole directories, and read a file before you change it.",
		"- Change files with edit_lines, naming the lines read_file numbered and the text to put in their place; every edit in the call lands together, so a multi-part change is one call. Use edit_file for an exact piece of text that is not a whole line range, and write_file for a new file or a full rewrite.",
		"- Use create_dir when a directory is missing; write_file creates missing parents on its own.",
		"- Use run_command to build, test, install and inspect the project with its own tools. It runs in the project root. For reading code, read_file, glob and grep cost far less context.",
		"- Use schedule_task when the user wants something to happen later rather than now — after a delay, or at a clock time. The instruction you give it runs in this conversation with your tools, so write it as what to do then, and tell the user the time it is set for.",
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
