// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package voice

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

// OpenAI Realtime API — audio-to-audio without STT+TTS roundtrip.
// Single model processes audio natively: <500ms latency, emotion detection, interruption handling.
// Protocol: WebSocket at wss://api.openai.com/v1/realtime
// Reference: https://platform.openai.com/docs/api-reference/realtime

const (
	realtimeWSURL = "wss://api.openai.com/v1/realtime"
)

type OpenAIRealtimeSession struct {
	apiKey       string
	model        string // gpt-4o-realtime-preview, gpt-4o-mini-realtime-preview
	voiceName    string // alloy, ash, ballad, coral, echo, sage, shimmer, verse
	instructions string
	conn         *websocket.Conn
	mu           sync.Mutex
	closed       bool

	// Callbacks
	onAudio      func(audio []byte)       // called when model sends audio
	onTranscript func(text string)         // called when model sends text transcript
	onInputTranscript func(text string)    // called when user speech is transcribed
	onError      func(err error)
	onSpeechStart func()                   // VAD detected speech start
	onSpeechStop  func()                   // VAD detected speech stop
}

type RealtimeConfig struct {
	APIKey       string
	Model        string
	Voice        string
	Instructions string
	// VAD settings
	TurnDetection string  // "server_vad" or "semantic_vad"
	Threshold     float64 // 0.0-1.0, default 0.5
	SilenceMs     int     // silence duration to detect end of speech, default 200
}

func NewOpenAIRealtimeSession(cfg RealtimeConfig) *OpenAIRealtimeSession {
	if cfg.Model == "" { cfg.Model = "gpt-4o-realtime-preview" }
	if cfg.Voice == "" { cfg.Voice = "alloy" }
	if cfg.TurnDetection == "" { cfg.TurnDetection = "server_vad" }
	if cfg.Threshold == 0 { cfg.Threshold = 0.5 }
	if cfg.SilenceMs == 0 { cfg.SilenceMs = 200 }
	return &OpenAIRealtimeSession{
		apiKey: cfg.APIKey, model: cfg.Model, voiceName: cfg.Voice, instructions: cfg.Instructions,
	}
}

// SetCallbacks sets the event handlers. Call before Connect.
func (s *OpenAIRealtimeSession) SetCallbacks(
	onAudio func([]byte), onTranscript func(string), onInputTranscript func(string),
	onError func(error), onSpeechStart func(), onSpeechStop func(),
) {
	s.onAudio = onAudio
	s.onTranscript = onTranscript
	s.onInputTranscript = onInputTranscript
	s.onError = onError
	s.onSpeechStart = onSpeechStart
	s.onSpeechStop = onSpeechStop
}

// Connect establishes the WebSocket connection and configures the session.
func (s *OpenAIRealtimeSession) Connect(ctx context.Context) error {
	url := fmt.Sprintf("%s?model=%s", realtimeWSURL, s.model)
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: map[string][]string{
			"Authorization": {"Bearer " + s.apiKey},
			"OpenAI-Beta":   {"realtime=v1"},
		},
	})
	if err != nil { return fmt.Errorf("realtime connect: %w", err) }
	s.conn = conn

	// Wait for session.created
	_, data, err := conn.Read(ctx)
	if err != nil { return fmt.Errorf("realtime read session.created: %w", err) }
	var created struct{ Type string `json:"type"` }
	json.Unmarshal(data, &created)
	if created.Type != "session.created" {
		return fmt.Errorf("expected session.created, got %s", created.Type)
	}

	// Configure session
	sessionUpdate := map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"modalities":          []string{"text", "audio"},
			"voice":               s.voiceName,
			"input_audio_format":  "pcm16",
			"output_audio_format": "pcm16",
			"turn_detection": map[string]any{
				"type":                "server_vad",
				"threshold":           0.5,
				"silence_duration_ms": 200,
				"create_response":     true,
			},
			"input_audio_transcription": map[string]any{
				"model": "whisper-1",
			},
		},
	}
	if s.instructions != "" {
		sessionUpdate["session"].(map[string]any)["instructions"] = s.instructions
	}
	if err := s.sendJSON(ctx, sessionUpdate); err != nil { return err }

	// Start reading events
	go s.readLoop(ctx)

	slog.Info("openai.realtime.connected", "model", s.model, "voice", s.voiceName)
	return nil
}

// SendAudio sends PCM16 audio data to the model.
func (s *OpenAIRealtimeSession) SendAudio(ctx context.Context, pcm16Audio []byte) error {
	return s.sendJSON(ctx, map[string]any{
		"type":  "input_audio_buffer.append",
		"audio": base64.StdEncoding.EncodeToString(pcm16Audio),
	})
}

// CommitAudio commits the audio buffer (triggers response in manual VAD mode).
func (s *OpenAIRealtimeSession) CommitAudio(ctx context.Context) error {
	return s.sendJSON(ctx, map[string]string{"type": "input_audio_buffer.commit"})
}

// CancelResponse cancels the current in-progress response (for interruptions).
func (s *OpenAIRealtimeSession) CancelResponse(ctx context.Context) error {
	return s.sendJSON(ctx, map[string]string{"type": "response.cancel"})
}

// Close closes the WebSocket connection.
func (s *OpenAIRealtimeSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.conn != nil {
		s.conn.Close(websocket.StatusNormalClosure, "")
	}
}

// --- Internal ---

func (s *OpenAIRealtimeSession) readLoop(ctx context.Context) {
	for {
		s.mu.Lock()
		if s.closed { s.mu.Unlock(); return }
		s.mu.Unlock()

		_, data, err := s.conn.Read(ctx)
		if err != nil {
			if s.onError != nil { s.onError(err) }
			return
		}

		var event struct {
			Type       string `json:"type"`
			Delta      string `json:"delta"`
			Transcript string `json:"transcript"`
			Audio      string `json:"audio"`
			Error      *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal(data, &event)

		switch event.Type {
		case "response.audio.delta":
			if s.onAudio != nil && event.Delta != "" {
				audio, _ := base64.StdEncoding.DecodeString(event.Delta)
				s.onAudio(audio)
			}

		case "response.audio_transcript.delta":
			if s.onTranscript != nil && event.Delta != "" {
				s.onTranscript(event.Delta)
			}

		case "conversation.item.input_audio_transcription.completed":
			if s.onInputTranscript != nil && event.Transcript != "" {
				s.onInputTranscript(event.Transcript)
			}

		case "input_audio_buffer.speech_started":
			if s.onSpeechStart != nil { s.onSpeechStart() }

		case "input_audio_buffer.speech_stopped":
			if s.onSpeechStop != nil { s.onSpeechStop() }

		case "error":
			if s.onError != nil && event.Error != nil {
				s.onError(fmt.Errorf("realtime: %s", event.Error.Message))
			}

		case "session.updated", "response.created", "response.done",
			"response.audio.done", "response.audio_transcript.done",
			"conversation.item.created", "rate_limits.updated":
			// Expected events — no action needed

		default:
			slog.Debug("openai.realtime.event", "type", event.Type)
		}
	}
}

func (s *OpenAIRealtimeSession) sendJSON(ctx context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil { return err }
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.conn.Write(ctx, websocket.MessageText, data)
}
