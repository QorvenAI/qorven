// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// MessageCoalescer batches rapid-fire messages into a single LLM turn.
// In group chats, 5 messages in 2 seconds become ONE turn with timing
// context so the agent "reads the room." DMs bypass coalescing (always immediate).
type MessageCoalescer struct {
	mu       sync.Mutex
	debounce time.Duration
	buffer   []coalescedMsg
	timer    *time.Timer
	onFlush  func(combined string)
}

type coalescedMsg struct {
	UserID  string
	Content string
	At      time.Time
}

func NewCoalescer(debounce time.Duration, onFlush func(combined string)) *MessageCoalescer {
	if debounce <= 0 {
		debounce = 2 * time.Second
	}
	return &MessageCoalescer{debounce: debounce, onFlush: onFlush}
}

// Add buffers a message. If the debounce timer fires without new messages,
// all buffered messages are flushed as one combined turn.
// Returns true if the message was buffered (caller should NOT process it yet).
// Returns false if coalescing is disabled (caller should process immediately).
func (c *MessageCoalescer) Add(userID, content string) bool {
	if c == nil || c.onFlush == nil {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.buffer = append(c.buffer, coalescedMsg{UserID: userID, Content: content, At: time.Now()})

	// Reset debounce timer
	if c.timer != nil {
		c.timer.Stop()
	}
	c.timer = time.AfterFunc(c.debounce, c.flush)

	return len(c.buffer) > 1 // only buffer if there are multiple messages
}

func (c *MessageCoalescer) flush() {
	c.mu.Lock()
	msgs := c.buffer
	c.buffer = nil
	c.timer = nil
	c.mu.Unlock()

	if len(msgs) == 0 {
		return
	}

	if len(msgs) == 1 {
		c.onFlush(msgs[0].Content)
		return
	}

	// Multiple messages — combine with timing context
	combined := FormatCoalesced(msgs)
	c.onFlush(combined)
}

// FormatCoalesced formats multiple messages into a single LLM turn with timing.
func FormatCoalesced(msgs []coalescedMsg) string {
	if len(msgs) == 1 {
		return msgs[0].Content
	}

	base := msgs[0].At
	var b strings.Builder
	fmt.Fprintf(&b, "[%d messages arrived in %.1fs]\n", len(msgs), msgs[len(msgs)-1].At.Sub(base).Seconds())
	for _, m := range msgs {
		elapsed := m.At.Sub(base).Seconds()
		if m.UserID != "" {
			fmt.Fprintf(&b, "User %s (%.1fs): %s\n", m.UserID, elapsed, m.Content)
		} else {
			fmt.Fprintf(&b, "(%.1fs): %s\n", elapsed, m.Content)
		}
	}
	return b.String()
}
