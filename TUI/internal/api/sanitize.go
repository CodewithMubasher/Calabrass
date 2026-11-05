package api

import "calabrass-tui/internal/termsafe"

// Everything a server sends that a model or a tool can write ends up on the
// person's screen. These clean it where it comes in, so no code further on has to
// remember to.

func cleanChat(c *Chat) {
	c.Title = termsafe.Line(c.Title)
	c.WorkspaceDir = termsafe.Line(c.WorkspaceDir)
}

func cleanMessage(m *Message) {
	m.Content = termsafe.Clean(m.Content)
	m.Reasoning = termsafe.Clean(m.Reasoning)
}

func cleanTrace(t *Trace) {
	for i := range t.Steps {
		parts := t.Steps[i].Parts
		for j := range parts {
			p := &parts[j]
			p.Text = termsafe.Clean(p.Text)
			p.Input = termsafe.Clean(p.Input)
			p.Output = termsafe.Clean(p.Output)
			p.Error = termsafe.Clean(p.Error)
			p.Name = termsafe.Line(p.Name)
			p.Server = termsafe.Line(p.Server)
		}
	}
}

func cleanApproval(a *Approval) {
	a.Summary = termsafe.Clean(a.Summary)
	a.Detail = termsafe.Clean(a.Detail)
	a.Pattern = termsafe.Line(a.Pattern)
	a.TierName = termsafe.Line(a.TierName)
	a.Tool = termsafe.Line(a.Tool)
}

func cleanTask(t *ScheduledTask) {
	t.Prompt = termsafe.Clean(t.Prompt)
	t.LastError = termsafe.Clip(t.LastError, 300)
}
