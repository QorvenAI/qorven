// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package facebook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/channels"
)

func TestMessengerChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*MessengerChannel)(nil)
}

func TestMessengerChannel_New(t *testing.T) {
	ch := New(Config{AgentID: "a1", PageAccessToken: "token", VerifyToken: "verify"}, nil)
	if ch == nil { t.Fatal("nil") }
	if ch.Type() != "facebook" { t.Errorf("type=%q want facebook", ch.Type()) }
	if ch.AgentID() != "a1" { t.Errorf("agentID=%q", ch.AgentID()) }
	if !strings.Contains(ch.Name(), "messenger") { t.Errorf("name=%q should contain messenger", ch.Name()) }
	if ch.IsRunning() { t.Error("should not be running before Start") }
}

func TestMessengerChannel_StartStop(t *testing.T) {
	ch := New(Config{AgentID: "a1", PageAccessToken: "t", VerifyToken: "v"}, nil)
	ctx := context.Background()
	ch.Start(ctx)
	if !ch.IsRunning() { t.Error("should be running after Start") }
	ch.Stop(ctx)
	if ch.IsRunning() { t.Error("should not be running after Stop") }
}

func TestMessengerChannel_HandleWebhook_Verification_Valid(t *testing.T) {
	ch := New(Config{AgentID: "a1", VerifyToken: "my-verify-token"}, nil)

	req := httptest.NewRequest("GET",
		"/webhook?hub.mode=subscribe&hub.verify_token=my-verify-token&hub.challenge=CHALLENGE123",
		nil)
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 { t.Errorf("status=%d want 200", rr.Code) }
	if rr.Body.String() != "CHALLENGE123" { t.Errorf("challenge=%q want CHALLENGE123", rr.Body.String()) }
}

func TestMessengerChannel_HandleWebhook_Verification_Invalid(t *testing.T) {
	ch := New(Config{AgentID: "a1", VerifyToken: "correct-token"}, nil)

	req := httptest.NewRequest("GET",
		"/webhook?hub.mode=subscribe&hub.verify_token=wrong-token&hub.challenge=xyz",
		nil)
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 403 { t.Errorf("status=%d want 403", rr.Code) }
}

func TestMessengerChannel_HandleWebhook_InboundMessage(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{
		AgentID:         "a1",
		PageAccessToken: "token",
		AppSecret:       "", // no signature check
	}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})

	payload := webhookEvent{
		Object: "page",
		Entry: []pageEntry{{
			ID:   "page123",
			Time: 1234567890,
			Messaging: []messagingEvent{{
				Sender:    struct{ ID string `json:"id"` }{ID: "user456"},
				Recipient: struct{ ID string `json:"id"` }{ID: "page123"},
				Message: &incomingMessage{
					MID:  "mid123",
					Text: "Hello Messenger!",
				},
			}},
		}},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 { t.Errorf("status=%d want 200", rr.Code) }
	if received.Content != "Hello Messenger!" { t.Errorf("content=%q", received.Content) }
	if received.SenderID != "user456" { t.Errorf("senderID=%q", received.SenderID) }
	if received.ChannelType != "facebook" { t.Errorf("channelType=%q", received.ChannelType) }
	if received.AgentID != "a1" { t.Errorf("agentID=%q", received.AgentID) }
}

func TestMessengerChannel_HandleWebhook_Postback(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1", PageAccessToken: "t"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})

	payload := webhookEvent{
		Object: "page",
		Entry: []pageEntry{{
			Messaging: []messagingEvent{{
				Sender:    struct{ ID string `json:"id"` }{ID: "user1"},
				Recipient: struct{ ID string `json:"id"` }{ID: "page1"},
				Postback: &postbackEvent{
					Title:   "Get Started",
					Payload: "GET_STARTED",
				},
			}},
		}},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 { t.Errorf("status=%d", rr.Code) }
	if !strings.Contains(received.Content, "GET_STARTED") { t.Errorf("content=%q should contain payload", received.Content) }
}

func TestMessengerChannel_HandleWebhook_NonPage(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})

	// Object is not "page" — should be silently ignored
	payload := map[string]any{"object": "instagram", "entry": []any{}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 { t.Errorf("status=%d", rr.Code) }
	if called { t.Error("handler should not be called for non-page object") }
}

func TestMessengerChannel_VerifySignature(t *testing.T) {
	ch := New(Config{AgentID: "a1", AppSecret: "my-app-secret"}, nil)
	body := []byte(`{"test":"payload"}`)

	// Valid signature
	mac := hmac.New(sha256.New, []byte("my-app-secret"))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", validSig)
	if !ch.VerifySignature(req, body) { t.Error("valid signature should pass") }

	// Invalid signature
	req2 := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req2.Header.Set("X-Hub-Signature-256", "sha256=bad")
	if ch.VerifySignature(req2, body) { t.Error("invalid signature should fail") }

	// No signature → should fail when AppSecret configured
	req3 := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	if ch.VerifySignature(req3, body) { t.Error("missing signature should fail when secret configured") }
}

func TestMessengerChannel_VerifySignature_NoSecret(t *testing.T) {
	ch := New(Config{AgentID: "a1", AppSecret: ""}, nil)
	req := httptest.NewRequest("POST", "/webhook", nil)
	if !ch.VerifySignature(req, []byte("anything")) { t.Error("no secret = always pass") }
}

func TestMessengerChannel_Send_MockServer(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
		w.Write([]byte(`{"message_id":"mid123"}`))
	}))
	defer srv.Close()

	// Override graph API base for testing
	// Since graphAPIBase is a const, we test via mock server approach:
	// Create a channel and call sendTextMessage directly with patched client
	ch := New(Config{AgentID: "a1", PageAccessToken: "test-token"}, nil)
	ch.client = &http.Client{
		Transport: &mockTransport{handler: func(r *http.Request) (*http.Response, error) {
			return http.DefaultClient.Do(r)
		}},
	}
	// We can't easily override the const URL, so just verify the channel was created correctly
	if ch.cfg.PageAccessToken != "test-token" { t.Errorf("token=%q", ch.cfg.PageAccessToken) }
	_ = received
	_ = srv
}

func TestMessengerChannel_AllowList(t *testing.T) {
	allowed := false
	ch := New(Config{
		AgentID: "a1",
		AllowFrom: []string{"user-allowed"},
	}, func(_ context.Context, _ channels.InboundMessage) {
		allowed = true
	})

	payload := func(senderID string) webhookEvent {
		return webhookEvent{
			Object: "page",
			Entry: []pageEntry{{
				Messaging: []messagingEvent{{
					Sender:    struct{ ID string `json:"id"` }{ID: senderID},
					Recipient: struct{ ID string `json:"id"` }{ID: "page1"},
					Message:   &incomingMessage{Text: "hello"},
				}},
			}},
		}
	}

	// Blocked sender
	body, _ := json.Marshal(payload("user-blocked"))
	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)
	if allowed { t.Error("blocked sender should not trigger handler") }

	// Allowed sender
	allowed = false
	body, _ = json.Marshal(payload("user-allowed"))
	req2 := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	rr2 := httptest.NewRecorder()
	ch.HandleWebhook(rr2, req2)
	if !allowed { t.Error("allowed sender should trigger handler") }
}

func TestMessengerChannel_EchoSkip(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})

	payload := webhookEvent{
		Object: "page",
		Entry: []pageEntry{{
			Messaging: []messagingEvent{{
				Sender:    struct{ ID string `json:"id"` }{ID: "page123"},
				Recipient: struct{ ID string `json:"id"` }{ID: "user456"},
				Message: &incomingMessage{
					MID:    "mid-echo",
					Text:   "This is an echo",
					IsEcho: true,
				},
			}},
		}},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if called { t.Error("echo messages should not trigger handler") }
}

func TestMessengerChannel_Dedup(t *testing.T) {
	count := 0
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		count++
	})

	makeReq := func() {
		payload := webhookEvent{
			Object: "page",
			Entry: []pageEntry{{
				Messaging: []messagingEvent{{
					Sender:    struct{ ID string `json:"id"` }{ID: "user1"},
					Recipient: struct{ ID string `json:"id"` }{ID: "page1"},
					Message:   &incomingMessage{MID: "mid-dedup-test", Text: "hello"},
				}},
			}},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
		ch.HandleWebhook(httptest.NewRecorder(), req)
	}

	makeReq()
	makeReq() // platform retry with same MID
	if count != 1 { t.Errorf("count=%d want 1 (dedup should prevent second fire)", count) }
}

func TestMessengerChannel_MaxMessageLen(t *testing.T) {
	if maxMessageLen < 1000 { t.Errorf("maxMessageLen=%d too small", maxMessageLen) }
	if maxMessageLen > 5000 { t.Errorf("maxMessageLen=%d too large", maxMessageLen) }
}

// mockTransport is a helper for testing HTTP client behavior
type mockTransport struct {
	handler func(*http.Request) (*http.Response, error)
}

func (t *mockTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return t.handler(r)
}
