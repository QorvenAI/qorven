// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/qorvenai/qorven/internal/channels"
)

// InboundDedup prevents duplicate message processing from webhook retries.
// Uses a TTL cache keyed by channel+sender+chat+messageID.
type InboundDedup struct {
	mu    sync.Mutex
	seen  map[string]time.Time
	ttl   time.Duration
	max   int
}

// NewInboundDedup creates a dedup cache.
func NewInboundDedup(ttl time.Duration, max int) *InboundDedup {
	d := &InboundDedup{seen: make(map[string]time.Time), ttl: ttl, max: max}
	go d.cleanupLoop()
	return d
}

// IsDuplicate returns true if this message was already seen within the TTL window.
func (d *InboundDedup) IsDuplicate(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.seen[key]; ok {
		return true
	}
	d.seen[key] = time.Now()
	// Evict oldest if over max
	if len(d.seen) > d.max {
		oldest := ""
		oldestTime := time.Now()
		for k, t := range d.seen {
			if t.Before(oldestTime) {
				oldest = k
				oldestTime = t
			}
		}
		if oldest != "" {
			delete(d.seen, oldest)
		}
	}
	return false
}

func (d *InboundDedup) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		d.mu.Lock()
		cutoff := time.Now().Add(-d.ttl)
		for k, t := range d.seen {
			if t.Before(cutoff) {
				delete(d.seen, k)
			}
		}
		d.mu.Unlock()
	}
}

// InboundDebouncer merges rapid messages from the same sender before processing.
// Messages arriving within the debounce window are concatenated.
type InboundDebouncer struct {
	mu       sync.Mutex
	pending  map[string]*debouncedMsg
	window   time.Duration
	process  func(channels.InboundMessage)
}

type debouncedMsg struct {
	msg   channels.InboundMessage
	timer *time.Timer
}

// NewInboundDebouncer creates a debouncer with the given window.
func NewInboundDebouncer(window time.Duration, process func(channels.InboundMessage)) *InboundDebouncer {
	return &InboundDebouncer{
		pending: make(map[string]*debouncedMsg),
		window:  window,
		process: process,
	}
}

// Push adds a message. If another message from the same sender arrives within the window,
// they're concatenated. When the window expires, the merged message is processed.
func (d *InboundDebouncer) Push(msg channels.InboundMessage) {
	key := msg.ChannelType + "|" + msg.SenderID + "|" + msg.AgentID

	d.mu.Lock()
	defer d.mu.Unlock()

	if existing, ok := d.pending[key]; ok {
		// Merge: append content
		existing.msg.Content += "\n" + msg.Content
		existing.timer.Reset(d.window)
		return
	}

	dm := &debouncedMsg{msg: msg}
	dm.timer = time.AfterFunc(d.window, func() {
		d.mu.Lock()
		m := d.pending[key]
		delete(d.pending, key)
		d.mu.Unlock()
		if m != nil {
			d.process(m.msg)
		}
	})
	d.pending[key] = dm
}

// Stop cancels all pending timers.
func (d *InboundDebouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, dm := range d.pending {
		dm.timer.Stop()
	}
	d.pending = make(map[string]*debouncedMsg)
}

// TruncateForReminder truncates content for storage, takes last line, ensures valid UTF-8.
func TruncateForReminder(content string, maxLen int) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	msg := lines[len(lines)-1]
	msg = strings.ToValidUTF8(msg, "")
	if maxLen > 0 && utf8.RuneCountInString(msg) > maxLen {
		r := []rune(msg)
		msg = string(r[:maxLen]) + "..."
	}
	return msg
}
