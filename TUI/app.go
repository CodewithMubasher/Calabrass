package main

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"calabrass-tui/internal/api"
	"calabrass-tui/internal/session"
	"calabrass-tui/internal/settings"
)

// appCtx is what every copy of the Model shares: the backend client and the
// saved settings.
type appCtx struct {
	client *api.Client
	st     *settings.Settings
	resume bool // -c: reopen the latest session of this folder
}

// --- messages from background work ------------------------------------------

type bootDoneMsg struct{ err error }
type modelsMsg struct {
	providers []api.ProviderInfo
	err       error
}
type connectorsMsg struct {
	list []api.Connector
	err  error
}
type chatsMsg struct {
	chats  []api.Chat
	err    error
	resume bool // pick the latest one for this folder instead of showing the list
}
type chatLoadedMsg struct {
	detail api.ChatDetail
	err    error
}
type providersMsg struct {
	rows []api.ProviderRow
	err  error
	open bool
}
type providerSavedMsg struct {
	rows    []api.ProviderRow
	err     error
	created bool // a new provider was added (not an edit)
}
type chatDeletedMsg struct {
	id  string
	err error
}
type renamedMsg struct {
	chat api.Chat
	err  error
}
type cdDoneMsg struct {
	path string
	err  error
}

// workspaceBoundMsg reports that a session was quietly bound to the folder the
// TUI was opened in.
type workspaceBoundMsg struct {
	path string
	err  error
}

func withTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

// --- commands -----------------------------------------------------------------

func bootCmd(app *appCtx) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout(20 * time.Second)
		defer cancel()
		_, err := session.Ensure(ctx, app.client, app.st)
		if err == nil {
			_ = app.st.Save()
		}
		return bootDoneMsg{err: err}
	}
}

func loadModelsCmd(c *api.Client, refresh bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout(60 * time.Second)
		defer cancel()
		p, err := c.Models(ctx, refresh)
		return modelsMsg{providers: p, err: err}
	}
}

func loadConnectorsCmd(c *api.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout(20 * time.Second)
		defer cancel()
		l, err := c.Connectors(ctx)
		return connectorsMsg{list: l, err: err}
	}
}

func toggleConnectorCmd(c *api.Client, id string, enabled bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout(20 * time.Second)
		defer cancel()
		l, err := c.SetConnectorEnabled(ctx, id, enabled)
		return connectorsMsg{list: l, err: err}
	}
}

func loadChatsCmd(c *api.Client, resume bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout(20 * time.Second)
		defer cancel()
		l, err := c.Chats(ctx)
		return chatsMsg{chats: l, err: err, resume: resume}
	}
}

func loadChatCmd(c *api.Client, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout(30 * time.Second)
		defer cancel()
		d, err := c.GetChat(ctx, id)
		return chatLoadedMsg{detail: d, err: err}
	}
}

func loadProvidersCmd(c *api.Client, open bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout(30 * time.Second)
		defer cancel()
		r, err := c.Providers(ctx)
		return providersMsg{rows: r, err: err, open: open}
	}
}

func saveProviderCmd(c *api.Client, s ProviderSaveMsg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout(30 * time.Second)
		defer cancel()
		var rows []api.ProviderRow
		var err error
		created := s.ID == ""
		if created {
			rows, err = c.CreateProvider(ctx, s.Update)
		} else {
			rows, err = c.UpdateProvider(ctx, s.ID, s.Update)
		}
		return providerSavedMsg{rows: rows, err: err, created: created}
	}
}

func deleteChatCmd(c *api.Client, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout(20 * time.Second)
		defer cancel()
		return chatDeletedMsg{id: id, err: c.DeleteChat(ctx, id)}
	}
}

func renameChatCmd(c *api.Client, id, title string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout(20 * time.Second)
		defer cancel()
		ch, err := c.RenameChat(ctx, id, title)
		return renamedMsg{chat: ch, err: err}
	}
}

func setWorkspaceCmd(c *api.Client, chatID, path string) tea.Cmd {
	return func() tea.Msg {
		if chatID != "" {
			ctx, cancel := withTimeout(20 * time.Second)
			defer cancel()
			if _, err := c.SetChatWorkspace(ctx, chatID, path); err != nil {
				return cdDoneMsg{path: path, err: err}
			}
		}
		return cdDoneMsg{path: path}
	}
}

// bindWorkspaceCmd points a session that had no folder at the folder the TUI
// was opened in. Unlike /cd it stays quiet: opening a session should not print
// a notice about a binding the user did not ask for.
func bindWorkspaceCmd(c *api.Client, chatID, path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := withTimeout(20 * time.Second)
		defer cancel()
		_, err := c.SetChatWorkspace(ctx, chatID, path)
		return workspaceBoundMsg{path: path, err: err}
	}
}

// --- boot screen --------------------------------------------------------------

func (m Model) bootView() string {
	title := lipgloss.NewStyle().Foreground(colorText).Bold(true)
	var lines []string
	if m.boot == bootFailed {
		lines = []string{
			title.Render("Can't reach the Calabrass backend"),
			"",
			msgMuted.Render(m.app.client.Base),
			"",
		}
		for _, l := range strings.Split(wrapText(m.bootErr, 60), "\n") {
			lines = append(lines, toolErrText.Render(l))
		}
		lines = append(lines, "",
			msgMuted.Render("Start the backend (cd Backend && go run ./cmd/server) or open the"),
			msgMuted.Render("desktop app, then press r to try again. ctrl+c quits."))
	} else {
		lines = []string{title.Render("Connecting…"), "", msgMuted.Render(m.app.client.Base)}
	}
	block := strings.Join(lines, "\n")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}

// --- models -------------------------------------------------------------------

func (m *Model) hasModel(provider, model string) bool {
	for _, e := range m.models {
		if e.ProviderID == provider && e.ID == model {
			return true
		}
	}
	return false
}

// applyModels stores the model list and makes sure a valid one is selected.
func (m *Model) applyModels(ps []api.ProviderInfo) {
	m.models = nil
	for _, p := range ps {
		for _, mo := range p.Models {
			label := mo.Label
			if label == "" {
				label = mo.ID
			}
			m.models = append(m.models, modelEntry{ProviderID: p.ID, ProviderName: p.Name, ID: mo.ID, Label: label})
		}
	}

	if !m.hasModel(m.selProvider, m.selModel) {
		m.selProvider, m.selModel = "", ""
		st := m.app.st
		switch {
		case m.hasModel(st.Provider, st.Model):
			m.selProvider, m.selModel = st.Provider, st.Model
		default:
			for _, p := range ps {
				if m.hasModel(p.ID, p.DefaultModel) {
					m.selProvider, m.selModel = p.ID, p.DefaultModel
					break
				}
			}
			if m.selModel == "" && len(m.models) > 0 {
				m.selProvider, m.selModel = m.models[0].ProviderID, m.models[0].ID
			}
		}
	}
	m.syncPromptModel()
	if len(m.models) == 0 {
		m.banner = "No models yet. Type /connect to add a provider API key."
	} else {
		m.banner = ""
	}
}

// syncPromptModel shows the selected model under the prompt.
func (m *Model) syncPromptModel() {
	m.prompt.Model, m.prompt.Provider = "", ""
	for _, e := range m.models {
		if e.ProviderID == m.selProvider && e.ID == m.selModel {
			m.prompt.Model, m.prompt.Provider = e.Label, e.ProviderName
			return
		}
	}
}

func (m *Model) selectModel(provider, model string) {
	m.selProvider, m.selModel = provider, model
	m.syncPromptModel()
	m.app.st.Provider, m.app.st.Model = provider, model
	_ = m.app.st.Save()
}

func (m Model) modelItems() []ListItem {
	items := make([]ListItem, 0, len(m.models))
	for _, e := range m.models {
		title := e.Label
		if e.ProviderID == m.selProvider && e.ID == m.selModel {
			title = "✓ " + title
		}
		items = append(items, ListItem{
			ID:      e.ProviderID + "\x00" + e.ID,
			Title:   title,
			Right:   e.ProviderName,
			Section: e.ProviderName, // group the list by provider
		})
	}
	return items
}

// --- MCP servers and providers --------------------------------------------------

func (m Model) mcpItems() []ListItem {
	items := make([]ListItem, 0, len(m.connectors))
	for _, c := range m.connectors {
		right := "Disabled"
		if c.Enabled {
			right = "Enabled"
		}
		items = append(items, ListItem{ID: c.ID, Title: c.Name, Right: right})
	}
	return items
}

func (m Model) enabledMCPs() int {
	n := 0
	for _, c := range m.connectors {
		if c.Enabled {
			n++
		}
	}
	return n
}

func (m Model) providerItems() []ListItem {
	items := []ListItem{{ID: "__new__", Title: "+ Add custom provider"}}
	for _, p := range m.providerRows {
		right := "Needs a key"
		switch {
		case p.Enabled && p.Connected:
			right = fmt.Sprintf("On · %d models", p.ModelCount)
		case p.Enabled:
			right = "On · no key"
		case strings.TrimSpace(p.APIKey) != "":
			right = "Off"
		}
		items = append(items, ListItem{ID: p.ID, Title: p.Name, Right: right})
	}
	return items
}

// --- sessions -------------------------------------------------------------------

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// chatsHere are the sessions to list, newest first. A session with no folder
// was made before chats recorded one (or by a client that did not choose a
// workspace), so it is treated as belonging to the folder the TUI is in; it is
// bound to that folder when it is opened.
func (m Model) chatsHere() []api.Chat {
	var out []api.Chat
	for _, c := range m.allChats {
		if m.sessionsAll || c.WorkspaceDir == "" || samePath(c.WorkspaceDir, m.cwdPath) {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out
}

func (m Model) sessionItems() []ListItem {
	chats := m.chatsHere()
	items := make([]ListItem, 0, len(chats))
	for _, c := range chats {
		title := strings.TrimSpace(c.Title)
		if title == "" {
			title = "New chat"
		}
		desc := ""
		if m.sessionsAll && c.WorkspaceDir != "" {
			desc = filepath.Base(c.WorkspaceDir)
		}
		right := relTime(c.UpdatedAt)
		if m.confirmDel == c.ID {
			right = "ctrl+d again to delete"
		}
		items = append(items, ListItem{ID: c.ID, Title: title, Desc: desc, Right: right})
	}
	return items
}

func (m Model) sessionsTitle() string {
	if m.sessionsAll {
		return "Sessions · all folders"
	}
	return "Sessions · this folder"
}

func relTime(ms int64) string {
	if ms <= 0 {
		return ""
	}
	d := time.Since(time.UnixMilli(ms))
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return time.UnixMilli(ms).Format("2006-01-02")
}

// setCwd changes the folder the agent works in.
func (m *Model) setCwd(path string) {
	m.cwdPath = path
	m.cwd = workDirLabel(path)
}

// notice adds a note from the app to the chat (and leaves the home screen).
func (m *Model) notice(text string, isErr bool) {
	m.nextID++
	m.messages = append(m.messages, Message{ID: m.nextID, Role: RoleNotice, Text: text, IsError: isErr})
	m.enterChat()
	m.scroll = 0
}

// enterChat switches from the home screen to the chat screen.
func (m *Model) enterChat() {
	if m.screen == screenHome {
		m.screen = screenChat
		m.prompt.Placeholder = ""
		m.syncPromptWidth()
	}
}

// newSession forgets the current conversation; the next message opens a new one.
func (m *Model) newSession() {
	m.interruptTurn()
	m.chat = nil
	m.usageIn, m.usageOut = 0, 0
	m.confirmDel = ""
	m.goHome()
}

// loadHistory shows a stored conversation.
func (m *Model) loadHistory(d api.ChatDetail) {
	m.interruptTurn()
	c := d.Chat
	m.chat = &c
	m.messages = nil
	m.usageIn, m.usageOut = 0, 0
	m.started = time.Now().UTC()
	// A folder the TUI bound is absolute. A name without a path belongs to the
	// backend's workspace library, and using it as a local path would make the
	// footer claim a folder that does not exist.
	if filepath.IsAbs(c.WorkspaceDir) {
		m.setCwd(c.WorkspaceDir)
	}
	if m.hasModel(c.Provider, c.Model) {
		m.selectModel(c.Provider, c.Model)
	}
	accent, label := m.prompt.ModeStyle()
	for _, sm := range d.Messages {
		m.nextID++
		switch sm.Role {
		case api.RoleUser:
			m.messages = append(m.messages, Message{ID: m.nextID, Role: RoleUser, Text: sm.Content, Accent: accent, ServerID: sm.ID})
		default:
			m.messages = append(m.messages, m.historyAssistant(sm, label, accent))
		}
	}
	m.screen = screenChat
	m.prompt.Placeholder = ""
	m.syncPromptWidth()
	m.scroll = 0
	if len(pendingQuestions(m.messages)) > 0 {
		m.notice("The agent asked questions in this session. Type /answer to reply.", false)
	}
}

func (m *Model) historyAssistant(sm api.Message, label string, accent color.Color) Message {
	msg := Message{
		ID: m.nextID, Role: RoleAssistant, ServerID: sm.ID, Label: label, Accent: accent,
		Model:   m.prompt.Model,
		Elapsed: time.Duration(sm.DurationMs) * time.Millisecond,
	}
	if tr, ok := sm.ParsedTrace(); ok && !sm.Compacted {
		parts := partsFromTrace(tr, partOpts{stopped: true})
		parts = withRunNote(parts, sm.Content)
		fillThinkingMs(parts, sm.ThinkingMs)
		msg.Parts = parts
		msg.Questions = questionsOfTrace(tr)
	}
	if len(msg.Parts) == 0 {
		if strings.TrimSpace(sm.Reasoning) != "" {
			msg.Parts = append(msg.Parts, Part{Kind: PartThought, Text: strings.TrimSpace(sm.Reasoning), Ms: int(sm.ThinkingMs)})
		}
		if strings.TrimSpace(sm.Content) != "" {
			msg.Parts = append(msg.Parts, Part{Kind: PartText, Text: sm.Content})
		}
	}
	return msg
}

// resolveDir turns what the user typed after /cd into an existing absolute folder.
func resolveDir(base, arg string) (string, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", errors.New("usage: /cd <folder>")
	}
	if arg == "~" || strings.HasPrefix(arg, "~/") || strings.HasPrefix(arg, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			arg = filepath.Join(home, arg[1:])
		}
	}
	if !filepath.IsAbs(arg) {
		arg = filepath.Join(base, arg)
	}
	arg = filepath.Clean(arg)
	info, err := os.Stat(arg)
	if err != nil {
		return "", fmt.Errorf("no such folder: %s", arg)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a folder: %s", arg)
	}
	return arg, nil
}
