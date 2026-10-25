package main

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"calabrass-tui/internal/api"
)

// turnEvent is one thing that happened on the stream of a turn.
type turnEvent struct {
	ev  api.Event
	end bool  // the stream is over
	err error // why it ended, if it did not end cleanly
}

// turnBatchMsg delivers the events that arrived within one short window.
type turnBatchMsg struct {
	ts     *turnState
	events []turnEvent
}

// turnState is one running turn: the request, its event channel, and the
// mirror of the backend's trace that the answer is drawn from.
type turnState struct {
	cancel context.CancelFunc
	ch     chan turnEvent
	msgID  int // local id of the assistant message being filled
	mirror *mirror
	done   *api.Done
}

// run creates the chat when there is none yet, sends the turn, and feeds every
// event to the channel. It always ends with an `end` event.
func (ts *turnState) run(ctx context.Context, c *api.Client, chat *api.Chat, create api.CreateChatRequest, send api.SendRequest) {
	defer close(ts.ch)
	emit := func(e api.Event) {
		select {
		case ts.ch <- turnEvent{ev: e}:
		case <-ctx.Done():
		}
	}

	var err error
	chatID := ""
	if chat != nil {
		chatID = chat.ID
	} else {
		created, e := c.CreateChat(ctx, create)
		if e != nil {
			err = e
		} else {
			chatID = created.ID
			emit(api.Event{Kind: api.EventChat, Chat: &created})
		}
	}
	if err == nil {
		err = c.Send(ctx, chatID, send, emit)
	}
	select {
	case ts.ch <- turnEvent{end: true, err: err}:
	case <-ctx.Done():
	}
}

// waitTurn waits for the next event, then gathers whatever else arrives in the
// next few milliseconds, so a fast stream redraws about 40 times a second
// instead of once per token.
func waitTurn(ts *turnState) tea.Cmd {
	return func() tea.Msg {
		first, ok := <-ts.ch
		if !ok {
			return turnBatchMsg{ts: ts, events: []turnEvent{{end: true}}}
		}
		batch := []turnEvent{first}
		if first.end {
			return turnBatchMsg{ts: ts, events: batch}
		}
		timer := time.NewTimer(25 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case te, ok := <-ts.ch:
				if !ok {
					return turnBatchMsg{ts: ts, events: append(batch, turnEvent{end: true})}
				}
				batch = append(batch, te)
				if te.end {
					return turnBatchMsg{ts: ts, events: batch}
				}
			case <-timer.C:
				return turnBatchMsg{ts: ts, events: batch}
			}
		}
	}
}

func (m *Model) indexOfID(id int) int {
	for i := range m.messages {
		if m.messages[i].ID == id {
			return i
		}
	}
	return -1
}

// keepExpanded carries the open/closed state of folded rows over to a rebuilt
// list of parts (rows keep their index while an answer grows).
func keepExpanded(old, parts []Part) []Part {
	for i := range parts {
		if i < len(old) && old[i].Kind == parts[i].Kind {
			parts[i].Expanded = old[i].Expanded
		}
	}
	return parts
}

// startTurn sends text to the agent. With regenerate, the last reply is
// replaced instead of a new user message being added.
func (m *Model) startTurn(text string, regenerate bool) tea.Cmd {
	accent, label := m.prompt.ModeStyle()
	if !regenerate {
		m.nextID++
		m.messages = append(m.messages, Message{ID: m.nextID, Role: RoleUser, Text: text, Accent: accent})
	}
	m.nextID++
	id := m.nextID
	m.messages = append(m.messages, Message{
		ID: id, Role: RoleAssistant, Pending: true,
		Started: time.Now(), Label: label, Accent: accent, Model: m.prompt.Model,
	})
	m.scroll = 0

	ctx, cancel := context.WithCancel(context.Background())
	ts := &turnState{cancel: cancel, ch: make(chan turnEvent, 256), msgID: id, mirror: newMirror()}
	m.turn = ts

	chat, client := m.chat, m.app.client
	create := api.CreateChatRequest{Provider: m.selProvider, Model: m.selModel, Mode: "chat", WorkspacePath: m.cwdPath}
	send := api.SendRequest{Content: text, Provider: m.selProvider, Model: m.selModel, Regenerate: regenerate}

	cmds := []tea.Cmd{
		func() tea.Msg { go ts.run(ctx, client, chat, create, send); return nil },
		waitTurn(ts),
	}
	if !m.spinActive {
		m.spinActive = true
		m.spin.Page = 0
		cmds = append(cmds, spinTick())
	}
	if !m.spBusy {
		m.spBusy = true
		cmds = append(cmds, m.sp.Tick)
	}
	return tea.Batch(cmds...)
}

// rebuildLive redraws the answer from the mirror while the turn runs.
func (m *Model) rebuildLive(msg *Message) {
	ts := m.turn
	parts := partsFromTrace(ts.mirror.trace, partOpts{live: true, thoughtMs: ts.mirror.thoughtMs()})
	msg.Parts = keepExpanded(msg.Parts, parts)
	msg.Pending, msg.Streaming = false, true
	msg.invalidateMsgRender()
}

// applyTurnEvent folds one event into the app. It reports true when the turn
// is over.
func (m *Model) applyTurnEvent(te turnEvent) bool {
	ts := m.turn
	i := m.indexOfID(ts.msgID)
	if i < 0 {
		return true
	}
	msg := &m.messages[i]

	if te.end {
		m.finishTurn(msg, te.err)
		return true
	}

	ev := te.ev
	switch ev.Kind {
	case api.EventChat:
		if ev.Chat != nil {
			c := *ev.Chat
			m.chat = &c
		}
		msg.Pending, msg.Streaming = false, true
		msg.invalidateMsgRender()
	case api.EventMessage:
		if ev.Message != nil && i > 0 && m.messages[i-1].Role == RoleUser {
			m.messages[i-1].ServerID = ev.Message.ID
		}
		msg.Pending, msg.Streaming = false, true
		msg.invalidateMsgRender()
	case api.EventReasoning:
		ts.mirror.addDelta(api.PartReasoning, ev.Text, time.Now())
		m.rebuildLive(msg)
	case api.EventToken:
		ts.mirror.addDelta(api.PartText, ev.Text, time.Now())
		m.rebuildLive(msg)
	case api.EventTrace:
		if ev.Trace != nil {
			ts.mirror.setTrace(*ev.Trace)
			m.rebuildLive(msg)
		}
	case api.EventDone:
		ts.done = ev.Done
	case api.EventError:
		msg.Err = ev.Text
		msg.invalidateMsgRender()
	}
	return false
}

// finishTurn settles the answer once the stream is over, however it ended.
func (m *Model) finishTurn(msg *Message, endErr error) {
	ts := m.turn
	tr := ts.mirror.trace
	interrupted := true
	content := ""

	if d := ts.done; d != nil {
		msg.ServerID = d.Message.ID
		if t, ok := d.Message.ParsedTrace(); ok {
			tr = t
		}
		content = d.Message.Content
		interrupted = d.Interrupted
		m.usageIn += d.Usage.InputTokens
		m.usageOut += d.Usage.OutputTokens
	}

	ms := ts.mirror.thoughtMs()
	parts := partsFromTrace(tr, partOpts{stopped: interrupted, thoughtMs: ms})
	if ts.done != nil {
		parts = withRunNote(parts, content)
		if len(parts) == 0 && strings.TrimSpace(content) != "" {
			parts = []Part{{Kind: PartText, Text: content}}
		}
		fillThinkingMs(parts, ts.done.Message.ThinkingMs)
	}
	msg.Parts = keepExpanded(msg.Parts, parts)
	msg.Pending, msg.Streaming = false, false
	msg.Interrupted = interrupted && (ts.done != nil || endErr != nil)
	msg.Elapsed = time.Since(msg.Started)
	if msg.Err == "" && endErr != nil && !errors.Is(endErr, context.Canceled) {
		msg.Err = friendlyError(endErr)
	}
	if !interrupted {
		msg.Questions = questionsOfTrace(tr)
	}
	msg.invalidateMsgRender()
	m.turn = nil
}

// interruptTurn stops the running turn (Esc). The backend keeps what it had
// generated so far; the screen keeps it too.
func (m *Model) interruptTurn() {
	ts := m.turn
	if ts == nil {
		return
	}
	ts.cancel()
	if i := m.indexOfID(ts.msgID); i >= 0 {
		msg := &m.messages[i]
		parts := partsFromTrace(ts.mirror.trace, partOpts{stopped: true, thoughtMs: ts.mirror.thoughtMs()})
		msg.Parts = keepExpanded(msg.Parts, parts)
		msg.Pending, msg.Streaming = false, false
		msg.Interrupted = true
		msg.Elapsed = time.Since(msg.Started)
		msg.invalidateMsgRender()
	}
	m.turn = nil
}

// fillThinkingMs gives the one thought of an answer its duration when the
// backend reported only a total (it cannot be split between several).
func fillThinkingMs(parts []Part, total int64) {
	if total <= 0 {
		return
	}
	idx, n := -1, 0
	for i, p := range parts {
		if p.Kind == PartThought {
			idx = i
			n++
		}
	}
	if n == 1 && parts[idx].Ms == 0 {
		parts[idx].Ms = int(total)
	}
}

// questionsOfTrace returns the questions of an ask_questions call that is the
// last thing the agent did, i.e. the answer is waiting on the user.
func questionsOfTrace(tr api.Trace) []askQuestion {
	for si := len(tr.Steps) - 1; si >= 0; si-- {
		parts := tr.Steps[si].Parts
		for pi := len(parts) - 1; pi >= 0; pi-- {
			p := parts[pi]
			if p.Kind == api.PartTool && p.Name == "ask_questions" && p.Status == api.StatusOK {
				return parseQuestions(p.Input)
			}
			if p.Kind == api.PartTool || (p.Kind == api.PartText && strings.TrimSpace(p.Text) != "") {
				return nil
			}
		}
	}
	return nil
}

// friendlyError words a transport failure for the screen.
func friendlyError(err error) string {
	if api.IsAPIError(err) {
		return err.Error()
	}
	return "Lost the connection to the backend: " + err.Error()
}
