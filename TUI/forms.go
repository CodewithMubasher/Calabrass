package main

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"calabrass-tui/internal/api"
)

// dialogInput builds a text input styled for the dialog background.
func dialogInput(placeholder string, secret bool) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 500
	ti.Placeholder = placeholder
	if secret {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
	}
	text := lipgloss.NewStyle().Background(dialogBg).Foreground(colorText)
	state := textinput.StyleState{
		Text: text, Placeholder: text.Foreground(dialogMuted),
		Suggestion: text.Foreground(dialogMuted), Prompt: text,
	}
	ti.SetStyles(textinput.Styles{Focused: state, Blurred: state,
		Cursor: textinput.CursorStyle{Color: dialogBlue, Shape: tea.CursorBlock, Blink: true}})
	return ti
}

// boxLines frames dialog content (lines are already styled) in the dialog box.
func boxLines(w int, lines []string) string {
	base := lipgloss.NewStyle().Background(dialogBg)
	inner := w - 2*dialogPadX
	blank := base.Render(strings.Repeat(" ", w))
	out := []string{blank}
	for _, c := range lines {
		c = ansi.Truncate(c, inner, "")
		gap := inner - lipgloss.Width(c)
		if gap < 0 {
			gap = 0
		}
		out = append(out, base.Render(strings.Repeat(" ", dialogPadX))+c+base.Render(strings.Repeat(" ", gap+dialogPadX)))
	}
	return strings.Join(append(out, blank), "\n")
}

func centerOverlay(bg, box string, sw, sh int) string {
	x, y := (sw-lipgloss.Width(box))/2, (sh-lipgloss.Height(box))/2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return overlay(bg, box, x, y)
}

func formWidth(sw int) int {
	w := dialogMaxWidth
	if sw-4 < w {
		w = sw - 4
	}
	if w < 24 {
		w = 24
	}
	return w
}

// --- provider form ------------------------------------------------------------

// ProviderSaveMsg asks the app to save the form.
type ProviderSaveMsg struct {
	ID     string // empty: create
	Update api.ProviderUpdate
}

// ProviderFormCancelMsg is sent when the form is closed without saving.
type ProviderFormCancelMsg struct{}

// ProviderForm edits one provider's URL, key and on/off switch.
type ProviderForm struct {
	open     bool
	create   bool
	row      api.ProviderRow
	name     textinput.Model
	baseURL  textinput.Model
	apiKey   textinput.Model
	enabled  bool
	field    int // 0 name, 1 url, 2 key, 3 enabled
	Err      string
	Saving   bool
	sw, sh   int
}

func NewProviderForm() ProviderForm {
	return ProviderForm{name: dialogInput("Name", false), baseURL: dialogInput("https://api.example.com/v1", false), apiKey: dialogInput("API key", true)}
}

func (f ProviderForm) IsOpen() bool { return f.open }

func (f *ProviderForm) SetSize(w, h int) { f.sw, f.sh = w, h }

// Open shows the form for an existing provider, or a new one when create is set.
func (f *ProviderForm) Open(row api.ProviderRow, create bool) tea.Cmd {
	f.open, f.create, f.row, f.Err, f.Saving = true, create, row, "", false
	f.name.SetValue(row.Name)
	f.baseURL.SetValue(row.BaseURL)
	f.apiKey.SetValue(row.APIKey)
	f.enabled = row.Enabled || (!create && strings.TrimSpace(row.APIKey) == "")
	if create {
		f.enabled = true
	}
	f.field = 1
	if create {
		f.field = 0
	}
	return f.focus()
}

func (f *ProviderForm) Close() { f.open = false }

func (f *ProviderForm) focus() tea.Cmd {
	f.name.Blur()
	f.baseURL.Blur()
	f.apiKey.Blur()
	switch f.field {
	case 0:
		return f.name.Focus()
	case 1:
		return f.baseURL.Focus()
	case 2:
		return f.apiKey.Focus()
	}
	return nil
}

func (f *ProviderForm) move(d int) tea.Cmd {
	first := 1
	if f.create {
		first = 0
	}
	f.field += d
	if f.field > 3 {
		f.field = first
	}
	if f.field < first {
		f.field = 3
	}
	return f.focus()
}

func (f ProviderForm) save() tea.Msg {
	u := api.ProviderUpdate{}
	en := f.enabled
	u.Enabled = &en
	if f.create {
		n, b, k := strings.TrimSpace(f.name.Value()), strings.TrimSpace(f.baseURL.Value()), strings.TrimSpace(f.apiKey.Value())
		u.Name, u.BaseURL, u.APIKey = &n, &b, &k
		return ProviderSaveMsg{Update: u}
	}
	if b := strings.TrimSpace(f.baseURL.Value()); b != f.row.BaseURL {
		u.BaseURL = &b
	}
	if k := strings.TrimSpace(f.apiKey.Value()); k != f.row.APIKey {
		u.APIKey = &k
	}
	return ProviderSaveMsg{ID: f.row.ID, Update: u}
}

func (f ProviderForm) Update(msg tea.Msg) (ProviderForm, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return f.pass(msg)
	}
	if f.Saving {
		return f, nil
	}
	switch key.String() {
	case "esc":
		f.Close()
		return f, func() tea.Msg { return ProviderFormCancelMsg{} }
	case "ctrl+s":
		f.Saving = true
		return f, func() tea.Msg { return f.save() }
	case "tab", "down":
		return f, f.move(1)
	case "shift+tab", "up":
		return f, f.move(-1)
	case "enter":
		if f.field == 3 {
			f.Saving = true
			return f, func() tea.Msg { return f.save() }
		}
		return f, f.move(1)
	case "space", "left", "right":
		if f.field == 3 {
			f.enabled = !f.enabled
			return f, nil
		}
	}
	return f.pass(msg)
}

func (f ProviderForm) pass(msg tea.Msg) (ProviderForm, tea.Cmd) {
	var cmd tea.Cmd
	switch f.field {
	case 0:
		f.name, cmd = f.name.Update(msg)
	case 1:
		f.baseURL, cmd = f.baseURL.Update(msg)
	case 2:
		f.apiKey, cmd = f.apiKey.Update(msg)
	}
	return f, cmd
}

func (f ProviderForm) View() string {
	w := formWidth(f.sw)
	inner := w - 2*dialogPadX
	for _, in := range []*textinput.Model{&f.name, &f.baseURL, &f.apiKey} {
		in.SetWidth(inner - 1)
	}
	base := lipgloss.NewStyle().Background(dialogBg)
	label := func(s string, on bool) string {
		st := base.Foreground(dialogMuted)
		if on {
			st = base.Foreground(dialogBlue).Bold(true)
		}
		return st.Render(s)
	}
	title := "Configure " + f.row.Name
	if f.create {
		title = "Add provider"
	}
	lines := []string{twoCol(base.Foreground(colorText).Bold(true).Render(title), base.Foreground(dialogMuted).Render("esc"), inner, base), ""}
	if f.create {
		lines = append(lines, label("Name", f.field == 0), f.name.View(), "")
	}
	toggle := "[ ] Enabled"
	if f.enabled {
		toggle = "[x] Enabled"
	}
	lines = append(lines,
		label("Base URL", f.field == 1), f.baseURL.View(), "",
		label("API key", f.field == 2), f.apiKey.View(), "",
		label(toggle, f.field == 3), "")
	switch {
	case f.Saving:
		lines = append(lines, base.Foreground(dialogMuted).Render("Saving…"))
	case f.Err != "":
		lines = append(lines, base.Foreground(lipgloss.Color("#e06c75")).Render(f.Err))
	default:
		lines = append(lines, base.Foreground(dialogMuted).Render("tab next · space toggle · ctrl+s save"))
	}
	return boxLines(w, lines)
}

func (f ProviderForm) Overlay(bg string, sw, sh int) string { return centerOverlay(bg, f.View(), sw, sh) }

// --- questions form -----------------------------------------------------------

// QuestionsAnsweredMsg carries the finished answers, ready to send.
type QuestionsAnsweredMsg struct{ Text string }

// QuestionForm walks through the questions of an ask_questions call.
type QuestionForm struct {
	open    bool
	qs      []askQuestion
	idx     int
	answers []string
	picked  map[int]bool
	cursor  int
	typing  bool // typing a free-text answer (no options, or "Other")
	input   textinput.Model
	sw, sh  int
}

func NewQuestionForm() QuestionForm { return QuestionForm{input: dialogInput("Type your answer", false)} }

func (q QuestionForm) IsOpen() bool { return q.open }

func (q *QuestionForm) SetSize(w, h int) { q.sw, q.sh = w, h }

func (q *QuestionForm) Open(qs []askQuestion) tea.Cmd {
	q.open, q.qs, q.idx = true, qs, 0
	q.answers = make([]string, len(qs))
	return q.enter()
}

func (q *QuestionForm) Close() { q.open = false }

func (q *QuestionForm) enter() tea.Cmd {
	q.picked, q.cursor = map[int]bool{}, 0
	q.input.SetValue(q.answers[q.idx])
	q.typing = len(q.qs[q.idx].Options) == 0
	if q.typing {
		return q.input.Focus()
	}
	q.input.Blur()
	return nil
}

func (q *QuestionForm) next(answer string) tea.Cmd {
	q.answers[q.idx] = answer
	if q.idx+1 >= len(q.qs) {
		q.Close()
		text := formatAnswers(q.qs, q.answers)
		return func() tea.Msg { return QuestionsAnsweredMsg{Text: text} }
	}
	q.idx++
	return q.enter()
}

func (q QuestionForm) Update(msg tea.Msg) (QuestionForm, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		q.input, cmd = q.input.Update(msg)
		return q, cmd
	}
	cur := q.qs[q.idx]
	switch key.String() {
	case "esc":
		q.Close()
		return q, nil
	case "shift+tab":
		if q.idx > 0 {
			q.idx--
			return q, q.enter()
		}
		return q, nil
	}
	if q.typing {
		if key.String() == "enter" {
			c := q.next(strings.TrimSpace(q.input.Value()))
			return q, c
		}
		var cmd tea.Cmd
		q.input, cmd = q.input.Update(msg)
		return q, cmd
	}
	n := len(cur.Options) + 1 // + "Other…"
	switch key.String() {
	case "up":
		q.cursor = (q.cursor - 1 + n) % n
	case "down":
		q.cursor = (q.cursor + 1) % n
	case "space":
		if cur.Multiple && q.cursor < len(cur.Options) {
			q.picked[q.cursor] = !q.picked[q.cursor]
		}
	case "enter":
		if q.cursor == len(cur.Options) { // Other…
			q.typing = true
			return q, q.input.Focus()
		}
		if cur.Multiple {
			var sel []string
			for i, o := range cur.Options {
				if q.picked[i] {
					sel = append(sel, o)
				}
			}
			if len(sel) == 0 {
				sel = []string{cur.Options[q.cursor]}
			}
			return q, q.next(strings.Join(sel, ", "))
		}
		return q, q.next(cur.Options[q.cursor])
	}
	return q, nil
}

func (q QuestionForm) View() string {
	w := formWidth(q.sw)
	inner := w - 2*dialogPadX
	q.input.SetWidth(inner - 1)
	base := lipgloss.NewStyle().Background(dialogBg)
	cur := q.qs[q.idx]
	head := twoCol(base.Foreground(colorText).Bold(true).Render("Question "+strconv.Itoa(q.idx+1)+" of "+strconv.Itoa(len(q.qs))),
		base.Foreground(dialogMuted).Render("esc"), inner, base)
	lines := []string{head, ""}
	for _, l := range strings.Split(wrapText(cur.Question, inner), "\n") {
		lines = append(lines, base.Foreground(colorText).Render(l))
	}
	lines = append(lines, "")
	if q.typing {
		lines = append(lines, q.input.View(), "", base.Foreground(dialogMuted).Render("enter confirm · shift+tab back"))
		return boxLines(w, lines)
	}
	rows := append(append([]string{}, cur.Options...), "Other…")
	for i, o := range rows {
		mark := "  "
		if cur.Multiple && i < len(cur.Options) {
			mark = "[ ] "
			if q.picked[i] {
				mark = "[x] "
			}
		}
		if i == q.cursor {
			sel := lipgloss.NewStyle().Background(dialogBlue).Foreground(dialogDark)
			lines = append(lines, sel.Render(ansi.Truncate(" "+mark+o+strings.Repeat(" ", inner), inner, "")))
		} else {
			lines = append(lines, base.Foreground(colorText).Render(" "+mark+o))
		}
	}
	hint := "↑↓ move · enter choose"
	if cur.Multiple {
		hint = "↑↓ move · space toggle · enter confirm"
	}
	lines = append(lines, "", base.Foreground(dialogMuted).Render(hint))
	return boxLines(w, lines)
}

func (q QuestionForm) Overlay(bg string, sw, sh int) string { return centerOverlay(bg, q.View(), sw, sh) }

