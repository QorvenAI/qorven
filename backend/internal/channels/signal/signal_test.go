// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package signal

import (
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

func TestSignalChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*SignalChannel)(nil)
}

func TestSignalChannel_New(t *testing.T) {
	ch := New(Config{AgentID: "a1", APIURL: "http://localhost:8080", PhoneNumber: "+4912345678"}, nil)
	if ch.Type() != "signal" {
		t.Errorf("type=%q", ch.Type())
	}
	if ch.AgentID() != "a1" {
		t.Errorf("agentID=%q", ch.AgentID())
	}
	if ch.IsRunning() {
		t.Error("should not be running before Start")
	}
}

func TestSignalChannel_StartStop(t *testing.T) {
	ch := New(Config{AgentID: "a1", APIURL: "http://localhost:8080", PhoneNumber: "+1"}, nil)
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

func TestSignalChannel_PhoneURLEncoding(t *testing.T) {
	// '+' must be explicitly encoded as '%2B' for signal-cli-rest-api URL paths.
	// url.PathEscape leaves '+' unencoded (valid RFC 3986 path char), so we use
	// strings.ReplaceAll directly.
	phone := "+919876543210"
	encoded := strings.ReplaceAll(phone, "+", "%2B")
	if !strings.Contains(encoded, "%2B") {
		t.Errorf("encoded=%q — must contain %%2B for signal-cli-rest-api", encoded)
	}
	if strings.Contains(encoded, "+") {
		t.Errorf("encoded=%q — raw '+' must not remain", encoded)
	}
}

func TestSignalChannel_ProcessMessage_TextDM(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1", APIURL: "http://localhost:8080"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	msg := signalMessage{}
	msg.Envelope.Source = "+14155551234"
	msg.Envelope.SourceName = "Alice"
	dm := &dataMessage{Message: "Hello Signal!", Timestamp: 1700000000}
	msg.Envelope.DataMessage = dm

	ch.processMessage(msg)
	flushDebouncer(ch.debouncer)

	if received.Content != "Hello Signal!" {
		t.Errorf("content=%q", received.Content)
	}
	if received.SenderID != "+14155551234" {
		t.Errorf("senderID=%q", received.SenderID)
	}
	if received.SenderName != "Alice" {
		t.Errorf("senderName=%q", received.SenderName)
	}
	if received.ChatID != "+14155551234" {
		t.Errorf("chatID=%q want sender number for DM", received.ChatID)
	}
	if received.PeerKind != "direct" {
		t.Errorf("peerKind=%q want 'direct'", received.PeerKind)
	}
}

func TestSignalChannel_ProcessMessage_GroupMessage(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	msg := signalMessage{}
	msg.Envelope.Source = "+14155551234"
	dm := &dataMessage{
		Message:   "Group hello!",
		Timestamp: 1700000001,
		GroupInfo: &groupInfo{GroupID: "base64group==", Type: "DELIVER"},
	}
	msg.Envelope.DataMessage = dm

	ch.processMessage(msg)
	flushDebouncer(ch.debouncer)

	if received.ChatID != "base64group==" {
		t.Errorf("chatID=%q want group ID for group message", received.ChatID)
	}
	if received.PeerKind != "group" {
		t.Errorf("peerKind=%q want 'group'", received.PeerKind)
	}
	if received.Metadata["group_id"] != "base64group==" {
		t.Errorf("group_id=%q", received.Metadata["group_id"])
	}
}

func TestSignalChannel_ProcessMessage_SkipsReceipt(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	raw := json.RawMessage(`{"timestamp":123}`)
	msg := signalMessage{}
	msg.Envelope.ReceiptMessage = &raw

	ch.processMessage(msg)
	flushDebouncer(ch.debouncer)

	if called {
		t.Error("receipt messages should be silently skipped")
	}
}

func TestSignalChannel_ProcessMessage_SkipsTyping(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	raw := json.RawMessage(`{}`)
	msg := signalMessage{}
	msg.Envelope.TypingMessage = &raw

	ch.processMessage(msg)
	flushDebouncer(ch.debouncer)

	if called {
		t.Error("typing messages should be silently skipped")
	}
}

func TestSignalChannel_ProcessMessage_SkipsSync(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	raw := json.RawMessage(`{}`)
	msg := signalMessage{}
	msg.Envelope.SyncMessage = &raw

	ch.processMessage(msg)
	flushDebouncer(ch.debouncer)

	if called {
		t.Error("sync messages should be silently skipped")
	}
}

func TestSignalChannel_ProcessMessage_EmptyContent_Skipped(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	msg := signalMessage{}
	msg.Envelope.Source = "+1"
	msg.Envelope.DataMessage = &dataMessage{Message: "", Timestamp: 1}

	ch.processMessage(msg)
	flushDebouncer(ch.debouncer)

	if called {
		t.Error("empty message with no attachments should be skipped")
	}
}

func TestSignalChannel_ProcessMessage_ImageAttachment(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1", APIURL: "http://localhost:8080"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	msg := signalMessage{}
	msg.Envelope.Source = "+14155551234"
	msg.Envelope.DataMessage = &dataMessage{
		Timestamp: 1700000002,
		Attachments: []attachment{{
			ContentType: "image/jpeg",
			Size:        102400,
		}},
	}

	ch.processMessage(msg)
	flushDebouncer(ch.debouncer)

	if received.Content != "[Image attachment]" {
		t.Errorf("content=%q want '[Image attachment]'", received.Content)
	}
}

func TestSignalChannel_ProcessMessage_SourceNumber_Fallback(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	msg := signalMessage{}
	// Source empty — should fall back to SourceNumber
	msg.Envelope.Source = ""
	msg.Envelope.SourceNumber = "+14155559999"
	msg.Envelope.DataMessage = &dataMessage{Message: "hello", Timestamp: 1}

	ch.processMessage(msg)
	flushDebouncer(ch.debouncer)

	if received.SenderID != "+14155559999" {
		t.Errorf("senderID=%q want SourceNumber fallback", received.SenderID)
	}
}

func TestSignalChannel_Send_DM(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(201)
		w.Write([]byte(`{"timestamp":1700000000}`))
	}))
	defer srv.Close()

	ch := New(Config{
		AgentID:     "a1",
		APIURL:      srv.URL,
		PhoneNumber: "+919876543210",
	}, nil)

	err := ch.Send(context.Background(), channels.OutboundMessage{
		RecipientID: "+14155551234",
		Content:     "Hello from agent",
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotBody["message"] != "Hello from agent" {
		t.Errorf("message=%q", gotBody["message"])
	}
	if gotBody["number"] != "+919876543210" {
		t.Errorf("number=%q want bot's phone number", gotBody["number"])
	}
	recipients, _ := gotBody["recipients"].([]any)
	if len(recipients) == 0 || recipients[0] != "+14155551234" {
		t.Errorf("recipients=%v", recipients)
	}
}

func TestSignalChannel_Send_Group(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(201)
		w.Write([]byte(`{"timestamp":1700000001}`))
	}))
	defer srv.Close()

	ch := New(Config{
		APIURL:      srv.URL,
		PhoneNumber: "+919876543210",
	}, nil)

	err := ch.Send(context.Background(), channels.OutboundMessage{
		Content: "Group message",
		Metadata: map[string]string{"group_id": "base64group=="},
	})
	if err != nil {
		t.Fatal(err)
	}

	if gotBody["group_id"] != "base64group==" {
		t.Errorf("group_id=%q — group replies must use group_id field, not recipients", gotBody["group_id"])
	}
}

func TestSignalChannel_Send_UsesV2Endpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(201)
		w.Write([]byte(`{"timestamp":1}`))
	}))
	defer srv.Close()

	ch := New(Config{APIURL: srv.URL, PhoneNumber: "+1"}, nil)
	ch.Send(context.Background(), channels.OutboundMessage{
		RecipientID: "+2",
		Content:     "test",
	})

	if gotPath != "/v2/send" {
		t.Errorf("path=%q want /v2/send (v1/send is deprecated in signal-cli-rest-api)", gotPath)
	}
}

func TestSignalChannel_ReceiveEndpointEncoding(t *testing.T) {
	// Go's http.Client decodes %2B back to + before the server sees r.URL.Path,
	// so we verify the URL string construction directly rather than via HTTP round-trip.
	phone := "+919876543210"
	ch := New(Config{APIURL: "http://localhost:8080", PhoneNumber: phone}, nil)

	encodedPhone := strings.ReplaceAll(ch.cfg.PhoneNumber, "+", "%2B")
	receiveURL := ch.cfg.APIURL + "/v1/receive/" + encodedPhone

	if strings.Contains(receiveURL, "/v1/receive/+") {
		t.Errorf("URL=%q — raw '+' must be %%2B for signal-cli-rest-api", receiveURL)
	}
	if !strings.Contains(receiveURL, "%2B") {
		t.Errorf("URL=%q — expected %%2B-encoded phone", receiveURL)
	}
}

func TestSignalChannel_ReceiptEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(200)
	}))
	defer srv.Close()

	ch := New(Config{APIURL: srv.URL, PhoneNumber: "+919876543210"}, nil)
	ch.sendReceipt("+14155551234", 1700000000)

	// Correct endpoint: POST /v1/receipts/{number}/read (plural, with type)
	if !strings.Contains(gotPath, "/v1/receipts/") {
		t.Errorf("path=%q want /v1/receipts/... (plural)", gotPath)
	}
	if !strings.HasSuffix(gotPath, "/read") {
		t.Errorf("path=%q should end with /read", gotPath)
	}
}
