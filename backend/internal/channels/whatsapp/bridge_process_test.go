package whatsapp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/qorvenai/qorven/internal/channels"
)

func TestBridgeProcess_WSServer(t *testing.T) {
	bp := NewBridgeProcess("test-channel-id", "/tmp", "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	port, err := bp.StartServer(ctx)
	if err != nil {
		t.Fatalf("StartServer: %v", err)
	}

	url := "ws://127.0.0.1" + port + "/ws"
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, http.Header{"X-Instance-Id": {"test-channel-id"}})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	qrCh := make(chan string, 1)
	bp.SubscribeQR(func(qr string) { qrCh <- qr })

	conn.WriteJSON(map[string]string{"type": "qr", "qr": "data:image/png;base64,iVBORw=="})

	select {
	case qr := <-qrCh:
		if !strings.HasPrefix(qr, "data:image/png") {
			t.Errorf("unexpected QR: %s", qr)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for QR fanout")
	}
}

func TestBridgeProcess_Send(t *testing.T) {
	bp := NewBridgeProcess("test-channel-id", "/tmp", "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	port, _ := bp.StartServer(ctx)
	url := "ws://127.0.0.1" + port + "/ws"
	conn, _, _ := websocket.DefaultDialer.DialContext(ctx, url, http.Header{"X-Instance-Id": {"test-channel-id"}})
	defer conn.Close()

	// Send a ping so the server registers us as a client; wait for the pong
	// reply before calling bp.Send so that pong is drained and the next read
	// will be the "send" command we care about.
	conn.WriteJSON(map[string]string{"type": "ping"})
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, pongRaw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read pong: %v", err)
	}
	var pong map[string]string
	json.Unmarshal(pongRaw, &pong)
	if pong["type"] != "pong" {
		t.Fatalf("expected pong, got: %s", string(pongRaw))
	}

	if err := bp.Send("91XX@s.whatsapp.net", "hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Read messages until we get type=send (guards against any extra frames).
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("ReadMessage: %v", err)
		}
		var cmd map[string]string
		json.Unmarshal(raw, &cmd)
		if cmd["type"] == "send" {
			if cmd["to"] != "91XX@s.whatsapp.net" || cmd["text"] != "hello" {
				t.Errorf("unexpected cmd: %v", cmd)
			}
			return
		}
		// Any other frame (e.g. an unexpected extra pong) — keep reading.
	}
}

func TestWhatsApp_OTPChallenge_StrangerGetsChallengeSent(t *testing.T) {
	otpSent := make(chan string, 1)

	handler := func(ctx context.Context, msg channels.InboundMessage) {
		t.Error("handler called for unapproved stranger")
	}

	cfg := Config{
		AgentID:   "agent1",
		Mode:      "bridge",
		DMPolicy:  "allowlist",
		AllowFrom: []string{"owner@s.whatsapp.net"}, // non-empty so strangers are blocked
	}
	ch := New(cfg, handler)
	// Override OTP send to capture the OTP
	ch.sendOTPMessage = func(ctx context.Context, to, otp string) {
		otpSent <- otp
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch.handleBridgeProcessMessage(ctx, BridgeMessage{
		ID: "m1", From: "99XX@s.whatsapp.net",
		FromName: "Stranger", Chat: "99XX@s.whatsapp.net", Body: "hi",
	})

	select {
	case otp := <-otpSent:
		if len(otp) != 6 {
			t.Errorf("expected 6-digit OTP, got %q", otp)
		}
	case <-ctx.Done():
		t.Fatal("expected OTP to be sent, timed out")
	}
}

func TestWhatsAppChannel_BridgeMode_ReceivesMessage(t *testing.T) {
	received := make(chan string, 1)
	handler := func(ctx context.Context, msg channels.InboundMessage) {
		received <- msg.Content
	}

	cfg := Config{
		AgentID:  "test-agent",
		Mode:     "bridge",
		DMPolicy: "open",
	}
	ch := New(cfg, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bp := NewBridgeProcess("test-channel-id", "/tmp", "")
	port, _ := bp.StartServer(ctx)
	ch.bridgeProc = bp
	ch.mu.Lock()
	ch.running = true
	ch.mu.Unlock()

	bp.SubscribeMessages(func(m BridgeMessage) {
		ch.handleBridgeProcessMessage(context.Background(), m)
	})

	wsURL := "ws://127.0.0.1" + port + "/ws"
	conn, _, _ := websocket.DefaultDialer.DialContext(ctx, wsURL, http.Header{"X-Instance-Id": {"test-channel-id"}})
	defer conn.Close()

	conn.WriteJSON(map[string]any{
		"type": "message", "id": "msg1", "from": "91XX@s.whatsapp.net",
		"from_name": "Jay", "chat": "91XX@s.whatsapp.net", "body": "hello world",
	})

	select {
	case content := <-received:
		if content != "hello world" {
			t.Errorf("expected 'hello world', got %q", content)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for message")
	}
}
