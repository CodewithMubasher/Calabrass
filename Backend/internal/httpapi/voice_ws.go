package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/coder/websocket"

	"github.com/CodewithMubasher/Calabrass/backend/internal/voice"
)

// handleVoiceStream runs one live conversation over a WebSocket: PCM frames in,
// transcript, tokens and audio out. Nothing the microphone captures is ever
// written to disk.
func (s *Server) handleVoiceStream(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The dev server is another origin, and voice needs the microphone.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	session := voice.NewSession()
	defer session.Close()

	transcriber, err := voice.NewTranscriber(s.mistralKey, s.groqKey(r.Context()), s.log)
	if err != nil {
		writeEvent(ctx, conn, voice.Event{Type: "error", Message: err.Error()})
		return
	}
	events, err := transcriber.Stream(ctx, session.Audio())
	if err != nil {
		s.log.Warn("voice: listener failed to start", "error", err)
		writeEvent(ctx, conn, voice.Event{Type: "error", Message: err.Error()})
		return
	}

	var wait sync.WaitGroup

	// Transcript events become captions on the page.
	wait.Add(1)
	go func() {
		defer wait.Done()
		for event := range events {
			if event.Err != nil {
				session.Emit(voice.Event{Type: "error", Message: event.Err.Error()})
				continue
			}
			session.SetTranscript(event.Text)
			session.Emit(voice.Event{Type: "transcript", Text: event.Text, Final: event.Final})
		}
		// The listener is gone — an error, or the service hung up. Closing the
		// session lets the browser reconnect instead of leaving a dead one open.
		cancel()
	}()

	wait.Add(1)
	go func() {
		defer wait.Done()
		s.readVoice(ctx, conn, session, transcriber)
	}()

	// A turn also ends on its own once the words stop arriving. The page asks
	// for the turn when its own detector hears the visitor stop, and that
	// normally gets there first; this is the floor beneath it, so a question is
	// answered as long as it was heard — whatever the microphone's level.
	wait.Add(1)
	go func() {
		defer wait.Done()
		s.watchVoice(ctx, conn, session, transcriber)
	}()

	s.writeVoice(ctx, conn, session)
	wait.Wait()
}

// readVoice takes the microphone's frames and the browser's control messages.
func (s *Server) readVoice(ctx context.Context, conn *websocket.Conn, session *voice.Session, transcriber voice.Transcriber) {
	for {
		kind, data, err := conn.Read(ctx)
		if err != nil {
			return
		}

		switch kind {
		case websocket.MessageBinary:
			session.PushAudio(data)
		case websocket.MessageText:
			var message struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal(data, &message) != nil {
				continue
			}
			switch message.Type {
			case "utterance_end":
				// The listener is flushed so the tail of the question comes back
				// before the turn reads it — a batch listener transcribes now.
				session.Flushed()
				transcriber.Finalize()
				go s.answerVoice(ctx, conn, session)
			case "cancel":
				// The user started speaking over Cal. The listener is flushed too,
				// so the half-heard answer's audio closes its utterance and the
				// interruption starts a clean one.
				transcriber.Finalize()
				session.CancelTurn()
			case "speak":
				// The greeting: read aloud without it being a turn.
				if strings.TrimSpace(message.Text) != "" {
					go s.speakVoice(ctx, conn, message.Text)
				}
			}
		}
	}
}

// answerVoice answers one heard utterance, then reads the answer aloud a
// sentence at a time so the first words are heard while the rest is still being
// spoken. Only one turn runs at a time.
func (s *Server) answerVoice(ctx context.Context, conn *websocket.Conn, session *voice.Session) {
	if !session.BeginTurn() {
		return
	}
	defer session.EndTurn()

	// The flush brings the tail of the question back a moment later; the settle
	// wait lets it land before the turn reads it.
	transcript := session.Settle(ctx, 400*time.Millisecond, 2*time.Second, 5*time.Second)
	if transcript == "" {
		return
	}
	session.SetAnswered(transcript)

	chat, err := s.voiceChat(ctx, session.Chat())
	if err != nil {
		writeEvent(ctx, conn, voice.Event{Type: "error", Message: "Could not start a chat for that."})
		return
	}
	session.SetChat(chat.ID)

	// The conversation is a chat like any other: the sidebar shows it, and each
	// thing said in it is a message.
	if data, err := json.Marshal(chat.Chat); err == nil {
		writeEvent(ctx, conn, voice.Event{Type: "chat", Chat: data})
	}
	writeEvent(ctx, conn, voice.Event{Type: "user", User: transcript})

	turnCtx, cancel := context.WithCancel(ctx)
	session.SetTurnCancel(cancel)
	defer func() {
		cancel()
		session.SetTurnCancel(nil)
	}()

	reply, trace, err := s.voiceTurn(turnCtx, chat, transcript, func(delta string) {
		writeEvent(turnCtx, conn, voice.Event{Type: "token", Text: delta})
	})
	if err != nil {
		// A cancelled turn is the user cutting Cal off, not a failure.
		if turnCtx.Err() == nil {
			writeEvent(ctx, conn, voice.Event{Type: "error", Message: err.Error()})
		}
		return
	}

	if err := s.speaker.Stream(turnCtx, reply, func(sentence voice.Sentence) error {
		return writeEvent(turnCtx, conn, voice.Event{Type: "audio", Text: sentence.Text, Audio: sentence.Audio})
	}); err != nil {
		// A cancelled turn is the user interrupting, which is not an error.
		if turnCtx.Err() == nil {
			s.log.Warn("could not read the reply aloud", "error", err)
		}
		return
	}

	if len(trace.Steps) > 0 {
		if data, err := json.Marshal(trace); err == nil {
			writeEvent(ctx, conn, voice.Event{Type: "trace", Trace: data})
		}
	}
	if data, err := json.Marshal(chat.Chat); err == nil {
		writeEvent(ctx, conn, voice.Event{Type: "done", Text: reply, Chat: data})
	} else {
		writeEvent(ctx, conn, voice.Event{Type: "done", Text: reply})
	}
}

// speakVoice reads a line aloud without it being a turn: the greeting the stage
// opens with. The page keeps its detector muted until "spoken" arrives, so Cal's
// own voice cannot be mistaken for the visitor's and start a conversation with
// herself.
func (s *Server) speakVoice(ctx context.Context, conn *websocket.Conn, text string) {
	err := s.speaker.Stream(ctx, text, func(sentence voice.Sentence) error {
		return writeEvent(ctx, conn, voice.Event{Type: "audio", Text: sentence.Text, Audio: sentence.Audio})
	})
	if err != nil && ctx.Err() == nil {
		s.log.Warn("could not read the greeting aloud", "error", err)
	}
	// Sent either way: the page must never stay deaf because the voice failed.
	writeEvent(ctx, conn, voice.Event{Type: "spoken"})
}

// watchVoice ends a turn on its own once the transcript has stopped growing.
// Waiting for the page alone would tie every question to an energy detector
// reading a microphone it has never heard: this way the words themselves are
// proof that someone spoke, and the answer follows.
func (s *Server) watchVoice(ctx context.Context, conn *websocket.Conn, session *voice.Session, transcriber voice.Transcriber) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	// Words already on their way to a turn. The flush that follows sends them
	// back, and they are not a second question.
	var pending string

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			text, ok := session.HeardSpurt(1200 * time.Millisecond)
			if !ok {
				continue
			}
			if heardLike(pending, text) || heardLike(session.LastAnswered(), text) {
				continue
			}
			pending = text
			// The flush brings the tail of the utterance back; the turn that
			// follows reads the whole of it.
			session.Flushed()
			transcriber.Finalize()
			go s.answerVoice(ctx, conn, session)
		}
	}
}

// heardLike reports whether two transcripts are the same words, give or take
// the punctuation and the tail a flush adds to them.
func heardLike(answered, heard string) bool {
	before, after := words(answered), words(heard)
	if before == "" || after == "" {
		return false
	}
	return strings.Contains(before, after) || strings.Contains(after, before)
}

// words reduces a transcript to just its words, so the same sentence written
// differently still counts as the same sentence.
func words(text string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			continue
		}
		out.WriteRune(' ')
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

// writeVoice is the connection's only writer, so events never interleave.
func (s *Server) writeVoice(ctx context.Context, conn *websocket.Conn, session *voice.Session) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-session.Events():
			if err := writeEvent(ctx, conn, event); err != nil {
				return
			}
		}
	}
}

func writeEvent(ctx context.Context, conn *websocket.Conn, event voice.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}
