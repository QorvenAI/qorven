// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package line

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

// flushDebouncer flushes and waits for goroutines dispatched by FlushAll to complete.
func flushDebouncer(d *channels.Debouncer) {
	d.FlushAll()
	time.Sleep(50 * time.Millisecond)
}

func TestLINEChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*LINEChannel)(nil)
}

func TestLINEChannel_New(t *testing.T) {
	ch := New(Config{AgentID: "a1", ChannelToken: "tok", ChannelSecret: "sec"}, nil)
	if ch == nil {
		t.Fatal("nil")
	}
	if ch.Type() != "line" {
		t.Errorf("type=%q want line", ch.Type())
	}
	if ch.AgentID() != "a1" {
		t.Errorf("agentID=%q", ch.AgentID())
	}
	if ch.IsRunning() {
		t.Error("should not be running before Start")
	}
}

func TestLINEChannel_StartStop(t *testing.T) {
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

func TestLINEChannel_VerifySignature(t *testing.T) {
	ch := New(Config{ChannelSecret: "mysecret"}, nil)
	body := []byte(`{"events":[]}`)

	mac := hmac.New(sha256.New, []byte("mysecret"))
	mac.Write(body)
	validSig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if !ch.verifySignature(validSig, body) {
		t.Error("valid signature should pass")
	}
	if ch.verifySignature("badsig", body) {
		t.Error("invalid signature should fail")
	}
}

func TestLINEChannel_VerifySignature_NoSecret(t *testing.T) {
	ch := New(Config{}, nil)
	if !ch.verifySignature("anything", []byte("body")) {
		t.Error("no secret should always pass")
	}
}

func TestLINEChannel_HandleWebhook_TextMessage(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	payload := webhookPayload{
		Destination: "Ubot123",
		Events: []webhookEvent{{
			Type:       "message",
			ReplyToken: "reply-token-abc",
			Source: struct {
				Type    string `json:"type"`
				UserID  string `json:"userId"`
				GroupID string `json:"groupId"`
				RoomID  string `json:"roomId"`
			}{Type: "user", UserID: "Uuser456"},
			Message: struct {
				ID        string  `json:"id"`
				Type      string  `json:"type"`
				Text      string  `json:"text"`
				Title     string  `json:"title"`
				FileName  string  `json:"fileName"`
				FileSize  int     `json:"fileSize"`
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
				PackageID string  `json:"packageId"`
				StickerID string  `json:"stickerId"`
			}{ID: "m1", Type: "text", Text: "Hello LINE!"},
		}},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 {
		t.Errorf("status=%d want 200", rr.Code)
	}
	// Debouncer fires async — flush it
	flushDebouncer(ch.debouncer)

	if received.Content != "Hello LINE!" {
		t.Errorf("content=%q want 'Hello LINE!'", received.Content)
	}
	if received.SenderID != "Uuser456" {
		t.Errorf("senderID=%q", received.SenderID)
	}
	if received.ChatID != "Uuser456" {
		t.Errorf("chatID=%q want Uuser456 (1-on-1 chat)", received.ChatID)
	}
	if received.Metadata["reply_token"] != "reply-token-abc" {
		t.Errorf("reply_token=%q", received.Metadata["reply_token"])
	}
}

func TestLINEChannel_HandleWebhook_GroupMessage(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	payload := webhookPayload{
		Events: []webhookEvent{{
			Type:       "message",
			ReplyToken: "rt1",
			Source: struct {
				Type    string `json:"type"`
				UserID  string `json:"userId"`
				GroupID string `json:"groupId"`
				RoomID  string `json:"roomId"`
			}{Type: "group", UserID: "Uuser1", GroupID: "Cgroup1"},
			Message: struct {
				ID        string  `json:"id"`
				Type      string  `json:"type"`
				Text      string  `json:"text"`
				Title     string  `json:"title"`
				FileName  string  `json:"fileName"`
				FileSize  int     `json:"fileSize"`
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
				PackageID string  `json:"packageId"`
				StickerID string  `json:"stickerId"`
			}{ID: "m2", Type: "text", Text: "Group message"},
		}},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)
	flushDebouncer(ch.debouncer)

	// ChatID should be the group ID, not the user ID
	if received.ChatID != "Cgroup1" {
		t.Errorf("chatID=%q want Cgroup1 for group source", received.ChatID)
	}
	if received.SenderID != "Uuser1" {
		t.Errorf("senderID=%q want Uuser1", received.SenderID)
	}
}

func TestLINEChannel_HandleWebhook_DestinationMismatch(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1", ChannelUserID: "Ubot-correct"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	payload := webhookPayload{
		Destination: "Ubot-different", // wrong bot
		Events: []webhookEvent{{
			Type: "message",
			Source: struct {
				Type    string `json:"type"`
				UserID  string `json:"userId"`
				GroupID string `json:"groupId"`
				RoomID  string `json:"roomId"`
			}{Type: "user", UserID: "Uuser1"},
			Message: struct {
				ID        string  `json:"id"`
				Type      string  `json:"type"`
				Text      string  `json:"text"`
				Title     string  `json:"title"`
				FileName  string  `json:"fileName"`
				FileSize  int     `json:"fileSize"`
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
				PackageID string  `json:"packageId"`
				StickerID string  `json:"stickerId"`
			}{ID: "m1", Type: "text", Text: "hello"},
		}},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)
	flushDebouncer(ch.debouncer)

	if rr.Code != 200 {
		t.Errorf("status=%d want 200", rr.Code)
	}
	if called {
		t.Error("handler should not be called when destination does not match")
	}
}

func TestLINEChannel_HandleWebhook_EmptyEvents(t *testing.T) {
	// LINE Verify button sends empty events[] — must return 200
	ch := New(Config{AgentID: "a1"}, nil)
	payload := webhookPayload{Destination: "Ubot1", Events: []webhookEvent{}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)
	if rr.Code != 200 {
		t.Errorf("status=%d want 200 (verify check)", rr.Code)
	}
}

func TestLINEChannel_HandleWebhook_Postback(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})

	payload := webhookPayload{
		Events: []webhookEvent{{
			Type:       "postback",
			ReplyToken: "rt-pb",
			Source: struct {
				Type    string `json:"type"`
				UserID  string `json:"userId"`
				GroupID string `json:"groupId"`
				RoomID  string `json:"roomId"`
			}{Type: "user", UserID: "Uuser1"},
			Postback: struct {
				Data string `json:"data"`
			}{Data: "action=buy&itemId=42"},
		}},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 {
		t.Errorf("status=%d", rr.Code)
	}
	if received.Content != "action=buy&itemId=42" {
		t.Errorf("postback content=%q", received.Content)
	}
}

func TestLINEChannel_HandleWebhook_GroupNullUserID(t *testing.T) {
	// Groups can have empty userId — must not crash or make profile fetch with empty ID
	called := false
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	payload := webhookPayload{
		Events: []webhookEvent{{
			Type:       "message",
			ReplyToken: "rt1",
			Source: struct {
				Type    string `json:"type"`
				UserID  string `json:"userId"`
				GroupID string `json:"groupId"`
				RoomID  string `json:"roomId"`
			}{Type: "group", UserID: "", GroupID: "Cgroup1"}, // null userId
			Message: struct {
				ID        string  `json:"id"`
				Type      string  `json:"type"`
				Text      string  `json:"text"`
				Title     string  `json:"title"`
				FileName  string  `json:"fileName"`
				FileSize  int     `json:"fileSize"`
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
				PackageID string  `json:"packageId"`
				StickerID string  `json:"stickerId"`
			}{ID: "m1", Type: "text", Text: "anonymous group msg"},
		}},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)
	flushDebouncer(ch.debouncer)

	if rr.Code != 200 {
		t.Errorf("status=%d want 200", rr.Code)
	}
	if !called {
		t.Error("handler should still be called even with null userId in group")
	}
}

func TestLINEChannel_SendQuickReply_MaxItems(t *testing.T) {
	if maxQuickReply != 13 {
		t.Errorf("maxQuickReply=%d want 13", maxQuickReply)
	}
}

func TestLINEChannel_SplitMessage(t *testing.T) {
	cases := []struct {
		input  string
		maxLen int
		count  int
	}{
		{"short", 5000, 1},
		{strings.Repeat("x", 5000), 5000, 1},
		{strings.Repeat("x", 5001), 5000, 2},
	}
	for _, c := range cases {
		got := splitMessage(c.input, c.maxLen)
		if len(got) != c.count {
			t.Errorf("splitMessage len=%d maxLen=%d: got %d chunks want %d", len(c.input), c.maxLen, len(got), c.count)
		}
	}
}

func TestLINEChannel_CachedName_EmptyID(t *testing.T) {
	ch := New(Config{}, nil)
	// Must return "" without panicking or launching goroutine with bad URL
	name := ch.cachedName("")
	if name != "" {
		t.Errorf("cachedName('') = %q want ''", name)
	}
}
