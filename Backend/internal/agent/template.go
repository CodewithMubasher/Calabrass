package agent

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// PromptVars are the values a prompt template may ask for. Every one of them
// is filled in when the turn is built, so a prompt never carries a stale fact
// about the machine it is running on.
type PromptVars struct {
	// Workspace is the folder the run works in.
	Workspace string
	// Date is when the turn started.
	Date string
	// Tools are the tools the run may call.
	Tools []ToolSummary
	// Workers are the agents the run may hand work to.
	Workers []WorkerSummary
}

// ToolSummary is one tool, as a prompt names it.
type ToolSummary struct {
	Name        string
	Description string
}

// WorkerSummary is one worker, as a prompt names it. The id rides along
// because delegate_task needs the id, not the label: two workers may share a
// name, but only one may hold an id.
type WorkerSummary struct {
	ID          string
	Name        string
	Description string
}

// The placeholders a template may use.
const (
	placeholderWorkspace = "{{workspace}}"
	placeholderDate      = "{{date}}"
	placeholderTools     = "{{tools_section}}"
	placeholderWorkers   = "{{workers_section}}"
)

// RenderTemplate fills a prompt template in.
//
// The placeholders are:
//
//	{{workspace}}       the folder the run works in
//	{{date}}            when the turn started
//	{{tools_section}}   the tools the run may call, under a heading
//	{{workers_section}} the workers the run may hand work to, under a heading
//
// A section whose value is empty leaves nothing behind — not even its heading.
// A prompt with no workers to name therefore says nothing at all about
// workers, rather than naming a heading with an empty list under it.
func RenderTemplate(template string, vars PromptVars) string {
	if !strings.Contains(template, "{{") {
		return template
	}

	out := strings.ReplaceAll(template, placeholderWorkspace, vars.Workspace)
	out = strings.ReplaceAll(out, placeholderDate, vars.Date)
	out = strings.ReplaceAll(out, placeholderTools, toolSection(vars.Tools))
	out = strings.ReplaceAll(out, placeholderWorkers, workerSection(vars.Workers))

	// A section that was empty left the blank line that separated it from the
	// text around it. Two blank lines read as a deliberate break; three read as
	// a mistake.
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(out)
}

// toolSection is the tools a run may call, one per line, each with the first
// sentence of its own description.
func toolSection(tools []ToolSummary) string {
	if len(tools) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Available tools:")
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		b.WriteString("\n")
		b.WriteString(name)
		if description := firstSentence(tool.Description); description != "" {
			b.WriteString(" — ")
			b.WriteString(description)
		}
	}
	return b.String()
}

// workerSection is the workers a run may hand work to, numbered, each with
// what it is for.
func workerSection(workers []WorkerSummary) string {
	if len(workers) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Available workers:")
	number := 0
	for _, worker := range workers {
		name := strings.TrimSpace(worker.Name)
		if name == "" {
			continue
		}
		number++
		b.WriteString("\n")
		b.WriteString(strconv.Itoa(number))
		b.WriteString(". ")
		b.WriteString(name)
		if id := strings.TrimSpace(worker.ID); id != "" {
			b.WriteString(" (id: ")
			b.WriteString(id)
			b.WriteString(")")
		}
		description := strings.TrimSpace(worker.Description)
		if description == "" {
			description = "no description given"
		}
		b.WriteString(" — can do: ")
		b.WriteString(description)
	}
	if number == 0 {
		return ""
	}
	return b.String()
}

// firstSentence is the opening of a description: up to its first full stop,
// never more than one line, and never cut in the middle of a word.
func firstSentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if at := strings.IndexAny(text, "\r\n"); at >= 0 {
		text = strings.TrimSpace(text[:at])
	}
	if at := strings.Index(text, ". "); at >= 0 {
		return text[:at+1]
	}
	if strings.HasSuffix(text, ".") {
		return text
	}
	return truncateWords(text, 160)
}

// truncateWords cuts text to at most limit runes, at a word boundary, and
// says that it was cut.
func truncateWords(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	cut := limit
	for cut > 0 && !isSpace(runes[cut]) {
		cut--
	}
	if cut == 0 {
		cut = limit
	}
	return strings.TrimSpace(string(runes[:cut])) + "…"
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}
