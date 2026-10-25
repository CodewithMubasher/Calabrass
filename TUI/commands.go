package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// runCommand runs a slash command. args is what was typed after its name.
func (m *Model) runCommand(name, args string) tea.Cmd {
	c := m.app.client
	switch name {
	case "/exit":
		return tea.Quit

	case "/new":
		m.newSession()

	case "/help":
		return m.showToast("Press ctrl+p to view all commands", false)

	case "/copy":
		return m.copyLastResponse()

	case "/models":
		cmd := m.list.Show("models", "Select model", m.modelItems())
		m.list.Empty = "No models yet. Close this and type /connect to add a provider key."
		return cmd

	case "/connect":
		return loadProvidersCmd(c, true)

	case "/mcps":
		cmd := m.list.Show("mcps", "MCP servers", m.mcpItems())
		m.list.KeepOpen = true
		m.list.Empty = "No MCP servers are registered with the backend."
		m.list.HintLeft = "enter turn on/off"
		return tea.Batch(cmd, loadConnectorsCmd(c))

	case "/continue":
		return loadChatsCmd(c, false)

	case "/cd":
		path, err := resolveDir(m.cwdPath, args)
		if err != nil {
			m.notice(err.Error(), true)
			return nil
		}
		id := ""
		if m.chat != nil {
			id = m.chat.ID
		}
		return setWorkspaceCmd(c, id, path)

	case "/rename":
		if m.chat == nil {
			m.notice("Send a message first; there is no session to rename yet.", true)
			return nil
		}
		if strings.TrimSpace(args) == "" {
			m.notice("usage: /rename <new title>", true)
			return nil
		}
		return renameChatCmd(c, m.chat.ID, strings.TrimSpace(args))

	case "/retry":
		return m.retry()

	case "/answer":
		qs := pendingQuestions(m.messages)
		if len(qs) == 0 {
			m.notice("The agent has no open questions.", false)
			return nil
		}
		return m.qform.Open(qs)

	case "/export":
		path, err := m.exportSession(args)
		if err != nil {
			m.notice("Export failed: "+err.Error(), true)
		} else {
			m.notice("Saved "+path, false)
		}
	case "/schedules":
		return loadSchedulesCmd(c)
	}
	return nil
}

// retry answers the last user message again, replacing the last reply.
func (m *Model) retry() tea.Cmd {
	if m.running() {
		return nil
	}
	if m.chat == nil {
		m.notice("Nothing to retry yet.", true)
		return nil
	}
	if m.selModel == "" {
		m.notice("No model is available. Type /connect to add a provider API key.", true)
		return nil
	}
	last := -1
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].Role == RoleUser {
			last = i
			break
		}
	}
	if last < 0 {
		m.notice("Nothing to retry yet.", true)
		return nil
	}
	text := m.messages[last].Text
	// Drop the reply (and any notes after it); the backend drops its copy.
	m.messages = m.messages[:last+1]
	return m.startTurn(text, true)
}

// exportSession writes the session to a Markdown file and returns its path.
func (m *Model) exportSession(arg string) (string, error) {
	name := strings.TrimSpace(arg)
	if name == "" {
		title := "session"
		if m.chat != nil && strings.TrimSpace(m.chat.Title) != "" {
			title = m.chat.Title
		}
		name = "calabrass-" + slug(title) + "-" + time.Now().Format("20060102-1504") + ".md"
	}
	if !filepath.IsAbs(name) {
		name = filepath.Join(m.cwdPath, name)
	}
	if err := os.WriteFile(name, []byte(sessionMarkdown(m.messages)), 0o644); err != nil {
		return "", err
	}
	return name, nil
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
		if b.Len() >= 40 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "session"
	}
	return out
}

// sessionMarkdown renders the conversation as Markdown.
func sessionMarkdown(msgs []Message) string {
	var b strings.Builder
	b.WriteString("# Calabrass session\n")
	for _, msg := range msgs {
		switch msg.Role {
		case RoleUser:
			b.WriteString("\n## You\n\n" + msg.Text + "\n")
		case RoleAssistant:
			b.WriteString("\n## Calabrass\n")
			for _, p := range msg.Parts {
				switch p.Kind {
				case PartText:
					b.WriteString("\n" + strings.TrimSpace(p.Text) + "\n")
				case PartEdit:
					b.WriteString(fmt.Sprintf("\n**Wrote `%s`**\n\n```%s\n%s\n```\n", p.File, strings.ToLower(p.Lang), strings.TrimRight(p.Code, "\n")))
				case PartPatch:
					b.WriteString(fmt.Sprintf("\n**Edited `%s`** (line %d)", p.File, p.Start))
					if p.Note != "" {
						b.WriteString(" — " + p.Note)
					}
					b.WriteString("\n\n```diff\n")
					for _, d := range p.Hunk {
						b.WriteString(string(d.Op) + d.Text + "\n")
					}
					b.WriteString("```\n")
				case PartTool:
					mark := "-"
					if p.Failed {
						mark = "- (failed)"
					}
					b.WriteString("\n" + mark + " " + p.Label + "\n")
				}
			}
			if msg.Err != "" {
				b.WriteString("\n> Error: " + msg.Err + "\n")
			}
		}
	}
	return b.String()
}
