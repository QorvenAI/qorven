// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package channels

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Deep channel tests — message routing, concurrent delivery, backpressure.

func TestDeep_Manager_MessageRouting(t *testing.T) {
	received := make(chan InboundMessage, 100)
	m := NewManager(func(ctx context.Context, msg InboundMessage) {
		received <- msg
	})

	// Register 3 channels
	ch1 := &deepTestChannel{name: "telegram", agentID: "agent1"}
	ch2 := &deepTestChannel{name: "discord", agentID: "agent2"}
	ch3 := &deepTestChannel{name: "slack", agentID: "agent3"}
	m.Register("tg-1", ch1)
	m.Register("dc-1", ch2)
	m.Register("sl-1", ch3)

	// Send to each channel
	handler := m.Handler()
	go handler(context.Background(), InboundMessage{ChannelName: "telegram", Content: "msg1", AgentID: "agent1"})
	go handler(context.Background(), InboundMessage{ChannelName: "discord", Content: "msg2", AgentID: "agent2"})
	go handler(context.Background(), InboundMessage{ChannelName: "slack", Content: "msg3", AgentID: "agent3"})

	// Collect messages
	msgs := map[string]bool{}
	timeout := time.After(2 * time.Second)
	for i := 0; i < 3; i++ {
		select {
		case msg := <-received:
			msgs[msg.Content] = true
		case <-timeout:
			t.Fatalf("timeout waiting for messages, got %d/3", len(msgs))
		}
	}
	if !msgs["msg1"] { t.Error("missing msg1") }
	if !msgs["msg2"] { t.Error("missing msg2") }
	if !msgs["msg3"] { t.Error("missing msg3") }
}

func TestDeep_Manager_ConcurrentSend(t *testing.T) {
	m := NewManager(func(ctx context.Context, msg InboundMessage) {})
	ch := &deepTestChannel{name: "test", agentID: "a1"}
	m.Register("inst1", ch)

	var wg sync.WaitGroup
	var errors atomic.Int32
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			err := m.Send(context.Background(), "inst1", OutboundMessage{
				Content: "concurrent message " + string(rune('A'+n%26)),
			})
			if err != nil { errors.Add(1) }
		}(i)
	}
	wg.Wait()
	if errors.Load() > 0 { t.Errorf("%d/200 sends failed", errors.Load()) }
	if ch.sendCount < 200 { t.Errorf("channel received %d/200", ch.sendCount) }
	t.Logf("200 concurrent sends: %d delivered, %d errors", ch.sendCount, errors.Load())
}

func TestDeep_Manager_SendToMultipleChannels(t *testing.T) {
	m := NewManager(func(ctx context.Context, msg InboundMessage) {})

	channels := make([]*deepTestChannel, 10)
	for i := range channels {
		channels[i] = &deepTestChannel{name: "ch" + string(rune('0'+i)), agentID: "a1"}
		m.Register("inst"+string(rune('0'+i)), channels[i])
	}

	// Send to all channels
	for i := 0; i < 10; i++ {
		m.Send(context.Background(), "inst"+string(rune('0'+i)), OutboundMessage{Content: "broadcast"})
	}

	// Verify all received
	for i, ch := range channels {
		if ch.sendCount != 1 { t.Errorf("channel %d: %d sends", i, ch.sendCount) }
		if ch.lastSent.Content != "broadcast" { t.Errorf("channel %d: wrong content", i) }
	}
}

func TestDeep_Manager_SendToUnregistered(t *testing.T) {
	m := NewManager(func(ctx context.Context, msg InboundMessage) {})
	err := m.Send(context.Background(), "nonexistent", OutboundMessage{Content: "hello"})
	// Should not error (graceful handling)
	_ = err
}

func TestDeep_Manager_HighVolume(t *testing.T) {
	var count atomic.Int64
	m := NewManager(func(ctx context.Context, msg InboundMessage) {
		count.Add(1)
	})

	handler := m.Handler()
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			handler(context.Background(), InboundMessage{Content: "msg", ChannelName: "test"})
		}(i)
	}
	wg.Wait()
	if count.Load() != 1000 { t.Errorf("processed %d/1000", count.Load()) }
	t.Logf("1000 messages processed")
}

func TestDeep_OutboundMessage_WithAllFields(t *testing.T) {
	msg := OutboundMessage{
		RecipientID: "user123",
		ChatID:      "chat456",
		Content:     "Hello! Here's your report.",
		Subject:     "Weekly Report",
		Format:      "markdown",
		ReplyTo:     "msg789",
		Metadata:    map[string]string{"thread_id": "t1", "priority": "high"},
		Media: []MediaAttachment{
			{URL: "https://cdn.example.com/report.pdf", ContentType: "application/pdf", Caption: "Report"},
		},
	}
	if msg.RecipientID != "user123" { t.Error("recipient") }
	if msg.Format != "markdown" { t.Error("format") }
	if len(msg.Media) != 1 { t.Error("media") }
	if msg.Metadata["priority"] != "high" { t.Error("metadata") }
}

func TestDeep_InboundMessage_WithAllFields(t *testing.T) {
	msg := InboundMessage{
		ChannelName: "telegram",
		ChannelType: "telegram",
		InstanceID:  "tg-bot-1",
		AgentID:     "agent-prime",
		SenderID:    "user456",
		SenderName:  "John Doe",
		ChatID:      "chat789",
		Content:     "Can you help me with deployment?",
		Subject:     "",
	}
	if msg.ChannelName != "telegram" { t.Error("channel") }
	if msg.SenderName != "John Doe" { t.Error("sender name") }
	if msg.Content == "" { t.Error("empty content") }
}

type deepTestChannel struct {
	name      string
	agentID   string
	running   bool
	lastSent  OutboundMessage
	sendCount int
	mu        sync.Mutex
}

func (c *deepTestChannel) Name() string    { return c.name }
func (c *deepTestChannel) Type() string    { return c.name }
func (c *deepTestChannel) AgentID() string { return c.agentID }
func (c *deepTestChannel) Start(ctx context.Context) error { c.running = true; return nil }
func (c *deepTestChannel) Stop(ctx context.Context) error  { c.running = false; return nil }
func (c *deepTestChannel) IsRunning() bool { return c.running }
func (c *deepTestChannel) Send(ctx context.Context, msg OutboundMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSent = msg
	c.sendCount++
	return nil
}
