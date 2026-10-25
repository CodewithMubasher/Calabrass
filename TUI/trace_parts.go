package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2/lexers"

	"calabrass-tui/internal/api"
)

// --- trace -> timeline parts ------------------------------------------------

// partOpts says how to read a trace.
type partOpts struct {
	// live: the turn is still running, so the last part may still be growing.
	live bool
	// stopped: the turn ended early, so no call is "running" any more.
	stopped bool
	// thoughtMs: how long each reasoning part took, keyed by (step, part).
	thoughtMs map[[2]int]int
}

// partsFromTrace turns the backend's record of a turn into timeline parts, in
// the order they happened.
func partsFromTrace(tr api.Trace, o partOpts) []Part {
	var parts []Part
	lastStep := len(tr.Steps) - 1
	for si, step := range tr.Steps {
		for pi, tp := range step.Parts {
			isLast := si == lastStep && pi == len(step.Parts)-1
			switch tp.Kind {
			case api.PartReasoning:
				text := strings.TrimSpace(tp.Text)
				if text == "" {
					continue
				}
				parts = append(parts, Part{
					Kind:    PartThought,
					Text:    text,
					Ms:      o.thoughtMs[[2]int{si, pi}],
					Working: o.live && isLast,
				})
			case api.PartText:
				if strings.TrimSpace(tp.Text) == "" {
					continue
				}
				parts = append(parts, Part{Kind: PartText, Text: tp.Text})
			case api.PartTool:
				parts = append(parts, toolParts(tp, o)...)
			}
		}
	}
	return parts
}

// toolParts turns one tool call into parts. The file-changing tools get their
// own file / diff nodes; everything else is a plain tool row.
func toolParts(tp api.TracePart, o partOpts) []Part {
	status := tp.Status
	running := status == "" || status == api.StatusPending || status == api.StatusRunning
	stopped := false
	if running && o.stopped {
		running, stopped = false, true
	}
	ok := status == api.StatusOK
	failed := status == api.StatusError || tp.Error != ""

	if tp.Server == "" && (ok || running) {
		switch tp.Name {
		case "write_file":
			var in struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			parsed := json.Unmarshal([]byte(tp.Input), &in) == nil
			if running || (ok && parsed) {
				return []Part{{Kind: PartEdit, File: in.Path, Lang: langFor(in.Path), Code: in.Content, Working: running}}
			}
		case "edit_file":
			var in struct {
				Path      string `json:"path"`
				OldString string `json:"oldString"`
				NewString string `json:"newString"`
			}
			parsed := json.Unmarshal([]byte(tp.Input), &in) == nil
			if running || (ok && parsed) {
				return []Part{{
					Kind: PartPatch, File: in.Path, Lang: langFor(in.Path),
					Start: lineFromOutput(tp.Output), Hunk: replaceHunk(in.OldString, in.NewString),
					Working: running,
				}}
			}
		case "edit_lines":
			var in struct {
				Path  string `json:"path"`
				Edits []struct {
					Start int    `json:"start"`
					End   int    `json:"end"`
					Text  string `json:"text"`
				} `json:"edits"`
			}
			parsed := json.Unmarshal([]byte(tp.Input), &in) == nil
			if running && !parsed {
				return []Part{{Kind: PartPatch, File: in.Path, Lang: langFor(in.Path), Working: true}}
			}
			if parsed && (ok || running) {
				var out []Part
				for _, e := range in.Edits {
					out = append(out, Part{
						Kind: PartPatch, File: in.Path, Lang: langFor(in.Path),
						Start: max(e.Start, 1), Note: lineRangeNote(e.Start, e.End, e.Text),
						Hunk: addedLines(e.Text), Working: running,
					})
				}
				if len(out) > 0 {
					return out
				}
			}
		}
	}

	// Everything else: one tool row.
	done, work := toolLabels(tp)
	p := Part{
		Kind: PartTool, Tool: tp.Name, Server: tp.Server,
		Label: done, WorkLabel: work, Working: running,
		Err: tp.Error, Failed: failed, Stopped: stopped, DurMs: tp.DurationMs,
		Output: tp.Output,
	}
	p.Explore, p.Summary = exploreInfo(tp)
	if tp.Name == "run_command" && tp.Server == "" {
		var cin struct {
			Command string `json:"command"`
		}
		if json.Unmarshal([]byte(tp.Input), &cin) == nil {
			p.Command = strings.TrimSpace(cin.Command)
		}
	}
	if tp.Name == "run_command" && tp.Server == "" && tp.Output != "" {
		var res struct {
			ExitCode int    `json:"exit_code"`
			TimedOut bool   `json:"timed_out"`
			Output   string `json:"output"`
		}
		if json.Unmarshal([]byte(tp.Output), &res) == nil {
			p.Output = res.Output
			if res.TimedOut {
				p.Failed, p.Err = true, "the command timed out"
			} else if res.ExitCode != 0 {
				p.Failed = true
				p.Err = fmt.Sprintf("exit code %d", res.ExitCode)
				p.Label += fmt.Sprintf(" (exit %d)", res.ExitCode)
			}
		}
	}
	return []Part{p}
}

var lineAtRe = regexp.MustCompile(`at line (\d+)`)

// lineFromOutput reads the line number edit_file reports ("... at line 12").
func lineFromOutput(out string) int {
	if m := lineAtRe.FindStringSubmatch(out); m != nil {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		if n > 0 {
			return n
		}
	}
	return 1
}

func splitLines(s string) []string {
	s = strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// replaceHunk is the diff of one edit_file call: the old lines removed, the new added.
func replaceHunk(oldText, newText string) []DiffLine {
	var h []DiffLine
	for _, l := range splitLines(oldText) {
		h = append(h, delL(l))
	}
	for _, l := range splitLines(newText) {
		h = append(h, addL(l))
	}
	return h
}

func addedLines(text string) []DiffLine {
	var h []DiffLine
	for _, l := range splitLines(text) {
		h = append(h, addL(l))
	}
	return h
}

// lineRangeNote describes what an edit_lines edit did to the old lines. The
// old text itself is not in the call, so it is described, not shown.
func lineRangeNote(start, end int, text string) string {
	switch {
	case end < start:
		return fmt.Sprintf("inserted before line %d", start)
	case strings.TrimSpace(text) == "" && start == end:
		return fmt.Sprintf("removed line %d", start)
	case strings.TrimSpace(text) == "":
		return fmt.Sprintf("removed lines %d-%d", start, end)
	case start == end:
		return fmt.Sprintf("replaced line %d", start)
	}
	return fmt.Sprintf("replaced lines %d-%d", start, end)
}

// langFor picks the highlighting language from a file name.
func langFor(path string) string {
	if path == "" {
		return ""
	}
	if l := lexers.Match(path); l != nil {
		return l.Config().Name
	}
	return ""
}

// toolLabels names a call: what it did, and what it is doing while it runs.
func toolLabels(tp api.TracePart) (done, working string) {
	args := map[string]any{}
	_ = json.Unmarshal([]byte(tp.Input), &args)
	str := func(key string) string {
		if v, ok := args[key].(string); ok {
			return oneLine(v)
		}
		return ""
	}
	with := func(verb, noun string) string { return strings.TrimSpace(verb + " " + noun) }

	if tp.Server != "" {
		name := tp.Server + " · " + tp.Name
		return name, name + "…"
	}
	switch tp.Name {
	case "read_file":
		return with("Read", str("path")), with("Reading", str("path")) + "…"
	case "list_dir":
		p := str("path")
		if p == "" {
			p = "."
		}
		return "Listed " + p, "Listing " + p + "…"
	case "glob":
		return with("Matched", str("pattern")), with("Matching", str("pattern")) + "…"
	case "grep":
		return with("Searched", str("pattern")), with("Searching", str("pattern")) + "…"
	case "create_dir":
		return with("Created folder", str("path")), with("Creating folder", str("path")) + "…"
	case "run_command":
		return with("Ran", str("command")), with("Running", str("command")) + "…"
	case "read_skill":
		return with("Read skill", str("skill")), with("Reading skill", str("skill")) + "…"
	case "ask_questions":
		n := len(parseQuestions(tp.Input))
		if n == 0 {
			return "Asked a question", "Asking…"
		}
		return "Asked " + plural(n, "question"), "Asking…"
	case "schedule_task":
		return "Scheduled a task", "Scheduling…"
	case "write_file":
		return with("Write", str("path")), with("Writing", str("path")) + "…"
	case "edit_file", "edit_lines":
		return with("Edit", str("path")), with("Editing", str("path")) + "…"
	}
	name := tp.Name
	if name == "" {
		name = "tool"
	}
	return name, "Running " + name + "…"
}

// exploreInfo says whether a call is a looking-around call ("read", "search"
// or "list") and gives the one line shown for it in an open Explored row.
func exploreInfo(tp api.TracePart) (kind, line string) {
	if tp.Server != "" {
		return "", ""
	}
	args := map[string]any{}
	_ = json.Unmarshal([]byte(tp.Input), &args)
	str := func(key string) string {
		if v, ok := args[key].(string); ok {
			return oneLine(v)
		}
		return ""
	}
	in := func(path string) string {
		if path == "" {
			return ""
		}
		return " in " + path
	}
	// "(120 matches)": one match per output line. Only once the call is done.
	count := func(one, many string) string {
		if tp.Status != api.StatusOK {
			return ""
		}
		n := 0
		for _, l := range strings.Split(tp.Output, "\n") {
			if strings.TrimSpace(l) != "" {
				n++
			}
		}
		switch n {
		case 0:
			return " (no " + many + ")"
		case 1:
			return " (1 " + one + ")"
		}
		return fmt.Sprintf(" (%d %s)", n, many)
	}

	switch tp.Name {
	case "read_file":
		return "read", strings.TrimSpace("Read " + str("path"))
	case "list_dir":
		p := str("path")
		if p == "" {
			p = "."
		}
		return "list", "List " + p
	case "grep":
		return "search", `Grep "` + str("pattern") + `"` + in(str("path")) + count("match", "matches")
	case "glob":
		return "search", `Glob "` + str("pattern") + `"` + in(str("path")) + count("file", "files")
	}
	return "", ""
}

// oneLine collapses whitespace so a multi-line command fits on a row.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// --- live mirror ------------------------------------------------------------

// mirror keeps the same picture of the turn that the backend's trace builder
// has. The backend sends the whole trace at tool boundaries and step ends, and
// the text / reasoning deltas in between; applying each delta the way the
// builder does keeps the two in step without waiting for the next snapshot.
type mirror struct {
	trace api.Trace
	spans map[[2]int]*span
}

type span struct{ first, last time.Time }

func newMirror() *mirror { return &mirror{spans: map[[2]int]*span{}} }

// setTrace replaces everything with the backend's own snapshot.
func (m *mirror) setTrace(t api.Trace) { m.trace = t }

// stepClosed: a step that called tools and has no call still pending is over,
// so the next delta opens a new step (as the backend's builder does).
func stepClosed(s api.TraceStep) bool {
	hasTool := false
	for _, p := range s.Parts {
		if p.Kind != api.PartTool {
			continue
		}
		hasTool = true
		if p.Status == "" || p.Status == api.StatusPending {
			return false
		}
	}
	return hasTool
}

// addDelta applies a reasoning or text delta.
func (m *mirror) addDelta(kind, text string, now time.Time) {
	if text == "" {
		return
	}
	n := len(m.trace.Steps)
	if n == 0 || stepClosed(m.trace.Steps[n-1]) {
		m.trace.Steps = append(m.trace.Steps, api.TraceStep{Number: n + 1})
		n++
	}
	step := &m.trace.Steps[n-1]
	if k := len(step.Parts); k > 0 && step.Parts[k-1].Kind == kind {
		step.Parts[k-1].Text += text
	} else {
		step.Parts = append(step.Parts, api.TracePart{Kind: kind, Text: text})
	}
	if kind == api.PartReasoning {
		key := [2]int{n - 1, len(step.Parts) - 1}
		sp := m.spans[key]
		if sp == nil {
			sp = &span{first: now}
			m.spans[key] = sp
		}
		sp.last = now
	}
}

// thoughtMs is how long each reasoning part streamed for, as seen by this client.
func (m *mirror) thoughtMs() map[[2]int]int {
	out := make(map[[2]int]int, len(m.spans))
	for k, sp := range m.spans {
		out[k] = int(sp.last.Sub(sp.first).Milliseconds())
	}
	return out
}

// --- run note ---------------------------------------------------------------

const runNoteMarker = "_Run note:"

// runNote extracts the line the backend appends to an answer when a limit cut
// the run short. It is part of the saved message but not of its trace.
func runNote(content string) string {
	i := strings.LastIndex(content, runNoteMarker)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(content[i:])
}

// withRunNote adds the note as a closing text part when the trace lacks it.
func withRunNote(parts []Part, content string) []Part {
	note := runNote(content)
	if note == "" {
		return parts
	}
	for _, p := range parts {
		if p.Kind == PartText && strings.Contains(p.Text, note) {
			return parts
		}
	}
	return append(parts, Part{Kind: PartText, Text: note})
}
