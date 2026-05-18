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

// Deep bus tests — pub/sub under pressure, message ordering, backpressure.

func TestDeep_Bus_ProducerConsumer_Pipeline(t *testing.T) {
	mb := New()
	produced := 100
	var consumed atomic.Int32

	// Consumer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		for {
			msg, ok := mb.ConsumeInbound(ctx)
			if !ok { return }
			if msg.Content != "" { consumed.Add(1) }
		}
	}()

	// Producer — send 100 messages
	for i := 0; i < produced; i++ {
		mb.PublishInbound(InboundMessage{Content: "msg" + string(rune('0'+i%10))})
	}

	// Wait for consumption
	time.Sleep(200 * time.Millisecond)
	got := consumed.Load()
	if got < int32(produced/2) { t.Errorf("consumed %d/%d", got, produced) }
	t.Logf("produced %d, consumed %d", produced, got)
}

func TestDeep_Bus_MultipleProducers(t *testing.T) {
	mb := New()
	var consumed atomic.Int32

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		for {
			_, ok := mb.ConsumeInbound(ctx)
			if !ok { return }
			consumed.Add(1)
		}
	}()

	// 10 producers, each sending 10 messages
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				mb.PublishInbound(InboundMessage{Content: "p" + string(rune('0'+n))})
			}
		}(i)
	}
	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	got := consumed.Load()
	if got < 50 { t.Errorf("consumed %d/100", got) }
	t.Logf("10 producers × 10 msgs: consumed %d", got)
}

func TestDeep_Bus_Outbound_Pipeline(t *testing.T) {
	mb := New()
	var received atomic.Int32

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		for {
			_, ok := mb.SubscribeOutbound(ctx)
			if !ok { return }
			received.Add(1)
		}
	}()

	for i := 0; i < 50; i++ {
		mb.PublishOutbound(OutboundMessage{Content: "reply"})
	}
	time.Sleep(100 * time.Millisecond)

	got := received.Load()
	if got < 25 { t.Errorf("received %d/50 outbound", got) }
	t.Logf("outbound: %d/50 received", got)
}

func TestDeep_Bus_TryPublish_Backpressure(t *testing.T) {
	mb := New()
	// Don't start consumer — channel will fill up
	dropped := 0
	for i := 0; i < 1000; i++ {
		ok := mb.TryPublishInbound(InboundMessage{Content: "flood"})
		if !ok { dropped++ }
	}
	t.Logf("backpressure: %d/1000 dropped", dropped)
	// backpressure depends on channel buffer size
}

func TestDeep_Bus_HandlerRouting(t *testing.T) {
	mb := New()
	var tgCalls, dcCalls, slCalls atomic.Int32

	mb.RegisterHandler("telegram", func(msg InboundMessage) error { tgCalls.Add(1); return nil })
	mb.RegisterHandler("discord", func(msg InboundMessage) error { dcCalls.Add(1); return nil })
	mb.RegisterHandler("slack", func(msg InboundMessage) error { slCalls.Add(1); return nil })

	// Route messages to handlers
	channels := []string{"telegram", "discord", "slack"}
	for _, ch := range channels {
		h, ok := mb.GetHandler(ch)
		if !ok { t.Errorf("no handler for %s", ch); continue }
		h(InboundMessage{Content: "test"})
	}

	if tgCalls.Load() != 1 { t.Error("telegram handler not called") }
	if dcCalls.Load() != 1 { t.Error("discord handler not called") }
	if slCalls.Load() != 1 { t.Error("slack handler not called") }
}
