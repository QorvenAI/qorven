// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package channels

import (
	"strings"
	"sync"
	"time"
)

// Debouncer merges rapid messages from the same sender before processing.
// Without this, "hey" + "check weather" + "in Dubai" = 3 agent runs = 3x token cost.
type Debouncer struct {
	mu       sync.Mutex
	buffers  map[string]*debounceBuffer
	window   time.Duration
	flushFn  func(InboundMessage)
}

type debounceBuffer struct {
	messages []InboundMessage
	timer    *time.Timer
}

func NewDebouncer(window time.Duration, flushFn func(InboundMessage)) *Debouncer {
	if window <= 0 {
		window = 600 * time.Millisecond
	}
	return &Debouncer{buffers: make(map[string]*debounceBuffer), window: window, flushFn: flushFn}
}

func debounceKey(msg InboundMessage) string {
	return msg.ChannelType + ":" + msg.Metadata["chat_id"] + ":" + msg.SenderID
}

// Push adds a message to the debounce buffer. If the message has media, flush immediately.
func (d *Debouncer) Push(msg InboundMessage) {
	// Media bypasses debounce — process immediately
	if msg.Metadata["has_media"] == "true" {
		d.mu.Lock()
		key := debounceKey(msg)
		if buf, ok := d.buffers[key]; ok {
			buf.timer.Stop()
			merged := d.merge(buf.messages)
			delete(d.buffers, key)
			d.mu.Unlock()
			d.flushFn(merged)
			d.flushFn(msg)
			return
		}
		d.mu.Unlock()
		d.flushFn(msg)
		return
	}

	d.mu.Lock()
	key := debounceKey(msg)
	buf, ok := d.buffers[key]
	if ok {
		buf.timer.Stop()
		buf.messages = append(buf.messages, msg)
	} else {
		buf = &debounceBuffer{messages: []InboundMessage{msg}}
		d.buffers[key] = buf
	}
	buf.timer = time.AfterFunc(d.window, func() {
		d.mu.Lock()
		msgs := buf.messages
		delete(d.buffers, key)
		d.mu.Unlock()
		if len(msgs) > 0 {
			d.flushFn(d.merge(msgs))
		}
	})
	d.mu.Unlock()
}

func (d *Debouncer) merge(msgs []InboundMessage) InboundMessage {
	if len(msgs) == 1 {
		return msgs[0]
	}
	var parts []string
	for _, m := range msgs {
		if m.Content != "" {
			parts = append(parts, m.Content)
		}
	}
	last := msgs[len(msgs)-1]
	last.Content = strings.Join(parts, "\n")
	return last
}

// FlushAll flushes all pending buffers (call on shutdown).
func (d *Debouncer) FlushAll() {
	d.mu.Lock()
	for key, buf := range d.buffers {
		buf.timer.Stop()
		msgs := buf.messages
		delete(d.buffers, key)
		if len(msgs) > 0 {
			go d.flushFn(d.merge(msgs))
		}
	}
	d.mu.Unlock()
}
