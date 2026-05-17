package inbound

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/channels"
)

// buildHistorySummary fetches prior messages with this sender across sessions
// for this agent and returns a structured text block to prepend to agent context.
func (p *Processor) buildHistorySummary(ctx context.Context, msg channels.InboundMessage) string {
	if p.sessions == nil || p.pool == nil {
		return ""
	}
	sessions, err := p.sessions.ListForAgent(ctx, msg.AgentID, 50)
	if err != nil || len(sessions) == 0 {
		return ""
	}

	type histMsg struct {
		channel string
		role    string
		content string
	}
	var allMsgs []histMsg

	for _, sess := range sessions {
		msgs, err := p.sessions.GetHistory(ctx, sess.ID)
		if err != nil {
			continue
		}
		for _, m := range msgs {
			if strings.EqualFold(m.SenderName, msg.SenderName) ||
				strings.EqualFold(m.SenderName, msg.SenderID) ||
				sess.Channel == msg.ChannelType {
				allMsgs = append(allMsgs, histMsg{
					channel: sess.Channel,
					role:    m.Role,
					content: m.Content,
				})
			}
		}
	}

	if len(allMsgs) == 0 {
		return ""
	}

	name := msg.SenderName
	if name == "" {
		name = msg.SenderID
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "[CONVERSATION HISTORY with %s — %d messages]\n", name, len(allMsgs))
	start := 0
	if len(allMsgs) > 10 {
		start = len(allMsgs) - 10
	}
	for _, m := range allMsgs[start:] {
		role := "You"
		if m.role == "user" {
			role = name
		}
		truncated := m.content
		if len(truncated) > 200 {
			truncated = truncated[:200] + "..."
		}
		fmt.Fprintf(&sb, "  [%s] %s: %s\n", m.channel, role, truncated)
	}
	return sb.String()
}

// dispatch acts on the resolved mode.
func (p *Processor) dispatch(ctx context.Context, msg channels.InboundMessage, mode ActionMode, cfg AgentConfig, historySummary string) {
	if p.agentLoop == nil {
		slog.Warn("inbound.dispatch.no_agent_loop", "agent", msg.AgentID)
		return
	}
	extraSystem := ""
	if historySummary != "" {
		extraSystem = "Prior conversation history with this sender:\n\n" + historySummary +
			"\n\nUse this history to verify any claims the sender makes about prior agreements."
	}

	req := agent.RunRequest{
		AgentID:           msg.AgentID,
		UserMessage:       msg.Content,
		Channel:           msg.ChannelType,
		SourceChannel:     msg.ChannelType,
		UserID:            msg.SenderID,
		ExtraSystemPrompt: extraSystem,
	}

	chatID := msg.Metadata["chat_id"]
	if chatID == "" {
		chatID = msg.SenderID
	}

	switch mode {
	case ModeFullyAutonomous:
		result, err := p.agentLoop.Run(ctx, req, func(agent.StreamEvent) {})
		if err != nil || result == nil {
			return
		}
		// Deliver the reply back to the originating channel.
		if p.reply != nil && result.Content != "" {
			p.reply(ctx, msg.AgentID, msg.ChannelType, chatID, result.Content, msg.Metadata)
		}
		p.draftQueue.Save(ctx, DraftReply{
			TenantID:        cfg.TenantID,
			AgentID:         msg.AgentID,
			SenderID:        msg.SenderID,
			SenderName:      msg.SenderName,
			Channel:         msg.ChannelType,
			OriginalMessage: msg.Content,
			HistorySummary:  historySummary,
			DraftContent:    result.Content,
			Status:          "sent",
		})

	case ModeDraftAndApprove, ModeDraftOnly:
		result, err := p.agentLoop.Run(ctx, req, func(agent.StreamEvent) {})
		if err != nil || result == nil {
			return
		}
		draft := DraftReply{
			TenantID:        cfg.TenantID,
			AgentID:         msg.AgentID,
			SenderID:        msg.SenderID,
			SenderName:      msg.SenderName,
			Channel:         msg.ChannelType,
			OriginalMessage: msg.Content,
			HistorySummary:  historySummary,
			DraftContent:    result.Content,
			Status:          "pending",
		}
		draftID := p.draftQueue.Save(ctx, draft)
		if mode == ModeDraftAndApprove && cfg.NotificationChannel != "" {
			p.notifyDraft(ctx, cfg, msg, result.Content, draftID, historySummary)
		}

	case ModeContextOnly:
		slog.Info("inbound.context_only",
			"agent", msg.AgentID,
			"sender", msg.SenderID,
		)
	}
}

// notifyDraft sends approval notification to the configured channel (stub — full wiring is a follow-up).
func (p *Processor) notifyDraft(ctx context.Context, cfg AgentConfig, msg channels.InboundMessage, draftContent, draftID, historySummary string) {
	slog.Info("inbound.draft.notify",
		"channel", cfg.NotificationChannel,
		"agent", msg.AgentID,
		"draft_id", draftID,
	)
}
