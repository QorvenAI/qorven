// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// compactionSummaryPrompt is the structured summarization instruction.
// Matching Qorven TS compaction.ts MERGE_SUMMARIES_INSTRUCTIONS.
const compactionSummaryPrompt = `Summarize this conversation concisely for the AI agent to resume work.

MUST PRESERVE:
- Active tasks and their current status (in-progress, blocked, pending)
- Pending subagent tasks (IDs, labels, statuses) — agent needs to know what is still running
- Pending team task results awaiting delivery (task IDs, assignees, statuses)
- Any "waiting for..." state — do NOT drop expectations of future results
- Batch operation progress (e.g., "5/17 items completed")
- The last thing the user requested and what was being done about it
- Decisions made and their rationale
- TODOs, open questions, and constraints
- Any commitments or follow-ups promised

IDENTIFIER PRESERVATION:
Preserve all opaque identifiers exactly as written (no shortening or reconstruction),
including UUIDs, hashes, IDs, tokens, API keys, hostnames, IPs, ports, URLs, and file names.

PRIORITIZE recent context over older history. The agent needs to know
what it was doing, not just what was discussed.

Conversation to summarize:

`

// CompactionAction describes what the compactor should do.
type CompactionAction int

const (
	NoCompaction         CompactionAction = iota
	PruneToolOutputs                             // Tier 1: free, no LLM — prune old tool outputs
	BackgroundCompaction                         // Tier 2: >70% — LLM summarize oldest 30%
	AggressiveCompaction                         // Tier 2: >85% — LLM summarize oldest 50%
	EmergencyTruncation                          // Tier 3: >95% — hard drop, no LLM
)

// Compactor monitors context size and triggers 3-tier compaction.
//
// Tier 1: Tool Output Pruning (free, no LLM)
//   - Prune old tool outputs beyond PRUNE_PROTECT threshold
//   - Protected tools (skill, memory) never pruned
//   - Dedup stale tool results
//
// Tier 2: LLM Compaction (background, non-blocking)
//   - Background (70%) → compact 30% oldest
//   - Aggressive (85%) → compact 50% oldest
//
// Tier 3: Emergency Truncation (synchronous, no LLM)
//   - Emergency (95%) → drop oldest 50%
//
type Compactor struct {
	contextWindow int // max tokens for this agent

	// Configurable thresholds (as fraction of context window)
	PruneThreshold      float64 // Tier 1 trigger (default 0.60)
	BackgroundThreshold float64 // Tier 2 background trigger (default 0.70)
	AggressiveThreshold float64 // Tier 2 aggressive trigger (default 0.85)
	EmergencyThreshold  float64 // Tier 3 trigger (default 0.95)

	// Pruning config
	PruneProtectTokens int // protect this many tokens of recent tool outputs (default 40000)
	PruneMinSavings    int // only prune if savings exceed this (default 20000)

	// Iterative summary — update previous summary instead of regenerating
	// from scratch on each compaction pass. Keeps compaction O(turn-delta)
	// instead of O(full-history).
	PreviousSummary string
	CompactionCount int
}

// Protected tools whose outputs are never pruned.
var pruneProtectedTools = map[string]bool{
	"skill":       true,
	"use_skill":   true,
	"memory":      true,
	"read_skill":  true,
}

func NewCompactor(contextWindow int) *Compactor {
	if contextWindow <= 0 {
		contextWindow = 128000
	}
	return &Compactor{
		contextWindow:       contextWindow,
		PruneThreshold:      0.75, // Tier 1: prune old tool outputs at 75%
		BackgroundThreshold: 0.85, // Tier 2: LLM summarize at 85%
		AggressiveThreshold: 0.92, // Tier 2: aggressive summarize at 92%
		EmergencyThreshold:  0.97, // Tier 3: hard drop at 97%
		PruneProtectTokens:  40000,
		PruneMinSavings:     20000,
	}
}

// Check evaluates whether compaction is needed based on estimated token count.
func (c *Compactor) Check(messages []providers.Message) CompactionAction {
	tokens := estimateTokens(messages)
	ratio := float64(tokens) / float64(c.contextWindow)

	switch {
	case ratio >= c.EmergencyThreshold:
		slog.Warn("compactor: emergency truncation", "tokens", tokens, "window", c.contextWindow, "ratio", fmt.Sprintf("%.1f%%", ratio*100))
		return EmergencyTruncation
	case ratio >= c.AggressiveThreshold:
		slog.Info("compactor: aggressive compaction needed", "tokens", tokens, "ratio", fmt.Sprintf("%.1f%%", ratio*100))
		return AggressiveCompaction
	case ratio >= c.BackgroundThreshold:
		slog.Info("compactor: background compaction needed", "tokens", tokens, "ratio", fmt.Sprintf("%.1f%%", ratio*100))
		return BackgroundCompaction
	case ratio >= c.PruneThreshold:
		slog.Info("compactor: tool output pruning", "tokens", tokens, "ratio", fmt.Sprintf("%.1f%%", ratio*100))
		return PruneToolOutputs
	default:
		return NoCompaction
	}
}

// PruneTools implements Tier 1: prune old tool outputs to free context space.
// Walks backwards, protects recent outputs (PruneProtectTokens), truncates older ones.
// Returns the pruned messages and number of tokens freed.
func (c *Compactor) PruneTools(messages []providers.Message) ([]providers.Message, int) {
	// Walk backwards through tool results, accumulate token count
	var totalToolTokens int
	var pruneTargets []int // indices of messages to prune

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "tool" {
			continue
		}

		// Skip protected tools
		toolName := extractToolNameFromResult(msg)
		if pruneProtectedTools[toolName] {
			continue
		}

		tokens := len(msg.Content) / 4
		totalToolTokens += tokens

		// Beyond protection threshold → mark for pruning
		if totalToolTokens > c.PruneProtectTokens {
			pruneTargets = append(pruneTargets, i)
		}
	}

	// Check if pruning would save enough
	var savings int
	for _, idx := range pruneTargets {
		savings += len(messages[idx].Content) / 4
	}

	if savings < c.PruneMinSavings {
		return messages, 0
	}

	// Apply pruning — Pass 1: soft trim (head+tail), Pass 2: hard clear for very old
	result := make([]providers.Message, len(messages))
	copy(result, messages)
	protectLast := 3 // protect last 3 tool results from hard clear
	for i, idx := range pruneTargets {
		original := result[idx]
		var content string
		if i < len(pruneTargets)-protectLast {
			// Pass 2: hard clear for oldest results
			content = hardClearToolOutput(original.Content)
		} else {
			// Pass 1: soft trim for recent results
			content = truncateToolOutput(original.Content)
		}
		result[idx] = providers.Message{
			Role:       original.Role,
			Content:    content,
			ToolCallID: original.ToolCallID,
		}
	}

	slog.Info("compactor.prune", "pruned", len(pruneTargets), "savings_tokens", savings)
	return result, savings
}

// Compact applies the compaction action to messages.
// Uses AnythingLLM's "cannonball" middle-out truncation pattern.
// Mentor token budget: system 15% / memory 5% / RAG 25% / history 30% / prompt+tools 25%
func (c *Compactor) Compact(messages []providers.Message, action CompactionAction) []providers.Message {
	if action == NoCompaction {
		return messages
	}
	if len(messages) < 4 {
		// Too few messages for middle-out — truncate individual large messages
		return c.truncateIndividualMessages(messages)
	}

	var cutRatio float64
	switch action {
	case BackgroundCompaction:
		cutRatio = 0.30
	case AggressiveCompaction:
		cutRatio = 0.50
	case EmergencyTruncation:
		cutRatio = 0.60
	}

	// Cannonball: middle-out truncation of history messages
	// Keep first 2 messages (system context) and last 3 pairs (most recent conversation)
	keepHead := 2
	keepTail := 6 // 3 user+assistant pairs
	if keepHead+keepTail >= len(messages) {
		// Not enough messages for middle-out — truncate individual large messages
		return c.truncateIndividualMessages(messages)
	}

	middle := messages[keepHead : len(messages)-keepTail]
	cutCount := int(float64(len(middle)) * cutRatio)
	if cutCount < 1 {
		return messages
	}

	// Middle-out: remove from center of the middle section
	midIdx := len(middle) / 2
	halfCut := cutCount / 2
	start := midIdx - halfCut
	end := midIdx + halfCut + (cutCount % 2)
	if start < 0 { start = 0 }
	if end > len(middle) { end = len(middle) }

	// Build compacted middle
	var compactedMiddle []providers.Message
	compactedMiddle = append(compactedMiddle, middle[:start]...)
	compactedMiddle = append(compactedMiddle, providers.Message{
		Role:    "system",
		Content: fmt.Sprintf("[%d messages truncated for context window]", end-start),
	})
	compactedMiddle = append(compactedMiddle, middle[end:]...)

	// Reassemble: head + compacted middle + tail
	var compacted []providers.Message
	compacted = append(compacted, messages[:keepHead]...)
	compacted = append(compacted, compactedMiddle...)
	compacted = append(compacted, messages[len(messages)-keepTail:]...)

	slog.Info("compactor.cannonball", "before", len(messages), "after", len(compacted),
		"cut", end-start, "action", action)
	return compacted
}

// estimateTokens gives a rough token count (1 token ≈ 4 chars for English).
func estimateTokens(messages []providers.Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)/4 + 4 // +4 for role/formatting overhead
		for _, tc := range m.ToolCalls {
			total += 20 // tool call overhead (name + id)
			for _, v := range tc.Arguments {
				if s, ok := v.(string); ok {
					total += len(s) / 4
				} else {
					total += 10 // non-string arg estimate
				}
			}
		}
		// Image/media content estimates
		if strings.Contains(m.Content, "data:image/") {
			total += 500 // base64 image token estimate
		}
	}
	return total
}

// truncateToolOutput replaces a tool output with a short summary marker.
func truncateToolOutput(content string) string {
	// Pass 1: Soft trim — keep head (70%) + tail (30%), drop middle
	if len(content) > 2000 {
		headLen := len(content) * 70 / 100
		tailLen := len(content) * 30 / 100
		if headLen > 1400 { headLen = 1400 }
		if tailLen > 600 { tailLen = 600 }
		return content[:headLen] + "\n\n[...middle trimmed...]\n\n" + content[len(content)-tailLen:]
	}
	lines := strings.SplitN(content, "\n", 3)
	preview := lines[0]
	if len(preview) > 100 {
		preview = preview[:100] + "..."
	}
	return fmt.Sprintf("[tool output pruned — %d chars, preview: %s]", len(content), preview)
}

// hardClearToolOutput replaces content entirely (Pass 2).
func hardClearToolOutput(content string) string {
	return "[Old tool result cleared]"
}

// extractToolNameFromResult tries to extract the tool name from a tool result message.
func extractToolNameFromResult(msg providers.Message) string {
	// Tool results often have the tool name in the ToolCallID or content prefix
	if msg.ToolCallID != "" {
		// Convention: tool call IDs often contain the tool name
		parts := strings.SplitN(msg.ToolCallID, "_", 2)
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return ""
}

// DedupToolResults removes consecutive identical tool results (same tool, same output).
func DedupToolResults(messages []providers.Message) []providers.Message {
	if len(messages) < 2 {
		return messages
	}

	result := make([]providers.Message, 0, len(messages))
	var lastToolContent string
	var lastToolCallID string
	dedupCount := 0

	for _, msg := range messages {
		if msg.Role == "tool" && msg.Content == lastToolContent && msg.ToolCallID == lastToolCallID {
			dedupCount++
			continue
		}
		if msg.Role == "tool" {
			lastToolContent = msg.Content
			lastToolCallID = msg.ToolCallID
		} else {
			lastToolContent = ""
			lastToolCallID = ""
		}
		result = append(result, msg)
	}

	if dedupCount > 0 {
		slog.Info("compactor.dedup", "removed", dedupCount)
	}
	return result
}

// compressSystemPrompt reduces system prompt size when context is tight.
// Removes verbose sections while keeping essential identity + tool names.
func compressSystemPrompt(prompt string, maxChars int) string {
	if len(prompt) <= maxChars {
		return prompt
	}

	// Remove the Web Tool Routing Guide (verbose, ~500 chars)
	if idx := strings.Index(prompt, "## Web Tool Routing Guide"); idx > 0 {
		end := strings.Index(prompt[idx:], "\n## ")
		if end > 0 {
			prompt = prompt[:idx] + prompt[idx+end:]
		} else {
			prompt = prompt[:idx]
		}
	}

	if len(prompt) <= maxChars {
		return prompt
	}

	// Truncate tool parameter descriptions (keep name + one-line desc)
	lines := strings.Split(prompt, "\n")
	var compressed []string
	skipParams := false
	for _, line := range lines {
		if strings.Contains(line, "Parameters:") || strings.Contains(line, "\"properties\"") {
			skipParams = true
			continue
		}
		if skipParams && (strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "  ")) {
			continue
		}
		skipParams = false
		compressed = append(compressed, line)
	}
	prompt = strings.Join(compressed, "\n")

	if len(prompt) <= maxChars {
		return prompt
	}

	// Last resort: hard truncate
	slog.Warn("compactor.system_prompt.hard_truncate", "from", len(prompt), "to", maxChars)
	return prompt[:maxChars] + "\n\n[System prompt truncated to fit context window]"
}

// compressionFailCount tracks consecutive compression failures to prevent death spirals.
var compressionFailCount int

// CheckDeathSpiral returns true if compression has failed too many times in a row.
// Qorven-inspired: if compression fails 2x consecutively, halt instead of looping.
func CheckDeathSpiral() bool {
	return compressionFailCount >= 2
}

// RecordCompressionResult tracks success/failure for death spiral detection.
func RecordCompressionResult(success bool) {
	if success {
		compressionFailCount = 0
	} else {
		compressionFailCount++
		if compressionFailCount >= 2 {
			slog.Warn("compactor.death_spiral_detected", "consecutive_failures", compressionFailCount)
		}
	}
}

// CompactWithLLM summarizes the first ~70% of messages into a condensed summary,
// keeping the last ~30% intact. Uses the structured compactionSummaryPrompt.
// Returns nil on failure (caller keeps original messages).
func CompactWithLLM(ctx context.Context, provider providers.Provider, model string, messages []providers.Message, keepCount int) []providers.Message {
	if len(messages) < 6 {
		return nil
	}

	// Ensure we keep at least 30% of messages.
	if minKeep := len(messages) * 3 / 10; minKeep > keepCount {
		keepCount = minKeep
	}

	splitIdx := len(messages) - keepCount

	// Walk backward from splitIdx to find a clean boundary —
	// avoid splitting tool_use → tool_result pairs.
	for splitIdx > 0 {
		m := messages[splitIdx]
		if m.Role == "tool" || (m.Role == "assistant" && len(m.ToolCalls) > 0) {
			splitIdx--
			continue
		}
		break
	}
	if splitIdx <= 1 {
		return nil
	}

	// Build summary input
	toSummarize := messages[:splitIdx]
	var sb strings.Builder
	for _, m := range toSummarize {
		switch m.Role {
		case "user":
			fmt.Fprintf(&sb, "user: %s\n", m.Content)
		case "assistant":
			fmt.Fprintf(&sb, "assistant: %s\n", SanitizeAssistantContent(m.Content))
		}
	}

	sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := provider.Chat(sctx, providers.ChatRequest{
		Messages: []providers.Message{{
			Role:    "user",
			Content: compactionSummaryPrompt + sb.String(),
		}},
		Model:   model,
		Options: map[string]any{"max_tokens": 1024, "temperature": 0.3},
	})
	if err != nil {
		slog.Warn("compactor.llm_compact_failed", "error", err)
		return nil
	}

	summary := providers.Message{
		Role:    "user",
		Content: "[Summary of earlier conversation]\n" + SanitizeAssistantContent(resp.Content),
	}
	result := make([]providers.Message, 0, 1+keepCount)
	result = append(result, summary)
	result = append(result, messages[splitIdx:]...)

	slog.Info("compactor.llm_compacted",
		"original_msgs", len(messages),
		"summarized", splitIdx,
		"kept", len(result))

	return result
}

// truncateIndividualMessages handles the case where there are too few messages
// for middle-out compaction — truncates individual large messages instead.
func (c *Compactor) truncateIndividualMessages(messages []providers.Message) []providers.Message {
	maxCharsPerMsg := c.contextWindow * 2 // ~2 chars per token, leave room
	if maxCharsPerMsg < 1000 { maxCharsPerMsg = 1000 }

	out := make([]providers.Message, len(messages))
	for i, m := range messages {
		out[i] = m
		if m.Role == "system" { continue } // never truncate system
		if len(m.Content) > maxCharsPerMsg {
			out[i].Content = m.Content[:maxCharsPerMsg] + "\n\n[message truncated — " + fmt.Sprintf("%d", len(m.Content)-maxCharsPerMsg) + " chars removed]"
		}
	}
	return out
}

const iterativeSummaryPrompt = `You are updating a context compaction summary.
A previous compaction produced the summary below. New conversation turns have occurred since then.

PREVIOUS SUMMARY:
%s

NEW TURNS TO INCORPORATE:
%s

Update the summary. PRESERVE all existing information that is still relevant.
ADD new progress. Move completed items from "In Progress" to "Done".
Remove information only if clearly obsolete.

MUST PRESERVE: active tasks, pending results, decisions, file paths, identifiers, commitments.
PRIORITIZE recent context over older history.`
