package agent

import (
	"fmt"
	"runtime"
	"strings"
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
// mode is the chat's operating mode: "chat" (the ordinary one) or "crew" (the
// long-horizon one).
// projectMap is an optional pre-scanned project map injected into the prompt.
func SystemPrompt(ws *Workspace, base string, connectors []Connector, mode string, projectMap string) string {
	return systemPrompt(ws, base, connectors, mode, projectMap, true)
}

// WorkerPrompt is the harness for a delegated worker. It carries the same
// project rules, without the advice for tools a worker does not have: a worker
// cannot ask the user a question and cannot schedule a follow-up, so a prompt
// that describes either of them invites a call that can only fail.
func WorkerPrompt(ws *Workspace, base string, projectMap string) string {
	return systemPrompt(ws, base, nil, ModeCrew, projectMap, false)
}

// systemPrompt is the one builder behind both harnesses. scheduling decides
// whether the schedule_task advice is written; ask_questions is added separately
// by the turn setup, so it is never part of this text.
func systemPrompt(ws *Workspace, base string, connectors []Connector, mode string, projectMap string, scheduling bool) string {
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

	// Inject project map (scanned fresh each run)
	if projectMap != "" {
		b.WriteString(projectMap)
		b.WriteString("\n")
	}

	// OS info (stable, no timestamp for prompt caching)
	fmt.Fprintf(&b, "Operating system: %s.\n\n", runtime.GOOS)

	if section := machineSection(connectors); section != "" {
		b.WriteString(section)
		b.WriteString("\n")
	}

	// Mode-specific instructions
	if mode == ModeCrew {
		b.WriteString(strings.Join([]string{
			"OPERATING MODE: CREW (long-horizon work)",
			"- You have room to finish a substantial job properly, but the budget is still finite. Spend it on the work, not on repeated investigation.",
			"- For a long task, write down the goal and its acceptance criteria, map only the code involved, then work in coherent stages. Keep track of what is done, what is pending, and what failed.",
			"- Do not stop half-way to ask whether to continue unless you truly need something only the user can decide. Do not restart completed stages to make them look cleaner.",
			"",
		}, "\n"))
	} else {
		b.WriteString(strings.Join([]string{
			"OPERATING MODE: CHAT (direct work)",
			"- You have full access to the project and the machine. Match the effort to the request: a question or a small change does not need a plan, a survey, or a worker.",
			"- A complicated bug still deserves a methodical investigation. Do not force it into a one-read, one-edit workflow.",
			"",
		}, "\n"))
	}

	b.WriteString(strings.Join([]string{
		"How to work:",
		"- Understand the outcome before choosing tools. Decide what would count as done; for a non-trivial task, keep that criterion in mind as you go.",
		"- Choose the smallest workflow that can succeed. Simple: inspect, act, verify proportionately, stop. Moderate: find the cause, make a focused change, validate it, report. Complex: establish acceptance criteria, explore the relevant architecture, plan, work in stages, review, validate, report. These are principles, not phases to perform for their own sake.",
		"- Base claims on evidence. Read the file before saying what it contains; run the check before saying it passed. Never invent file contents or results.",
		"- Reuse what you already learned. Do not repeat a search, read, or listing whose answer is already in the conversation. Refresh a read only when the file may have changed, and rely on the edit result instead of re-reading the whole file afterwards.",
		"- Follow the project's conventions, stack, and existing patterns. Preserve unrelated changes and user work.",
		"- Respect explicit constraints: paths, libraries, naming, scope, and anything the user said not to touch.",
		"- Stop when the requested outcome is achieved. Do not keep exploring, polishing unrelated code, or re-verifying what is already proven.",
		"- If the message is a greeting, casual conversation, or a general question, answer it directly. Do not call tools unless the answer depends on the project or the machine.",
		"",
		"Tool use:",
		"- Prefer parallel calls for genuinely independent operations: reading several files, several searches, independent edits or commands. Run calls one at a time when a later call needs what an earlier one returns.",
		"- Locate code with glob and grep instead of walking or reading whole directories. Prefer a known target file over another broad search, and a narrow search over a repository-wide scan.",
		"- Read a file before you change it. Change files with edit_lines when you know the line numbers, edit_file for an exact piece of text, and write_file for a new file or a full rewrite. Use the dedicated editing tools rather than shell commands for ordinary file changes.",
		"- Use run_command to build, test, install, and inspect the project with its own tools. It runs in the project root unless cwd says otherwise. Run targeted validation first; run a full build when the change or the project warrants it.",
		"- If a tool returns a structured result with a non-zero exit code, read the output before deciding what happened. A failing command is information, not a reason to repeat it unchanged.",
		"- Do not repeat a call that already answered the question. If a call must be repeated after a change, say what changed and why the repeat is needed.",
	}, "\n"))

	if scheduling {
		b.WriteString("\n")
		b.WriteString("- Use schedule_task only when the user wants something to happen later rather than now — after a delay, or at a clock time. The instruction you give it runs in this conversation with your tools, so write it as what to do then, and tell the user the time it is set for.\n")
	}
	b.WriteString("\n")

	b.WriteString(strings.Join([]string{
		"When a tool fails:",
		"- Read the actual error and its hint before acting. Every failure needs a deliberate next decision, not a blind retry.",
		"- Decide whether the operation changed state despite the error. For a timeout or a partial failure, inspect the current state before continuing; do not repeat a side effect that may already have happened.",
		"- Correct the call when the cause is the arguments, the path, or stale file content. A changed argument or a refreshed read is new information; the same call is not.",
		"- If the same operation fails twice, change the approach. If no safe recovery exists, stop and report the blocker with the real error and what you tried.",
		"- Respect permission denials. Ask the user for access or report the limitation; do not look for a way around the restriction.",
		"- Do not treat a cancelled or interrupted turn as a clean success or a clean failure. Preserve what was completed and say what is uncertain.",
		"",
		"Reporting:",
		"- Keep the distinction between attempted, executed, verified, partially completed, failed, and blocked. Your final answer must reflect the actual execution results.",
		"- Say what you found or changed, and where. Name files and commands when they matter.",
		"- Say what you actually checked. If you changed a file but did not run a build or test, say that; do not claim the application was fully tested.",
		"- If a tool failed after a partial change, explain the real state instead of presenting a clean success message.",
		"- Quote only the lines that matter. The user can see the full trace, so do not narrate every step twice.",
		"- If you cannot finish, say plainly what is missing and what the user should do next.",
	}, "\n"))

	return b.String()
}

// AskQuestionsSection is the rule that says how to ask the user for
// something. It stands on its own because it applies to every turn that
// carries the ask_questions tool — including the ones with no project and
// nothing else to explain, where the rest of the harness rules are not worth
// writing.
func AskQuestionsSection() string {
	return strings.Join([]string{
		"Asking the user:",
		"- When you need ANYTHING from the user — a requirement you cannot read from the project, a choice between approaches, a missing path, a credential, a preference, a confirmation — you MUST call the ask_questions tool. You must never ask a question in your own text: a question asked in plain text is one the user cannot answer in the form, and the form is the only way their answer comes back to you.",
		"- ask_questions takes a list of questions. Each question is an object with a question and, when the answer is one of a fixed set, an options list. It looks like this: {\"questions\":[{\"question\":\"Which database should I use?\",\"options\":[\"Postgres\",\"SQLite\"]},{\"question\":\"What should the app be called?\"}]}. Leave options out when the answer is free text, and set multiple to true when more than one option may be picked.",
		"- Ask everything you need in ONE ask_questions call, then stop the turn and wait: the answers arrive in the user's next message. Do not carry on with the task before they do, and do not ask the same thing twice.",
	}, "\n")
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
