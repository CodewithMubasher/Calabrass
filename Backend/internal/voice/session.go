package voice

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// Event is one message on the live voice channel. Only the fields the type
// needs are set, so the wire stays small.
type Event struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	User    string          `json:"user,omitempty"`
	Audio   string          `json:"audio,omitempty"`
	Final   bool            `json:"final,omitempty"`
	Trace   json.RawMessage `json:"trace,omitempty"`
	Chat    json.RawMessage `json:"chat,omitempty"`
	Message string          `json:"message,omitempty"`
}

// Session holds one live conversation: the mic's bytes in, the transcript and
// Cal's voice out. Nothing here touches the disk.
type Session struct {
	events chan Event

	audio chan []byte

	mu         sync.Mutex
	transcript string
	heardAt    time.Time
	// awaiting is set when the listener has been asked to flush the utterance and
	// nothing has come back yet, so the text in hand is known to be incomplete.
	awaiting bool
	// answered is the last thing a turn was started on, so the flush that follows
	// a turn is not mistaken for the visitor saying it all over again.
	answered string

	chatMu sync.Mutex
	chatID string

	turnMu     sync.Mutex
	turnCancel func()
}

func NewSession() *Session {
	return &Session{
		events: make(chan Event, 64),
		audio:  make(chan []byte, 256),
	}
}

// SetChat remembers which chat this conversation belongs to, so the next turn
// continues it instead of starting over.
func (s *Session) SetChat(chatID string) {
	s.chatMu.Lock()
	s.chatID = chatID
	s.chatMu.Unlock()
}

func (s *Session) Chat() string {
	s.chatMu.Lock()
	defer s.chatMu.Unlock()
	return s.chatID
}

// Events is what the connection writes to the browser.
func (s *Session) Events() <-chan Event { return s.events }

// Audio is what the transcriber reads the mic from.
func (s *Session) Audio() <-chan []byte { return s.audio }

// PushAudio hands the listener one PCM frame from the microphone.
func (s *Session) PushAudio(pcm []byte) {
	select {
	case s.audio <- pcm:
	default:
		// A slow listener must not stall the microphone: drop the oldest frame.
		select {
		case <-s.audio:
		default:
		}
		select {
		case s.audio <- pcm:
		default:
		}
	}
}

// Emit sends one event to the browser, dropping it if the socket is behind.
func (s *Session) Emit(event Event) {
	select {
	case s.events <- event:
	default:
	}
}

// SetTranscript records what has been heard so far.
func (s *Session) SetTranscript(text string) {
	s.mu.Lock()
	s.transcript = text
	s.heardAt = time.Now()
	s.awaiting = false
	s.mu.Unlock()
}

// Flushed says the listener has been asked to flush the utterance. Until the
// answer to that flush arrives, the text already heard is known to be only part
// of the question.
func (s *Session) Flushed() {
	s.mu.Lock()
	s.awaiting = true
	s.mu.Unlock()
}

// TakeTranscript returns what has been heard and starts a new utterance.
func (s *Session) TakeTranscript() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	text := s.transcript
	s.transcript = ""
	return text
}

// SetAnswered remembers the words a turn was started on.
func (s *Session) SetAnswered(text string) {
	s.mu.Lock()
	s.answered = text
	s.mu.Unlock()
}

// LastAnswered is the last thing a turn answered.
func (s *Session) LastAnswered() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.answered
}

// HeardSpurt returns what has been heard, once nothing new has arrived for at
// least quiet: the words have stopped coming and the utterance is ready to read.
func (s *Session) HeardSpurt(quiet time.Duration) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.transcript == "" || s.heardAt.IsZero() || time.Since(s.heardAt) < quiet {
		return "", false
	}
	return s.transcript, true
}

// Settle waits for the whole question to have landed, then takes it. A flush
// brings the tail of an utterance back a moment later, so the text already in
// hand is not enough to answer with; after that it waits for the text to stop
// changing. It gives up after max — and after flushWait when nothing comes back
// at all, so a silent turn is not held up.
func (s *Session) Settle(ctx context.Context, quiet, flushWait, max time.Duration) string {
	deadline := time.Now().Add(max)
	flushDeadline := time.Now().Add(flushWait)

	for {
		s.mu.Lock()
		heard := s.transcript
		lastAt := s.heardAt
		awaiting := s.awaiting
		s.mu.Unlock()

		settled := heard != "" && time.Since(lastAt) >= quiet
		if awaiting && time.Now().Before(flushDeadline) {
			settled = false
		}
		if settled || !time.Now().Before(deadline) {
			return s.TakeTranscript()
		}

		select {
		case <-ctx.Done():
			return s.TakeTranscript()
		case <-time.After(40 * time.Millisecond):
		}
	}
}

// BeginTurn claims the right to answer. A second turn while one is running is
// refused, so two "the user stopped" messages cannot overlap.
func (s *Session) BeginTurn() bool {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	return s.turnCancel == nil
}

// EndTurn releases the turn for the next utterance.
func (s *Session) EndTurn() {
	s.turnMu.Lock()
	s.turnCancel = nil
	s.turnMu.Unlock()
}

// SetTurnCancel remembers how to stop the turn that is running, so a new
// utterance can cut it short.
func (s *Session) SetTurnCancel(cancel func()) {
	s.turnMu.Lock()
	s.turnCancel = cancel
	s.turnMu.Unlock()
}

// CancelTurn stops the running turn — the barge-in path.
func (s *Session) CancelTurn() {
	s.turnMu.Lock()
	cancel := s.turnCancel
	s.turnCancel = nil
	s.turnMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Close releases the session's channels.
func (s *Session) Close() {
	close(s.audio)
}
