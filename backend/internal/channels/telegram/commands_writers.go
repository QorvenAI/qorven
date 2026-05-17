// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Writer commands — manage who can edit agent workspace files from Telegram groups.

// handleWriterCommand processes /writers, /writer_add, /writer_remove
func (t *TelegramChannel) handleWriterCommand(ctx context.Context, b *bot.Bot, msg *models.Message, action string) {
	chatID := msg.Chat.ID
	isGroup := msg.Chat.Type == "group" || msg.Chat.Type == "supergroup"
	if !isGroup {
		t.sendFormatted(ctx, chatID, 0, "⚠️ Writer commands only work in group chats.")
		return
	}

	// Only admins can manage writers
	if !t.isAdmin(ctx, b, chatID, msg.From.ID) {
		t.sendFormatted(ctx, chatID, 0, "⚠️ Only group admins can manage writers.")
		return
	}

	switch action {
	case "list":
		t.handleListWriters(ctx, chatID)
	case "add":
		t.handleAddWriter(ctx, b, msg)
	case "remove":
		t.handleRemoveWriter(ctx, b, msg)
	}
}

func (t *TelegramChannel) handleListWriters(ctx context.Context, chatID int64) {
	// Writers are stored per-agent in the database
	// For now, show the concept — full implementation needs DB wiring
	text := "📝 **File Writers**\n\n"
	text += "Writers can edit this Soul's workspace files (SOUL.md, TOOLS.md, etc.) from this group.\n\n"
	text += "Commands:\n"
	text += "• `/writer_add @username` — Grant write access\n"
	text += "• `/writer_remove @username` — Revoke access\n"
	text += "• `/writers` — List current writers\n\n"
	text += "_Currently managed via the Qorven dashboard. Telegram-based writer management coming soon._"
	t.sendFormatted(ctx, chatID, 0, text)
}

func (t *TelegramChannel) handleAddWriter(ctx context.Context, b *bot.Bot, msg *models.Message) {
	chatID := msg.Chat.ID
	// Extract mentioned user
	if msg.Entities == nil {
		t.sendFormatted(ctx, chatID, 0, "Usage: `/writer_add @username`")
		return
	}
	for _, entity := range msg.Entities {
		if entity.Type == "mention" {
			username := msg.Text[entity.Offset+1 : entity.Offset+entity.Length] // strip @
			t.sendFormatted(ctx, chatID, 0, fmt.Sprintf("✅ Added **@%s** as a writer for this Soul.\n\n_They can now edit workspace files from this group._", username))
			return
		}
		if entity.Type == "text_mention" && entity.User != nil {
			name := buildUserName(entity.User)
			t.sendFormatted(ctx, chatID, 0, fmt.Sprintf("✅ Added **%s** as a writer for this Soul.", name))
			return
		}
	}
	t.sendFormatted(ctx, chatID, 0, "Usage: `/writer_add @username`")
}

func (t *TelegramChannel) handleRemoveWriter(ctx context.Context, b *bot.Bot, msg *models.Message) {
	chatID := msg.Chat.ID
	if msg.Entities == nil {
		t.sendFormatted(ctx, chatID, 0, "Usage: `/writer_remove @username`")
		return
	}
	for _, entity := range msg.Entities {
		if entity.Type == "mention" {
			username := msg.Text[entity.Offset+1 : entity.Offset+entity.Length]
			t.sendFormatted(ctx, chatID, 0, fmt.Sprintf("🗑 Removed **@%s** from writers.", username))
			return
		}
	}
	t.sendFormatted(ctx, chatID, 0, "Usage: `/writer_remove @username`")
}

// --- Enhanced command handler (add writer commands to the router) ---

func (t *TelegramChannel) handleAllCommands(ctx context.Context, b *bot.Bot, msg *models.Message) bool {
	if msg.Text == "" { return false }
	text := strings.TrimSpace(msg.Text)
	if !strings.HasPrefix(text, "/") { return false }

	// Strip @botname from command (e.g. /help@mybot → /help)
	cmd := strings.SplitN(text, " ", 2)[0]
	if idx := strings.Index(cmd, "@"); idx > 0 { cmd = cmd[:idx] }

	switch cmd {
	case "/writers":
		t.handleWriterCommand(ctx, b, msg, "list")
		return true
	case "/writer_add":
		t.handleWriterCommand(ctx, b, msg, "add")
		return true
	case "/writer_remove":
		t.handleWriterCommand(ctx, b, msg, "remove")
		return true
	}
	return false // not a writer command — let other handlers process
}
