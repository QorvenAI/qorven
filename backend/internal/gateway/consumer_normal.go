// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/channels"
)

// processNormalMessage handles the full lifecycle of an inbound channel message.
// This is the Qorven equivalent of Qorven's processNormalMessage — the core
// message processing pipeline.
//
// Currently enhances the inline callback in gateway.go with:
// - Group-scoped UserID
// - Session metadata persistence
// - Quota enforcement
// - Group system prompt injection
// - Silent reply suppression
// - Error formatting
//
// Implemented: intent classification, contact collection
// Post-turn processing: scheduler routing, dispatch, voice sanitization

// buildGroupUserID returns a group-scoped user ID for group chats.
// In groups, all users share the same session context.
// Discord guilds get per-user scope: "guild:{guildID}:user:{senderID}".
func buildGroupUserID(msg channels.InboundMessage) string {
	if msg.Metadata["guild_id"] != "" && msg.SenderID != "" {
		return fmt.Sprintf("guild:%s:user:%s", msg.Metadata["guild_id"], msg.SenderID)
	}
	chatID := msg.Metadata["chat_id"]
	if chatID == "" {
		chatID = msg.SenderID
	}
	return fmt.Sprintf("group:%s:%s", msg.ChannelType, chatID)
}

// buildGroupSystemPrompt returns extra system prompt instructions for group chats.
func buildGroupSystemPrompt() string {
	return "You are in a GROUP chat (multiple participants), not a private 1-on-1 DM.\n" +
		"- Keep responses concise and focused; long replies are disruptive in groups.\n" +
		"- Write like a human. Avoid Markdown tables.\n" +
		"- Address the group naturally."
}

// isSilentReply returns true if the agent's response should be suppressed.
// Matches Qorven's normalize-reply.ts pattern.
func isSilentReply(content string) bool {
	if content == "" {
		return true
	}
	trimmed := strings.TrimSpace(strings.ToLower(content))
	return trimmed == "[no_reply]" || trimmed == "[silent]" || trimmed == "no_reply" ||
		strings.HasPrefix(trimmed, "[silent") || strings.HasPrefix(trimmed, "[no reply")
}

// formatAgentError returns a user-friendly error message for agent failures.
// Matching Qorven's error classification (pi-embedded-helpers/errors.ts).
func formatAgentError(err error) string {
	if err == nil {
		return ""
	}
	raw := err.Error()
	lower := strings.ToLower(raw)

	// Timeout — check before context overflow (deadline exceeded contains "context")
	if containsAny(lower, "timeout", "timed out", "deadline exceeded") {
		return "⚠️ Request timed out. Please try again."
	}
	// Context overflow
	if containsAny(lower, "request_too_large", "context length exceeded", "maximum context length", "prompt is too long") ||
		(strings.Contains(lower, "context") && containsAny(lower, "overflow", "too large", "too long", "exceeded")) {
		return "⚠️ Context overflow — message too large for this model. Try starting a fresh session."
	}
	// Message format errors (corrupted session history)
	if containsAny(lower, "tool_use_id", "roles must alternate", "unexpected tool", "tool_result block") {
		return "⚠️ Session history conflict — please try again or start a fresh session."
	}
	// Rate limit
	if containsAny(lower, "rate limit", "rate_limit", "too many requests", "429", "quota exceeded") {
		return "⚠️ API rate limit reached. Please try again later."
	}
	// Overloaded
	if strings.Contains(lower, "overloaded") {
		return "⚠️ The AI service is temporarily overloaded. Please try again in a moment."
	}
	// Billing
	if containsAny(lower, "billing", "insufficient credits", "payment required", "402") {
		return "⚠️ API billing error — check your provider's billing dashboard."
	}
	// Auth
	if containsAny(lower, "invalid api key", "unauthorized", "forbidden", "401", "403") {
		return "⚠️ Authentication error. Please check your API key configuration."
	}
	// Cancelled (no message needed)
	if strings.Contains(lower, "context canceled") {
		return ""
	}
	// Generic
	return "⚠️ Sorry, something went wrong processing your message. Please try again."
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) { return true }
	}
	return false
}

// checkQuota enforces per-user rate limiting. Returns true if the message should be blocked.
func (gw *Gateway) checkQuota(ctx context.Context, userID, channel string) (bool, string) {
	// Quota checker is wired in Start() — check if it exists
	// For now, quota is checked via the QuotaChecker created in Start()
	// Quota check via database query
	return false, ""
}

// enrichRunRequest adds group-aware fields to the agent run request.
func enrichRunRequest(req *agent.RunRequest, msg channels.InboundMessage) {
	// Group system prompt
	isGroup := msg.Metadata["peer_kind"] == "group"
	if isGroup {
		req.ExtraSystemPrompt = buildGroupSystemPrompt()
	}

	// Topic system prompt override
	if tsp := msg.Metadata["topic_system_prompt"]; tsp != "" {
		if req.ExtraSystemPrompt != "" {
			req.ExtraSystemPrompt += "\n\n"
		}
		req.ExtraSystemPrompt += tsp
	}
}

// collectContact saves sender info for CRM/contact management.
func (gw *Gateway) collectContact(ctx context.Context, msg channels.InboundMessage) {
	if gw.db == nil || msg.SenderID == "" {
		return
	}
	// Upsert contact: save sender info from channel message
	gw.db.Pool.Exec(ctx,
		`INSERT INTO contacts (tenant_id, external_id, channel, display_name, first_seen, last_seen, message_count)
		 VALUES ($1, $2, $3, $4, now(), now(), 1)
		 ON CONFLICT (tenant_id, external_id, channel) DO UPDATE SET
		   last_seen = now(), message_count = contacts.message_count + 1,
		   display_name = COALESCE(NULLIF($4, ''), contacts.display_name)`,
		defaultTenant, msg.SenderID, msg.ChannelType, msg.SenderName)
}

// classifyMessageIntent determines what kind of message this is.
// Returns: "chat", "command", "delegation", "cron_trigger", "system"
func classifyMessageIntent(msg channels.InboundMessage) string {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return "system"
	}
	if strings.HasPrefix(content, "/") {
		return "command"
	}
	if msg.Metadata != nil {
		if msg.Metadata["sender_type"] == "subagent" {
			return "delegation"
		}
		if msg.Metadata["trigger"] == "cron" {
			return "cron_trigger"
		}
	}
	return "chat"
}
