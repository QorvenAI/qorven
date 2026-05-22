// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	if t.DB == nil {
		t.sendFormatted(ctx, chatID, 0, "⚠️ Database not available.")
		return
	}

	rows, err := t.DB.Query(ctx,
		`SELECT username, display_name, created_at FROM agent_writers WHERE agent_id = $1 ORDER BY created_at`,
		t.cfg.AgentID,
	)
	if err != nil {
		t.sendFormatted(ctx, chatID, 0, "⚠️ Could not load writers: "+err.Error())
		return
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var username, displayName string
		var createdAt time.Time
		if rows.Scan(&username, &displayName, &createdAt) == nil {
			name := "@" + username
			if displayName != "" && displayName != username {
				name = displayName + " (@" + username + ")"
			}
			lines = append(lines, "• "+name)
		}
	}

	text := "📝 **File Writers**\n\n"
	if len(lines) == 0 {
		text += "_No writers yet._\n\n"
	} else {
		text += strings.Join(lines, "\n") + "\n\n"
	}
	text += "Commands:\n"
	text += "• `/writer_add @username` — Grant write access\n"
	text += "• `/writer_remove @username` — Revoke access\n"
	text += "• `/writers` — List current writers"
	t.sendFormatted(ctx, chatID, 0, text)
}

func (t *TelegramChannel) handleAddWriter(ctx context.Context, b *bot.Bot, msg *models.Message) {
	chatID := msg.Chat.ID
	if t.DB == nil {
		t.sendFormatted(ctx, chatID, 0, "⚠️ Database not available.")
		return
	}
	if msg.Entities == nil {
		t.sendFormatted(ctx, chatID, 0, "Usage: `/writer_add @username`")
		return
	}
	for _, entity := range msg.Entities {
		if entity.Type == "mention" {
			username := msg.Text[entity.Offset+1 : entity.Offset+entity.Length] // strip @
			_, err := t.DB.Exec(ctx,
				`INSERT INTO agent_writers (agent_id, username, display_name, granted_by)
				 VALUES ($1::uuid, $2, $2, $3)
				 ON CONFLICT (agent_id, username) DO NOTHING`,
				t.cfg.AgentID, username, fmt.Sprintf("%d", chatID),
			)
			if err != nil {
				t.sendFormatted(ctx, chatID, 0, "⚠️ Failed to add writer: "+err.Error())
				return
			}
			t.sendFormatted(ctx, chatID, 0, fmt.Sprintf("✅ Added **@%s** as a writer.", username))
			return
		}
		if entity.Type == "text_mention" && entity.User != nil {
			name := buildUserName(entity.User)
			username := fmt.Sprintf("id%d", entity.User.ID)
			_, err := t.DB.Exec(ctx,
				`INSERT INTO agent_writers (agent_id, username, user_id, display_name, granted_by)
				 VALUES ($1::uuid, $2, $3, $4, $5)
				 ON CONFLICT (agent_id, username) DO UPDATE SET display_name = $4`,
				t.cfg.AgentID, username, entity.User.ID, name, fmt.Sprintf("%d", chatID),
			)
			if err != nil {
				t.sendFormatted(ctx, chatID, 0, "⚠️ Failed to add writer: "+err.Error())
				return
			}
			t.sendFormatted(ctx, chatID, 0, fmt.Sprintf("✅ Added **%s** as a writer.", name))
			return
		}
	}
	t.sendFormatted(ctx, chatID, 0, "Usage: `/writer_add @username`")
}

func (t *TelegramChannel) handleRemoveWriter(ctx context.Context, b *bot.Bot, msg *models.Message) {
	chatID := msg.Chat.ID
	if t.DB == nil {
		t.sendFormatted(ctx, chatID, 0, "⚠️ Database not available.")
		return
	}
	if msg.Entities == nil {
		t.sendFormatted(ctx, chatID, 0, "Usage: `/writer_remove @username`")
		return
	}
	for _, entity := range msg.Entities {
		if entity.Type == "mention" {
			username := msg.Text[entity.Offset+1 : entity.Offset+entity.Length]
			tag, err := t.DB.Exec(ctx,
				`DELETE FROM agent_writers WHERE agent_id = $1::uuid AND username = $2`,
				t.cfg.AgentID, username,
			)
			if err != nil {
				t.sendFormatted(ctx, chatID, 0, "⚠️ Failed to remove writer: "+err.Error())
				return
			}
			if tag.RowsAffected() == 0 {
				t.sendFormatted(ctx, chatID, 0, fmt.Sprintf("@%s is not a writer.", username))
				return
			}
			t.sendFormatted(ctx, chatID, 0, fmt.Sprintf("🗑 Removed **@%s** from writers.", username))
			return
		}
		if entity.Type == "text_mention" && entity.User != nil {
			username := fmt.Sprintf("id%d", entity.User.ID)
			name := buildUserName(entity.User)
			tag, err := t.DB.Exec(ctx,
				`DELETE FROM agent_writers WHERE agent_id = $1::uuid AND username = $2`,
				t.cfg.AgentID, username,
			)
			if err != nil {
				t.sendFormatted(ctx, chatID, 0, "⚠️ Failed to remove writer: "+err.Error())
				return
			}
			if tag.RowsAffected() == 0 {
				t.sendFormatted(ctx, chatID, 0, fmt.Sprintf("%s is not a writer.", name))
				return
			}
			t.sendFormatted(ctx, chatID, 0, fmt.Sprintf("🗑 Removed **%s** from writers.", name))
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
