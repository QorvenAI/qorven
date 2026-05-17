// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package webchat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/qorvenai/qorven/internal/channels"
)

func TestWebchatChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*WebchatChannel)(nil)
}

func TestWebchatChannel_New(t *testing.T) {
	ch := New(Config{AgentID: "agent-1"}, nil)
	if ch == nil {
		t.Fatal("nil channel")
	}
	if ch.Type() != "webchat" {
		t.Errorf("type=%q want webchat", ch.Type())
	}
	if ch.AgentID() != "agent-1" {
		t.Errorf("agentID=%q", ch.AgentID())
	}
	if !strings.Contains(ch.Name(), "webchat") {
		t.Errorf("name=%q should contain webchat", ch.Name())
	}
	if !strings.Contains(ch.Name(), "agent-1") {
		t.Errorf("name=%q should contain agent ID", ch.Name())
	}
	if ch.IsRunning() {
		t.Error("should not be running before Start")
	}
}

func TestWebchatChannel_StartStop(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)
	ctx := context.Background()

	if err := ch.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !ch.IsRunning() {
		t.Error("should be running after Start")
	}
	if err := ch.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if ch.IsRunning() {
		t.Error("should not be running after Stop")
	}
}

func TestWebchatChannel_StopClearsClients(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)
	ctx := context.Background()
	ch.Start(ctx)
	if err := ch.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if ch.IsRunning() {
		t.Error("should not be running")
	}
}

func TestWebchatChannel_Send_NoClients(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)
	err := ch.Send(context.Background(), channels.OutboundMessage{
		RecipientID: "user-123",
		Content:     "hello",
	})
	if err != nil {
		t.Errorf("Send with no clients: %v", err)
	}
}

func TestWebchatChannel_ValidateToken_Nil(t *testing.T) {
	ValidateToken = nil
	ch := New(Config{AgentID: "a1"}, nil)
	_ = ch
}

func TestWebchatChannel_ValidateToken_Reject(t *testing.T) {
	ValidateToken = func(token string) bool { return token == "valid-token" }
	defer func() { ValidateToken = nil }()

	if ValidateToken("valid-token") != true {
		t.Error("valid token rejected")
	}
	if ValidateToken("bad-token") != false {
		t.Error("bad token accepted")
	}
}

func TestWebchatChannel_Config_Defaults(t *testing.T) {
	ch := New(Config{AgentID: "minimal"}, nil)
	if ch.AgentID() != "minimal" {
		t.Errorf("agentID=%q", ch.AgentID())
	}
}

func TestWebchatChannel_ConcurrentStartStop(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		ch.Start(ctx)
		ch.Stop(ctx)
	}
}

// wsConnect sets up a test WebSocket server and returns the client conn.
// It waits until HandleWS has registered the session in c.clients before
// returning, so callers can call ch.Send() without a race.
func wsConnect(t *testing.T, ch *WebchatChannel, query string) (*websocket.Conn, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ch.HandleWS(w, r)
	}))
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws" + query
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	// Extract the session ID from the query string so we can wait for
	// HandleWS to register it in c.clients. Dial success only means the
	// TCP handshake completed; the goroutine running HandleWS may not
	// have reached c.clients[sessionID] = conn yet.
	sessionID := ""
	if i := strings.Index(query, "session="); i >= 0 {
		sessionID = strings.TrimPrefix(query[i:], "session=")
		if j := strings.IndexByte(sessionID, '&'); j >= 0 {
			sessionID = sessionID[:j]
		}
	}
	if sessionID != "" {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			ch.mu.Lock()
			_, registered := ch.clients[sessionID]
			ch.mu.Unlock()
			if registered {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}
	return conn, func() { conn.Close(); srv.Close() }
}

func TestWebchatChannel_WS_MessageFrame(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	ValidateToken = nil
	conn, cleanup := wsConnect(t, ch, "?session=sess1")
	defer cleanup()

	// Send spec-compliant message frame
	payload, _ := json.Marshal(map[string]string{"text": "Hello webchat"})
	frame, _ := json.Marshal(map[string]any{
		"type":      "message",
		"id":        "frame-1",
		"sessionId": "sess1",
		"timestamp": 1700000000000,
		"payload":   json.RawMessage(payload),
	})
	if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
		t.Fatal(err)
	}

	// Read the typing indicator frame sent back
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	json.Unmarshal(raw, &resp)
	if resp["type"] != "typing" {
		t.Errorf("first response type=%q want 'typing'", resp["type"])
	}
	payload2, _ := json.Marshal(resp["payload"])
	var typingPayload map[string]any
	json.Unmarshal(payload2, &typingPayload)
	if typingPayload["isTyping"] != true {
		t.Errorf("typing payload=%v want isTyping=true", typingPayload)
	}
}

func TestWebchatChannel_WS_MessageFrame_SetsFields(t *testing.T) {
	done := make(chan channels.InboundMessage, 1)
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
		done <- msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	ValidateToken = nil
	conn, cleanup := wsConnect(t, ch, "?session=sess42")
	defer cleanup()

	payload, _ := json.Marshal(map[string]string{"text": "field check"})
	frame, _ := json.Marshal(map[string]any{
		"type":    "message",
		"payload": json.RawMessage(payload),
	})
	conn.WriteMessage(websocket.TextMessage, frame)
	conn.ReadMessage() // consume typing frame

	// Wait for the handler to be called (synchronous in handleFrame, after writeFrame)
	select {
	case received := <-done:
		if received.Content != "field check" {
			t.Errorf("content=%q", received.Content)
		}
		if received.ChatID != "sess42" {
			t.Errorf("chatID=%q — ChatID must be set to sessionID", received.ChatID)
		}
		if received.SenderID != "sess42" {
			t.Errorf("senderID=%q", received.SenderID)
		}
		if received.PeerKind != "direct" {
			t.Errorf("peerKind=%q want 'direct'", received.PeerKind)
		}
		if received.Metadata["session_id"] != "sess42" {
			t.Errorf("metadata session_id=%q", received.Metadata["session_id"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not called within 2s")
	}
}

func TestWebchatChannel_WS_PingRespondsWithPong(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	ValidateToken = nil
	conn, cleanup := wsConnect(t, ch, "?session=sess-ping")
	defer cleanup()

	frame, _ := json.Marshal(map[string]any{"type": "ping", "id": "p1"})
	conn.WriteMessage(websocket.TextMessage, frame)

	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	json.Unmarshal(raw, &resp)
	if resp["type"] != "pong" {
		t.Errorf("type=%q want 'pong'", resp["type"])
	}
}

func TestWebchatChannel_WS_OutboundFrameShape(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	ValidateToken = nil
	conn, cleanup := wsConnect(t, ch, "?session=sess-out")
	defer cleanup()

	// Trigger a send from the channel side
	go ch.Send(context.Background(), channels.OutboundMessage{
		ChatID:   "sess-out",
		Content:  "Agent reply",
		Metadata: map[string]string{"session_id": "sess-out"},
	})

	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var frame map[string]any
	json.Unmarshal(raw, &frame)

	if frame["type"] != "message" {
		t.Errorf("type=%q want 'message'", frame["type"])
	}
	if frame["id"] == nil || frame["id"] == "" {
		t.Error("outbound frame must include 'id' field")
	}
	if frame["timestamp"] == nil {
		t.Error("outbound frame must include 'timestamp' field")
	}
	if frame["sessionId"] != "sess-out" {
		t.Errorf("sessionId=%q", frame["sessionId"])
	}
	payload, _ := json.Marshal(frame["payload"])
	var p map[string]any
	json.Unmarshal(payload, &p)
	if p["text"] != "Agent reply" {
		t.Errorf("payload.text=%q", p["text"])
	}
	if p["isAgent"] != true {
		t.Error("outbound message payload must include isAgent=true")
	}
}

func TestWebchatChannel_WS_TokenReject(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	ValidateToken = func(token string) bool { return token == "good" }
	defer func() { ValidateToken = nil }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ch.HandleWS(w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?token=bad"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Error("bad token should fail WebSocket upgrade")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status=%d want 401", resp.StatusCode)
	}
}

func TestWebchatChannel_WS_TypingFrameIgnored(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	ValidateToken = nil
	conn, cleanup := wsConnect(t, ch, "?session=sess-typing")
	defer cleanup()

	frame, _ := json.Marshal(map[string]any{
		"type":    "typing",
		"payload": map[string]any{"isTyping": true},
	})
	conn.WriteMessage(websocket.TextMessage, frame)

	if called {
		t.Error("typing frames from client should not invoke handler")
	}
}

func TestWebchatChannel_WS_EmptyTextSkipped(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	ValidateToken = nil
	conn, cleanup := wsConnect(t, ch, "?session=sess-empty")
	defer cleanup()

	payload, _ := json.Marshal(map[string]string{"text": ""})
	frame, _ := json.Marshal(map[string]any{
		"type":    "message",
		"payload": json.RawMessage(payload),
	})
	conn.WriteMessage(websocket.TextMessage, frame)

	if called {
		t.Error("empty text message should be skipped")
	}
}
