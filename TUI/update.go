package main

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"calabrass-tui/internal/api"
)

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.syncPromptWidth()
		m.dialog.SetSize(msg.Width, msg.Height)
		m.list.SetSize(msg.Width, msg.Height)
		m.provForm.SetSize(msg.Width, msg.Height)
		m.qform.SetSize(msg.Width, msg.Height)
		m.quick.SetSize(msg.Width, msg.Height)
		return m, nil

	// --- the backend answers ---------------------------------------------
	case bootDoneMsg:
		if msg.err != nil {
			m.boot, m.bootErr = bootFailed, msg.err.Error()
			return m, nil
		}
		m.boot = bootReady
		cmds := []tea.Cmd{loadModelsCmd(m.app.client, false), loadConnectorsCmd(m.app.client), loadSchedulesCmd(m.app.client), schedulePollCmd()}
		if m.app.resume {
			m.app.resume = false
			cmds = append(cmds, loadChatsCmd(m.app.client, true))
		}
		return m, tea.Batch(cmds...)

	case schedulePollTick:
		return m, tea.Batch(loadSchedulesCmd(m.app.client), schedulePollCmd())

	case modelsMsg:
		if msg.err != nil {
			return m, m.apiFailed(msg.err, "Could not load models")
		}
		m.applyModels(msg.providers)
		if m.list.IsOpen() && m.list.Kind == "models" {
			m.list.SetItems(m.modelItems())
		}
		return m, nil

	case connectorsMsg:
		if msg.err != nil {
			if m.list.IsOpen() && m.list.Kind == "mcps" {
				m.list.SetNotice(msg.err.Error(), true)
				return m, nil
			}
			return m, m.apiFailed(msg.err, "Could not load MCP servers")
		}
		m.connectors = msg.list
		if m.list.IsOpen() && m.list.Kind == "mcps" {
			m.list.SetNotice("", false)
			m.list.SetItems(m.mcpItems())
		}
		return m, nil

	case providersMsg:
		if msg.err != nil {
			return m, m.apiFailed(msg.err, "Could not load providers")
		}
		m.providerRows = msg.rows
		if msg.open {
			return m, m.openProviders()
		}
		return m, nil

	case providerSavedMsg:
		if msg.err != nil {
			if api.IsUnauthorized(msg.err) {
				return m, m.apiFailed(msg.err, "Could not save the provider")
			}
			m.provForm.Err, m.provForm.Saving = msg.err.Error(), false
			return m, nil
		}
		m.provForm.Close()
		m.providerRows = msg.rows
		text := "Provider saved"
		if msg.created {
			text = "Provider added"
		}
		return m, tea.Batch(m.showToast(text, false), m.openProviders(), loadModelsCmd(m.app.client, true))

	case chatsMsg:
		if msg.err != nil {
			return m, m.apiFailed(msg.err, "Could not load sessions")
		}
		m.allChats = msg.chats
		if msg.resume {
			if here := m.chatsHere(); len(here) > 0 {
				return m, loadChatCmd(m.app.client, here[0].ID)
			}
			m.notice("No earlier session in this folder; starting a new one.", false)
			return m, nil
		}
		return m, m.openSessions()

	case chatLoadedMsg:
		if msg.err != nil {
			return m, m.apiFailed(msg.err, "Could not open the session")
		}
		m.loadHistory(msg.detail)
		// An older session has no folder yet. Bind it to the folder the TUI was
		// opened in, so the next turn does not quietly run in the default
		// workspace under the app's data directory.
		if m.chat != nil && strings.TrimSpace(m.chat.WorkspaceDir) == "" {
			return m, bindWorkspaceCmd(m.app.client, m.chat.ID, m.cwdPath)
		}
		return m, nil

	case workspaceBoundMsg:
		if msg.err != nil {
			return m, m.apiFailed(msg.err, "Could not set the working folder")
		}
		if m.chat != nil {
			m.chat.WorkspaceDir = msg.path
		}
		return m, nil

	case chatDeletedMsg:
		m.confirmDel = ""
		if msg.err != nil {
			m.list.SetNotice(msg.err.Error(), true)
			return m, nil
		}
		kept := m.allChats[:0:0]
		for _, c := range m.allChats {
			if c.ID != msg.id {
				kept = append(kept, c)
			}
		}
		m.allChats = kept
		if m.chat != nil && m.chat.ID == msg.id {
			m.newSession()
		}
		if m.list.IsOpen() && m.list.Kind == "sessions" {
			m.list.SetNotice("Deleted.", false)
			m.list.SetItems(m.sessionItems())
		}
		return m, nil

	case renamedMsg:
		if msg.err != nil {
			return m, m.apiFailed(msg.err, "Could not rename the session")
		}
		c := msg.chat
		m.chat = &c
		m.notice("Renamed to “"+c.Title+"”.", false)
		return m, nil

	case cdDoneMsg:
		if msg.err != nil {
			return m, m.apiFailed(msg.err, "Could not change folder")
		}
		m.setCwd(msg.path)
		if m.chat != nil {
			m.chat.WorkspaceDir = msg.path
		}
		m.notice("Working in "+msg.path, false)
		return m, nil

	case schedulesMsg:
		if msg.err != nil {
			return m, m.apiFailed(msg.err, "Could not load schedules")
		}
		m.schedules = msg.tasks
		return m, nil

	case scheduleCreatedMsg:
		if msg.err != nil {
			return m, m.apiFailed(msg.err, "Could not create schedule")
		}
		m.schedules = msg.tasks
		return m, m.showToast("Schedule created", false)

	case scheduleUpdatedMsg:
		if msg.err != nil {
			return m, m.apiFailed(msg.err, "Could not update schedule")
		}
		m.schedules = msg.tasks
		return m, m.showToast("Schedule updated", false)

	case scheduleDeletedMsg:
		if msg.err != nil {
			return m, m.apiFailed(msg.err, "Could not delete schedule")
		}
		// Remove the deleted schedule from the list
		filtered := m.schedules[:0]
		for _, s := range m.schedules {
			if s.ID != msg.id {
				filtered = append(filtered, s)
			}
		}
		m.schedules = filtered
		return m, m.showToast("Schedule deleted", false)

	case turnBatchMsg:
		if m.turn != msg.ts {
			return m, nil // an interrupted turn still draining: ignore
		}
		finished := false
		for _, te := range msg.events {
			if m.applyTurnEvent(te) {
				finished = true
				break
			}
		}
		if !finished {
			return m, waitTurn(msg.ts)
		}
		return m, m.afterTurn()

	// --- animation -----------------------------------------------------------
	case spinner.TickMsg:
		if !m.running() {
			m.spBusy = false
			return m, nil
		}
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(msg)
		return m, cmd
	case spinTickMsg:
		if !m.running() {
			m.spinActive = false
			return m, nil
		}
		m.advanceSpinner()
		return m, spinTick()

	case toastMsg:
		if msg.seq == m.toastSeq {
			m.toastText = ""
		}
		return m, nil

	case holdTickMsg:
		if msg.seq != m.holdSeq {
			return m, nil // a newer gesture already replaced this timer
		}
		// A short tap can't open the menu: the key must still be repeating.
		if m.holdCount >= 4 && time.Since(m.holdLast) < 450*time.Millisecond &&
			!m.modalOpen() && m.prompt.Value() == "" {
			m.quick.Open()
			m.holdSeen = true // swallow the rest of the repeats while held
		}
		m.holdCount = 0
		m.holdSeq++
		return m, nil

	// --- things the dialogs and the prompt report --------------------------------
	case ListSelectMsg:
		return m.onListSelect(msg)
	case ListKeyMsg:
		return m.onListKey(msg)
	case ProviderSaveMsg:
		return m, saveProviderCmd(m.app.client, msg)
	case ProviderFormCancelMsg:
		return m, m.openProviders()
	case QuestionsAnsweredMsg:
		return m, m.submit(msg.Text)
	case DialogSelectMsg:
		return m.onPalette(msg.Command.ID)
	case QuickSelectMsg:
		return m.onPalette(msg.ID)
	case SlashSelectMsg:
		return m, m.runCommand(msg.Command.Name, "")
	case PromptSubmitMsg:
		return m, m.onSubmit(msg.Value)

	// --- keyboard and mouse ----------------------------------------------------
	case tea.KeyPressMsg:
		return m.onKey(msg)
	case tea.MouseWheelMsg:
		m.clearSelection()
		if !m.modalOpen() {
			switch msg.Button {
			case tea.MouseWheelUp:
				m.scrollBy(3)
			case tea.MouseWheelDown:
				m.scrollBy(-3)
			}
		}
		return m, nil
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			m.press(msg.X, msg.Y)
		}
		return m, nil
	case tea.MouseMotionMsg:
		if msg.Button == tea.MouseLeft {
			m.drag(msg.X, msg.Y)
			return m, nil
		}
		if !m.modalOpen() {
			m.handleHover(msg.X, msg.Y)
		}
		return m, nil
	case tea.MouseReleaseMsg:
		if m.selPressing && (msg.Button == tea.MouseLeft || msg.Button == tea.MouseNone) {
			m.release(msg.X, msg.Y)
		}
		return m, nil
	}

	// Everything else (cursor blink, ...) goes to whoever has the keyboard.
	return m.forward(msg)
}

// forward hands a non-key message to the prompt and the open dialog.
func (m Model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.prompt, cmd = m.prompt.Update(msg)
	cmds = append(cmds, cmd)
	m.slash.Sync(m.prompt.Value())
	switch {
	case m.dialog.IsOpen():
		m.dialog, cmd = m.dialog.Update(msg)
	case m.quick.IsOpen():
		m.quick, cmd = m.quick.Update(msg)
	case m.list.IsOpen():
		m.list, cmd = m.list.Update(msg)
	case m.provForm.IsOpen():
		m.provForm, cmd = m.provForm.Update(msg)
	case m.qform.IsOpen():
		m.qform, cmd = m.qform.Update(msg)
	default:
		cmd = nil
	}
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// apiFailed reports a failed backend call. A 401 signs in again by itself.
func (m *Model) apiFailed(err error, what string) tea.Cmd {
	if api.IsUnauthorized(err) {
		m.app.client.Token, m.app.st.Token = "", ""
		m.boot = bootConnecting
		return bootCmd(m.app)
	}
	m.notice(what+": "+err.Error(), true)
	return nil
}

// afterTurn runs when a turn is over: it opens the questions form when the
// agent asked something.
func (m *Model) afterTurn() tea.Cmd {
	if qs := pendingQuestions(m.messages); len(qs) > 0 {
		return m.qform.Open(qs)
	}
	return nil
}

// onKey routes a key press: dialogs first, then the "/" menu, then the app.
func (m Model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.boot != bootReady {
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "r":
			if m.boot == bootFailed {
				m.boot = bootConnecting
				return m, bootCmd(m.app)
			}
		}
		return m, nil
	}

	// ctrl+c is handled before anything else: it copies a selection, or arms a
	// second press that quits. Every other key drops the selection and the arm.
	if key == "ctrl+c" {
		return m.onCtrlC()
	}
	m.quitArmed = false
	m.clearSelection()

	// Holding ↓ must not move the quick menu while the key is still repeating.
	if key == "down" && m.holdSeen && m.quick.IsOpen() {
		return m, nil
	}
	if key != "down" {
		m.holdSeen = false
		if m.holdCount > 0 {
			m.holdCount = 0
			m.holdSeq++ // invalidate the pending hold timer
		}
	}

	var cmd tea.Cmd
	switch {
	case m.quick.IsOpen():
		m.quick, cmd = m.quick.Update(msg)
		if !m.quick.IsOpen() {
			m.holdSeen = false
		}
		return m, cmd
	case m.provForm.IsOpen():
		m.provForm, cmd = m.provForm.Update(msg)
		return m, cmd
	case m.qform.IsOpen():
		m.qform, cmd = m.qform.Update(msg)
		return m, cmd
	case m.list.IsOpen():
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	case m.dialog.IsOpen():
		if key == keyCommandPalette {
			m.dialog.Close()
			return m, nil
		}
		m.dialog, cmd = m.dialog.Update(msg)
		return m, cmd
	}

	// "/" menu: while it is open it takes the navigation keys.
	if m.slashVisible() {
		value := m.prompt.Value()
		switch key {
		case "esc":
			m.slash.Dismiss()
			return m, nil
		case "up":
			m.slash.Move(-1, value)
			return m, nil
		case "down":
			m.slash.Move(1, value)
			return m, nil
		case "tab":
			if c, ok := m.slash.Selected(value); ok {
				m.prompt.SetValue(c.Name + " ")
				m.slash.Sync(m.prompt.Value())
			}
			return m, nil
		case "enter":
			if c, ok := m.slash.Selected(value); ok {
				m.prompt.SetValue("")
				m.slash.Sync("")
				return m, func() tea.Msg { return SlashSelectMsg{Command: c} }
			}
		}
	}

	// Hold ↓ for two seconds: quick actions. The first press starts the timer;
	// the terminal's key-repeat events count as "still holding".
	if key == "down" && !m.modalOpen() && m.prompt.Value() == "" {
		if m.holdCount == 0 {
			m.holdSeq++
			m.holdCount = 1
			m.holdLast = time.Now()
			return m, holdTick(m.holdSeq)
		}
		m.holdCount++
		m.holdLast = time.Now()
		return m, nil
	}

	switch key {
	case "esc":
		if m.running() {
			m.interruptTurn()
		}
		return m, nil
	case keyCommandPalette:
		return m, m.dialog.Open()
	case "pgup":
		m.scrollBy(m.chatHeight() / 2)
		return m, nil
	case "pgdown":
		m.scrollBy(-m.chatHeight() / 2)
		return m, nil
	case keyToggleRight:
		m.showRight = !m.showRight
		m.syncPromptWidth()
		return m, nil
	case keyToggleRows:
		m.toggleRows()
		return m, nil
	case "enter":
		if m.running() {
			return m, nil // keep what was typed until the answer is done
		}
	}

	var pc tea.Cmd
	m.prompt, pc = m.prompt.Update(msg)
	m.slash.Sync(m.prompt.Value())
	return m, pc
}

// onSubmit handles Enter in the prompt: a command, or a message to the agent.
func (m *Model) onSubmit(text string) tea.Cmd {
	if strings.HasPrefix(text, "/") {
		name, args, _ := strings.Cut(text, " ")
		for _, c := range defaultSlashCommands() {
			if c.Name == name {
				return m.runCommand(name, strings.TrimSpace(args))
			}
		}
	}
	return m.submit(text)
}

// submit sends a message to the agent.
func (m *Model) submit(text string) tea.Cmd {
	if m.running() {
		return nil
	}
	if m.selModel == "" {
		m.enterChat()
		m.prompt.SetValue(text)
		m.notice("No model is available. Type /connect to add a provider API key.", true)
		return nil
	}
	m.enterChat()
	return m.startTurn(text, false)
}

// toggleRows opens every folded row, or closes them all when all are open.
func (m *Model) toggleRows() {
	allOpen := true
	each := func(f func(msg *Message, i int)) {
		for mi := range m.messages {
			msg := &m.messages[mi]
			for i := range msg.Parts {
				if msg.Parts[i].Kind != PartText && (i == 0 || msg.Parts[i-1].Kind == PartText) {
					f(msg, i)
				}
			}
		}
	}
	each(func(msg *Message, i int) {
		if !msg.Parts[i].Expanded {
			allOpen = false
		}
	})
	each(func(msg *Message, i int) {
		msg.Parts[i].Expanded = !allOpen
		msg.invalidateMsgRender()
	})
}

// onPalette runs a command-palette entry.
func (m Model) onPalette(id string) (tea.Model, tea.Cmd) {
	switch id {
	case "switch-model":
		return m, m.runCommand("/models", "")
	case "switch-session":
		return m, m.runCommand("/continue", "")
	case "new-session":
		return m, m.runCommand("/new", "")
	case "connect":
		return m, m.runCommand("/connect", "")
	case "copy":
		return m, m.copyLastResponse()
	case "mcps":
		return m, m.runCommand("/mcps", "")
	case "schedules":
		return m, m.runCommand("/schedules", "")
	case "help":
		return m, m.runCommand("/help", "")
	case "toggle-right":
		m.showRight = !m.showRight
		m.syncPromptWidth()
	case "toggle-rows":
		m.toggleRows()
	}
	return m, nil
}

// onListSelect handles Enter on a dialog row.
func (m Model) onListSelect(msg ListSelectMsg) (tea.Model, tea.Cmd) {
	switch msg.Kind {
	case "models":
		provider, model, _ := strings.Cut(msg.Item.ID, "\x00")
		m.selectModel(provider, model)
		return m, m.showToast("Model switched to "+m.prompt.Model, false)
	case "sessions":
		return m, loadChatCmd(m.app.client, msg.Item.ID)
	case "mcps":
		for _, c := range m.connectors {
			if c.ID == msg.Item.ID {
				return m, toggleConnectorCmd(m.app.client, c.ID, !c.Enabled)
			}
		}
	case "providers":
		if msg.Item.ID == "__new__" {
			return m, m.provForm.Open(api.ProviderRow{}, true)
		}
		for _, p := range m.providerRows {
			if p.ID == msg.Item.ID {
				return m, m.provForm.Open(p, false)
			}
		}
	}
	return m, nil
}

// onListKey handles the extra keys of the sessions list.
func (m Model) onListKey(msg ListKeyMsg) (tea.Model, tea.Cmd) {
	if msg.Kind != "sessions" {
		return m, nil
	}
	switch msg.Key {
	case "tab":
		m.sessionsAll = !m.sessionsAll
		m.confirmDel = ""
		m.list.Title = m.sessionsTitle()
		m.list.SetItems(m.sessionItems())
	case "ctrl+d":
		if !msg.HasItem {
			return m, nil
		}
		if m.confirmDel == msg.Item.ID {
			return m, deleteChatCmd(m.app.client, msg.Item.ID)
		}
		m.confirmDel = msg.Item.ID
		m.list.SetItems(m.sessionItems())
	}
	return m, nil
}

func (m *Model) openProviders() tea.Cmd {
	cmd := m.list.Show("providers", "Providers", m.providerItems())
	m.list.Empty = "No providers"
	m.list.HintLeft = "enter configure"
	return cmd
}

func (m *Model) openSessions() tea.Cmd {
	m.confirmDel = ""
	cmd := m.list.Show("sessions", m.sessionsTitle(), m.sessionItems())
	m.list.Empty = "No sessions yet. Send a message to start one."
	m.list.HintLeft = "enter open · ctrl+d delete"
	m.list.HintRight = "tab all folders"
	m.list.ExtraKeys = []string{"tab", "ctrl+d"}
	return cmd
}

var _ = time.Second
