// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// WorkingMemory tracks recent events with progressive compression.
// Today = full detail, yesterday = summary, older = discarded.
// Injected into system prompt as a "memory bulletin" for context awareness.
//
type WorkingMemory struct {
	events   []WorkingEvent
	maxEvents int
	mu       sync.Mutex
}

// WorkingEvent is a single event in working memory.
type WorkingEvent struct {
	Type       EventType
	Content    string
	Importance float64 // 0.0-1.0
	Channel    string  // optional channel scope
	Timestamp  time.Time
}

// EventType categorizes working memory events.
type EventType string

const (
	EventUserMessage    EventType = "user_message"
	EventAgentResponse  EventType = "agent_response"
	EventToolCall       EventType = "tool_call"
	EventToolResult     EventType = "tool_result"
	EventMemorySaved    EventType = "memory_saved"
	EventWorkerSpawned  EventType = "worker_spawned"
	EventWorkerComplete EventType = "worker_complete"
	EventError          EventType = "error"
	EventSessionStart   EventType = "session_start"
	EventSessionReset   EventType = "session_reset"
	EventCompaction     EventType = "compaction"
	EventHeartbeat      EventType = "heartbeat"
)

// NewWorkingMemory creates a working memory with default capacity.
func NewWorkingMemory() *WorkingMemory {
	return &WorkingMemory{maxEvents: 100}
}

// Emit records a new event.
func (wm *WorkingMemory) Emit(eventType EventType, content string) *eventBuilder {
	return &eventBuilder{
		wm: wm,
		event: WorkingEvent{
			Type:       eventType,
			Content:    content,
			Importance: 0.3, // default
			Timestamp:  time.Now(),
		},
	}
}

type eventBuilder struct {
	wm    *WorkingMemory
	event WorkingEvent
}

func (b *eventBuilder) Importance(v float64) *eventBuilder {
	b.event.Importance = v
	return b
}

func (b *eventBuilder) Channel(ch string) *eventBuilder {
	b.event.Channel = ch
	return b
}

func (b *eventBuilder) Record() {
	b.wm.mu.Lock()
	defer b.wm.mu.Unlock()

	b.wm.events = append(b.wm.events, b.event)

	// Trim to max
	if len(b.wm.events) > b.wm.maxEvents {
		b.wm.events = b.wm.events[len(b.wm.events)-b.wm.maxEvents:]
	}
}

// ForSystemPrompt returns the memory bulletin for the system prompt.
// Progressive compression: today=detail, yesterday=summary, older=discarded.
func (wm *WorkingMemory) ForSystemPrompt() string {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if len(wm.events) == 0 {
		return ""
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterdayStart := todayStart.AddDate(0, 0, -1)

	var todayEvents, yesterdayEvents []WorkingEvent
	for _, e := range wm.events {
		if e.Timestamp.After(todayStart) {
			todayEvents = append(todayEvents, e)
		} else if e.Timestamp.After(yesterdayStart) {
			yesterdayEvents = append(yesterdayEvents, e)
		}
		// Older events discarded from bulletin
	}

	var b strings.Builder
	b.WriteString("## Working Memory\n")

	if len(todayEvents) > 0 {
		b.WriteString("### Today\n")
		for _, e := range todayEvents {
			if e.Importance < 0.2 {
				continue // skip low-importance for prompt
			}
			b.WriteString(fmt.Sprintf("- [%s] %s (%s)\n",
				e.Timestamp.Format("15:04"),
				truncateWM(e.Content, 120),
				e.Type))
		}
	}

	if len(yesterdayEvents) > 0 {
		// Yesterday: summarize by type (compressed)
		typeCounts := make(map[EventType]int)
		for _, e := range yesterdayEvents {
			typeCounts[e.Type]++
		}
		b.WriteString("### Yesterday (summary)\n")
		for t, count := range typeCounts {
			b.WriteString(fmt.Sprintf("- %s: %d events\n", t, count))
		}
	}

	return b.String()
}

// Clear removes all events.
func (wm *WorkingMemory) Clear() {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.events = wm.events[:0]
}

func truncateWM(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
