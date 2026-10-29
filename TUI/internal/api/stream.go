package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Event kinds delivered by Send.
const (
	EventChat      = "chat"      // the chat record, updated for this turn
	EventMessage   = "message"   // the stored user message
	EventToken     = "token"     // a piece of answer text
	EventReasoning = "reasoning" // a piece of reasoning text
	EventTrace     = "trace"     // the whole turn so far
	EventDone      = "done"      // the stored assistant message
	EventError     = "error"     // generation failed

	// EventApproval is the agent asking permission for something that needs a
	// person's say-so. EventApprovalResolved says it was answered, from here or
	// from another face, and the prompt should go away.
	EventApproval         = "approval"
	EventApprovalResolved = "approval_resolved"
)

// Event is one decoded server-sent event.
type Event struct {
	Kind    string
	Chat    *Chat
	Message *Message
	Text    string // token / reasoning delta, or the error message
	Trace   *Trace
	Done    *Done
	// Approval is set for EventApproval and EventApprovalResolved.
	Approval *Approval
}

// Send posts one user turn and delivers the streamed reply to emit, in order.
// It returns when the stream ends. A cancelled ctx stops it; the backend still
// saves what it had generated.
func (c *Client) Send(ctx context.Context, chatID string, req SendRequest, emit func(Event)) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.Base+"/api/chats/"+url.PathEscape(chatID)+"/messages", bytes.NewReader(data))
	if err != nil {
		return err
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Accept", "text/event-stream")
	c.authorize(hr)

	resp, err := c.stream.Do(hr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return readAPIError(resp)
	}
	return ReadEvents(resp.Body, emit)
}

// ReadEvents decodes a server-sent-event stream into Events.
func ReadEvents(r io.Reader, emit func(Event)) error {
	return readSSE(r, func(name, data string) {
		if ev, ok := decodeEvent(name, data); ok {
			emit(ev)
		}
	})
}

func decodeEvent(name, data string) (Event, bool) {
	ev := Event{Kind: name}
	switch name {
	case EventChat:
		var chat Chat
		if json.Unmarshal([]byte(data), &chat) != nil {
			return ev, false
		}
		ev.Chat = &chat
	case EventMessage:
		var msg Message
		if json.Unmarshal([]byte(data), &msg) != nil {
			return ev, false
		}
		ev.Message = &msg
	case EventToken, EventReasoning:
		var p struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(data), &p) != nil {
			return ev, false
		}
		ev.Text = p.Text
	case EventTrace:
		var p struct {
			Trace Trace `json:"trace"`
		}
		if json.Unmarshal([]byte(data), &p) != nil {
			return ev, false
		}
		ev.Trace = &p.Trace
	case EventDone:
		var d Done
		if json.Unmarshal([]byte(data), &d) != nil {
			return ev, false
		}
		ev.Done = &d
	case EventApproval, EventApprovalResolved:
		var a Approval
		if json.Unmarshal([]byte(data), &a) != nil || a.ID == "" {
			return ev, false
		}
		ev.Approval = &a
	case EventError:
		var p struct {
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(data), &p) != nil {
			return ev, false
		}
		ev.Text = p.Message
	default:
		return ev, false // an event this client does not know: ignore it
	}
	return ev, true
}

// readSSE splits an event stream into (event, data) pairs. It reads line by
// line without a size limit, because a trace snapshot can be large.
func readSSE(r io.Reader, fn func(event, data string)) error {
	br := bufio.NewReaderSize(r, 64*1024)
	var event string
	var data []string

	dispatch := func() {
		if event != "" || len(data) > 0 {
			fn(event, strings.Join(data, "\n"))
		}
		event, data = "", nil
	}

	for {
		line, err := br.ReadString('\n')
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case trimmed == "" && line != "":
			dispatch()
		case strings.HasPrefix(trimmed, ":"):
			// comment / keep-alive
		case strings.HasPrefix(trimmed, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		case strings.HasPrefix(trimmed, "data:"):
			v := strings.TrimPrefix(trimmed, "data:")
			data = append(data, strings.TrimPrefix(v, " "))
		}
		if err != nil {
			if err == io.EOF {
				dispatch()
				return nil
			}
			return err
		}
	}
}
