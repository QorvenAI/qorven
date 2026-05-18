// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package voice

import (
	"context"
	"encoding/json"
	"encoding/base64"
	"log/slog"
	"strings"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Voice WS heartbeat — matches the /ws + /ws/realtime values in
// gateway/resilience.go. Can't import from there (that package depends
// on this one) so the constants are duplicated. Keep them in sync.
const (
	voiceWSPingInterval = 20 * time.Second
	voiceWSPongWait     = 40 * time.Second
	voiceWSWriteWait    = 10 * time.Second
)

// RealtimeSession handles a voice conversation over WebSocket.
// Flow: user audio → STT → agent loop → TTS → audio back
type RealtimeSession struct {
	conn      *websocket.Conn
	mgr       *Manager
	agentChat func(ctx context.Context, agentID, sessionID, msg string) (string, error)
	agentID   string
	mu        sync.Mutex
	cancelled bool
}

type voiceEvent struct {
	Type        string `json:"type"`
	Data        string `json:"data,omitempty"`
	Audio       []byte `json:"audio,omitempty"`
	AudioBase64 string `json:"audio_base64,omitempty"`
	Format      string `json:"format,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleRealtimeVoice upgrades to WebSocket and runs voice loop.
func (m *Manager) HandleRealtimeVoice(agentChat func(ctx context.Context, agentID, sessionID, msg string) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := r.URL.Query().Get("agent_id")
		if agentID == "" {
			agentID = "chief" // default to chief/prime agent
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("voice.ws.upgrade", "error", err)
			return
		}

		sess := &RealtimeSession{
			conn: conn, mgr: m, agentChat: agentChat, agentID: agentID,
		}
		go sess.run()
	}
}

func (s *RealtimeSession) run() {
	defer s.conn.Close()

	// Heartbeat. Browser voice sessions go idle during long TTS
	// playback — without pings we'd never notice a dropped NAT flow
	// until the next user audio chunk. SetReadDeadline + pong handler
	// is the gorilla-websocket idiom. The writer goroutine sends
	// pings on an interval that's half the pong-wait window.
	s.conn.SetReadDeadline(time.Now().Add(voiceWSPongWait))
	s.conn.SetPongHandler(func(string) error {
		s.conn.SetReadDeadline(time.Now().Add(voiceWSPongWait))
		return nil
	})
	stopPing := make(chan struct{})
	defer close(stopPing)
	go func() {
		ticker := time.NewTicker(voiceWSPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopPing:
				return
			case <-ticker.C:
				s.mu.Lock()
				err := s.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(voiceWSWriteWait))
				s.mu.Unlock()
				if err != nil {
					slog.Debug("voice.ws.ping_failed", "agent", s.agentID, "error", err)
					return
				}
			}
		}
	}()

	for {
		_, msg, err := s.conn.ReadMessage()
		if err != nil {
			return
		}

		var evt voiceEvent
		if json.Unmarshal(msg, &evt) != nil {
			continue
		}

		slog.Info("voice.ws.event", "type", evt.Type, "has_audio", len(evt.Audio) > 0, "has_base64", evt.AudioBase64 != "")
		switch evt.Type {
		case "audio":
			// User sent audio chunk — transcribe and respond
			audio := evt.Audio
			if len(audio) == 0 && evt.AudioBase64 != "" {
				decoded, err := base64.StdEncoding.DecodeString(evt.AudioBase64)
				if err == nil { audio = decoded }
			}
			if len(audio) > 0 {
				go s.handleAudio(audio, evt.Format)
			}
		case "text":
			// User sent text — skip STT, go straight to agent
			go s.handleText(evt.Data)
		case "interrupt":
			// User interrupted — cancel current TTS
			s.mu.Lock()
			s.cancelled = true
			s.mu.Unlock()
		}
	}
}

func (s *RealtimeSession) handleAudio(audio []byte, format string) {
	slog.Info("voice.handleAudio", "bytes", len(audio), "format", format)
	if !s.mgr.HasSTT() {
		s.send(voiceEvent{Type: "error", Data: "no STT provider"})
		return
	}

	// STT
	text, err := s.mgr.Transcribe(context.Background(), audio, format)
	slog.Info("voice.stt.result", "text", text, "err", err)
	if err != nil {
		s.send(voiceEvent{Type: "error", Data: err.Error()})
		return
	}
	s.send(voiceEvent{Type: "transcript", Data: text})
	text = BuildVoiceMessagePrefix(text)
	s.handleText(text)
}

func (s *RealtimeSession) handleText(text string) {
	s.mu.Lock()
	s.cancelled = false
	s.mu.Unlock()

	// Agent response
	s.send(voiceEvent{Type: "thinking"})
	slog.Info("voice.agent.calling", "agent", s.agentID, "text", text)
	resp, err := s.agentChat(context.Background(), s.agentID, "voice-"+s.agentID, text)
	slog.Info("voice.agent.result", "resp_len", len(resp), "err", err)
	if err != nil {
		s.send(voiceEvent{Type: "error", Data: err.Error()})
		return
	}
	s.send(voiceEvent{Type: "response", Data: resp})

	// Short vs long response handling
	spokenText := resp
	if len(resp) > 200 {
		// Long response: send redirect hint + speak summary
		s.send(voiceEvent{Type: "redirect", Data: s.agentID})
		// Truncate for TTS — speak first sentence or first 150 chars
		if idx := strings.IndexAny(resp, ".!?\n"); idx > 0 && idx < 200 {
			spokenText = resp[:idx+1] + " Check the chat for full details."
		} else {
			spokenText = resp[:150] + "... Check the chat for full details."
		}
	}

	// Check if interrupted before TTS
	s.mu.Lock()
	if s.cancelled {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	// TTS
	if s.mgr.HasTTS() {
		audio, err := s.mgr.Synthesize(context.Background(), FormatForSpeech(spokenText), TTSOptions{Format: "mp3"})
		if err != nil {
			slog.Warn("voice.tts.error", "error", err)
			return
		}
		// Check interruption again before sending audio
		s.mu.Lock()
		if s.cancelled {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		s.send(voiceEvent{Type: "audio", AudioBase64: base64.StdEncoding.EncodeToString(audio.Audio), Format: audio.Extension})
	}
}

func (s *RealtimeSession) send(evt voiceEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn.WriteJSON(evt)
}
