// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package telegram

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/qorvenai/qorven/internal/channels"
)

// TelegramApproval implements native Telegram inline keyboard buttons for approval.

type tgPendingApproval struct {
	requestID string
	chatID    int64
	messageID int
	command   string
	result    *channels.ApprovalResult
	expiresAt time.Time
}

var (
	tgApprovalsMu sync.Mutex
	tgApprovals   = map[string]*tgPendingApproval{}
)

// SendApprovalRequest sends a Telegram message with inline keyboard approve/deny buttons.
func (t *TelegramChannel) SendApprovalRequest(ctx context.Context, chatID, message string, opts channels.ApprovalOptions) (string, error) {
	requestID := fmt.Sprintf("tg_approve_%d", time.Now().UnixMilli())

	var chatIDInt int64
	fmt.Sscanf(chatID, "%d", &chatIDInt)

	text := fmt.Sprintf("⚠️ *Approval Required*\n\n%s\n\n`%s`\n\nRisk: *%s*", message, opts.Command, opts.Risk)

	msg, err := t.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatIDInt,
		Text:      text,
		ParseMode: models.ParseModeMarkdown,
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{{
				{Text: opts.ApproveText, CallbackData: "approve:" + requestID},
				{Text: opts.DenyText, CallbackData: "deny:" + requestID},
			}},
		},
	})
	if err != nil { return "", err }

	tgApprovalsMu.Lock()
	tgApprovals[requestID] = &tgPendingApproval{
		requestID: requestID, chatID: chatIDInt, messageID: msg.ID,
		command: opts.Command,
		expiresAt: time.Now().Add(time.Duration(opts.Timeout) * time.Second),
	}
	tgApprovalsMu.Unlock()

	return requestID, nil
}

// GetApprovalResult checks if a Telegram approval request has been answered.
func (t *TelegramChannel) GetApprovalResult(_ context.Context, requestID string) (*channels.ApprovalResult, error) {
	tgApprovalsMu.Lock()
	defer tgApprovalsMu.Unlock()

	pa, ok := tgApprovals[requestID]
	if !ok { return nil, fmt.Errorf("approval %s not found", requestID) }

	if pa.result == nil && time.Now().After(pa.expiresAt) {
		pa.result = &channels.ApprovalResult{RequestID: requestID, Approved: false, Reason: "timeout"}
	}

	return pa.result, nil
}

// HandleTelegramApprovalCallback processes a Telegram callback query from inline buttons.
func HandleTelegramApprovalCallback(data string, userName string) {
	var requestID string
	var approved bool

	if len(data) > 8 && data[:8] == "approve:" {
		requestID = data[8:]
		approved = true
	} else if len(data) > 5 && data[:5] == "deny:" {
		requestID = data[5:]
		approved = false
	} else {
		return
	}

	tgApprovalsMu.Lock()
	defer tgApprovalsMu.Unlock()

	pa, ok := tgApprovals[requestID]
	if !ok { return }

	pa.result = &channels.ApprovalResult{RequestID: requestID, Approved: approved}
	if approved {
		pa.result.ApprovedBy = userName
	} else {
		pa.result.DeniedBy = userName
		pa.result.Reason = "denied by " + userName
	}
}
