// Package voice is the audio half of Cal Live: speech to text, and text back
// to speech. It knows nothing about chats or HTTP — the httpapi layer puts the
// two ends around an ordinary agent turn.
package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// Groq's transcription endpoint speaks the OpenAI audio API.
const (
	transcribeURL = "https://api.groq.com/openai/v1/audio/transcriptions"

	// TranscribeModel is what Cal listens with when there is no Mistral key.
	TranscribeModel = "whisper-large-v3-turbo"
)

// Mistral's speech endpoint is what Cal talks with. It answers with mp3, base64
// encoded inside a small JSON envelope — never a file on disk.
const (
	speechURL = "https://api.mistral.ai/v1/audio/speech"

	// SpeechModel is Mistral's speech model.
	SpeechModel = "voxtral-mini-tts-2603"
	// SpeechVoice is the built-in voice. Only Paul has English voices, so the
	// emotional variants are the choice; neutral is the one to read answers with.
	SpeechVoice = "en_paul_neutral"
)

// httpClient is long enough for a slow transcription of a long answer.
var httpClient = &http.Client{Timeout: 90 * time.Second}

// Transcribe turns one audio clip into text. The clip is whatever the browser
// recorded — Chrome hands over webm/opus — so it is uploaded as-is.
func Transcribe(ctx context.Context, audio []byte, filename, key string) (string, error) {
	if len(audio) == 0 {
		return "", errors.New("voice: nothing was recorded")
	}
	if key == "" {
		return "", errors.New("voice: no Groq key to listen with")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("voice: prepare audio: %w", err)
	}
	if _, err := part.Write(audio); err != nil {
		return "", fmt.Errorf("voice: prepare audio: %w", err)
	}
	if err := writer.WriteField("model", TranscribeModel); err != nil {
		return "", fmt.Errorf("voice: prepare audio: %w", err)
	}
	if err := writer.WriteField("response_format", "json"); err != nil {
		return "", fmt.Errorf("voice: prepare audio: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("voice: prepare audio: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, transcribeURL, &body)
	if err != nil {
		return "", fmt.Errorf("voice: prepare transcription: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+key)

	res, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("voice: transcription failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("voice: %s", upstreamError(res.Body, res.StatusCode))
	}
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("voice: read transcript: %w", err)
	}
	return strings.TrimSpace(payload.Text), nil
}

// Sentence is one stretch of the reply, with the audio for it. Sentences travel
// as base64 so a turn answers with text and speech in one response.
type Sentence struct {
	Text  string `json:"text"`
	Audio string `json:"audio,omitempty"`
}

// Speaker reads answers aloud with Mistral's speech model.
type Speaker struct {
	key   string
	voice string
}

// NewSpeaker returns the speaker. Without a Mistral key it cannot speak, and
// every request says so.
func NewSpeaker(key string) Speaker {
	return Speaker{key: key, voice: SpeechVoice}
}

// Stream reads text out loud and hands each sentence over as soon as it is
// spoken, so the first words are on their way while the rest is still being
// generated. Cancelling the context ends the stream early.
func (s Speaker) Stream(ctx context.Context, text string, emit func(Sentence) error) error {
	if s.key == "" {
		return errors.New("voice: no Mistral key to speak with")
	}

	for _, chunk := range Sentences(text) {
		if err := ctx.Err(); err != nil {
			return err
		}
		audio, err := s.speak(ctx, chunk)
		if err != nil {
			return err
		}
		if err := emit(Sentence{Text: chunk, Audio: audio}); err != nil {
			return err
		}
	}
	return nil
}

// speak turns one sentence into base64 mp3.
func (s Speaker) speak(ctx context.Context, text string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"model": SpeechModel,
		"voice": s.voice,
		"input": text,
	})
	if err != nil {
		return "", fmt.Errorf("voice: prepare speech: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, speechURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("voice: prepare speech: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.key)

	res, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("voice: speech failed: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("voice: %s", upstreamError(res.Body, res.StatusCode))
	}

	var spoken struct {
		Audio string `json:"audio_data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&spoken); err != nil {
		return "", fmt.Errorf("voice: read speech: %w", err)
	}
	if spoken.Audio == "" {
		return "", errors.New("voice: the speech service returned no audio")
	}
	return spoken.Audio, nil
}

// upstreamError keeps a provider's message, which says what went wrong (a bad
// model name, a missing key) better than a status code alone.
func upstreamError(body io.Reader, status int) string {
	raw, err := io.ReadAll(io.LimitReader(body, 400))
	message := strings.TrimSpace(string(raw))
	if err != nil || message == "" {
		return fmt.Sprintf("audio request failed (%d)", status)
	}
	return fmt.Sprintf("audio request failed (%d): %s", status, message)
}
