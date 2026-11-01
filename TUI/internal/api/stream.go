package api

import (
	"bufio"
	"bytes"
	"calabrass-tui/internal/termsafe"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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

	// A context of our own, so a stalled stream can be cut off without the
	// caller having cancelled anything.
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()

	hr, err := http.NewRequestWithContext(sctx, http.MethodPost,
		c.Base+"/api/chats/"+url.PathEscape(chatID)+"/messages", bytes.NewReader(data))
	if err != nil {
		return err
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Accept", "text/event-stream")
	c.authorize(hr)

	resp, err := c.stream.Do(hr)
	if err != nil {
		return wrapTransport(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return readAPIError(resp)
	}

	guard := newIdleGuard(cancel)
	defer guard.stop()
	body := guard.wrap(resp.Body)

	finished := false // a "done" or "error" event arrived: the reply really ended
	readErr := readSSE(body, func(name, payload string) {
		if ev, ok := decodeEvent(name, payload); ok {
			if ev.Kind == EventDone || ev.Kind == EventError {
				finished = true
			}
			emit(ev)
		}
	}, guard.onComment)

	switch {
	case ctx.Err() != nil:
		// The caller stopped it. That is not a failure of the connection.
		return ctx.Err()
	case guard.tripped():
		return &ConnError{Kind: ConnIdle}
	case readErr != nil:
		return wrapTransport(ctx, readErr)
	case !finished:
		// The stream closed cleanly and said nothing about how the reply ended.
		// A host that finished a reply says so; one that was stopped halfway does
		// not. What came before is a part of an answer, not an answer.
		return &ConnError{Kind: ConnLost, Err: io.ErrUnexpectedEOF}
	}
	return nil
}

// ReadEvents decodes a server-sent-event stream into Events.
func ReadEvents(r io.Reader, emit func(Event)) error {
	return readSSE(r, func(name, data string) {
		if ev, ok := decodeEvent(name, data); ok {
			emit(ev)
		}
	}, nil)
}

// --- stalled connections ------------------------------------------------------

// heartbeatPrefix is what a host that sends keep-alives says first. A host that
// says it promises a byte at least that often, so silence for several intervals
// means the connection is dead, not that a long command is running.
const heartbeatPrefix = "calabrass-heartbeat"

// Timings, as variables so a test can shrink them.
var (
	idleFloor    = 15 * time.Second // never cut a stream off sooner than this
	idleCheck    = time.Second      // how often the watchdog looks
	idleBeatsMax = 3                // missed heartbeats that mean the connection is dead
)

// idleGuard watches a stream for silence. It does nothing until the host has said
// it sends heartbeats, so an older host, which is silent for as long as a tool
// runs, is never cut off.
type idleGuard struct {
	cancel  context.CancelFunc
	last    atomic.Int64 // unix nanoseconds of the last byte read
	limit   atomic.Int64 // nanoseconds of silence allowed; 0 until armed
	trip    atomic.Bool
	closing chan struct{}
	once    sync.Once
	check   time.Duration // how often to look, fixed when the guard is made
}

func newIdleGuard(cancel context.CancelFunc) *idleGuard {
	g := &idleGuard{cancel: cancel, closing: make(chan struct{}), check: idleCheck}
	g.last.Store(time.Now().UnixNano())
	go g.watch()
	return g
}

func (g *idleGuard) wrap(r io.Reader) io.Reader { return &activityReader{r: r, g: g} }

// onComment is called for every ": ..." line of the stream.
func (g *idleGuard) onComment(text string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, heartbeatPrefix) {
		return
	}
	seconds := 10
	if f := strings.Fields(strings.TrimPrefix(text, heartbeatPrefix)); len(f) > 0 {
		if n, err := strconv.Atoi(f[0]); err == nil && n > 0 && n <= 300 {
			seconds = n
		}
	}
	// Three missed beats is a dead connection; never less than the floor.
	limit := time.Duration(seconds) * time.Duration(idleBeatsMax) * time.Second
	if limit < idleFloor {
		limit = idleFloor
	}
	g.limit.Store(int64(limit))
}

func (g *idleGuard) watch() {
	tick := time.NewTicker(g.check)
	defer tick.Stop()
	for {
		select {
		case <-g.closing:
			return
		case <-tick.C:
			limit := g.limit.Load()
			if limit > 0 && time.Now().UnixNano()-g.last.Load() > limit {
				g.trip.Store(true)
				g.cancel()
				return
			}
		}
	}
}

func (g *idleGuard) tripped() bool { return g.trip.Load() }
func (g *idleGuard) stop()         { g.once.Do(func() { close(g.closing) }) }

type activityReader struct {
	r io.Reader
	g *idleGuard
}

func (a *activityReader) Read(p []byte) (int, error) {
	n, err := a.r.Read(p)
	if n > 0 {
		a.g.last.Store(time.Now().UnixNano())
	}
	return n, err
}

func decodeEvent(name, data string) (Event, bool) {
	ev := Event{Kind: name}
	switch name {
	case EventChat:
		var chat Chat
		if json.Unmarshal([]byte(data), &chat) != nil {
			return ev, false
		}
		cleanChat(&chat)
		ev.Chat = &chat
	case EventMessage:
		var msg Message
		if json.Unmarshal([]byte(data), &msg) != nil {
			return ev, false
		}
		cleanMessage(&msg)
		ev.Message = &msg
	case EventToken, EventReasoning:
		var p struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(data), &p) != nil {
			return ev, false
		}
		ev.Text = termsafe.Clean(p.Text)
	case EventTrace:
		var p struct {
			Trace Trace `json:"trace"`
		}
		if json.Unmarshal([]byte(data), &p) != nil {
			return ev, false
		}
		cleanTrace(&p.Trace)
		ev.Trace = &p.Trace
	case EventDone:
		var d Done
		if json.Unmarshal([]byte(data), &d) != nil {
			return ev, false
		}
		cleanMessage(&d.Message)
		ev.Done = &d
	case EventApproval, EventApprovalResolved:
		var a Approval
		if json.Unmarshal([]byte(data), &a) != nil || a.ID == "" {
			return ev, false
		}
		cleanApproval(&a)
		ev.Approval = &a
	case EventError:
		var p struct {
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(data), &p) != nil {
			return ev, false
		}
		ev.Text = termsafe.Clip(p.Message, 600)
	default:
		return ev, false // an event this client does not know: ignore it
	}
	return ev, true
}

// maxLine bounds one line of the stream. A trace snapshot can be large, but no
// line of a real reply is this long: a server that sends one is broken, or is
// trying to use all the memory there is.
const maxLine = 16 << 20

var errLineTooLong = &BadReplyError{Detail: "a line of the reply was too long"}

// readSSE splits an event stream into (event, data) pairs. onComment, when set,
// is called with the text of each ": ..." line.
func readSSE(r io.Reader, fn func(event, data string), onComment func(string)) error {
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
		line, err := readBoundedLine(br)
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case trimmed == "" && line != "":
			dispatch()
		case strings.HasPrefix(trimmed, ":"):
			if onComment != nil {
				onComment(strings.TrimPrefix(trimmed, ":"))
			}
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

// readBoundedLine reads up to and including a newline, refusing a line longer
// than maxLine.
func readBoundedLine(br *bufio.Reader) (string, error) {
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > maxLine {
			return "", errLineTooLong
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return string(buf), err
	}
}
