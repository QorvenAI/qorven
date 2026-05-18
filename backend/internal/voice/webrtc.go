// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package voice

import (
	"context"
	"encoding/json"
	"encoding/base64"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// WebRTCSession manages a WebRTC voice connection.
// Signaling happens over WebSocket, media flows peer-to-peer.
type WebRTCSession struct {
	conn      *websocket.Conn
	mgr       *Manager
	agentChat func(ctx context.Context, agentID, msg string) (string, error)
	agentID   string
	mu        sync.Mutex
	cancelled bool
}

// SignalMessage is exchanged during WebRTC signaling.
type SignalMessage struct {
	Type      string `json:"type"`       // "offer", "answer", "ice-candidate", "audio", "text", "interrupt"
	SDP       string `json:"sdp,omitempty"`
	Candidate string `json:"candidate,omitempty"`
	Data      string `json:"data,omitempty"`
	Audio     []byte `json:"audio,omitempty"`
	AudioBase64 string `json:"audio_base64,omitempty"`
	Format    string `json:"format,omitempty"`
}

// HandleWebRTCSignaling handles the WebSocket signaling channel for WebRTC.
// Flow:
//   1. Client sends "offer" with SDP
//   2. Server responds with "answer" SDP
//   3. ICE candidates exchanged
//   4. Once connected, audio flows via WebRTC DataChannel
//   5. Server-side: audio → STT → agent → TTS → audio back via DataChannel
//
// For environments without full WebRTC (e.g., server-side Go without pion),
// this falls back to WebSocket audio streaming (same as realtime.go).
func (m *Manager) HandleWebRTCSignaling(agentChat func(ctx context.Context, agentID, msg string) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := r.URL.Query().Get("agent_id")
		if agentID == "" {
			http.Error(w, "agent_id required", 400)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("webrtc.ws.upgrade", "error", err)
			return
		}

		sess := &WebRTCSession{
			conn: conn, mgr: m, agentChat: agentChat, agentID: agentID,
		}
		go sess.run()
	}
}

func (s *WebRTCSession) run() {
	defer s.conn.Close()
	slog.Info("webrtc.session.start", "agent", s.agentID)

	for {
		_, msg, err := s.conn.ReadMessage()
		if err != nil {
			return
		}

		var signal SignalMessage
		if json.Unmarshal(msg, &signal) != nil {
			continue
		}

		switch signal.Type {
		case "offer":
			// Client sent WebRTC offer — respond with answer
			// In a full implementation, this would use pion/webrtc to create a peer connection.
			// For now, we acknowledge and fall back to WebSocket audio transport.
			s.send(SignalMessage{
				Type: "answer",
				Data: "websocket-fallback", // indicates we're using WS transport, not full WebRTC
			})
			slog.Info("webrtc.signaling.offer_received", "agent", s.agentID, "mode", "ws-fallback")

		case "ice-candidate":
			// ICE candidate from client — would forward to pion peer connection
			slog.Debug("webrtc.ice.candidate", "agent", s.agentID)

		case "audio":
			// Audio data received (via DataChannel or WS fallback)
			s.mu.Lock()
			s.cancelled = false
			s.mu.Unlock()
			audio := signal.Audio
			if len(audio) == 0 && signal.AudioBase64 != "" {
				if d, err := base64.StdEncoding.DecodeString(signal.AudioBase64); err == nil { audio = d }
			}
			if len(audio) > 0 { go s.processAudio(audio, signal.Format) }

		case "text":
			// Text input (skip STT)
			s.mu.Lock()
			s.cancelled = false
			s.mu.Unlock()
			go s.processText(signal.Data)

		case "interrupt":
			s.mu.Lock()
			s.cancelled = true
			s.mu.Unlock()
			s.send(SignalMessage{Type: "interrupted"})
		}
	}
}

func (s *WebRTCSession) processAudio(audio []byte, format string) {
	if !s.mgr.HasSTT() {
		s.send(SignalMessage{Type: "error", Data: "no STT provider"})
		return
	}

	text, err := s.mgr.Transcribe(context.Background(), audio, format)
	if err != nil {
		s.send(SignalMessage{Type: "error", Data: err.Error()})
		return
	}
	s.send(SignalMessage{Type: "transcript", Data: text})
	s.processText(text)
}

func (s *WebRTCSession) processText(text string) {
	s.send(SignalMessage{Type: "thinking"})

	resp, err := s.agentChat(context.Background(), s.agentID, text)
	if err != nil {
		s.send(SignalMessage{Type: "error", Data: err.Error()})
		return
	}
	s.send(SignalMessage{Type: "response", Data: resp})

	// Check interruption before TTS
	s.mu.Lock()
	if s.cancelled {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	if s.mgr.HasTTS() {
		audio, err := s.mgr.Synthesize(context.Background(), resp, TTSOptions{Format: "opus"})
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.cancelled {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		s.send(SignalMessage{Type: "audio", Audio: audio.Audio, Format: audio.Extension})
	}
}

func (s *WebRTCSession) send(msg SignalMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn.WriteJSON(msg)
}
