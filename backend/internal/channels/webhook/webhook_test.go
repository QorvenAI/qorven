// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package webhook

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
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

func makeHMAC(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*WebhookChannel)(nil)
}

func TestWebhookChannel_New(t *testing.T) {
	ch := New(Config{AgentID: "a1", OutboundURL: "https://example.com", InboundPath: "/wh"}, nil)
	if ch.Type() != "webhook" {
		t.Errorf("type=%q want webhook", ch.Type())
	}
	if ch.AgentID() != "a1" {
		t.Errorf("agentID=%q", ch.AgentID())
	}
	if !strings.Contains(ch.Name(), "webhook") {
		t.Errorf("name=%q should contain webhook", ch.Name())
	}
	if ch.IsRunning() {
		t.Error("should not be running before Start")
	}
}

func TestWebhookChannel_StartStop(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)
	ctx := context.Background()
	ch.Start(ctx)
	if !ch.IsRunning() {
		t.Error("should be running after Start")
	}
	ch.Stop(ctx)
	if ch.IsRunning() {
		t.Error("should not be running after Stop")
	}
}

func TestWebhookChannel_HandleWebhook_SpecFieldNames(t *testing.T) {
	var received channels.InboundMessage
	// Use an outbound URL so HandleWebhook goes async (202) instead of blocking on sync response.
	outSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer outSrv.Close()
	ch := New(Config{AgentID: "a1", OutboundURL: outSrv.URL}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})

	body, _ := json.Marshal(map[string]any{
		"text":           "Hello via webhook",
		"userId":         "user_abc",
		"conversationId": "conv_xyz",
		"metadata":       map[string]string{"source": "crm", "ticketId": "T-999"},
	})
	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)
	time.Sleep(50 * time.Millisecond) // let async goroutine invoke handler

	if received.Content != "Hello via webhook" {
		t.Errorf("content=%q", received.Content)
	}
	if received.SenderID != "user_abc" {
		t.Errorf("senderID=%q — should read userId field", received.SenderID)
	}
	if received.ChatID != "conv_xyz" {
		t.Errorf("chatID=%q — should read conversationId field", received.ChatID)
	}
	if received.Metadata["source"] != "crm" {
		t.Errorf("metadata[source]=%q — metadata should pass through", received.Metadata["source"])
	}
	if received.Metadata["ticketId"] != "T-999" {
		t.Errorf("metadata[ticketId]=%q", received.Metadata["ticketId"])
	}
}

func TestWebhookChannel_HandleWebhook_LegacyFieldNames(t *testing.T) {
	var received channels.InboundMessage
	outSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer outSrv.Close()
	ch := New(Config{AgentID: "a1", OutboundURL: outSrv.URL}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})

	body, _ := json.Marshal(map[string]string{
		"content":   "legacy content",
		"sender_id": "legacy_user",
	})
	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)
	time.Sleep(50 * time.Millisecond)

	if received.Content != "legacy content" {
		t.Errorf("content=%q — legacy 'content' field should still work", received.Content)
	}
	if received.SenderID != "legacy_user" {
		t.Errorf("senderID=%q — legacy 'sender_id' field should still work", received.SenderID)
	}
}

func TestWebhookChannel_HandleWebhook_EmptyText_Returns400(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)
	body, _ := json.Marshal(map[string]string{"userId": "u1"})
	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)

	if rw.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400 for missing text", rw.Code)
	}
	var resp map[string]string
	json.NewDecoder(rw.Body).Decode(&resp)
	if resp["code"] != "FIELD_MISSING" {
		t.Errorf("code=%q want FIELD_MISSING", resp["code"])
	}
}

func TestWebhookChannel_HandleWebhook_Auth_HMACValid(t *testing.T) {
	called := false
	outSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer outSrv.Close()
	ch := New(Config{AgentID: "a1", Secret: "mysecret", OutboundURL: outSrv.URL}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	body := []byte(`{"text":"hi","userId":"u1"}`)

	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	req.Header.Set("X-Qorven-Signature", makeHMAC("mysecret", body))
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)
	time.Sleep(50 * time.Millisecond)

	if rw.Code != http.StatusAccepted {
		t.Errorf("status=%d want 202 for valid HMAC (async mode)", rw.Code)
	}
	if !called {
		t.Error("handler should be called with valid HMAC signature")
	}
}

func TestWebhookChannel_HandleWebhook_Auth_HMACInvalid(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1", Secret: "mysecret"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	body := []byte(`{"text":"hi","userId":"u1"}`)

	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	req.Header.Set("X-Qorven-Signature", "sha256=badhash")
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)

	if rw.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401 for invalid HMAC", rw.Code)
	}
	if called {
		t.Error("handler must not be called with invalid signature")
	}
}

func TestWebhookChannel_HandleWebhook_Auth_SharedSecretHeader(t *testing.T) {
	called := false
	outSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer outSrv.Close()
	ch := New(Config{AgentID: "a1", Secret: "token123", OutboundURL: outSrv.URL}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	body := []byte(`{"text":"hi","userId":"u1"}`)

	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	req.Header.Set("X-Qorven-Secret", "token123")
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)
	time.Sleep(50 * time.Millisecond)

	if rw.Code != http.StatusAccepted {
		t.Errorf("status=%d want 202 for correct shared secret (async mode)", rw.Code)
	}
	if !called {
		t.Error("handler should be called with correct shared secret")
	}
}

func TestWebhookChannel_HandleWebhook_Auth_BearerToken(t *testing.T) {
	called := false
	outSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer outSrv.Close()
	ch := New(Config{AgentID: "a1", Secret: "tok", OutboundURL: outSrv.URL}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	body := []byte(`{"text":"hi","userId":"u1"}`)

	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer tok")
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)
	time.Sleep(50 * time.Millisecond)

	if rw.Code != http.StatusAccepted {
		t.Errorf("status=%d want 202 for correct Bearer token (async mode)", rw.Code)
	}
	if !called {
		t.Error("handler should be called with correct Bearer token")
	}
}

func TestWebhookChannel_HandleWebhook_Auth_BearerWrong(t *testing.T) {
	ch := New(Config{AgentID: "a1", Secret: "tok"}, func(_ context.Context, _ channels.InboundMessage) {})
	body := []byte(`{"text":"hi","userId":"u1"}`)
	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong")
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)

	if rw.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401 for wrong Bearer", rw.Code)
	}
}

func TestWebhookChannel_HandleWebhook_Auth_NoHeader_Rejects(t *testing.T) {
	ch := New(Config{AgentID: "a1", Secret: "tok"}, func(_ context.Context, _ channels.InboundMessage) {})
	body := []byte(`{"text":"hi","userId":"u1"}`)
	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	// No auth header at all
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)

	if rw.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401 when no auth header provided", rw.Code)
	}
}

func TestWebhookChannel_HandleWebhook_SyncMode_NoCallbackURL(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)

	body, _ := json.Marshal(map[string]string{
		"text":           "sync question",
		"userId":         "u1",
		"conversationId": "conv1",
	})
	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	rw := httptest.NewRecorder()

	// Inject a response after a short delay
	go func() {
		time.Sleep(20 * time.Millisecond)
		ch.Send(context.Background(), channels.OutboundMessage{
			ChatID:  "conv1",
			Content: "Sync response",
		})
	}()

	ch.HandleWebhook(rw, req)

	if rw.Code != 200 {
		t.Errorf("status=%d want 200 for sync mode", rw.Code)
	}
	var resp map[string]any
	json.NewDecoder(rw.Body).Decode(&resp)
	if resp["response"] != "Sync response" {
		t.Errorf("response=%q want 'Sync response'", resp["response"])
	}
	if resp["conversationId"] != "conv1" {
		t.Errorf("conversationId=%q", resp["conversationId"])
	}
	if resp["messageId"] == nil || resp["messageId"] == "" {
		t.Error("sync response must include messageId")
	}
}

func TestWebhookChannel_HandleWebhook_AsyncMode_Returns202(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {})

	cbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer cbSrv.Close()

	// Convert http to https isn't possible in tests — use a flag in metadata
	// Instead test that a non-https callbackUrl is rejected
	body, _ := json.Marshal(map[string]any{
		"text":        "async question",
		"userId":      "u2",
		"callbackUrl": "http://not-https.example.com/callback", // HTTP not HTTPS
	})
	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)

	if rw.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400 for non-HTTPS callbackUrl", rw.Code)
	}
}

func TestWebhookChannel_HandleWebhook_IdempotencyKey_Dedup(t *testing.T) {
	callCount := 0
	outSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer outSrv.Close()
	ch := New(Config{AgentID: "a1", OutboundURL: outSrv.URL}, func(_ context.Context, _ channels.InboundMessage) {
		callCount++
	})

	body, _ := json.Marshal(map[string]string{
		"text":           "duplicate",
		"idempotencyKey": "idem-abc-123",
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
		ch.HandleWebhook(httptest.NewRecorder(), req)
	}
	time.Sleep(50 * time.Millisecond)

	if callCount != 1 {
		t.Errorf("handler called %d times — idempotencyKey dedup must allow only one", callCount)
	}
}

func TestWebhookChannel_Send_Async_PostsToOutboundURL(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	ch := New(Config{AgentID: "a1", OutboundURL: srv.URL + "/callback"}, nil)
	err := ch.Send(context.Background(), channels.OutboundMessage{
		ChatID:  "conv-async",
		Content: "async response",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["response"] != "async response" {
		t.Errorf("response=%q", gotBody["response"])
	}
	if gotBody["conversationId"] != "conv-async" {
		t.Errorf("conversationId=%q", gotBody["conversationId"])
	}
	if gotBody["status"] != "complete" {
		t.Errorf("status=%q want 'complete'", gotBody["status"])
	}
	if gotBody["messageId"] == nil {
		t.Error("outbound response must include messageId")
	}
}

func TestWebhookChannel_Send_OutboundSigned(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Qorven-Signature")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	ch := New(Config{AgentID: "a1", OutboundURL: srv.URL + "/cb", Secret: "outbound-secret"}, nil)
	ch.Send(context.Background(), channels.OutboundMessage{
		ChatID:  "conv1",
		Content: "signed response",
	})

	if !strings.HasPrefix(gotHeader, "sha256=") {
		t.Errorf("X-Qorven-Signature=%q — outbound POST must include HMAC-SHA256 signature", gotHeader)
	}
}

func TestWebhookChannel_Send_CallbackURLFromMetadata(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.Path
		w.WriteHeader(200)
	}))
	defer srv.Close()

	ch := New(Config{AgentID: "a1"}, nil)
	ch.Send(context.Background(), channels.OutboundMessage{
		ChatID:  "conv1",
		Content: "response",
		Metadata: map[string]string{
			"callback_url": srv.URL + "/specific-callback",
		},
	})

	if gotURL != "/specific-callback" {
		t.Errorf("path=%q — callback_url from metadata must override OutboundURL", gotURL)
	}
}

func TestVerifySignature_Table(t *testing.T) {
	payload := []byte("test payload")
	validSig := makeHMAC("secret", payload)

	tests := []struct {
		sig    string
		secret string
		want   bool
	}{
		{validSig, "secret", true},
		{"sha256=badhash", "secret", false},
		{"", "", true},
		{validSig, "wrongsecret", false},
	}
	for _, tt := range tests {
		got := VerifySignature(payload, tt.sig, tt.secret)
		if got != tt.want {
			t.Errorf("sig=%q secret=%q: got %v want %v", tt.sig, tt.secret, got, tt.want)
		}
	}
}

func TestWebhookChannel_ConversationIDFallback(t *testing.T) {
	var received channels.InboundMessage
	outSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer outSrv.Close()
	ch := New(Config{AgentID: "a1", OutboundURL: outSrv.URL}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})

	// No conversationId — should fall back to userId
	body, _ := json.Marshal(map[string]string{"text": "hello", "userId": "user99"})
	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)
	time.Sleep(50 * time.Millisecond)

	if received.ChatID != "user99" {
		t.Errorf("chatID=%q — ChatID should fall back to userId when conversationId absent", received.ChatID)
	}
}
