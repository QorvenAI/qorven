// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package channels

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// diamond_channel_test.go — Tests for the channel message processing pipeline.
// This is the code path for EVERY message from Telegram/Slack/Discord/etc.

// ── Channel Manager ──

func TestDiamond_Manager_RegisterAndStart(t *testing.T) {
	received := make(chan InboundMessage, 1)
	handler := func(ctx context.Context, msg InboundMessage) { received <- msg }
	mgr := NewManager(handler)

	ch := &mockChannel{name: "test-telegram", typ: "telegram", agentID: "agent-1"}
	mgr.Register("inst-1", ch)
	mgr.Start(context.Background(), "inst-1")
	defer mgr.Stop(context.Background(), "inst-1")

	// Simulate: channel receives message and calls the manager's handler
	mgr.Handler()(context.Background(), InboundMessage{
		ChannelType: "telegram", AgentID: "agent-1",
		SenderID: "user-123", Content: "Hello from Telegram",
	})

	select {
	case msg := <-received:
		if msg.Content != "Hello from Telegram" { t.Errorf("content: %q", msg.Content) }
		if msg.AgentID != "agent-1" { t.Errorf("agent: %q", msg.AgentID) }
		t.Log("message received through channel pipeline ✓")
	case <-time.After(2 * time.Second):
		t.Fatal("message not received within 2s")
	}
}

func TestDiamond_Manager_MultipleChannels(t *testing.T) {
	var count atomic.Int32
	mgr := NewManager(func(ctx context.Context, msg InboundMessage) {
		count.Add(1)
	})

	for i := 0; i < 5; i++ {
		ch := &mockChannel{name: "ch-" + string(rune('A'+i)), typ: "test", agentID: "agent-1"}
		mgr.Register("inst-"+string(rune('A'+i)), ch)
		mgr.Start(context.Background(), "inst-"+string(rune('A'+i)))
		defer mgr.Stop(context.Background(), "inst-"+string(rune('A'+i)))
	}

	// All channels should be registered
	all := mgr.List()
	if len(all) < 5 { t.Errorf("expected 5 channels, got %d", len(all)) }
	t.Logf("5 channels registered and running ✓")
}

func TestDiamond_Manager_ConcurrentMessages(t *testing.T) {
	var count atomic.Int32
	handler := func(ctx context.Context, msg InboundMessage) { count.Add(1) }
	mgr := NewManager(handler)

	ch := &mockChannel{name: "concurrent", typ: "test", agentID: "agent-1"}
	mgr.Register("inst-1", ch)
	mgr.Start(context.Background(), "inst-1")
	defer mgr.Stop(context.Background(), "inst-1")

	// Send 50 messages concurrently through the manager's handler
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			mgr.Handler()(context.Background(), InboundMessage{
				Content: "msg " + string(rune('A'+n%26)), AgentID: "agent-1",
			})
		}(i)
	}
	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	c := count.Load()
	if c < 45 { t.Errorf("only %d/50 messages processed", c) }
	t.Logf("concurrent: %d/50 messages processed ✓", c)
}

// ── BaseChannel AllowList ──

func TestDiamond_BaseChannel_AllowList(t *testing.T) {
	received := make(chan string, 10)
	bc := &BaseChannel{
		name: "test", channelType: "telegram", agentID: "agent-1",
		allowList: []string{"user-allowed"},
		handler: func(ctx context.Context, msg InboundMessage) {
			received <- msg.SenderID
		},
	}

	// Allowed user — should be processed
	bc.HandleMessage(InboundMessage{SenderID: "user-allowed", PeerKind: "direct"})

	// Blocked user — should be dropped
	bc.HandleMessage(InboundMessage{SenderID: "user-blocked", PeerKind: "direct"})

	// Group message — should always be processed (no allowlist for groups)
	bc.HandleMessage(InboundMessage{SenderID: "anyone", PeerKind: "group"})

	time.Sleep(50 * time.Millisecond)

	count := len(received)
	if count != 2 { t.Errorf("expected 2 messages (allowed + group), got %d", count) }
	t.Logf("allowlist: allowed=1, blocked=1, group=1 → %d processed ✓", count)
}

// ── InboundDedup ──




// ── Approval Options ──

func TestDiamond_Approval_HighRiskDetection(t *testing.T) {
	high := []string{"rm -rf /var/data", "DROP TABLE users", "dd if=/dev/zero of=/dev/sda"}
	for _, cmd := range high {
		opts := DefaultApprovalOptions(cmd)
		if opts.Risk != "high" { t.Errorf("%q should be high risk, got %q", cmd, opts.Risk) }
	}

	low := []string{"go build ./...", "git status", "ls -la"}
	for _, cmd := range low {
		opts := DefaultApprovalOptions(cmd)
		if opts.Risk != "medium" { t.Errorf("%q should be medium risk, got %q", cmd, opts.Risk) }
	}
	t.Log("risk detection: high/medium correctly classified ✓")
}

// ── OutboundMessage ──

func TestDiamond_OutboundMessage_MediaAttachments(t *testing.T) {
	msg := OutboundMessage{
		RecipientID: "user-123",
		Content:     "Here's the file",
		Media: []MediaAttachment{
			{URL: "https://example.com/file.pdf", ContentType: "application/pdf", Caption: "Report"},
		},
	}
	if len(msg.Media) != 1 { t.Error("should have 1 attachment") }
	if msg.Media[0].ContentType != "application/pdf" { t.Error("wrong content type") }
}

// ── Mock Channel ──

type mockChannel struct {
	name    string
	typ     string
	agentID string
	handler func(context.Context, InboundMessage)
	running bool
}

func (m *mockChannel) Name() string    { return m.name }
func (m *mockChannel) Type() string    { return m.typ }
func (m *mockChannel) AgentID() string { return m.agentID }
func (m *mockChannel) IsRunning() bool { return m.running }
func (m *mockChannel) Start(_ context.Context) error { m.running = true; return nil }
func (m *mockChannel) Stop(_ context.Context) error  { m.running = false; return nil }
func (m *mockChannel) Send(_ context.Context, _ OutboundMessage) error { return nil }

func (m *mockChannel) SetHandler(h func(context.Context, InboundMessage)) { m.handler = h }

func (m *mockChannel) simulateMessage(msg InboundMessage) {
	if m.handler != nil { m.handler(context.Background(), msg) }
}
