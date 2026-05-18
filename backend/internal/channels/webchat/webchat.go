// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package webchat

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/qorvenai/qorven/internal/channels"
)

// Webchat — built-in channel; no external credentials required.
//
// WebSocket frame format (both directions):
//
//	{ "type": "message|typing|read|ping|pong|stream|stream_end|system|error",
//	  "id": "<uuid>", "sessionId": "<uuid>", "timestamp": <ms>,
//	  "payload": { ... } }
//
// Inbound payload (client → server, type="message"): { "text": "..." }
// Outbound payload (server → client, type="message"): { "text": "...", "isAgent": true }
// Typing payload: { "isTyping": true|false }

type Config struct {
	AgentID string `json:"agent_id"`
}

// ValidateToken is set by gateway to verify JWT/session tokens.
var ValidateToken func(token string) bool

type WebchatChannel struct {
	cfg     Config
	handler channels.InboundHandler
	clients map[string]*websocket.Conn
	running bool
	mu      sync.Mutex
}

func New(cfg Config, handler channels.InboundHandler) *WebchatChannel {
	return &WebchatChannel{cfg: cfg, handler: handler, clients: make(map[string]*websocket.Conn)}
}

func (c *WebchatChannel) Name() string    { return "webchat-" + c.cfg.AgentID }
func (c *WebchatChannel) Type() string    { return "webchat" }
func (c *WebchatChannel) AgentID() string { return c.cfg.AgentID }
func (c *WebchatChannel) IsRunning() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.running }

func (c *WebchatChannel) Start(_ context.Context) error {
	c.mu.Lock()
	c.running = true
	c.mu.Unlock()
	slog.Info("webchat.started", "agent", c.cfg.AgentID)
	return nil
}

func (c *WebchatChannel) Stop(_ context.Context) error {
	c.mu.Lock()
	c.running = false
	for id, conn := range c.clients {
		conn.Close()
		delete(c.clients, id)
	}
	c.mu.Unlock()
	return nil
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// wsFrame is the canonical frame shape for both directions.
type wsFrame struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	SessionID string          `json:"sessionId"`
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// HandleWS upgrades HTTP to WebSocket with auth check.
func (c *WebchatChannel) HandleWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.URL.Query().Get("t")
	}
	if ValidateToken != nil && !ValidateToken(token) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	c.mu.Lock()
	c.clients[sessionID] = conn
	c.mu.Unlock()
	slog.Info("webchat.connected", "session", sessionID)

	defer func() {
		c.mu.Lock()
		delete(c.clients, sessionID)
		c.mu.Unlock()
		conn.Close()
	}()

	// Keepalive: ping every 30s; if pong not received within 60s, close
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			c.mu.Lock()
			_, exists := c.clients[sessionID]
			c.mu.Unlock()
			if !exists {
				return
			}
			if conn.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		}
	}()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		c.handleFrame(conn, sessionID, raw, r.Context())
	}
}

func (c *WebchatChannel) handleFrame(conn *websocket.Conn, sessionID string, raw []byte, ctx context.Context) {
	var frame wsFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		return
	}

	switch frame.Type {
	case "message":
		var p struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(frame.Payload, &p); err != nil || p.Text == "" {
			return
		}
		c.writeFrame(conn, sessionID, "typing", map[string]any{"isTyping": true})
		if c.handler != nil {
			c.handler(ctx, channels.InboundMessage{
				ChannelName: c.Name(),
				ChannelType: "webchat",
				AgentID:     c.cfg.AgentID,
				SenderID:    sessionID,
				ChatID:      sessionID,
				Content:     p.Text,
				PeerKind:    "direct",
				Metadata:    map[string]string{"session_id": sessionID},
			})
		}
	case "ping":
		c.writeFrame(conn, sessionID, "pong", nil)
	case "typing", "read":
		// client presence frames — no agent action needed
	}
}

// writeFrame sends a spec-compliant frame to the given connection.
func (c *WebchatChannel) writeFrame(conn *websocket.Conn, sessionID, frameType string, payload any) error {
	frame := map[string]any{
		"type":      frameType,
		"id":        uuid.New().String(),
		"sessionId": sessionID,
		"timestamp": time.Now().UnixMilli(),
	}
	if payload != nil {
		frame["payload"] = payload
	}
	return conn.WriteJSON(frame)
}

// Send delivers a message to the WebSocket client for the given session.
func (c *WebchatChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	sessionID := msg.Metadata["session_id"]
	if sessionID == "" {
		sessionID = msg.RecipientID
	}
	if sessionID == "" {
		sessionID = msg.ChatID
	}

	c.mu.Lock()
	conn, exists := c.clients[sessionID]
	c.mu.Unlock()
	if !exists {
		return nil
	}

	return c.writeFrame(conn, sessionID, "message", map[string]any{
		"text":    msg.Content,
		"isAgent": true,
	})
}

// SendTyping sends a typing indicator to a specific session.
func (c *WebchatChannel) SendTyping(sessionID string, isTyping bool) {
	c.mu.Lock()
	conn, exists := c.clients[sessionID]
	c.mu.Unlock()
	if !exists {
		return
	}
	c.writeFrame(conn, sessionID, "typing", map[string]any{"isTyping": isTyping})
}
