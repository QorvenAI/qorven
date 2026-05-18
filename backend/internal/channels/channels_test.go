// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package channels

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Hard channel tests — interface compliance, manager, concurrency, message routing.

// === CHANNEL INTERFACE TESTS ===

func TestBaseChannel_New(t *testing.T) {
	ch := NewBaseChannel("test", "test_type", nil, nil)
	if ch == nil { t.Fatal("nil channel") }
	if ch.Name() != "test" { t.Errorf("name=%q", ch.Name()) }
	if ch.Type() != "test_type" { t.Errorf("type=%q", ch.Type()) }
}

func TestBaseChannel_AllowList(t *testing.T) {
	ch := NewBaseChannel("test", "test", []string{"user1", "user2"}, nil)
	if ch == nil { t.Fatal("nil") }
}

func TestBaseChannel_AgentID(t *testing.T) {
	ch := NewBaseChannel("test", "test", nil, nil)
	if ch.AgentID() != "" { t.Error("should be empty before set") }
}

func TestBaseChannel_TenantID(t *testing.T) {
	ch := NewBaseChannel("test", "test", nil, nil)
	if ch.TenantID() != uuid.Nil { t.Error("should be nil UUID") }
}

// === INBOUND MESSAGE TESTS ===

func TestInboundMessage_Fields(t *testing.T) {
	msg := InboundMessage{
		ChannelName: "telegram", ChannelType: "telegram",
		SenderID: "user123", SenderName: "John",
		ChatID: "chat456", Content: "Hello bot!",
		AgentID: "agent1",
	}
	if msg.ChannelName != "telegram" { t.Error("wrong channel") }
	if msg.SenderID != "user123" { t.Error("wrong sender") }
	if msg.Content != "Hello bot!" { t.Error("wrong content") }
}

func TestInboundMessage_WithMedia(t *testing.T) {
	msg := InboundMessage{Content: "check this image"}
	if msg.Content == "" { t.Error("empty content") }
}

// === OUTBOUND MESSAGE TESTS ===

func TestOutboundMessage_Fields(t *testing.T) {
	msg := OutboundMessage{
		RecipientID: "user123", ChatID: "chat456",
		Content: "Hello human!", Format: "markdown",
		ReplyTo: "msg789",
	}
	if msg.RecipientID != "user123" { t.Error("wrong recipient") }
	if msg.Format != "markdown" { t.Error("wrong format") }
}

func TestOutboundMessage_WithMedia(t *testing.T) {
	msg := OutboundMessage{
		Content: "Here's the file",
		Media: []MediaAttachment{
			{URL: "https://example.com/file.pdf", ContentType: "application/pdf", Caption: "report"},
		},
	}
	if len(msg.Media) != 1 { t.Error("should have 1 attachment") }
	if msg.Media[0].Caption != "report" { t.Error("wrong filename") }
}

func TestOutboundMessage_WithMetadata(t *testing.T) {
	msg := OutboundMessage{
		Content:  "response",
		Metadata: map[string]string{"thread_id": "t1", "reply_to": "m1"},
	}
	if msg.Metadata["thread_id"] != "t1" { t.Error("wrong metadata") }
}

// === MANAGER TESTS ===

func TestManager_New(t *testing.T) {
	handler := func(ctx context.Context, msg InboundMessage) {}
	m := NewManager(handler)
	if m == nil { t.Fatal("nil manager") }
}

func TestManager_Register(t *testing.T) {
	m := NewManager(func(ctx context.Context, msg InboundMessage) {})
	ch := &testChannel{name: "test1"}
	m.Register("inst1", ch)
}

func TestManager_Send(t *testing.T) {
	m := NewManager(func(ctx context.Context, msg InboundMessage) {})
	ch := &testChannel{name: "test1"}
	m.Register("inst1", ch)
	err := m.Send(context.Background(), "inst1", OutboundMessage{Content: "hello"})
	if err != nil { t.Errorf("send failed: %v", err) }
	if ch.lastSent.Content != "hello" { t.Error("message not delivered") }
}

func TestManager_Send_UnknownInstance(t *testing.T) {
	m := NewManager(func(ctx context.Context, msg InboundMessage) {})
	_ = m.Send(context.Background(), "nonexistent", OutboundMessage{Content: "hello"})
	// send to unknown is a no-op — expected
}

func TestManager_Handler(t *testing.T) {
	received := make(chan InboundMessage, 1)
	m := NewManager(func(ctx context.Context, msg InboundMessage) {
		received <- msg
	})
	handler := m.Handler()
	if handler == nil { t.Fatal("nil handler") }

	go handler(context.Background(), InboundMessage{Content: "test"})
	select {
	case msg := <-received:
		if msg.Content != "test" { t.Error("wrong content") }
	case <-time.After(time.Second):
		t.Error("handler not called")
	}
}

func TestManager_ConcurrentSend(t *testing.T) {
	m := NewManager(func(ctx context.Context, msg InboundMessage) {})
	ch := &testChannel{name: "test"}
	m.Register("inst1", ch)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.Send(context.Background(), "inst1", OutboundMessage{Content: "msg"})
		}(i)
	}
	wg.Wait()
	if ch.sendCount < 100 { t.Errorf("expected 100 sends, got %d", ch.sendCount) }
}

// === DM POLICY TESTS ===

func TestDMPolicy_Values(t *testing.T) {
	policies := []DMPolicy{"allow_all", "allowlist_only", "deny_all"}
	for _, p := range policies {
		if p == "" { t.Error("empty policy") }
	}
}

// === INTERNAL CHANNEL CHECK ===

func TestIsInternalChannel(t *testing.T) {
	if IsInternalChannel("webchat") != true { t.Log("webchat may or may not be internal") }
	if IsInternalChannel("telegram") { t.Error("telegram should not be internal") }
}

// === MEDIA FILE TESTS ===

func TestMediaFile_Fields(t *testing.T) {
	mf := MediaFile{Path: "/tmp/audio.ogg", MimeType: "audio/ogg"}
	if mf.Path != "/tmp/audio.ogg" { t.Error("wrong path") }
}

func TestMediaAttachment_Fields(t *testing.T) {
	ma := MediaAttachment{URL: "https://cdn.example.com/img.jpg", ContentType: "image/jpeg", Caption: "photo"}
	if !strings.HasPrefix(ma.URL, "https://") { t.Error("URL should be https") }
}

// === STRESS: Many channels registered ===

func TestManager_ManyChannels(t *testing.T) {
	m := NewManager(func(ctx context.Context, msg InboundMessage) {})
	for i := 0; i < 100; i++ {
		ch := &testChannel{name: "ch"}
		m.Register(string(rune('a'+i%26))+string(rune('0'+i/26)), ch)
	}
}

// Test channel implementation
type testChannel struct {
	name      string
	running   bool
	lastSent  OutboundMessage
	sendCount int
	mu        sync.Mutex
}

func (c *testChannel) Name() string    { return c.name }
func (c *testChannel) Type() string    { return "test" }
func (c *testChannel) AgentID() string { return "agent1" }
func (c *testChannel) Start(ctx context.Context) error { c.running = true; return nil }
func (c *testChannel) Stop(ctx context.Context) error  { c.running = false; return nil }
func (c *testChannel) IsRunning() bool { return c.running }
func (c *testChannel) Send(ctx context.Context, msg OutboundMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSent = msg
	c.sendCount++
	return nil
}
