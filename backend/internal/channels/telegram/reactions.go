// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package telegram

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Full reaction state machine with debounce, stall detection, status transitions.

const (
	reactionDebounceMs = 700
	stallTimeoutSec    = 30
)

var statusEmoji = map[string]string{
	"thinking": "🤔",
	"tool":     "🔧",
	"done":     "✅",
	"error":    "❌",
	"stall":    "⏳",
}

// ReactionController manages per-message reaction state with debounce and stall detection.
type ReactionController struct {
	b            *bot.Bot
	chatID       int64
	messageID    int
	currentEmoji string
	lastUpdate   time.Time
	stallTimer   *time.Timer
	debounceTimer *time.Timer
	pendingEmoji string
	mu           sync.Mutex
	stopped      bool
}

func newReactionController(b *bot.Bot, chatID int64, messageID int) *ReactionController {
	rc := &ReactionController{b: b, chatID: chatID, messageID: messageID}
	// Start stall timer — if no status update for 30s, show hourglass
	rc.stallTimer = time.AfterFunc(time.Duration(stallTimeoutSec)*time.Second, func() {
		rc.applyReaction(context.Background(), statusEmoji["stall"])
	})
	return rc
}

// SetStatus transitions the reaction to a new status with debounce.
func (rc *ReactionController) SetStatus(ctx context.Context, status string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.stopped { return }

	emoji, ok := statusEmoji[status]
	if !ok { return }

	// Reset stall timer on any activity
	if rc.stallTimer != nil { rc.stallTimer.Reset(time.Duration(stallTimeoutSec) * time.Second) }

	// Debounce: don't spam reactions
	if time.Since(rc.lastUpdate) < time.Duration(reactionDebounceMs)*time.Millisecond {
		rc.pendingEmoji = emoji
		if rc.debounceTimer == nil {
			rc.debounceTimer = time.AfterFunc(time.Duration(reactionDebounceMs)*time.Millisecond, func() {
				rc.mu.Lock()
				pending := rc.pendingEmoji
				rc.pendingEmoji = ""
				rc.debounceTimer = nil
				rc.mu.Unlock()
				if pending != "" { rc.applyReaction(ctx, pending) }
			})
		}
		return
	}

	rc.applyReaction(ctx, emoji)
}

// Stop cleans up timers and removes the reaction.
func (rc *ReactionController) Stop() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.stopped = true
	if rc.stallTimer != nil { rc.stallTimer.Stop() }
	if rc.debounceTimer != nil { rc.debounceTimer.Stop() }
}

func (rc *ReactionController) applyReaction(ctx context.Context, emoji string) {
	rc.mu.Lock()
	if rc.stopped || emoji == rc.currentEmoji {
		rc.mu.Unlock()
		return
	}
	rc.currentEmoji = emoji
	rc.lastUpdate = time.Now()
	rc.mu.Unlock()

	_, err := rc.b.SetMessageReaction(ctx, &bot.SetMessageReactionParams{
		ChatID:    rc.chatID,
		MessageID: rc.messageID,
		Reaction:  []models.ReactionType{{ReactionTypeEmoji: &models.ReactionTypeEmoji{Emoji: emoji}}},
	})
	if err != nil {
		slog.Debug("telegram.reaction.failed", "emoji", emoji, "error", err)
	}
}

// ClearReaction removes all reactions from a message.
func (rc *ReactionController) ClearReaction(ctx context.Context) {
	rc.mu.Lock()
	rc.currentEmoji = ""
	rc.mu.Unlock()
	rc.b.SetMessageReaction(ctx, &bot.SetMessageReactionParams{
		ChatID:    rc.chatID,
		MessageID: rc.messageID,
		Reaction:  []models.ReactionType{},
	})
}
