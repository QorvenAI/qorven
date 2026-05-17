// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/mail"
	"github.com/qorvenai/qorven/internal/tools"
	"context"
	"fmt"
	"strings"
	"log/slog"

	"github.com/qorvenai/qorven/internal/bus"
	"github.com/qorvenai/qorven/internal/channels"
)

// handleResetCommand processes /reset: clears session history.
// Returns true if handled.
func (gw *Gateway) handleResetCommand(ctx context.Context, msg channels.InboundMessage) bool {
	if msg.Metadata["command"] != "reset" {
		return false
	}

	if gw.sessions == nil || msg.AgentID == "" {
		return false
	}

	// Find the active session for this agent+sender
	sessions, _ := gw.sessions.ListForAgent(ctx, msg.AgentID, 1)
	if len(sessions) > 0 {
		gw.sessions.Reset(ctx, sessions[0].ID)
		slog.Info("channel.reset", "agent", msg.AgentID, "session", sessions[0].ID)
	}

	// Send confirmation
	chatID := msg.Metadata["chat_id"]
	if chatID == "" {
		chatID = msg.SenderID
	}
	gw.sendToChannel(ctx, msg.AgentID, chatID, "🔄 Session reset. Send a message to start fresh.", "", "")
	return true
}

// handleStopCommand processes /stop: cancels the active agent run.
// Returns true if handled.
func (gw *Gateway) handleStopCommand(ctx context.Context, msg channels.InboundMessage) bool {
	cmd := msg.Metadata["command"]
	if cmd != "stop" && cmd != "stopall" {
		return false
	}

	// Cancel via scheduler if available
	// For now, just send feedback — scheduler integration comes when scheduler is wired into consumer
	chatID := msg.Metadata["chat_id"]
	if chatID == "" {
		chatID = msg.SenderID
	}

	feedback := "Task stopped."
	if cmd == "stopall" {
		feedback = "All tasks stopped."
	}

	gw.sendToChannel(ctx, msg.AgentID, chatID, feedback, "", "")
	slog.Info("channel.stop", "command", cmd, "agent", msg.AgentID)
	return true
}

// handleSubagentAnnounce processes subagent completion messages.
// Routes results through the announce queue to the lead agent.
// Returns true if handled.
func (gw *Gateway) handleSubagentAnnounce(ctx context.Context, msg channels.InboundMessage) bool {
	if msg.Metadata["sender_type"] != "subagent" {
		return false
	}

	// Route through SoulDesk announce queue if available
	if gw.soulDesk != nil {
		primeID := msg.Metadata["parent_agent_id"]
		sessionID := msg.Metadata["session_id"]
		if primeID != "" && sessionID != "" {
			slog.Info("subagent.announce", "from", msg.SenderID, "prime", primeID)
			// The SoulDesk.deliverResult already handles announce queue routing
			return true
		}
	}

	return false
}

// sendToChannel sends a message to the first running channel for an agent.
func (gw *Gateway) sendToChannel(ctx context.Context, agentID, chatID, content, subject, replyTo string) {
	gw.sendToChannelByType(ctx, agentID, "", chatID, content, subject, replyTo, nil)
}

func (gw *Gateway) sendToChannelByType(ctx context.Context, agentID, channelType, chatID, content, subject, replyTo string, media []channels.MediaAttachment) {
	gw.sendToChannelByTypeWithMeta(ctx, agentID, channelType, chatID, content, subject, replyTo, media, nil)
}

func (gw *Gateway) sendToChannelByTypeWithMeta(ctx context.Context, agentID, channelType, chatID, content, subject, replyTo string, media []channels.MediaAttachment, metadata map[string]string) {
	if gw.chanMgr == nil {
		return
	}
	for _, ch := range gw.chanMgr.List() {
		if fmt.Sprintf("%v", ch["agent_id"]) != agentID || ch["running"] != true {
			continue
		}
		if channelType != "" && fmt.Sprintf("%v", ch["type"]) != channelType {
			continue
		}
		_ = gw.chanMgr.Send(ctx, ch["id"].(string), channels.OutboundMessage{
			RecipientID: chatID,
			Content:     content,
			Subject:     subject,
			ReplyTo:     replyTo,
			Media:       media,
			Metadata:    metadata,
		})
		return
	}
}

// buildTaskBoardSnapshot returns a formatted summary of task statuses for announce messages.
func buildTaskBoardSnapshot(ctx context.Context, taskStore interface{ ListAll(ctx context.Context, tenantID, status string, limit int) ([]interface{}, error) }, tenantID string) string {
	// Simplified version — full implementation needs team store pg
	return ""
}

// PublishOutbound sends a message through the bus to be delivered by the outbound consumer.
func (gw *Gateway) PublishOutbound(channel, chatID, content string) {
	if gw.msgBus != nil {
		gw.msgBus.PublishOutbound(bus.OutboundMessage{
			Channel: channel,
			ChatID:  chatID,
			Content: content,
		})
	}
}

// WireCommandHandlers adds /reset and /stop handling to the channel consumer.
// Called from the channel manager callback before normal message processing.
func (gw *Gateway) handleChannelCommand(ctx context.Context, msg channels.InboundMessage) bool {
	if gw.handleResetCommand(ctx, msg) {
		return true
	}
	if gw.handleStopCommand(ctx, msg) {
		return true
	}
	if gw.handleApprovalCommand(ctx, msg) {
		return true
	}
	if gw.handleSubagentAnnounce(ctx, msg) {
		return true
	}
	return false
}

// handleApprovalCommand processes /approve and /deny commands from Telegram/channels.
func (gw *Gateway) handleApprovalCommand(ctx context.Context, msg channels.InboundMessage) bool {
	if gw.db == nil {
		return false
	}
	content := strings.TrimSpace(msg.Content)
	if !strings.HasPrefix(content, "/approve") && !strings.HasPrefix(content, "/deny") && !strings.HasPrefix(content, "/pending") {
		return false
	}
	chatID := msg.SenderID
	if msg.Metadata != nil {
		if cid, ok := msg.Metadata["chat_id"]; ok { chatID = cid }
	}

	if strings.HasPrefix(content, "/pending") {
		pending, _ := tools.ListPending(ctx, gw.db.Pool)
		if len(pending) == 0 {
			gw.sendToChannel(ctx, msg.AgentID, chatID, "✅ No pending approvals", "", "")
			return true
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📋 %d pending approvals:\n\n", len(pending)))
		for _, a := range pending {
			sb.WriteString(fmt.Sprintf("• %s — %s\n  /approve %s | /deny %s\n\n", a.ActionType, a.ID[:8], a.ID[:8], a.ID[:8]))
		}
		gw.sendToChannel(ctx, msg.AgentID, chatID, sb.String(), "", "")
		return true
	}

	parts := strings.Fields(content)
	if len(parts) < 2 {
		gw.sendToChannel(ctx, msg.AgentID, chatID, "Usage: /approve <id> or /deny <id>", "", "")
		return true
	}
	shortID := parts[1]

	// Find full ID from short prefix
	pending, _ := tools.ListPending(ctx, gw.db.Pool)
	var fullID string
	for _, a := range pending {
		if strings.HasPrefix(a.ID, shortID) {
			fullID = a.ID
			break
		}
	}
	if fullID == "" {
		gw.sendToChannel(ctx, msg.AgentID, chatID, "❌ Approval not found: "+shortID, "", "")
		return true
	}

	if strings.HasPrefix(content, "/approve") {
		tools.ApproveAction(ctx, gw.db.Pool, fullID, msg.SenderName, "approved via "+msg.ChannelType)
		gw.sendToChannel(ctx, msg.AgentID, chatID, "✅ Approved: "+shortID, "", "")
	} else {
		tools.RejectAction(ctx, gw.db.Pool, fullID, msg.SenderName, "denied via "+msg.ChannelType)
		gw.sendToChannel(ctx, msg.AgentID, chatID, "❌ Denied: "+shortID, "", "")
	}
	return true
}

func mediaFromResult(result *agent.RunResult) []channels.MediaAttachment {
	if result == nil || len(result.Media) == 0 { return nil }
	out := make([]channels.MediaAttachment, len(result.Media))
	for i, m := range result.Media {
		out[i] = channels.MediaAttachment{URL: m.Path, ContentType: m.ContentType}
	}
	return out
}

// mailSaverAdapter wraps the mail store to implement emailch.MailSaver
type mailSaverAdapter struct {
	store *mail.Store
}

func (a *mailSaverAdapter) SaveInbound(ctx context.Context, tenantID, agentID, messageID, from, fromName, subject, body string, to []string) error {
	_, err := a.store.StoreInbound(ctx, tenantID, agentID, "", messageID, messageID, from, fromName, subject, body, "", to)
	return err
}

func (a *mailSaverAdapter) SaveOutbound(ctx context.Context, tenantID, agentID, messageID, subject, body string, to []string) error {
	_, err := a.store.StoreSend(ctx, tenantID, agentID, "", messageID, "", "", subject, body, "", "sent", to)
	return err
}
