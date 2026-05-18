// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package teams

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

// flushDebouncer flushes and waits for goroutines dispatched by FlushAll.
func flushDebouncer(d *channels.Debouncer) {
	d.FlushAll()
	time.Sleep(50 * time.Millisecond)
}

// setToken pre-populates the OAuth token cache so outbound send tests don't need a real token endpoint.
func setToken(ch *TeamsChannel, token string) {
	ch.tokenMu.Lock()
	ch.accessToken = token
	ch.tokenExpiry = time.Now().Add(time.Hour)
	ch.tokenMu.Unlock()
}

func TestTeamsChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*TeamsChannel)(nil)
}

func TestTeamsChannel_New(t *testing.T) {
	ch := New(Config{AgentID: "a1", AppID: "app-id"}, nil)
	if ch == nil {
		t.Fatal("nil")
	}
	if ch.Type() != "teams" {
		t.Errorf("type=%q want teams", ch.Type())
	}
	if ch.AgentID() != "a1" {
		t.Errorf("agentID=%q", ch.AgentID())
	}
	if ch.IsRunning() {
		t.Error("should not be running before Start")
	}
}

func TestTeamsChannel_StartStop(t *testing.T) {
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

func TestTeamsChannel_HandleWebhook_Message(t *testing.T) {
	var received channels.InboundMessage
	// AppID empty — JWT validation skipped (dev mode)
	ch := New(Config{AgentID: "a1", AppID: ""}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	activity := Activity{
		Type: "message",
		ID:   "act1",
		Text: "Hello Teams!",
		From: Actor{ID: "user1", Name: "Alice"},
		Conversation: Conversation{
			ID:               "conv1",
			ConversationType: "personal",
		},
		ServiceURL: "https://smba.trafficmanager.net/teams/",
	}

	body, _ := json.Marshal(activity)
	req := httptest.NewRequest("POST", "/api/messages", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 {
		t.Errorf("status=%d want 200", rr.Code)
	}

	// Let debouncer fire
	flushDebouncer(ch.debouncer)

	if received.Content != "Hello Teams!" {
		t.Errorf("content=%q", received.Content)
	}
	if received.SenderID != "user1" {
		t.Errorf("senderID=%q", received.SenderID)
	}
	if received.ChatID != "conv1" {
		t.Errorf("chatID=%q want conv1", received.ChatID)
	}
	if received.Metadata["conversation_type"] != "personal" {
		t.Errorf("conversation_type=%q", received.Metadata["conversation_type"])
	}
	if received.Metadata["service_url"] != "https://smba.trafficmanager.net/teams/" {
		t.Errorf("service_url=%q", received.Metadata["service_url"])
	}
	if received.Metadata["message_id"] != "act1" {
		t.Errorf("message_id=%q", received.Metadata["message_id"])
	}
}

func TestTeamsChannel_HandleWebhook_AuthRequired(t *testing.T) {
	// With AppID configured, JWT validation is required
	ch := New(Config{AgentID: "a1", AppID: "real-app-id"}, nil)

	activity := Activity{
		Type: "message",
		Text: "test",
		From: Actor{ID: "user1"},
		Conversation: Conversation{ID: "conv1"},
		ServiceURL: "https://smba.trafficmanager.net/",
	}
	body, _ := json.Marshal(activity)

	// No Authorization header → 401
	req := httptest.NewRequest("POST", "/api/messages", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 401 {
		t.Errorf("status=%d want 401 (no JWT)", rr.Code)
	}
}

func TestTeamsChannel_HandleWebhook_SkipSelfMessage(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1", AppID: ""}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})

	activity := Activity{
		Type:         "message",
		Text:         "bot's own message",
		From:         Actor{ID: ""}, // AppID matches From.ID — but AppID is empty here
		Conversation: Conversation{ID: "conv1"},
		ServiceURL:   "https://smba.trafficmanager.net/",
	}
	body, _ := json.Marshal(activity)
	req := httptest.NewRequest("POST", "/api/messages", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)

	if called {
		t.Error("handler should not fire for own messages")
	}
}

func TestTeamsChannel_HandleWebhook_ConvRef_Stored(t *testing.T) {
	ch := New(Config{AgentID: "a1", AppID: ""}, nil)
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	activity := Activity{
		Type:         "message",
		Text:         "hi",
		From:         Actor{ID: "user-proactive"},
		Conversation: Conversation{ID: "conv-proactive"},
		ServiceURL:   "https://smba.trafficmanager.net/teams/",
	}
	body, _ := json.Marshal(activity)
	req := httptest.NewRequest("POST", "/api/messages", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)

	ref, ok := ch.convRefs.Load("user-proactive")
	if !ok {
		t.Fatal("conversation reference should be stored after message")
	}
	convRef := ref.(*ConversationRef)
	if convRef.ServiceURL != "https://smba.trafficmanager.net/teams/" {
		t.Errorf("serviceURL=%q", convRef.ServiceURL)
	}
	if convRef.ConversationID != "conv-proactive" {
		t.Errorf("conversationID=%q", convRef.ConversationID)
	}
}

func TestTeamsChannel_HandleWebhook_Invoke(t *testing.T) {
	ch := New(Config{AgentID: "a1", AppID: ""}, nil)

	activity := Activity{
		Type:         "invoke",
		From:         Actor{ID: "user1"},
		Conversation: Conversation{ID: "conv1"},
		ServiceURL:   "https://smba.trafficmanager.net/",
	}
	body, _ := json.Marshal(activity)
	req := httptest.NewRequest("POST", "/api/messages", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 {
		t.Errorf("status=%d want 200", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != float64(200) {
		t.Errorf("invoke response status=%v want 200", resp["status"])
	}
}

func TestTeamsChannel_Send_TextFormat_Markdown(t *testing.T) {
	var sentActivity Activity
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&sentActivity)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	ch := New(Config{AgentID: "a1", AppID: ""}, nil)
	setToken(ch, "test-token")
	// Pre-store a conversation reference
	ch.convRefs.Store("user1", &ConversationRef{
		ServiceURL:     srv.URL + "/",
		ConversationID: "conv1",
		UserID:         "user1",
	})

	err := ch.Send(context.Background(), channels.OutboundMessage{
		ChatID:  "conv1",
		Content: "**bold** response",
		Metadata: map[string]string{
			"service_url":     srv.URL + "/",
			"conversation_id": "conv1",
		},
	})
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if sentActivity.TextFormat != "markdown" {
		t.Errorf("textFormat=%q want markdown", sentActivity.TextFormat)
	}
	if sentActivity.Text != "**bold** response" {
		t.Errorf("text=%q", sentActivity.Text)
	}
}

func TestTeamsChannel_Send_ReplyToId(t *testing.T) {
	var sentActivity Activity
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&sentActivity)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	ch := New(Config{AgentID: "a1", AppID: ""}, nil)
	setToken(ch, "test-token")

	err := ch.Send(context.Background(), channels.OutboundMessage{
		Content: "threaded reply",
		Metadata: map[string]string{
			"service_url":     srv.URL + "/",
			"conversation_id": "conv1",
			"message_id":      "original-act-id",
		},
	})
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if sentActivity.ReplyToID != "original-act-id" {
		t.Errorf("replyToId=%q want original-act-id", sentActivity.ReplyToID)
	}
}

func TestTeamsChannel_StripTeamsMention(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"<at>BotName</at> hello", "hello"},
		{"<at>Bot</at>", ""},
		{"hello <at>Bot</at> world", "hello  world"},
		{"no mention here", "no mention here"},
	}
	for _, c := range cases {
		got := stripTeamsMention(c.in)
		got = strings.TrimSpace(got)
		want := strings.TrimSpace(c.want)
		if got != want {
			t.Errorf("stripTeamsMention(%q) = %q want %q", c.in, got, want)
		}
	}
}

func TestTeamsChannel_SplitMessage(t *testing.T) {
	if maxMessageLen != 28000 {
		t.Errorf("maxMessageLen=%d want 28000", maxMessageLen)
	}
	chunks := splitMessage(strings.Repeat("x", 28001), 28000)
	if len(chunks) != 2 {
		t.Errorf("chunks=%d want 2", len(chunks))
	}
}

func TestTeamsChannel_AdaptiveCardVersion(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)
	card := ch.buildWelcomeCard("TestUser")
	version, _ := card["version"].(string)
	if version != "1.5" {
		t.Errorf("adaptive card version=%q want 1.5", version)
	}
}

func TestTeamsChannel_ConvRef_ChatID(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1", AppID: ""}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	activity := Activity{
		Type: "message",
		Text: "channel msg",
		From: Actor{ID: "user1", Name: "User"},
		Conversation: Conversation{
			ID:               "conv-ch",
			ConversationType: "channel",
		},
		ServiceURL: "https://smba.trafficmanager.net/",
	}
	body, _ := json.Marshal(activity)
	req := httptest.NewRequest("POST", "/api/messages", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)
	flushDebouncer(ch.debouncer)

	if received.ChatID != "conv-ch" {
		t.Errorf("ChatID=%q want conv-ch", received.ChatID)
	}
	if received.Metadata["conversation_type"] != "channel" {
		t.Errorf("conversation_type=%q want channel", received.Metadata["conversation_type"])
	}
}
