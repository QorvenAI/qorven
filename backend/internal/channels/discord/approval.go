// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package discord

import (
	"context"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/qorvenai/qorven/internal/channels"
)

// discordPendingApproval tracks an in-flight approval request.
type discordPendingApproval struct {
	requestID string
	channelID string
	messageID string
	command   string
	result    *channels.ApprovalResult
	expiresAt time.Time
}

// SendApprovalRequest sends a Discord message with interactive Approve/Deny buttons
// using Discord's component API. Implements channels.ApprovalChannel.
func (d *DiscordChannel) SendApprovalRequest(ctx context.Context, chatID, message string, opts channels.ApprovalOptions) (string, error) {
	if d.session == nil {
		return "", fmt.Errorf("not connected")
	}

	requestID := fmt.Sprintf("dapprove_%d", time.Now().UnixMilli())

	approveText := opts.ApproveText
	if approveText == "" {
		approveText = "✅ Approve"
	}
	denyText := opts.DenyText
	if denyText == "" {
		denyText = "❌ Deny"
	}

	riskEmoji := map[string]string{
		"low": "🟢", "medium": "🟡", "high": "🔴", "critical": "💀",
	}
	emoji := riskEmoji[opts.Risk]
	if emoji == "" {
		emoji = "⚠️"
	}

	body := fmt.Sprintf("**%s Approval Required**\n\n%s\n\n```\n%s\n```\nRisk: **%s** | Timeout: %ds",
		emoji, message, opts.Command, opts.Risk, opts.Timeout)

	msg, err := d.session.ChannelMessageSendComplex(chatID, &discordgo.MessageSend{
		Content: body,
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    approveText,
						Style:    discordgo.SuccessButton,
						CustomID: "approval_yes:" + requestID,
					},
					discordgo.Button{
						Label:    denyText,
						Style:    discordgo.DangerButton,
						CustomID: "approval_no:" + requestID,
					},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("discord approval send: %w", err)
	}

	pa := &discordPendingApproval{
		requestID: requestID,
		channelID: chatID,
		messageID: msg.ID,
		command:   opts.Command,
		expiresAt: time.Now().Add(time.Duration(opts.Timeout) * time.Second),
	}
	d.approvals.Store(requestID, pa)

	// Register the interaction handler once (idempotent — discordgo dedups handlers).
	d.session.AddHandler(d.onInteractionCreate)

	return requestID, nil
}

// GetApprovalResult polls the result of an approval request.
// Returns nil result (no error) if still pending.
func (d *DiscordChannel) GetApprovalResult(_ context.Context, requestID string) (*channels.ApprovalResult, error) {
	v, ok := d.approvals.Load(requestID)
	if !ok {
		return nil, fmt.Errorf("approval %s not found", requestID)
	}
	pa := v.(*discordPendingApproval)

	// Auto-deny on timeout.
	if pa.result == nil && time.Now().After(pa.expiresAt) {
		pa.result = &channels.ApprovalResult{RequestID: requestID, Approved: false, Reason: "timeout"}
		d.updateApprovalMessage(pa, false, "timed out")
	}

	return pa.result, nil
}

// onInteractionCreate handles Discord button clicks for approval flows.
func (d *DiscordChannel) onInteractionCreate(_ *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}
	data := i.MessageComponentData()
	customID := data.CustomID

	var approved bool
	var requestID string
	switch {
	case len(customID) > 13 && customID[:13] == "approval_yes:":
		approved = true
		requestID = customID[13:]
	case len(customID) > 12 && customID[:12] == "approval_no:":
		approved = false
		requestID = customID[12:]
	default:
		return
	}

	v, ok := d.approvals.Load(requestID)
	if !ok {
		return
	}
	pa := v.(*discordPendingApproval)
	if pa.result != nil {
		return // already answered
	}

	userName := ""
	if i.Member != nil && i.Member.User != nil {
		userName = i.Member.User.Username
	} else if i.User != nil {
		userName = i.User.Username
	}

	pa.result = &channels.ApprovalResult{RequestID: requestID, Approved: approved}
	if approved {
		pa.result.ApprovedBy = userName
	} else {
		pa.result.DeniedBy = userName
		pa.result.Reason = "denied by " + userName
	}

	// Ack the interaction and update the message.
	d.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
	d.updateApprovalMessage(pa, approved, userName)
}

func (d *DiscordChannel) updateApprovalMessage(pa *discordPendingApproval, approved bool, by string) {
	if d.session == nil || pa.messageID == "" {
		return
	}
	status := "✅ Approved"
	if !approved {
		status = "❌ Denied"
	}
	label := fmt.Sprintf("%s by **%s**", status, by)
	// Edit the original message to show the outcome and remove buttons.
	d.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    pa.channelID,
		ID:         pa.messageID,
		Content:    &label,
		Components: &[]discordgo.MessageComponent{}, // remove buttons
	})
}
