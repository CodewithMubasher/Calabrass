package main

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/x/ansi"
)

// The assistant's answer is a timeline of parts, in order:
//
//   thought  -> "Thought: 579ms" + the reasoning text
//   edit     -> "# Wrote main.py" + numbered, highlighted code
//   text     -> Markdown
//
// Thoughts and edits that follow each other are folded into ONE row:
//
//   + 1 edit, 1 thought        (click to open, hover to brighten)

// PartKind says what a timeline part is.
type PartKind int

const (
	PartText    PartKind = iota
	PartThought          // reasoning
	PartEdit             // a file that was written:  "# Wrote main.py"
	PartPatch            // a change to a file:       "← Edit main.py"  (old - / new +)
	PartTool             // any other tool call:      "✓ Read main.py"
)

// maxNodeLines is how many lines of a file or a diff an open row shows; the
// rest is summarised, so one huge write cannot flood the screen.
const maxNodeLines = 200

// DiffLine is one line of a patch: Op is ' ' (unchanged), '+' (added) or
// '-' (removed).
type DiffLine struct {
	Op   byte
	Text string
}

// Short helpers to write patches: ctxL("x"), addL("x"), delL("x").
func ctxL(s string) DiffLine { return DiffLine{' ', s} }
func addL(s string) DiffLine { return DiffLine{'+', s} }
func delL(s string) DiffLine { return DiffLine{'-', s} }

// Part is one node of the timeline.
type Part struct {
	Kind PartKind

	Text string // PartText: Markdown. PartThought: the reasoning.
	Ms   int    // PartThought: how long it took

	File string // PartEdit: file name
	Lang string // PartEdit: language for highlighting ("python", "go", ...)
	Code string // PartEdit: file content

	Start int        // PartPatch: line number of the first line of the hunk
	Hunk  []DiffLine // PartPatch: the lines (with a little context around the change)

	Note string // PartPatch: a line under the title, e.g. "lines 4-6 replaced"

	// PartTool: one tool call of the agent.
	Tool      string // tool name, e.g. "read_file"
	Server    string // MCP server it came from, if any
	Label     string // what it did:   "Read main.py"
	WorkLabel string // while running:  "Reading main.py…"
	Command   string // run_command: the full command, as written
	Explore   string // read_file / grep / glob / list_dir: "read", "search" or "list"
	Summary   string // Explored line, e.g.  Grep "workspace" in Backend (120 matches)
	Output    string // what it returned (shown when the row is open)
	Err       string // what went wrong, when it failed
	Failed    bool   // the call failed (or the command exited non-zero)
	Stopped   bool   // the turn ended before the call finished
	DurMs     int64  // how long the call took

	Working bool // still being made: the folded row shows a spinner

	// Only used on the FIRST part of a folded row: is the row open?
	Expanded bool
}

// lineHit marks a screen line as the clickable row of a folded group.
type lineHit struct {
	Part int // index of the group's first part, or -1
	W    int // width of the row's text (the clickable area)
}

var noHit = lineHit{Part: -1}

func noHits(n int) []lineHit {
	h := make([]lineHit, n)
	for i := range h {
		h[i] = noHit
	}
	return h
}

// Colors.
var (
	tlMuted   = lipgloss.Color("#808080") // folded row
	tlHover   = lipgloss.Color("#d4d4d4") // folded row under the mouse (brighter)
	tlPanelBg = lipgloss.Color("#141414") // background of an edit node
	tlAddBg   = lipgloss.Color("#1f3541") // added lines
	tlDelBg   = lipgloss.Color("#3b1f2b") // removed lines
)

// plural: (1, "edit") -> "1 edit", (2, "edit") -> "2 edits".
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// workingPart returns the unfinished part of a group, if any.
func workingPart(parts []Part) *Part {
	for i := range parts {
		if parts[i].Working {
			return &parts[i]
		}
	}
	return nil
}

// workingLabel is the text next to the spinner.
func workingLabel(p Part) string {
	switch p.Kind {
	case PartThought:
		return "Thinking…"
	case PartEdit:
		if p.File == "" {
			return "Writing…"
		}
		return "Writing " + p.File + "…"
	case PartPatch:
		if p.File == "" {
			return "Editing…"
		}
		return "Editing " + p.File + "…"
	case PartTool:
		if p.WorkLabel != "" {
			return p.WorkLabel
		}
		return "Working…"
	}
	return "Working…"
}

// groupSummary names a folded group: "1 edit, 2 thoughts".
func groupSummary(parts []Part) string {
	edits, tools, thoughts := 0, 0, 0
	for _, p := range parts {
		switch p.Kind {
		case PartEdit, PartPatch:
			edits++
		case PartTool:
			tools++
		case PartThought:
			thoughts++
		}
	}
	var items []string
	if edits > 0 {
		items = append(items, plural(edits, "edit"))
	}
	if tools > 0 {
		items = append(items, plural(tools, "tool call"))
	}
	if thoughts > 0 {
		items = append(items, plural(thoughts, "thought"))
	}
	return strings.Join(items, ", ")
}

// renderTimeline draws all parts, `width` columns wide. It returns the lines
// and, for every line, whether it is a clickable group row.
// hover is the first-part index of the group under the mouse (or -1).
func renderTimeline(parts []Part, width, hover int, streaming bool, spin string) ([]string, []lineHit) {
	pad := strings.Repeat(" ", bubbleIndent)
	textW := width - bubbleIndent - 1

	var lines []string
	var hits []lineHit
	add := func(l string, h lineHit) {
		lines = append(lines, l)
		hits = append(hits, h)
	}

	first := true
	for i := 0; i < len(parts); {
		if !first {
			add("", noHit) // blank row between blocks
		}
		first = false

		// Text block.
		if parts[i].Kind == PartText {
			var md []string
			if streaming && i == len(parts)-1 {
				md = streamMarkdownLines(parts[i].Text, textW) // still being typed
			} else {
				md = markdownLines(parts[i].Text, textW)
			}
			for _, l := range md {
				add(pad+l, noHit)
			}
			i++
			continue
		}

		// Run of read / grep / glob / list calls: its own "Explored" row.
		if isExplore(parts[i]) {
			j := i
			for j < len(parts) && isExplore(parts[j]) {
				j++
			}
			rows, w := exploreBlock(parts[i:j], width, hover == i, spin)
			add(rows[0], lineHit{Part: i, W: w})
			for _, l := range rows[1:] {
				add(l, noHit)
			}
			i = j
			continue
		}

		// Group of thoughts / edits, folded into one row.
		j := i
		for j < len(parts) && parts[j].Kind != PartText && !isExplore(parts[j]) {
			j++
		}
		group := parts[i:j]
		open := parts[i].Expanded

		// While a part of the group is still being made, the row shows the
		// spinner and what is happening; when it is done it becomes "+ 1 thought".
		icon := "+"
		if open {
			icon = "−"
		}
		label := icon + " " + groupSummary(group)
		tone := tlMuted
		if hover == i {
			tone = tlHover
		}
		rowStyle := lipgloss.NewStyle().Foreground(tone).Bold(true)
		var row string
		if w := workingPart(group); w != nil {
			// The spinner has its own colors, so it is joined, not nested.
			text := workingLabel(*w)
			label = "  " + text // same width as "⠋ " + text
			row = spin + " " + rowStyle.Render(text)
		} else {
			row = rowStyle.Render(label)
		}
		add(pad+row, lineHit{Part: i, W: lipgloss.Width(label)})

		if open {
			for _, node := range group {
				if node.Working {
					continue // not finished yet, nothing to show
				}
				add("", noHit)
				for _, l := range renderNode(node, width) {
					add(l, noHit)
				}
			}
		}
		i = j
	}
	return lines, hits
}

func renderNode(p Part, width int) []string {
	switch p.Kind {
	case PartThought:
		return thoughtNode(p, width)
	case PartEdit:
		return editNode(p, width)
	case PartPatch:
		return patchNode(p, width)
	case PartTool:
		return toolNode(p, width)
	}
	return nil
}

// thoughtNode: orange "Thought: 579ms" and the reasoning, all inside ONE
// unbroken bar on the left (the blank row between them has the bar too).
func thoughtNode(p Part, width int) []string {
	blockW := width - bubbleIndent - 2 - 1
	title := "Thought"
	if p.Ms > 0 {
		title = fmt.Sprintf("Thought: %s", formatMs(p.Ms))
	}
	body := []string{msgThoughtTitle.Render(title), ""}
	text := msgMuted.Render(wrapText(p.Text, blockW))
	body = append(body, strings.Split(text, "\n")...)
	return barBlock(body)
}

// editNode: a darker panel with "# Wrote main.py" and numbered code.
func editNode(p Part, width int) []string {
	base := lipgloss.NewStyle().Background(tlPanelBg)
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color("#3a3a3a")).Background(tlPanelBg).Render("▎")

	inner := width - 1 - 2 - 1 // bar, left padding, right padding
	if inner < 1 {
		inner = 1
	}
	row := func(content string) string {
		content = ansi.Truncate(content, inner, "")
		fill := inner - ansi.StringWidth(content)
		if fill < 0 {
			fill = 0
		}
		return bar + base.Render("  ") + content + base.Render(strings.Repeat(" ", fill+1))
	}

	total := countLines(p.Code)
	code := highlightCode(firstLines(p.Code, maxNodeLines), p.Lang)
	numW := len(strconv.Itoa(total))
	numStyle := base.Foreground(lipgloss.Color("#6b6b6b"))
	title := base.Foreground(lipgloss.Color("#8a8a8a")).Bold(true).Render("# Wrote " + p.File)

	lines := []string{row(""), row(title), row("")}
	for i, l := range code {
		num := numStyle.Render(fmt.Sprintf("%*d", numW, i+1))
		lines = append(lines, row(base.Render("  ")+num+base.Render(" ")+l))
	}
	if total > len(code) {
		more := base.Foreground(lipgloss.Color("#6b6b6b")).Render(fmt.Sprintf("  … %d more lines", total-len(code)))
		lines = append(lines, row(more))
	}
	return append(lines, row(""))
}

// --- syntax highlighting ----------------------------------------------------

// tokenStyle picks a color for a code token (all with the panel background).
func tokenStyle(t chroma.TokenType, bg color.Color) lipgloss.Style {
	st := lipgloss.NewStyle().Background(bg).Foreground(colorText)
	switch {
	case t.InCategory(chroma.Comment):
		return st.Foreground(lipgloss.Color("#6b6b6b")).Italic(true)
	case t.InSubCategory(chroma.LiteralString):
		return st.Foreground(lipgloss.Color("#85d68e")) // green
	case t.InSubCategory(chroma.LiteralNumber):
		return st.Foreground(lipgloss.Color("#f0a868")) // orange
	case t == chroma.OperatorWord, t.InCategory(chroma.Keyword):
		return st.Foreground(lipgloss.Color("#a78bfa")).Italic(true).Bold(true) // purple
	case t.InCategory(chroma.Operator):
		return st.Foreground(lipgloss.Color("#56c2d6")) // cyan
	case t == chroma.NameFunction, t == chroma.NameFunctionMagic,
		t == chroma.NameClass, t == chroma.NameDecorator:
		return st.Foreground(lipgloss.Color("#f0a868")) // orange
	case t == chroma.NameBuiltin, t == chroma.NameBuiltinPseudo:
		return st.Bold(true)
	case t == chroma.Name:
		return st.Foreground(lipgloss.Color("#e06c75")) // red: variables, parameters
	}
	return st
}

// highlightCode returns one colored string per line of code.
func highlightCode(code, lang string) []string {
	code = strings.ReplaceAll(strings.TrimRight(code, "\n"), "\t", "    ")

	plain := func() []string {
		st := lipgloss.NewStyle().Background(tlPanelBg).Foreground(colorText)
		lines := strings.Split(code, "\n")
		for i, l := range lines {
			lines[i] = st.Render(l)
		}
		return lines
	}

	lexer := lexers.Get(lang)
	if lexer == nil {
		return plain()
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		return plain()
	}

	var lines []string
	var cur strings.Builder
	for tok := it(); tok != chroma.EOF; tok = it() {
		st := tokenStyle(tok.Type, tlPanelBg)
		for k, part := range strings.Split(tok.Value, "\n") {
			if k > 0 {
				lines = append(lines, cur.String())
				cur.Reset()
			}
			if part != "" {
				cur.WriteString(st.Render(part))
			}
		}
	}
	lines = append(lines, cur.String())

	for len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] // the lexer adds a final newline
	}
	return lines
}

// newHighlighter returns a function that colors ONE line of code on the given
// background (used for diff lines, which have different backgrounds).
func newHighlighter(lang string) func(text string, bg color.Color) string {
	lexer := lexers.Get(lang)
	if lexer != nil {
		lexer = chroma.Coalesce(lexer)
	}
	return func(text string, bg color.Color) string {
		text = strings.ReplaceAll(text, "\t", "    ")
		base := lipgloss.NewStyle().Background(bg).Foreground(colorText)
		if lexer == nil || text == "" {
			return base.Render(text)
		}
		it, err := lexer.Tokenise(nil, text)
		if err != nil {
			return base.Render(text)
		}
		var b strings.Builder
		for tok := it(); tok != chroma.EOF; tok = it() {
			v := strings.ReplaceAll(tok.Value, "\n", "")
			if v != "" {
				b.WriteString(tokenStyle(tok.Type, bg).Render(v))
			}
		}
		return b.String()
	}
}

// patchNode: "← Edit main.py" and the changed lines:
//
//	11     if b == 0:          unchanged (new line numbers)
//	24 -   print("5. Exit")    removed  (old line number, red row)
//	24 +   print("5. Percent") added    (new line number, blue row)
func patchNode(p Part, width int) []string {
	base := lipgloss.NewStyle().Background(tlPanelBg)
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color("#3a3a3a")).Background(tlPanelBg).Render("▎")

	inner := width - 1 - 2 - 1 // bar, left padding, right padding
	if inner < 1 {
		inner = 1
	}

	// One row: panel background around, `bg` behind the content.
	row := func(bg color.Color, content string) string {
		st := lipgloss.NewStyle().Background(bg)
		content = ansi.Truncate(content, inner, "")
		fill := inner - ansi.StringWidth(content)
		if fill < 0 {
			fill = 0
		}
		return bar + base.Render("  ") + content + st.Render(strings.Repeat(" ", fill+1))
	}

	hunk, hidden := p.Hunk, 0
	if len(hunk) > maxNodeLines {
		hidden = len(hunk) - maxNodeLines
		hunk = hunk[:maxNodeLines]
	}

	// Work out the line numbers first, to know how wide the number column is.
	// Unchanged and added lines use the new numbering, removed lines the old.
	nums := make([]int, len(hunk))
	oldNo, newNo, maxNo := p.Start, p.Start, 0
	for i, d := range hunk {
		switch d.Op {
		case '-':
			nums[i] = oldNo
			oldNo++
		case '+':
			nums[i] = newNo
			newNo++
		default:
			nums[i] = newNo
			oldNo++
			newNo++
		}
		if nums[i] > maxNo {
			maxNo = nums[i]
		}
	}
	numW := len(strconv.Itoa(maxNo))

	hl := newHighlighter(p.Lang)
	title := base.Foreground(lipgloss.Color("#8a8a8a")).Bold(true).Render("← Edit " + p.File)

	lines := []string{row(tlPanelBg, ""), row(tlPanelBg, title)}
	if p.Note != "" {
		lines = append(lines, row(tlPanelBg, base.Foreground(lipgloss.Color("#6b6b6b")).Render(p.Note)))
	}
	lines = append(lines, row(tlPanelBg, ""))
	for i, d := range hunk {
		bg := tlPanelBg
		marker := " "
		st := lipgloss.NewStyle().Background(tlPanelBg)
		switch d.Op {
		case '+':
			bg = tlAddBg
			st = lipgloss.NewStyle().Background(bg)
			marker = st.Foreground(lipgloss.Color("#a3e635")).Bold(true).Render("+")
		case '-':
			bg = tlDelBg
			st = lipgloss.NewStyle().Background(bg)
			marker = st.Foreground(lipgloss.Color("#e06c75")).Bold(true).Render("-")
		default:
			marker = st.Render(" ")
		}

		num := st.Foreground(lipgloss.Color("#6b6b6b")).Render(fmt.Sprintf("%*d", numW, nums[i]))
		content := st.Render("  ") + num + st.Render(" ") + marker + st.Render(" ") + hl(d.Text, bg)
		lines = append(lines, row(bg, content))
	}
	if hidden > 0 {
		more := base.Foreground(lipgloss.Color("#6b6b6b")).Render(fmt.Sprintf("  … %d more lines", hidden))
		lines = append(lines, row(tlPanelBg, more))
	}
	return append(lines, row(tlPanelBg, ""))
}
