// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
)

// LimitHistoryTurns keeps only the last N user turns (and their associated
// assistant/tool messages) from history. A "turn" = one user message plus
// all subsequent non-user messages until the next user message.
func LimitHistoryTurns(msgs []providers.Message, limit int) []providers.Message {
	if limit <= 0 || len(msgs) == 0 {
		return msgs
	}

	userCount := 0
	lastUserIndex := len(msgs)

	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			userCount++
			if userCount > limit {
				return msgs[lastUserIndex:]
			}
			lastUserIndex = i
		}
	}
	return msgs
}

// SanitizeHistory repairs tool_use/tool_result pairing in session history.
// Problems this fixes:
//   - Orphaned tool messages at start of history (after truncation)
//   - tool_result without matching tool_use in preceding assistant message
//   - assistant with tool_calls but missing tool_results
//   - Duplicate tool call IDs across turns
//
// Returns the cleaned messages and the number of messages that were dropped or synthesized.
func SanitizeHistory(msgs []providers.Message) ([]providers.Message, int) {
	if len(msgs) == 0 {
		return msgs, 0
	}

	dropped := 0

	// Skip leading orphaned tool messages
	start := 0
	for start < len(msgs) && msgs[start].Role == "tool" {
		slog.Debug("sanitizeHistory: dropping orphaned tool message at history start",
			"tool_call_id", msgs[start].ToolCallID)
		dropped++
		start++
	}

	if start >= len(msgs) {
		return nil, dropped
	}

	var result []providers.Message
	globalSeen := make(map[string]bool)

	for i := start; i < len(msgs); i++ {
		msg := msgs[i]

		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			// Deep-copy ToolCalls to avoid mutating original
			oldCalls := msg.ToolCalls
			msg.ToolCalls = make([]providers.ToolCall, len(oldCalls))
			copy(msg.ToolCalls, oldCalls)

			// Dedup IDs
			idQueue := make(map[string][]string, len(msg.ToolCalls))
			expectedIDs := make(map[string]bool, len(msg.ToolCalls))
			for j := range msg.ToolCalls {
				origID := msg.ToolCalls[j].ID
				newID := origID
				if globalSeen[origID] {
					newID = fmt.Sprintf("%s_dedup_%d", origID, j)
					slog.Debug("sanitizeHistory: dedup tool call ID", "orig", origID, "new", newID)
				}
				msg.ToolCalls[j].ID = newID
				globalSeen[newID] = true
				idQueue[origID] = append(idQueue[origID], newID)
				expectedIDs[newID] = true
			}

			result = append(result, msg)

			// Collect matching tool results
			for i+1 < len(msgs) && msgs[i+1].Role == "tool" {
				i++
				toolMsg := msgs[i]
				if queue, ok := idQueue[toolMsg.ToolCallID]; ok && len(queue) > 0 {
					newID := queue[0]
					idQueue[toolMsg.ToolCallID] = queue[1:]
					toolMsg.ToolCallID = newID
					result = append(result, toolMsg)
					delete(expectedIDs, newID)
				} else {
					slog.Debug("sanitizeHistory: dropping mismatched tool result",
						"tool_call_id", toolMsg.ToolCallID)
					dropped++
				}
			}

			// Synthesize missing tool results
			for _, tc := range msg.ToolCalls {
				if expectedIDs[tc.ID] {
					slog.Debug("sanitizeHistory: synthesizing missing tool result", "tool_call_id", tc.ID)
					result = append(result, providers.Message{
						Role:       "tool",
						Content:    "[Tool result missing — session was compacted]",
						ToolCallID: tc.ID,
					})
					dropped++
				}
			}
		} else if msg.Role == "tool" {
			// Orphaned tool message mid-history
			slog.Debug("sanitizeHistory: dropping orphaned tool message mid-history",
				"tool_call_id", msg.ToolCallID)
			dropped++
		} else {
			result = append(result, msg)
		}
	}

	return result, dropped
}

// SanitizeAssistantContent removes internal directives from assistant content.
func SanitizeAssistantContent(content string) string {
	// Remove [[...]] directives
	return StripMessageDirectives(content)
}

// BuildHistoryForSummary formats history messages for summarization.
func BuildHistoryForSummary(messages []providers.Message) string {
	var sb strings.Builder
	for _, m := range messages {
		switch m.Role {
		case "user":
			sb.WriteString(fmt.Sprintf("user: %s\n", m.Content))
		case "assistant":
			sb.WriteString(fmt.Sprintf("assistant: %s\n", SanitizeAssistantContent(m.Content)))
		case "tool":
			// Skip tool results in summary
		}
	}
	return sb.String()
}

// PrepareHistoryPipeline applies the full history processing pipeline:
// limitHistoryTurns → pruneContext → sanitizeHistory
func PrepareHistoryPipeline(history []providers.Message, historyLimit, contextWindow int, pruningCfg *PruningConfig) ([]providers.Message, int) {
	trimmed := LimitHistoryTurns(history, historyLimit)
	pruned := PruneContextMessages(trimmed, contextWindow, pruningCfg)
	return SanitizeHistory(pruned)
}

// ExtractMediaKinds extracts unique media kinds from messages.
// Note: Requires MediaRefs field on providers.Message (add if needed).
func ExtractMediaKinds(messages []providers.Message) []string {
	// Placeholder - implement when MediaRefs is added to providers.Message
	return nil
}

// FormatMediaNote formats a note about media files for summarization.
func FormatMediaNote(mediaKinds []string) string {
	if len(mediaKinds) == 0 {
		return ""
	}
	return fmt.Sprintf("Note: user shared media files (%s) which are no longer in context. Mention briefly if relevant.\n\n",
		strings.Join(mediaKinds, ", "))
}
