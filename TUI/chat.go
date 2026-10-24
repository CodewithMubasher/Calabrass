package main

import (
    "fmt"
    "strings"
    "time"
    "unicode"

    tea "github.com/charmbracelet/bubbletea"
)

// --- fake model timing ------------------------------------------------------

const (
    fakeReplyDelay = 800 * time.Millisecond // "Thinking…" before the first part
    streamInterval = 30 * time.Millisecond  // between two steps of typing
    streamChunk    = 10                     // about this many characters per step
    nodeDelay      = 450 * time.Millisecond // pause before a thought / edit appears
    textGap        = 250 * time.Millisecond // pause before the next part after text
)

// fakeReplyMsg is sent when the fake model has finished "thinking" and
// starts answering.
type fakeReplyMsg struct {
    ID int
}

// streamTickMsg reveals the next step of an answer that is being typed.
type streamTickMsg struct {
    ID int
}

func streamTick(id int, d time.Duration) tea.Cmd {
    return tea.Tick(d, func(time.Time) tea.Msg {
        return streamTickMsg{ID: id}
    })
}

// --- fake answer ------------------------------------------------------------

// A "§" stands for a backtick (Go raw strings can't contain backticks).
const fakeSummaryRaw = `I've created a simple Python calculator in §main.py§! Here's what it includes:

### Features

- Basic operations: Add, Subtract, Multiply, Divide
- Interactive menu system
- Error handling for invalid inputs
- Division by zero protection
- Option to exit

### How to use

1. Run the file: §python main.py§
2. Choose an operation (1-4), or 5 to exit
3. Enter two numbers
4. See the result

Want me to add more, like exponents or a history of results?`

const fakePercentRaw = `I've added percentage support to §main.py§.

### What changed

- New §percentage(a, b)§ function: what percent §a§ is of §b§
- Menu option **5. Percentage**, and **Exit** moved to **6**
- The choice check and the exit check use the new numbers

### How to try it

1. Run the file: §python main.py§
2. Choose §5§ for percentage
3. Enter §25§ and §200§, and you get §12.5§`

// fakePercentEdits are the 4 edits of the second demo, as diff hunks.
// Line numbers follow the calculator written by the first demo.
func fakePercentEdits() []Part {
    const lang = "python"
    return []Part{
        // 1. new function
        {Kind: PartPatch, File: "main.py", Lang: lang, Start: 11, Hunk: []DiffLine{
            ctxL("    if b == 0:"),
            ctxL(`        return "Error: Cannot divide by zero"`),
            ctxL("    return a / b"),
            ctxL(""),
            addL("def percentage(a, b):"),
            addL("    return (a / b) * 100"),
            addL(""),
            ctxL("def calculator():"),
            ctxL(`    print("Simple Calculator")`),
            ctxL(`    print("1. Add")`),
            ctxL(`    print("2. Subtract")`),
        }},
        // 2. menu
        {Kind: PartPatch, File: "main.py", Lang: lang, Start: 20, Hunk: []DiffLine{
            ctxL(`    print("1. Add")`),
            ctxL(`    print("2. Subtract")`),
            ctxL(`    print("3. Multiply")`),
            ctxL(`    print("4. Divide")`),
            delL(`    print("5. Exit")`),
            addL(`    print("5. Percentage")`),
            addL(`    print("6. Exit")`),
            ctxL(""),
            ctxL("    while True:"),
        }},
        // 3. prompt + exit check + valid choices
        {Kind: PartPatch, File: "main.py", Lang: lang, Start: 27, Hunk: []DiffLine{
            ctxL("    while True:"),
            delL(`        choice = input("Choose an operation (1-5): ")`),
            addL(`        choice = input("Choose an operation (1-6): ")`),
            ctxL(""),
            delL(`        if choice == "5":`),
            addL(`        if choice == "6":`),
            ctxL(`            print("Goodbye!")`),
            ctxL("            break"),
            ctxL(""),
            delL(`        if choice not in ("1", "2", "3", "4"):`),
            addL(`        if choice not in ("1", "2", "3", "4", "5"):`),
            ctxL(`            print("Invalid choice, try again.")`),
            ctxL("            continue"),
        }},
        // 4. the new branch
        {Kind: PartPatch, File: "main.py", Lang: lang, Start: 49, Hunk: []DiffLine{
            ctxL(`        elif choice == "3":`),
            ctxL(`            print("Result:", multiply(a, b))`),
            delL("        else:"),
            addL(`        elif choice == "4":`),
            ctxL(`            print("Result:", divide(a, b))`),
            addL("        else:"),
            addL(`            print("Result:", percentage(a, b))`),
        }},
    }
}

const fakeCalcCode = `def add(a, b):
    return a + b

def subtract(a, b):
    return a - b

def multiply(a, b):
    return a * b

def divide(a, b):
    if b == 0:
        return "Error: Cannot divide by zero"
    return a / b

def calculator():
    print("Simple Calculator")
    print("1. Add")
    print("2. Subtract")
    print("3. Multiply")
    print("4. Divide")
    print("5. Exit")

    while True:
        choice = input("Choose an operation (1-5): ")

        if choice == "5":
            print("Goodbye!")
            break

        if choice not in ("1", "2", "3", "4"):
            print("Invalid choice, try again.")
            continue

        try:
            a = float(input("First number: "))
            b = float(input("Second number: "))
        except ValueError:
            print("Please enter valid numbers.")
            continue

        if choice == "1":
            print("Result:", add(a, b))
        elif choice == "2":
            print("Result:", subtract(a, b))
        elif choice == "3":
            print("Result:", multiply(a, b))
        else:
            print("Result:", divide(a, b))

if __name__ == "__main__":
    calculator()
`

// fakeScript is the answer the fake model "types", part by part.
// Replace this with a real backend that sends the same kinds of parts.
func fakeScript(query string) []Part {
    q := strings.TrimSpace(query)

    switch strings.ToLower(q) {
    case "hi", "hello", "hey", "hi!", "hello!":
        return []Part{
            {Kind: PartThought, Ms: 579, Text: fmt.Sprintf(`The user said "%s" — this is a simple greeting. I should respond politely and ask how I can help them. There's no need to use any tools for this — just a simple text response.`, q)},
            {Kind: PartText, Text: "Hello! How can I help you today?"},
        }
    }

    // A request to edit: show 4 diffs of main.py (old - / new +).
    lower := strings.ToLower(q)
    if strings.Contains(lower, "edit") || strings.Contains(lower, "%") || strings.Contains(lower, "percent") {
        parts := []Part{
            {Kind: PartText, Text: "I'll add support for percentage calculation to the calculator."},
        }
        parts = append(parts, fakePercentEdits()...)
        return append(parts, Part{Kind: PartText, Text: strings.ReplaceAll(fakePercentRaw, "§", "`")})
    }

    shown := q
    if len([]rune(shown)) > 60 {
        shown = string([]rune(shown)[:60]) + "…"
    }
    return []Part{
        {Kind: PartThought, Ms: 612, Text: fmt.Sprintf(`The user wrote "%s". I'll keep the answer small: one file with the four basic operations, a menu loop, and a check for division by zero.`, shown)},
        {Kind: PartText, Text: "I'll create a simple Python calculator for you in a file called main.py."},
        {Kind: PartEdit, File: "main.py", Lang: "python", Code: fakeCalcCode},
        {Kind: PartThought, Ms: 289, Text: "The file is written. Now I should summarize what it contains and how to run it."},
        {Kind: PartText, Text: strings.ReplaceAll(fakeSummaryRaw, "§", "`")},
    }
}

// send adds the user message and a "thinking" assistant message, and starts
// the timer for the fake answer.
func (m *Model) send(text string) tea.Cmd {
    accent, label := m.prompt.ModeStyle()

    m.nextID++
    m.messages = append(m.messages, Message{
        ID: m.nextID, Role: RoleUser, Text: text, Accent: accent,
    })

    m.nextID++
    id := m.nextID
    m.messages = append(m.messages, Message{
        ID: id, Role: RoleAssistant, Query: text, Pending: true,
        Started: time.Now(), Label: label, Accent: accent, Model: m.prompt.Model,
    })

    m.scroll = 0 // jump to the newest message
    return tea.Tick(fakeReplyDelay, func(time.Time) tea.Msg {
        return fakeReplyMsg{ID: id}
    })
}

// finishReply ends the "thinking" phase and starts the answer.
func (m *Model) finishReply(id int) tea.Cmd {
    for i := range m.messages {
        if m.messages[i].ID != id {
            continue
        }
        msg := &m.messages[i]
        msg.Script = fakeScript(msg.Query)
        msg.ScriptIdx = 0
        msg.Pos = 0
        msg.Parts = nil
        msg.Pending = false
        msg.Streaming = true
        return streamTick(id, 100*time.Millisecond)
    }
    return nil
}

// streamStep shows the next bit of the answer: a whole thought / edit node
// at once, or a few more characters of a text. It returns the next tick, or
// nil when the answer is complete.
func (m *Model) streamStep(id int) tea.Cmd {
    for i := range m.messages {
        msg := &m.messages[i]
        if msg.ID != id || !msg.Streaming {
            continue
        }

        // Called when the current part is complete.
        next := func(delay time.Duration) tea.Cmd {
            if msg.ScriptIdx >= len(msg.Script) {
                msg.Streaming = false
                msg.Elapsed = time.Since(msg.Started)
                return nil
            }
            return streamTick(id, delay)
        }

        if msg.ScriptIdx >= len(msg.Script) {
            return next(0)
        }
        cur := msg.Script[msg.ScriptIdx]

        // Thought or edit: appears all at once.
        if cur.Kind != PartText {
            msg.Parts = append(msg.Parts, cur)
            msg.ScriptIdx++
            return next(nodeDelay)
        }

        // Text: type a few more characters, always finishing the word.
        if msg.Pos == 0 {
            msg.Parts = append(msg.Parts, Part{Kind: PartText})
        }
        r := []rune(cur.Text)
        end := msg.Pos + streamChunk + (msg.Pos/13)%8
        for end < len(r) && !unicode.IsSpace(r[end]) {
            end++
        }
        if end > len(r) {
            end = len(r)
        }
        msg.Pos = end
        msg.Parts[len(msg.Parts)-1].Text = string(r[:end])

        if end >= len(r) {
            msg.ScriptIdx++
            msg.Pos = 0
            return next(textGap)
        }
        return streamTick(id, streamInterval)
    }
    return nil
}

// --- layout -----------------------------------------------------------------

// chatHeight is the number of rows above the prompt-input
// (the footer row under the prompt takes 1 more row).
func (m Model) chatHeight() int {
    h := m.height - m.prompt.Height() - 1
    if h < 1 {
        h = 1
    }
    return h
}

// chatHit says which timeline row (if any) is on a chat line.
type chatHit struct {
    msg  int // message index, -1 if none
    part int // first-part index of the group
    w    int // width of the row text
}

// chatLines renders every message. hits[i] describes line i.
func (m Model) chatLines() (lines []string, hits []chatHit) {
    w := m.centerWidth()
    for i, msg := range m.messages {
        hover := -1
        if i == m.hoverMsg {
            hover = m.hoverPart
        }
        ml, mh := msg.Render(w, hover)
        for j, l := range ml {
            lines = append(lines, l)
            if mh[j].Part >= 0 {
                hits = append(hits, chatHit{msg: i, part: mh[j].Part, w: mh[j].W})
            } else {
                hits = append(hits, chatHit{msg: -1, part: -1})
            }
        }
    }
    return lines, hits
}

// visibleRange picks which lines are on screen. scroll is how many lines the
// user has scrolled up from the bottom.
func visibleRange(n, h, scroll int) (start, end int) {
    if n <= h {
        return 0, n
    }
    if scroll > n-h {
        scroll = n - h
    }
    if scroll < 0 {
        scroll = 0
    }
    end = n - scroll
    return end - h, end
}

func (m Model) maxScroll() int {
    lines, _ := m.chatLines()
    n := len(lines) - m.chatHeight()
    if n < 0 {
        return 0
    }
    return n
}

func (m *Model) scrollBy(delta int) {
    m.scroll += delta
    if max := m.maxScroll(); m.scroll > max {
        m.scroll = max
    }
    if m.scroll < 0 {
        m.scroll = 0
    }
}

// hitAt finds the timeline row under screen position (x, y), if any.
func (m Model) hitAt(x, y int) (chatHit, bool) {
    if m.screen != screenChat {
        return chatHit{}, false
    }
    chatH := m.chatHeight()
    if y < 0 || y >= chatH {
        return chatHit{}, false
    }

    cx := x - marginLeft // column inside the center column
    if cx < bubbleIndent {
        return chatHit{}, false
    }

    lines, hits := m.chatLines()
    start, _ := visibleRange(len(lines), chatH, m.scroll)
    idx := start + y
    if idx < 0 || idx >= len(hits) {
        return chatHit{}, false
    }

    h := hits[idx]
    if h.msg < 0 || cx >= bubbleIndent+h.w {
        return chatHit{}, false
    }
    return h, true
}

// handleClick opens or closes a timeline row.
func (m *Model) handleClick(x, y int) {
    if h, ok := m.hitAt(x, y); ok {
        p := &m.messages[h.msg].Parts[h.part]
        p.Expanded = !p.Expanded
    }
}

// handleHover remembers which timeline row the mouse is over.
func (m *Model) handleHover(x, y int) {
    if h, ok := m.hitAt(x, y); ok {
        m.hoverMsg, m.hoverPart = h.msg, h.part
        return
    }
    m.hoverMsg, m.hoverPart = -1, -1
}
