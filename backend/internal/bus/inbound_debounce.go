// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package bus

import (
	"log/slog"
	"strings"
	"sync"
	"time"
)

// InboundDebouncer buffers rapid inbound messages from the same sender
// and merges them into a single message before calling flushFn.
type InboundDebouncer struct {
	debounce time.Duration
	mu         sync.Mutex
	buffers    map[string]*debounceBuffer
	flushFn    func(InboundMessage)
}

type debounceBuffer struct {
	messages []InboundMessage
	timer    *time.Timer
}

// NewInboundDebouncer creates a debouncer with the given window and flush callback.
func NewInboundDebouncer(window time.Duration, flushFn func(InboundMessage)) *InboundDebouncer {
	return &InboundDebouncer{
		debounce: window,
		buffers:    make(map[string]*debounceBuffer),
		flushFn:    flushFn,
	}
}

// Push adds a message to the debounce buffer.
func (d *InboundDebouncer) Push(msg InboundMessage) {
	if d.debounce <= 0 {
		d.flushFn(msg)
		return
	}

	key := debounceKey(msg)

	// Media messages bypass debounce
	if len(msg.Media) > 0 {
		d.flushKey(key)
		d.flushFn(msg)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	buf, exists := d.buffers[key]
	if !exists {
		buf = &debounceBuffer{}
		d.buffers[key] = buf
	}

	buf.messages = append(buf.messages, msg)

	if buf.timer != nil {
		buf.timer.Stop()
	}
	buf.timer = time.AfterFunc(d.debounce, func() {
		d.flushKey(key)
	})

	if len(buf.messages) == 1 {
		slog.Debug("inbound debounce: buffering", "key", key)
	}
}

// Stop flushes all pending buffers immediately.
func (d *InboundDebouncer) Stop() {
	d.mu.Lock()
	keys := make([]string, 0, len(d.buffers))
	for k := range d.buffers {
		keys = append(keys, k)
	}
	d.mu.Unlock()

	for _, key := range keys {
		d.flushKey(key)
	}
}

func (d *InboundDebouncer) flushKey(key string) {
	d.mu.Lock()
	buf, exists := d.buffers[key]
	if !exists || len(buf.messages) == 0 {
		d.mu.Unlock()
		return
	}

	if buf.timer != nil {
		buf.timer.Stop()
	}

	msgs := buf.messages
	delete(d.buffers, key)
	d.mu.Unlock()

	merged := mergeInboundMessages(msgs)

	if len(msgs) > 1 {
		slog.Info("inbound debounce: merged messages", "key", key, "count", len(msgs))
	}

	d.flushFn(merged)
}

func debounceKey(msg InboundMessage) string {
	return msg.Channel + ":" + msg.ChatID + ":" + msg.SenderID
}

func mergeInboundMessages(msgs []InboundMessage) InboundMessage {
	if len(msgs) == 1 {
		return msgs[0]
	}

	last := msgs[len(msgs)-1]

	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m.Content != "" {
			parts = append(parts, m.Content)
		}
	}
	last.Content = strings.Join(parts, "\n")

	allMedia := []MediaFile{}
	for _, m := range msgs {
		allMedia = append(allMedia, m.Media...)
	}
	last.Media = allMedia

	return last
}
