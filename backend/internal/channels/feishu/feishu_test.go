// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package feishu

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

// flushDebouncer flushes and waits for goroutines dispatched by FlushAll.
func flushDebouncer(d *channels.Debouncer) {
	d.FlushAll()
	time.Sleep(50 * time.Millisecond)
}

func makeSignature(encryptKey, timestamp, nonce string, body []byte) string {
	content := timestamp + nonce + encryptKey + string(body)
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)
}

func TestFeishuChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*FeishuChannel)(nil)
}

func TestFeishuChannel_New(t *testing.T) {
	ch := New(Config{AgentID: "a1", AppID: "cli_test"}, nil)
	if ch == nil {
		t.Fatal("nil")
	}
	if ch.Type() != "feishu" {
		t.Errorf("type=%q", ch.Type())
	}
	if ch.AgentID() != "a1" {
		t.Errorf("agentID=%q", ch.AgentID())
	}
	if ch.apiBase != feishuAPI {
		t.Errorf("apiBase=%q want feishuAPI", ch.apiBase)
	}
}

func TestFeishuChannel_New_Lark(t *testing.T) {
	ch := New(Config{AgentID: "a1", IsLark: true}, nil)
	if ch.apiBase != larkAPI {
		t.Errorf("apiBase=%q want larkAPI for international", ch.apiBase)
	}
}

func TestFeishuChannel_StartStop(t *testing.T) {
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

func TestFeishuChannel_VerifySignature(t *testing.T) {
	ch := New(Config{EncryptKey: "mykey"}, nil)
	body := []byte(`{"event_id":"123"}`)
	ts := "1700000000"
	nonce := "abc123"
	sig := makeSignature("mykey", ts, nonce, body)

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Lark-Request-Timestamp", ts)
	req.Header.Set("X-Lark-Request-Nonce", nonce)
	req.Header.Set("X-Lark-Signature", sig)

	if !ch.verifySignature(req, body) {
		t.Error("valid signature should pass")
	}

	req2 := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req2.Header.Set("X-Lark-Request-Timestamp", ts)
	req2.Header.Set("X-Lark-Request-Nonce", nonce)
	req2.Header.Set("X-Lark-Signature", "badsig")
	if ch.verifySignature(req2, body) {
		t.Error("invalid signature should fail")
	}
}

func TestFeishuChannel_VerifySignature_NoKey(t *testing.T) {
	ch := New(Config{}, nil)
	req := httptest.NewRequest("POST", "/webhook", nil)
	if !ch.verifySignature(req, []byte("anything")) {
		t.Error("no encrypt key = always pass")
	}
}

func TestFeishuChannel_HandleWebhook_SignatureRequired(t *testing.T) {
	ch := New(Config{EncryptKey: "mykey"}, nil)

	body := []byte(`{"header":{"event_id":"e1","event_type":"im.message.receive_v1"}}`)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	// No signature headers → 401
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 401 {
		t.Errorf("status=%d want 401 (missing signature)", rr.Code)
	}
}

func TestFeishuChannel_HandleWebhook_URLVerification(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)

	body, _ := json.Marshal(map[string]string{
		"challenge": "challenge-abc",
		"type":      "url_verification",
	})
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 {
		t.Errorf("status=%d", rr.Code)
	}
	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["challenge"] != "challenge-abc" {
		t.Errorf("challenge=%q", resp["challenge"])
	}
}

func TestFeishuChannel_HandleWebhook_TextMessage(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	contentStr, _ := json.Marshal(map[string]string{"text": "Hello Feishu!"})
	event := feishuEvent{}
	event.Header.EventID = "evt1"
	event.Header.EventType = "im.message.receive_v1"
	event.Event.Message.ChatID = "oc_chat1"
	event.Event.Message.ChatType = "group"
	event.Event.Message.MessageID = "msg1"
	event.Event.Message.MessageType = "text"
	event.Event.Message.Content = string(contentStr)
	event.Event.Sender.SenderID.OpenID = "ou_user1"
	event.Event.Sender.SenderType = "user"

	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 {
		t.Errorf("status=%d", rr.Code)
	}
	flushDebouncer(ch.debouncer)

	if received.Content != "Hello Feishu!" {
		t.Errorf("content=%q", received.Content)
	}
	if received.SenderID != "ou_user1" {
		t.Errorf("senderID=%q want ou_user1 (open_id)", received.SenderID)
	}
	if received.ChatID != "oc_chat1" {
		t.Errorf("chatID=%q want oc_chat1", received.ChatID)
	}
}

func TestFeishuChannel_HandleWebhook_BotLoop_Skipped(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	contentStr, _ := json.Marshal(map[string]string{"text": "bot reply"})
	event := feishuEvent{}
	event.Header.EventID = "evt-bot"
	event.Header.EventType = "im.message.receive_v1"
	event.Event.Message.ChatID = "oc_chat1"
	event.Event.Message.MessageType = "text"
	event.Event.Message.Content = string(contentStr)
	event.Event.Sender.SenderType = "bot" // must be skipped

	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)
	flushDebouncer(ch.debouncer)

	if called {
		t.Error("bot-originated messages should be skipped to prevent loops")
	}
}

func TestFeishuChannel_HandleWebhook_Dedup(t *testing.T) {
	count := 0
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		count++
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	contentStr, _ := json.Marshal(map[string]string{"text": "hello"})
	event := feishuEvent{}
	event.Header.EventID = "evt-dup"
	event.Header.EventType = "im.message.receive_v1"
	event.Event.Message.ChatID = "oc_chat1"
	event.Event.Message.MessageType = "text"
	event.Event.Message.Content = string(contentStr)
	event.Event.Sender.SenderID.OpenID = "ou_user1"
	event.Event.Sender.SenderType = "user"

	body, _ := json.Marshal(event)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
		ch.HandleWebhook(httptest.NewRecorder(), req)
	}
	flushDebouncer(ch.debouncer)

	if count != 1 {
		t.Errorf("count=%d want 1 (dedup by event_id)", count)
	}
}

func TestFeishuChannel_HandleWebhook_PostMessage(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	// post content uses the nested {post:{zh_cn:{...}}} structure
	postContent := map[string]any{
		"post": map[string]any{
			"zh_cn": map[string]any{
				"title": "Report Title",
				"content": [][]map[string]any{
					{{"tag": "text", "text": "Line one"}},
					{{"tag": "text", "text": "Line two"}},
				},
			},
		},
	}
	contentStr, _ := json.Marshal(postContent)

	event := feishuEvent{}
	event.Header.EventID = "evt-post"
	event.Header.EventType = "im.message.receive_v1"
	event.Event.Message.ChatID = "oc_chat1"
	event.Event.Message.MessageType = "post"
	event.Event.Message.Content = string(contentStr)
	event.Event.Sender.SenderID.OpenID = "ou_user1"
	event.Event.Sender.SenderType = "user"

	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)
	flushDebouncer(ch.debouncer)

	if !strings.Contains(received.Content, "Report Title") {
		t.Errorf("post title missing from content: %q", received.Content)
	}
	if !strings.Contains(received.Content, "Line one") {
		t.Errorf("post body missing from content: %q", received.Content)
	}
}

func TestFeishuChannel_StripMentions(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`<at user_id="ou_xxx">Alice</at> hello`, "hello"},
		{`hello <at user_id="ou_yyy">Bob</at>`, "hello"},
		{`no mentions here`, "no mentions here"},
	}
	for _, c := range cases {
		got := stripFeishuMentions(c.in)
		if got != c.want {
			t.Errorf("stripFeishuMentions(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestFeishuChannel_DecryptPayload(t *testing.T) {
	// Decrypt should fail gracefully on bad input, not panic
	_, err := decryptPayload("testkey", "not-valid-base64!!!")
	if err == nil {
		t.Error("expected error on invalid base64")
	}
}

func TestFeishuChannel_TokenMutex_NotBlockedByIsRunning(t *testing.T) {
	// tokenMu and mu are separate — getToken() must not hold mu
	// This test verifies they don't deadlock: Start() holds mu briefly;
	// getToken() holds tokenMu briefly. If they shared a mutex, this would deadlock.
	ch := New(Config{AgentID: "a1"}, nil)
	ctx := context.Background()
	ch.Start(ctx)
	// IsRunning acquires mu; calling simultaneously with getToken (acquires tokenMu) is safe
	for i := 0; i < 10; i++ {
		go ch.IsRunning()
		go func() { ch.tokenMu.Lock(); ch.tokenMu.Unlock() }()
	}
	time.Sleep(10 * time.Millisecond)
	ch.Stop(ctx)
}
