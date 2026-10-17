package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// MistralRealtimeModel is Mistral's live transcription model: it streams partial
// text with sub-200ms latency, which is what makes the conversation feel live.
const MistralRealtimeModel = "voxtral-mini-transcribe-realtime-2602"

// voiceDebug prints every frame the transcription service sends, so the wire
// format can be read off a real session. Off unless CALABRASS_VOICE_DEBUG is set.
var voiceDebug = os.Getenv("CALABRASS_VOICE_DEBUG") != ""

func dumpFrame(data []byte) {
	if !voiceDebug {
		return
	}
	text := string(data)
	if len(text) > 400 {
		text = text[:400] + "…"
	}
	fmt.Fprintf(os.Stderr, "voice frame (%d bytes): %s\n", len(data), text)
}

// Transcriber turns a stream of PCM audio (16-bit mono) into transcript events.
// It stays open for the whole conversation, emitting partials as they arrive and
// one final event when the utterance ends.
type Transcriber interface {
	Stream(ctx context.Context, audio <-chan []byte) (<-chan TranscriptEvent, error)
	// Finalize ends the current utterance: a streaming listener flushes what it
	// is still holding back and answers with the final text, a batch one
	// transcribes what it has buffered.
	Finalize()
}

// TranscriptEvent is one piece of transcript: a partial, or the final text.
type TranscriptEvent struct {
	Text  string `json:"text"`
	Final bool   `json:"final,omitempty"`
	Err   error  `json:"-"`
}

// MistralTranscriber listens with Mistral's realtime transcription endpoint.
type MistralTranscriber struct {
	key string
	log *slog.Logger

	// The open stream, so Finalize can commit to it. Writes on a connection are
	// safe from several goroutines.
	mu   sync.Mutex
	conn *websocket.Conn

	// heard counts the mic bytes handed over, and flushed is that count as of the
	// last flush, so a flush with nothing new behind it is left out: the service
	// refuses one, and a refused flush is not worth the round trip.
	heard   atomic.Int64
	flushed atomic.Int64
}

func NewMistralTranscriber(key string, log *slog.Logger) *MistralTranscriber {
	return &MistralTranscriber{key: key, log: log}
}

func (m *MistralTranscriber) Finalize() {
	m.mu.Lock()
	conn := m.conn
	m.mu.Unlock()
	if conn == nil {
		return
	}
	if m.heard.Load() == m.flushed.Load() {
		return
	}

	// The endpoint holds the tail of an utterance back until the buffer is
	// flushed: without this the question arrives half-transcribed — "what is"
	// and no more — and the rest only arrives when the service times out. A flush
	// is what makes the tail, and the final text, arrive at once.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"input_audio.flush"}`)); err != nil {
		if m.log != nil {
			m.log.Warn("voice: could not flush the listener", "error", err)
		}
		return
	}
	m.flushed.Store(m.heard.Load())
}

func (m *MistralTranscriber) setConn(conn *websocket.Conn) {
	m.mu.Lock()
	m.conn = conn
	m.mu.Unlock()
}

func (m *MistralTranscriber) Stream(ctx context.Context, audio <-chan []byte) (<-chan TranscriptEvent, error) {
	if m.key == "" {
		return nil, errors.New("voice: no Mistral key to listen with")
	}

	url := "wss://api.mistral.ai/v1/audio/transcriptions/realtime?model=" + MistralRealtimeModel
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + m.key}},
	})
	if err != nil {
		if m.log != nil {
			m.log.Warn("voice: could not reach the realtime transcription service", "error", err)
		}
		return nil, fmt.Errorf("voice: reach the realtime transcription service: %w", err)
	}
	if m.log != nil {
		m.log.Info("voice: listening with the realtime transcription service", "model", MistralRealtimeModel)
	}
	m.setConn(conn)
	go func() {
		<-ctx.Done()
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()

	out := make(chan TranscriptEvent, 16)
	go func() {
		defer close(out)
		defer m.setConn(nil)
		defer conn.Close(websocket.StatusNormalClosure, "")

		// Mic bytes out, transcript events in.
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case chunk, ok := <-audio:
					if !ok {
						return
					}
					if err := conn.Write(ctx, websocket.MessageBinary, chunk); err != nil {
						return
					}
					m.heard.Add(int64(len(chunk)))
				}
			}
		}()

		var partial strings.Builder
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			dumpFrame(data)
			text, final, failed := parseRealtimeEvent(data)
			if failed != "" {
				// The service refuses a flush it has nothing to flush for. That is
				// a no-op, not the end of the conversation: ending the stream here
				// would take the answer down with it.
				if strings.Contains(strings.ToLower(failed), "flush") {
					if m.log != nil {
						m.log.Info("voice: the listener had nothing to flush")
					}
					continue
				}
				out <- TranscriptEvent{Err: errors.New("voice: " + failed)}
				return
			}
			if text == "" {
				continue
			}
			if final {
				// The closing frame carries the whole utterance, so it wins over
				// the pieces that have been accumulated.
				heard := strings.TrimSpace(partial.String())
				partial.Reset()
				if whole := strings.TrimSpace(text); len(whole) >= len(heard) {
					heard = whole
				}
				if heard == "" {
					continue
				}
				out <- TranscriptEvent{Text: heard, Final: true}
				continue
			}
			partial.WriteString(text)
			out <- TranscriptEvent{Text: strings.TrimSpace(partial.String())}
		}
	}()

	return out, nil
}

// parseRealtimeEvent reads one frame from the transcription service. The wire
// schema is deliberately tolerant: the events are named differently by the
// hosted endpoint and by open-weight deployments of the same model, so the text
// is taken from whichever field carries it.
func parseRealtimeEvent(data []byte) (text string, final bool, failure string) {
	var frame map[string]any
	if err := json.Unmarshal(data, &frame); err != nil {
		return "", false, ""
	}

	name := strings.ToLower(stringField(frame, "type", "event", "kind"))
	if strings.Contains(name, "error") {
		if message := errorMessage(frame); message != "" {
			return "", false, message
		}
	}

	text = firstString(frame, "text", "delta", "partial", "transcript")
	final = strings.Contains(name, "done") || strings.Contains(name, "final") ||
		strings.Contains(name, "complete") || boolField(frame, "final", "is_final")
	return text, final, ""
}

// errorMessage takes a failure from whichever shape the service used: a plain
// string, or the nested object some endpoints send.
func errorMessage(frame map[string]any) string {
	for _, field := range []string{"error", "message"} {
		switch value := frame[field].(type) {
		case string:
			if value != "" {
				return value
			}
		case map[string]any:
			if message, ok := value["message"].(string); ok && message != "" {
				return message
			}
		}
	}
	return ""
}

func firstString(frame map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := frame[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func stringField(frame map[string]any, keys ...string) string {
	return firstString(frame, keys...)
}

func boolField(frame map[string]any, keys ...string) bool {
	for _, key := range keys {
		if value, ok := frame[key].(bool); ok && value {
			return true
		}
	}
	return false
}

// GroqTranscriber is the fallback when there is no Mistral key: the utterance is
// held in memory and transcribed once, when it ends. Groq's Whisper is one-shot
// per clip, so there are no partials on this path.
type GroqTranscriber struct {
	key      string
	finalize chan struct{}
}

func NewGroqTranscriber(key string) *GroqTranscriber {
	return &GroqTranscriber{key: key, finalize: make(chan struct{}, 1)}
}

func (g *GroqTranscriber) Finalize() {
	select {
	case g.finalize <- struct{}{}:
	default:
	}
}

func (g *GroqTranscriber) Stream(ctx context.Context, audio <-chan []byte) (<-chan TranscriptEvent, error) {
	out := make(chan TranscriptEvent, 4)
	go func() {
		defer close(out)

		var buffer bytes.Buffer
		for {
			select {
			case <-ctx.Done():
				text, err := Transcribe(context.WithoutCancel(ctx), buffer.Bytes(), "utterance.wav", g.key)
				if err != nil {
					out <- TranscriptEvent{Err: fmt.Errorf("could not understand that: %w", err)}
					return
				}
				out <- TranscriptEvent{Text: strings.TrimSpace(text), Final: true}
				return

			case <-g.finalize:
				if buffer.Len() == 0 {
					continue
				}
				text, err := Transcribe(context.WithoutCancel(ctx), buffer.Bytes(), "utterance.wav", g.key)
				buffer.Reset()
				if err != nil {
					out <- TranscriptEvent{Err: fmt.Errorf("could not understand that: %w", err)}
					continue
				}
				out <- TranscriptEvent{Text: strings.TrimSpace(text), Final: true}

			case chunk, ok := <-audio:
				if !ok {
					return
				}
				buffer.Write(chunk)
			}
		}
	}()
	return out, nil
}

// NewTranscriber picks the listener: Mistral's realtime endpoint when there is a
// key for it, Groq's Whisper otherwise.
func NewTranscriber(mistralKey, groqKey string, log *slog.Logger) (Transcriber, error) {
	if strings.TrimSpace(mistralKey) != "" {
		return NewMistralTranscriber(mistralKey, log), nil
	}
	if strings.TrimSpace(groqKey) != "" {
		if log != nil {
			log.Info("voice: no Mistral key, listening with Groq Whisper", "note", "no live partials on this path")
		}
		return NewGroqTranscriber(groqKey), nil
	}
	return nil, errors.New("voice: no key to listen with (set MISTRAL_API_KEY or add a Groq key)")
}
