package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// writeClipboard is a variable so tests can replace the system clipboard.
var writeClipboard = clipboard.WriteAll

// copyCmd writes text to the system clipboard. It sends both an OSC52 escape
// (works over SSH and in most modern terminals) and the native clipboard, so
// at least one of them lands everywhere.
func copyCmd(text string) tea.Cmd {
	return tea.Batch(
		tea.SetClipboard(text),
		func() tea.Msg {
			_ = writeClipboard(text)
			return nil
		},
	)
}

// lastResponseText returns the copyable text of the newest assistant message.
// Prose wins; edit / diff nodes are only used when the answer has no prose.
func lastResponseText(msgs []Message) (string, bool) {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != RoleAssistant {
			continue
		}
		return assistantCopyText(msgs[i]), true
	}
	return "", false
}

func assistantCopyText(m Message) string {
	var prose []string
	for _, p := range m.Parts {
		if p.Kind == PartText && strings.TrimSpace(p.Text) != "" {
			prose = append(prose, strings.TrimSpace(p.Text))
		}
	}
	if len(prose) > 0 {
		return strings.Join(prose, "\n\n")
	}

	// A turn that only changed files still has something useful to copy.
	var blocks []string
	for _, p := range m.Parts {
		switch p.Kind {
		case PartEdit:
			if strings.TrimSpace(p.Code) == "" {
				continue
			}
			lang := strings.ToLower(strings.TrimSpace(p.Lang))
			blocks = append(blocks, fmt.Sprintf("```%s\n%s\n```", lang, strings.TrimRight(p.Code, "\n")))
		case PartPatch:
			if len(p.Hunk) == 0 {
				continue
			}
			var b strings.Builder
			fmt.Fprintf(&b, "```diff\n")
			for _, d := range p.Hunk {
				fmt.Fprintf(&b, "%c%s\n", d.Op, d.Text)
			}
			b.WriteString("```")
			blocks = append(blocks, b.String())
		}
	}
	return strings.Join(blocks, "\n\n")
}

// copyLastResponse copies the newest assistant answer and tells the user.
func (m *Model) copyLastResponse() tea.Cmd {
	text, ok := lastResponseText(m.messages)
	if !ok {
		return m.showToast("Nothing to copy yet: send a message first.", true)
	}
	if strings.TrimSpace(text) == "" {
		return m.showToast("The last response has no text to copy.", true)
	}
	toast := m.showToast("Copied the last response.", false)
	return tea.Batch(toast, copyCmd(text))
}
