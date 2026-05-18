// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package dingtalk

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

func makeSign(secret, timestampMs string) string {
	stringToSign := timestampMs + "\n" + secret
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestDingTalkChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*DingTalkChannel)(nil)
}

func TestDingTalkChannel_New(t *testing.T) {
	ch := New(Config{AgentID: "a1", AppKey: "key1"}, nil)
	if ch == nil {
		t.Fatal("nil")
	}
	if ch.Type() != "dingtalk" {
		t.Errorf("type=%q", ch.Type())
	}
	if ch.AgentID() != "a1" {
		t.Errorf("agentID=%q", ch.AgentID())
	}
}

func TestDingTalkChannel_StartStop(t *testing.T) {
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

func TestDingTalkChannel_APIBase(t *testing.T) {
	if dingtalkAPI != "https://api.dingtalk.com" {
		t.Errorf("dingtalkAPI=%q want https://api.dingtalk.com (oapi.dingtalk.com is deprecated)", dingtalkAPI)
	}
}

func TestDingTalkChannel_VerifySignature_Valid(t *testing.T) {
	ch := New(Config{AppSecret: "mysecret"}, nil)
	ts := fmt.Sprintf("%d", time.Now().UnixMilli())
	sign := makeSign("mysecret", ts)

	req := httptest.NewRequest("POST", "/webhook", nil)
	req.Header.Set("timestamp", ts)
	req.Header.Set("sign", sign)

	if !ch.verifyWebhookSignature(req) {
		t.Error("valid signature should pass")
	}
}

func TestDingTalkChannel_VerifySignature_BadSign(t *testing.T) {
	ch := New(Config{AppSecret: "mysecret"}, nil)
	ts := fmt.Sprintf("%d", time.Now().UnixMilli())

	req := httptest.NewRequest("POST", "/webhook", nil)
	req.Header.Set("timestamp", ts)
	req.Header.Set("sign", "badsig")

	if ch.verifyWebhookSignature(req) {
		t.Error("invalid signature should fail")
	}
}

func TestDingTalkChannel_VerifySignature_ReplayRejected(t *testing.T) {
	ch := New(Config{AppSecret: "mysecret"}, nil)
	// Timestamp 70 minutes ago — should be rejected
	oldTs := fmt.Sprintf("%d", time.Now().Add(-70*time.Minute).UnixMilli())
	sign := makeSign("mysecret", oldTs)

	req := httptest.NewRequest("POST", "/webhook", nil)
	req.Header.Set("timestamp", oldTs)
	req.Header.Set("sign", sign)

	if ch.verifyWebhookSignature(req) {
		t.Error("old timestamp should be rejected as replay")
	}
}

func TestDingTalkChannel_VerifySignature_NoSecret(t *testing.T) {
	ch := New(Config{}, nil)
	req := httptest.NewRequest("POST", "/webhook", nil)
	if !ch.verifyWebhookSignature(req) {
		t.Error("no secret = always pass")
	}
}

func TestDingTalkChannel_HandleWebhook_TextMessage(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	msg := dingtalkMessage{
		MsgID:            "m1",
		MsgType:          "text",
		SenderID:         "user1",
		SenderNick:       "Alice",
		ConversationID:   "conv1",
		ConversationType: "1", // DM
		SessionWebhook:   "https://webhook.example.com/session",
	}
	msg.Text.Content = "Hello DingTalk!"

	body, _ := json.Marshal(msg)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 {
		t.Errorf("status=%d", rr.Code)
	}
	flushDebouncer(ch.debouncer)

	if received.Content != "Hello DingTalk!" {
		t.Errorf("content=%q", received.Content)
	}
	if received.SenderID != "user1" {
		t.Errorf("senderID=%q", received.SenderID)
	}
	if received.ChatID != "conv1" {
		t.Errorf("chatID=%q want conv1", received.ChatID)
	}
	if received.Metadata["webhook_url"] != "https://webhook.example.com/session" {
		t.Errorf("webhook_url=%q", received.Metadata["webhook_url"])
	}
}

func TestDingTalkChannel_HandleWebhook_BotLoop_Skipped(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1", BotUserID: "bot-uid"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})

	msg := dingtalkMessage{
		MsgType:          "text",
		ChatbotUserID:    "bot-uid", // matches BotUserID — should be skipped
		ConversationID:   "conv1",
		ConversationType: "1",
	}
	msg.Text.Content = "bot echo"
	body, _ := json.Marshal(msg)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)

	if called {
		t.Error("bot echo messages should be skipped")
	}
}

func TestDingTalkChannel_HandleWebhook_GroupMention_Required(t *testing.T) {
	count := 0
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		count++
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	makeMsg := func(isInAtList bool) dingtalkMessage {
		m := dingtalkMessage{
			MsgType:          "text",
			SenderID:         "user1",
			ConversationID:   "group1",
			ConversationType: "2", // group
			IsInAtList:       isInAtList,
		}
		m.Text.Content = "hello bot"
		return m
	}

	// Not @mentioned — should be ignored
	body, _ := json.Marshal(makeMsg(false))
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)
	flushDebouncer(ch.debouncer)
	if count != 0 {
		t.Error("group message without @mention should be ignored")
	}

	// @mentioned — should fire
	body, _ = json.Marshal(makeMsg(true))
	req2 := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req2)
	flushDebouncer(ch.debouncer)
	if count != 1 {
		t.Errorf("count=%d want 1 after @mention", count)
	}
}

func TestDingTalkChannel_HandleWebhook_SignatureRequired(t *testing.T) {
	ch := New(Config{AgentID: "a1", AppSecret: "mysecret"}, nil)

	msg := dingtalkMessage{ConversationType: "1"}
	msg.Text.Content = "hi"
	body, _ := json.Marshal(msg)

	// No signature headers → 401
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 401 {
		t.Errorf("status=%d want 401 (missing signature)", rr.Code)
	}
}

func TestDingTalkChannel_HandleWebhook_MediaTypes(t *testing.T) {
	cases := []struct {
		msgType string
		want    string
	}{
		{"picture", "[Image attachment]"},
		{"audio", "[Audio attachment]"},
		{"video", "[Video attachment]"},
		{"file", "[File attachment]"},
	}
	for _, c := range cases {
		var received channels.InboundMessage
		ch := New(Config{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
			received = msg
		})
		ch.Start(context.Background())

		msg := dingtalkMessage{
			MsgType:          c.msgType,
			SenderID:         "user1",
			ConversationID:   "conv1",
			ConversationType: "1",
		}
		body, _ := json.Marshal(msg)
		req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
		ch.HandleWebhook(httptest.NewRecorder(), req)
		flushDebouncer(ch.debouncer)
		ch.Stop(context.Background())

		if received.Content != c.want {
			t.Errorf("msgType=%q content=%q want %q", c.msgType, received.Content, c.want)
		}
	}
}

func TestDingTalkChannel_SplitMessage(t *testing.T) {
	if maxTextLen != 3800 {
		t.Errorf("maxTextLen=%d want 3800", maxTextLen)
	}
	chunks := splitMessage(strings.Repeat("x", 3801), 3800)
	if len(chunks) != 2 {
		t.Errorf("chunks=%d want 2", len(chunks))
	}
}

func TestDingTalkChannel_SignWebhook(t *testing.T) {
	ch := New(Config{WebhookSecret: "secret123"}, nil)
	ts := "1700000000000"
	got := ch.signWebhook(ts)
	expected := makeSign("secret123", ts)
	if got != expected {
		t.Errorf("signWebhook=%q want %q", got, expected)
	}
}

func TestDingTalkChannel_ExtractRichText(t *testing.T) {
	elems := []dingtalkRichElem{
		{Type: "text", Text: "Hello"},
		{Type: "picture", DownloadCode: "abc"},
		{Type: "text", Text: "World"},
	}
	got := extractRichText(elems)
	if got != "Hello World" {
		t.Errorf("extractRichText=%q want 'Hello World'", got)
	}
}

func TestDingTalkChannel_TokenMutex_Separate(t *testing.T) {
	// tokenMu and mu must be separate; verify no deadlock
	ch := New(Config{AgentID: "a1"}, nil)
	ctx := context.Background()
	ch.Start(ctx)
	for i := 0; i < 10; i++ {
		go ch.IsRunning()
		go func() { ch.tokenMu.Lock(); ch.tokenMu.Unlock() }()
	}
	time.Sleep(10 * time.Millisecond)
	ch.Stop(ctx)
}
