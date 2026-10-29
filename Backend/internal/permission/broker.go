package permission

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// AnswerTimeout is how long an approval waits for a person before it counts as
// a refusal. An agent must not sit on a half-finished job forever, and a
// request nobody answered is not consent.
const AnswerTimeout = 10 * time.Minute

// Request is an approval waiting for an answer. It is what every face shows:
// a native dialog on the desktop, an inline prompt in the terminal, a card on
// a phone.
type Request struct {
	ID        string `json:"id"`
	UserID    string `json:"-"`
	ChatID    string `json:"chatId"`
	Tool      string `json:"tool"`
	Tier      Tier   `json:"tier"`
	TierName  string `json:"tierName"`
	Summary   string `json:"summary"`
	Detail    string `json:"detail,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	// CanRemember is true when the person may answer "allow this kind of
	// command in this workspace" or "allow for this run". Tiers 3 and 4 are
	// asked every time.
	CanRemember bool      `json:"canRemember"`
	Pattern     string    `json:"pattern,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// Entry is one line of the audit trail: what was asked, and how it ended.
type Entry struct {
	Time       time.Time `json:"time"`
	UserID     string    `json:"userId"`
	ChatID     string    `json:"chatId"`
	Tool       string    `json:"tool"`
	Tier       Tier      `json:"tier"`
	Summary    string    `json:"summary"`
	Outcome    string    `json:"outcome"` // allowed, remembered, approved, refused, unattended-refused, timed-out
	Decision   Decision  `json:"decision,omitempty"`
	Unattended bool      `json:"unattended,omitempty"`
}

type pending struct {
	req    Request
	answer chan Decision
}

// Broker holds the approvals that are waiting and the answers people chose to
// remember. One Broker serves the whole host, so a request raised by a crew
// worker can be answered from any face.
type Broker struct {
	mu       sync.Mutex
	pending  map[string]*pending
	patterns map[string]bool // userID|workspace|pattern
	runs     map[string]bool // userID|chatID
	watchers map[chan struct{}]struct{}
	// Audit, when set, receives every decision that touched tier 2 or higher.
	Audit func(Entry)
	now   func() time.Time
	// Timeout overrides AnswerTimeout, for tests.
	Timeout time.Duration
}

// NewBroker returns an empty broker.
func NewBroker() *Broker {
	return &Broker{
		pending:  map[string]*pending{},
		patterns: map[string]bool{},
		runs:     map[string]bool{},
		watchers: map[chan struct{}]struct{}{},
		now:      time.Now,
		Timeout:  AnswerTimeout,
	}
}

// Session is the permission context one turn runs in. It is fixed when the
// turn is set up, so a tool that runs on a goroutine of its own, or a worker a
// crew starts, answers to the same rules as the turn that started it.
type Session struct {
	Mode      Mode
	UserID    string
	ChatID    string
	Workspace string
	Scope     Scope // used when Mode is Unattended
	Broker    *Broker
	// Notify is called, from the tool's goroutine, when an approval is raised
	// or settled, so a live stream can show it at once. It may be nil: the
	// request is still listed by the broker for any face to poll.
	Notify func(event string, req Request)
}

// Authorize returns nil when the action may run, or an error to hand the
// model. It blocks while a person decides.
func (s *Session) Authorize(ctx context.Context, act Action) error {
	if s == nil {
		// No session at all is a bug in the caller; refuse anything that
		// needs trust rather than run it.
		s = &Session{Mode: Unattended, Scope: DefaultScope()}
	}
	if act.Tier <= TierChange {
		return nil
	}

	if s.Mode == Unattended {
		if s.Scope.Allows(act) {
			s.audit(act, "allowed", "", true)
			return nil
		}
		s.audit(act, "unattended-refused", "", true)
		return &Denied{Action: act, Reason: "No one was available to approve it, and this run was not pre-approved for it. " +
			"A person can allow commands for a scheduled task when they set it up."}
	}

	if s.Broker == nil {
		return &Denied{Action: act, Reason: "No approval service is available."}
	}
	if s.Broker.remembered(s, act) {
		s.audit(act, "remembered", "", false)
		return nil
	}

	decision, err := s.Broker.await(ctx, s, act)
	switch {
	case errors.Is(err, errTimedOut):
		s.audit(act, "timed-out", "", false)
		return &Denied{Action: act, Reason: "Nobody answered the approval in time."}
	case err != nil:
		return err
	case decision == Deny:
		s.audit(act, "refused", Deny, false)
		return &Denied{Action: act, Reason: "The user refused it."}
	}
	s.audit(act, "approved", decision, false)
	return nil
}

func (s *Session) audit(act Action, outcome string, d Decision, unattended bool) {
	if s.Broker == nil || s.Broker.Audit == nil || act.Tier < TierExecute {
		return
	}
	s.Broker.Audit(Entry{
		Time: s.Broker.now(), UserID: s.UserID, ChatID: s.ChatID, Tool: act.Tool,
		Tier: act.Tier, Summary: act.Summary, Outcome: outcome, Decision: d, Unattended: unattended,
	})
}

var errTimedOut = errors.New("approval timed out")

func key(parts ...string) string {
	out := ""
	for _, p := range parts {
		out += p + "\x00"
	}
	return out
}

func (b *Broker) remembered(s *Session, act Action) bool {
	if !act.Tier.Remembered() {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.runs[key(s.UserID, s.ChatID)] {
		return true
	}
	return act.Pattern != "" && b.patterns[key(s.UserID, s.Workspace, act.Pattern)]
}

func (b *Broker) await(ctx context.Context, s *Session, act Action) (Decision, error) {
	id := newID()
	now := b.now()
	timeout := b.Timeout
	if timeout <= 0 {
		timeout = AnswerTimeout
	}
	req := Request{
		ID: id, UserID: s.UserID, ChatID: s.ChatID, Tool: act.Tool, Tier: act.Tier,
		TierName: act.Tier.String(), Summary: act.Summary, Detail: act.Detail,
		Workspace: s.Workspace, CanRemember: act.Tier.Remembered(), Pattern: act.Pattern,
		CreatedAt: now, ExpiresAt: now.Add(timeout),
	}
	p := &pending{req: req, answer: make(chan Decision, 1)}

	b.mu.Lock()
	b.pending[id] = p
	b.mu.Unlock()
	b.changed()
	if s.Notify != nil {
		s.Notify("approval", req)
	}

	defer func() {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
		b.changed()
		if s.Notify != nil {
			s.Notify("approval_resolved", req)
		}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case d := <-p.answer:
		b.remember(s, act, d)
		return d, nil
	case <-timer.C:
		return Deny, errTimedOut
	case <-ctx.Done():
		return Deny, ctx.Err()
	}
}

func (b *Broker) remember(s *Session, act Action, d Decision) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch d {
	case AllowPattern:
		if act.Tier.Remembered() && act.Pattern != "" {
			b.patterns[key(s.UserID, s.Workspace, act.Pattern)] = true
		}
	case AllowRun:
		if act.Tier.Remembered() {
			b.runs[key(s.UserID, s.ChatID)] = true
		}
	}
}

// ErrNotFound means no pending approval has that id for that person.
var ErrNotFound = errors.New("that approval is no longer waiting")

// ErrBadDecision means the answer is not one the broker knows.
var ErrBadDecision = errors.New("the answer must be allow_once, allow_pattern, allow_run or deny")

// Decide records a person's answer. An approval belongs to the person whose
// turn raised it; anyone else's id is simply not found.
func (b *Broker) Decide(userID, id string, d Decision) error {
	if !d.Valid() {
		return ErrBadDecision
	}
	b.mu.Lock()
	p, ok := b.pending[id]
	if !ok || p.req.UserID != userID {
		b.mu.Unlock()
		return ErrNotFound
	}
	// A remembered answer is only honoured where remembering is allowed; the
	// request already says so, and a stale client cannot widen it.
	if (d == AllowPattern || d == AllowRun) && !p.req.CanRemember {
		d = AllowOnce
	}
	delete(b.pending, id)
	b.mu.Unlock()

	p.answer <- d
	b.changed()
	return nil
}

// Pending lists a person's waiting approvals, oldest first.
func (b *Broker) Pending(userID string) []Request {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Request, 0, len(b.pending))
	for _, p := range b.pending {
		if p.req.UserID == userID {
			out = append(out, p.req)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].CreatedAt.Before(out[j-1].CreatedAt); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Forget drops what a person chose to remember. It is what "reset my
// permissions" calls.
func (b *Broker) Forget(userID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	prefix := userID + "\x00"
	for k := range b.patterns {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(b.patterns, k)
		}
	}
	for k := range b.runs {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(b.runs, k)
		}
	}
}

// Watch returns a channel that receives a signal whenever the set of waiting
// approvals changes, and a function that stops watching.
func (b *Broker) Watch() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	b.watchers[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.watchers, ch)
		b.mu.Unlock()
	}
}

func (b *Broker) changed() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.watchers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func newID() string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(raw)
}

// --- context plumbing --------------------------------------------------------

type ctxKey struct{}

// WithSession attaches a session to a context, for code that builds a turn
// further down the call stack than the handler that knows who is asking.
func WithSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// SessionFrom returns the session on ctx, or nil.
func SessionFrom(ctx context.Context) *Session {
	s, _ := ctx.Value(ctxKey{}).(*Session)
	return s
}
