// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package bus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Hard bus tests — pub/sub, concurrent producers/consumers, backpressure.

func TestMessageBus_New(t *testing.T) {
	mb := New()
	if mb == nil { t.Fatal("nil bus") }
}

func TestMessageBus_PublishConsume_Inbound(t *testing.T) {
	mb := New()
	go func() {
		mb.PublishInbound(InboundMessage{Content: "hello"})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msg, ok := mb.ConsumeInbound(ctx)
	if !ok { t.Fatal("should consume") }
	if msg.Content != "hello" { t.Errorf("content=%q", msg.Content) }
}

func TestMessageBus_PublishConsume_Outbound(t *testing.T) {
	mb := New()
	go func() {
		mb.PublishOutbound(OutboundMessage{Content: "reply"})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msg, ok := mb.SubscribeOutbound(ctx)
	if !ok { t.Fatal("should consume") }
	if msg.Content != "reply" { t.Errorf("content=%q", msg.Content) }
}

func TestMessageBus_TryPublish_Success(t *testing.T) {
	mb := New()
	// Start consumer to prevent channel full
	go func() {
		ctx := context.Background()
		for { mb.ConsumeInbound(ctx) }
	}()
	time.Sleep(10 * time.Millisecond)
	ok := mb.TryPublishInbound(InboundMessage{Content: "test"})
	if !ok { t.Error("should publish when consumer is running") }
}

func TestMessageBus_ConsumeInbound_Timeout(t *testing.T) {
	mb := New()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, ok := mb.ConsumeInbound(ctx)
	if ok { t.Error("should timeout with no messages") }
}

func TestMessageBus_RegisterHandler(t *testing.T) {
	mb := New()
	mb.RegisterHandler("telegram", func(msg InboundMessage) error {
		
		return nil
	})
	h, ok := mb.GetHandler("telegram")
	if !ok { t.Error("should find handler") }
	if h == nil { t.Error("nil handler") }
}

func TestMessageBus_GetHandler_NotFound(t *testing.T) {
	mb := New()
	_, ok := mb.GetHandler("nonexistent")
	if ok { t.Error("should not find unregistered handler") }
}

func TestMessageBus_ConcurrentPublish(t *testing.T) {
	mb := New()
	var consumed atomic.Int32

	// Consumer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		for {
			_, ok := mb.ConsumeInbound(ctx)
			if !ok { return }
			consumed.Add(1)
		}
	}()

	// 100 concurrent publishers
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			mb.PublishInbound(InboundMessage{Content: "msg"})
		}(i)
	}
	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	if consumed.Load() < 50 { t.Errorf("consumed only %d/100", consumed.Load()) }
}

func TestMessageBus_MultipleHandlers(t *testing.T) {
	mb := New()
	mb.RegisterHandler("telegram", func(msg InboundMessage) error { return nil })
	mb.RegisterHandler("discord", func(msg InboundMessage) error { return nil })
	mb.RegisterHandler("slack", func(msg InboundMessage) error { return nil })

	for _, ch := range []string{"telegram", "discord", "slack"} {
		_, ok := mb.GetHandler(ch)
		if !ok { t.Errorf("missing handler for %s", ch) }
	}
}

func TestMessageBus_OverwriteHandler(t *testing.T) {
	mb := New()
	mb.RegisterHandler("test", func(msg InboundMessage) error { return nil })
	mb.RegisterHandler("test", func(msg InboundMessage) error { return nil })
	_, ok := mb.GetHandler("test")
	if !ok { t.Error("should still have handler after overwrite") }
}
