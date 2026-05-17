// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package imessage

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

func flushDebouncer(d *channels.Debouncer) {
	d.FlushAll()
	time.Sleep(50 * time.Millisecond)
}

func TestIMessageChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*IMessageChannel)(nil)
}

func TestIMessageChannel_New(t *testing.T) {
	ch := New(Config{AgentID: "a1", ServerURL: "http://localhost:1234", Password: "pw"}, nil)
	if ch.Type() != "imessage" {
		t.Errorf("type=%q", ch.Type())
	}
	if ch.AgentID() != "a1" {
		t.Errorf("agentID=%q", ch.AgentID())
	}
	if ch.IsRunning() {
		t.Error("should not be running before Start")
	}
}

func TestIMessageChannel_StartStop(t *testing.T) {
	ch := New(Config{AgentID: "a1", UseWebhook: true}, nil)
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

func TestIMessageChannel_HandleWebhook_NewMessage_DM(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1", UseWebhook: true}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	event := bbEvent{
		Type: "new-message",
		Data: bbEventData{
			GUID:     "msg-guid-1",
			Text:     "Hello from iPhone",
			IsFromMe: false,
			Handle:   bbHandle{Address: "+14155551234", FirstName: "Alice"},
			Chats:    []bbChat{{GUID: "iMessage;-;+14155551234"}},
		},
	}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)
	flushDebouncer(ch.debouncer)

	if rw.Code != 200 {
		t.Errorf("status=%d", rw.Code)
	}
	if received.Content != "Hello from iPhone" {
		t.Errorf("content=%q", received.Content)
	}
	if received.SenderID != "+14155551234" {
		t.Errorf("senderID=%q", received.SenderID)
	}
	if received.SenderName != "Alice" {
		t.Errorf("senderName=%q", received.SenderName)
	}
	if received.ChatID != "iMessage;-;+14155551234" {
		t.Errorf("chatID=%q — must come from chats[0].guid", received.ChatID)
	}
	if received.PeerKind != "direct" {
		t.Errorf("peerKind=%q want 'direct'", received.PeerKind)
	}
}

func TestIMessageChannel_HandleWebhook_NewMessage_Group(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1", UseWebhook: true}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	event := bbEvent{
		Type: "new-message",
		Data: bbEventData{
			GUID:     "msg-guid-2",
			Text:     "Hello group",
			IsFromMe: false,
			Handle:   bbHandle{Address: "+14155551234"},
			Chats:    []bbChat{{GUID: "iMessage;+;chat123456789"}},
		},
	}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)
	flushDebouncer(ch.debouncer)

	if received.ChatID != "iMessage;+;chat123456789" {
		t.Errorf("chatID=%q want group GUID from chats[0]", received.ChatID)
	}
	if received.PeerKind != "group" {
		t.Errorf("peerKind=%q want 'group'", received.PeerKind)
	}
}

func TestIMessageChannel_HandleWebhook_SkipsIsFromMe(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1", UseWebhook: true}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	event := bbEvent{
		Type: "new-message",
		Data: bbEventData{
			Text:     "I sent this",
			IsFromMe: true,
			Handle:   bbHandle{Address: "+14155551234"},
			Chats:    []bbChat{{GUID: "iMessage;-;+14155551234"}},
		},
	}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)
	flushDebouncer(ch.debouncer)

	if called {
		t.Error("isFromMe=true must be skipped to prevent bot loop")
	}
}

func TestIMessageChannel_HandleWebhook_SecretVerification(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1", WebhookSecret: "correct-secret", UseWebhook: true}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	// Wrong secret
	event := bbEvent{
		Type:   "new-message",
		Secret: "wrong-secret",
		Data: bbEventData{
			Text:     "hi",
			IsFromMe: false,
			Handle:   bbHandle{Address: "+1"},
			Chats:    []bbChat{{GUID: "iMessage;-;+1"}},
		},
	}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)
	flushDebouncer(ch.debouncer)

	if called {
		t.Error("wrong secret should block message from reaching handler")
	}
	// Still returns 200 to prevent BB retry storms
	if rw.Code != 200 {
		t.Errorf("status=%d — must ack 200 even on secret mismatch to prevent retry storms", rw.Code)
	}
}

func TestIMessageChannel_HandleWebhook_SecretCorrect(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1", WebhookSecret: "correct-secret", UseWebhook: true}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	event := bbEvent{
		Type:   "new-message",
		Secret: "correct-secret",
		Data: bbEventData{
			Text:     "authorized message",
			IsFromMe: false,
			Handle:   bbHandle{Address: "+14155551234"},
			Chats:    []bbChat{{GUID: "iMessage;-;+14155551234"}},
		},
	}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)
	flushDebouncer(ch.debouncer)

	if received.Content != "authorized message" {
		t.Errorf("content=%q — correct secret should pass", received.Content)
	}
}

func TestIMessageChannel_HandleWebhook_BBURLChange(t *testing.T) {
	ch := New(Config{AgentID: "a1", ServerURL: "https://old.trycloudflare.com", UseWebhook: true}, nil)
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	event := bbEvent{
		Type: "bb-url-change",
		Data: bbEventData{NewURL: "https://new.trycloudflare.com"},
	}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rw := httptest.NewRecorder()
	ch.HandleWebhook(rw, req)

	if got := ch.serverURL(); got != "https://new.trycloudflare.com" {
		t.Errorf("serverURL=%q — bb-url-change must update ServerURL immediately", got)
	}
}

func TestIMessageChannel_HandleWebhook_SkipsUpdatedMessage(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1", UseWebhook: true}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	event := bbEvent{Type: "updated-message", Data: bbEventData{GUID: "msg-1"}}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)
	flushDebouncer(ch.debouncer)

	if called {
		t.Error("updated-message events should be skipped")
	}
}

func TestIMessageChannel_HandleWebhook_SkipsTypingIndicator(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1", UseWebhook: true}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	event := bbEvent{Type: "typing-indicator", Data: bbEventData{GUID: "chat-1"}}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)
	flushDebouncer(ch.debouncer)

	if called {
		t.Error("typing-indicator events should be skipped")
	}
}

func TestIMessageChannel_HandleWebhook_ImageAttachment(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1", UseWebhook: true}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	event := bbEvent{
		Type: "new-message",
		Data: bbEventData{
			IsFromMe: false,
			Handle:   bbHandle{Address: "+1"},
			Chats:    []bbChat{{GUID: "iMessage;-;+1"}},
			Attachments: []bbAttachment{{
				GUID:     "att-1",
				MimeType: "image/jpeg",
			}},
		},
	}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)
	flushDebouncer(ch.debouncer)

	if received.Content != "[Image attachment]" {
		t.Errorf("content=%q want '[Image attachment]'", received.Content)
	}
}

func TestIMessageChannel_HandleWebhook_EmptyMessage_Skipped(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1", UseWebhook: true}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	event := bbEvent{
		Type: "new-message",
		Data: bbEventData{
			Text:     "",
			IsFromMe: false,
			Handle:   bbHandle{Address: "+1"},
			Chats:    []bbChat{{GUID: "iMessage;-;+1"}},
		},
	}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)
	flushDebouncer(ch.debouncer)

	if called {
		t.Error("empty message with no attachments should be skipped")
	}
}

func TestIMessageChannel_HandleWebhook_ChatGUIDFallback(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1", UseWebhook: true}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	// No chats array — should fall back to sender address
	event := bbEvent{
		Type: "new-message",
		Data: bbEventData{
			Text:     "fallback test",
			IsFromMe: false,
			Handle:   bbHandle{Address: "+19999999999"},
			Chats:    []bbChat{},
		},
	}
	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)
	flushDebouncer(ch.debouncer)

	if received.ChatID != "+19999999999" {
		t.Errorf("chatID=%q — empty chats[] must fall back to sender address", received.ChatID)
	}
}

func TestIMessageChannel_Send_TempGUID(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	ch := New(Config{
		AgentID:   "a1",
		ServerURL: srv.URL,
		Password:  "pw",
	}, nil)

	err := ch.Send(context.Background(), channels.OutboundMessage{
		ChatID:  "iMessage;-;+14155551234",
		Content: "Hello!",
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotBody["tempGuid"] == nil || gotBody["tempGuid"] == "" {
		t.Error("tempGuid must be set on every send — BB silently drops messages with duplicate or missing tempGuid")
	}
	tg, _ := gotBody["tempGuid"].(string)
	if !strings.HasPrefix(tg, "temp-") {
		t.Errorf("tempGuid=%q — must be prefixed with 'temp-'", tg)
	}
	if gotBody["chatGuid"] != "iMessage;-;+14155551234" {
		t.Errorf("chatGuid=%q", gotBody["chatGuid"])
	}
	if gotBody["message"] != "Hello!" {
		t.Errorf("message=%q", gotBody["message"])
	}
}

func TestIMessageChannel_Send_UniqueTempGUID(t *testing.T) {
	var guids []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if tg, ok := body["tempGuid"].(string); ok {
			guids = append(guids, tg)
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	ch := New(Config{ServerURL: srv.URL, Password: "pw"}, nil)
	for i := 0; i < 3; i++ {
		ch.Send(context.Background(), channels.OutboundMessage{
			ChatID:  "iMessage;-;+1",
			Content: "msg",
		})
	}

	seen := map[string]bool{}
	for _, g := range guids {
		if seen[g] {
			t.Errorf("duplicate tempGuid=%q — each send must use a fresh UUID", g)
		}
		seen[g] = true
	}
}

func TestIMessageChannel_Send_PasswordInURL(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	ch := New(Config{ServerURL: srv.URL, Password: "secret123"}, nil)
	ch.Send(context.Background(), channels.OutboundMessage{ChatID: "iMessage;-;+1", Content: "hi"})

	if !strings.Contains(gotURL, "password=secret123") {
		t.Errorf("URL=%q — password must be a query param, not a header", gotURL)
	}
}

func TestIMessageChannel_Send_ChatIDFromMetadata(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	ch := New(Config{ServerURL: srv.URL, Password: "pw"}, nil)
	err := ch.Send(context.Background(), channels.OutboundMessage{
		Content:  "test",
		Metadata: map[string]string{"chat_guid": "iMessage;+;chat123456789"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["chatGuid"] != "iMessage;+;chat123456789" {
		t.Errorf("chatGuid=%q — chat_guid from metadata must be used when set", gotBody["chatGuid"])
	}
}

func TestIMessageChannel_ChatGUIDFormat_DirectVsGroup(t *testing.T) {
	dmGUID := "iMessage;-;+14155551234"
	groupGUID := "iMessage;+;chat123456789"
	smsGUID := "SMS;-;+14155551234"

	if strings.Contains(dmGUID, ";+;") {
		t.Error("DM GUID should not contain ';+;'")
	}
	if !strings.Contains(groupGUID, ";+;") {
		t.Error("group GUID must contain ';+;'")
	}
	if strings.Contains(smsGUID, ";+;") {
		t.Error("SMS DM GUID should not contain ';+;'")
	}
}

func TestIMessageChannel_SendTyping_CorrectEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(200)
	}))
	defer srv.Close()

	ch := New(Config{ServerURL: srv.URL, Password: "pw"}, nil)
	ch.SendTyping("iMessage;-;+14155551234")

	if gotPath != "/api/v1/chat/typing" {
		t.Errorf("path=%q want /api/v1/chat/typing", gotPath)
	}
}

func TestIMessageChannel_RegisterWebhook(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"success","data":{"id":"webhook-uuid"}}`))
	}))
	defer srv.Close()

	ch := New(Config{ServerURL: srv.URL, Password: "pw"}, nil)
	err := ch.RegisterWebhook("https://my.app/webhooks/imessage", nil)
	if err != nil {
		t.Fatal(err)
	}

	if gotBody["url"] != "https://my.app/webhooks/imessage" {
		t.Errorf("url=%q", gotBody["url"])
	}
	events, _ := gotBody["events"].([]any)
	if len(events) == 0 {
		t.Error("RegisterWebhook must include default events list")
	}
}

func TestIMessageChannel_BuildContent_AttachmentLabels(t *testing.T) {
	cases := []struct {
		mime string
		want string
	}{
		{"image/jpeg", "[Image attachment]"},
		{"audio/m4a", "[Audio attachment]"},
		{"video/mp4", "[Video attachment]"},
	}
	for _, c := range cases {
		atts := []bbAttachment{{GUID: "g", MimeType: c.mime}}
		text, _ := buildContent("", atts)
		if text != c.want {
			t.Errorf("mime=%q: content=%q want %q", c.mime, text, c.want)
		}
	}
}

func TestIMessageChannel_BuildContent_TextPlusAttachment(t *testing.T) {
	atts := []bbAttachment{{GUID: "g", MimeType: "image/png"}}
	text, _ := buildContent("check this out", atts)
	if text != "check this out [Image attachment]" {
		t.Errorf("content=%q", text)
	}
}
