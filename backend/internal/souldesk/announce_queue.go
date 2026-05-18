// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package souldesk

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// AnnounceEntry holds one Soul's completion result waiting to be announced.
type AnnounceEntry struct {
	SoulKey     string
	DisplayName string
	Content     string
	TaskID      string
	Failed      bool
}

// AnnounceQueue batches delegation results for a leader session.
// When multiple Souls complete within the debounce window, their results
// are merged into ONE announcement instead of N separate interruptions.
type AnnounceQueue struct {
	debounce time.Duration
	deliver  func(ctx context.Context, primeID, sessionID string, entries []AnnounceEntry)

	mu     sync.Mutex
	queues map[string]*sessionAnnounce // key: primeID:sessionID
}

type sessionAnnounce struct {
	mu      sync.Mutex
	entries []AnnounceEntry
	timer   *time.Timer
	primeID string
	sessID  string
}

// NewAnnounceQueue creates a batched announce queue.
// deliver is called with the merged batch when the debounce window expires.
func NewAnnounceQueue(debounce time.Duration, deliver func(ctx context.Context, primeID, sessionID string, entries []AnnounceEntry)) *AnnounceQueue {
	return &AnnounceQueue{
		debounce: debounce,
		deliver:  deliver,
		queues:   make(map[string]*sessionAnnounce),
	}
}

// Enqueue adds a completion result. If this is the first entry, starts a debounce timer.
// If more entries arrive before the timer fires, they're batched together.
func (aq *AnnounceQueue) Enqueue(primeID, sessionID string, entry AnnounceEntry) {
	key := primeID + ":" + sessionID

	aq.mu.Lock()
	sa, ok := aq.queues[key]
	if !ok {
		sa = &sessionAnnounce{primeID: primeID, sessID: sessionID}
		aq.queues[key] = sa
	}
	aq.mu.Unlock()

	sa.mu.Lock()
	sa.entries = append(sa.entries, entry)

	// Reset or start debounce timer
	if sa.timer != nil {
		sa.timer.Stop()
	}
	sa.timer = time.AfterFunc(aq.debounce, func() {
		sa.mu.Lock()
		entries := sa.entries
		sa.entries = nil
		sa.mu.Unlock()

		// Clean up
		aq.mu.Lock()
		delete(aq.queues, key)
		aq.mu.Unlock()

		if len(entries) > 0 {
			slog.Info("announce.batch", "prime", primeID, "session", sessionID[:min(len(sessionID), 8)], "count", len(entries))
			aq.deliver(context.Background(), primeID, sessionID, entries)
		}
	})
	sa.mu.Unlock()
}

// BuildMergedContent creates the announce message for one or more completed/failed tasks.
func BuildMergedContent(entries []AnnounceEntry) string {
	var sb strings.Builder

	if len(entries) == 1 {
		e := entries[0]
		label := soulLabel(e)
		if e.Failed {
			fmt.Fprintf(&sb, "[System Message] Soul %q failed to complete task.\n\nError: %s", label, e.Content)
			sb.WriteString("\n\nInform the user about the failure.")
		} else {
			fmt.Fprintf(&sb, "[System Message] Soul %q completed task.\n\nResult:\n%s", label, e.Content)
		}
	} else {
		var failed, succeeded int
		for _, e := range entries {
			if e.Failed {
				failed++
			} else {
				succeeded++
			}
		}
		if failed > 0 && succeeded > 0 {
			fmt.Fprintf(&sb, "[System Message] %d task(s) completed, %d task(s) failed.\n", succeeded, failed)
		} else if failed > 0 {
			fmt.Fprintf(&sb, "[System Message] %d task(s) failed.\n", failed)
		} else {
			fmt.Fprintf(&sb, "[System Message] %d tasks completed.\n", succeeded)
		}
		for _, e := range entries {
			label := soulLabel(e)
			if e.Failed {
				fmt.Fprintf(&sb, "\n--- FAILED: %q ---\nError: %s\n", label, e.Content)
			} else {
				fmt.Fprintf(&sb, "\n--- Result from %q ---\n%s\n", label, e.Content)
			}
		}
	}

	sb.WriteString("\n\nPresent these results to the user. Any media files are forwarded automatically.")
	return sb.String()
}

func soulLabel(e AnnounceEntry) string {
	if e.DisplayName != "" {
		return fmt.Sprintf("%s (@%s)", e.DisplayName, e.SoulKey)
	}
	return "@" + e.SoulKey
}
