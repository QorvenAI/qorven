// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/qorvenai/qorven/internal/llm"
)

// Compressor implements 5-phase context compression.
// Inspired by the best compression algorithm in open-source agent frameworks.
type Compressor struct {
	provider       llm.Provider
	summaryModel   string
	thresholdRatio float64 // compress at this % of context window (default 0.50)
	protectFirstN  int     // protect first N messages (default 3)
	tailBudgetRatio float64 // % of context for tail protection (default 0.20)
}

func NewCompressor(provider llm.Provider, summaryModel string) *Compressor {
	// Leave summaryModel as "" — let the provider use its own default.
	return &Compressor{
		provider: provider, summaryModel: summaryModel,
		thresholdRatio: 0.50, protectFirstN: 3, tailBudgetRatio: 0.20,
	}
}

// Compress runs the 5-phase algorithm on a message list.
func (c *Compressor) Compress(ctx context.Context, messages []llm.Message, contextWindow int, prevSummary string) ([]llm.Message, string, error) {
	totalTokens := estimateTokens(messages)
	threshold := int(float64(contextWindow) * c.thresholdRatio)

	if totalTokens < threshold {
		return messages, prevSummary, nil // no compression needed
	}

	slog.Info("compressor.start", "tokens", totalTokens, "threshold", threshold, "messages", len(messages))

	// Prune old tool results (free — no LLM call)
	messages = c.pruneToolResults(messages)

	// Check if Phase 1 was enough
	if estimateTokens(messages) < threshold {
		slog.Info("compressor.phase1_sufficient")
		return messages, prevSummary, nil
	}

	// Determine boundaries
	head := messages[:min(c.protectFirstN, len(messages))]
	tailStart := c.findTailBoundary(messages, contextWindow)
	tail := messages[tailStart:]
	middle := messages[c.protectFirstN:tailStart]

	if len(middle) == 0 {
		return messages, prevSummary, nil // nothing to compress
	}

	// Generate structured summary
	summary, err := c.generateSummary(ctx, middle, prevSummary)
	if err != nil {
		slog.Warn("compressor.summary_failed", "error", err)
		return messages, prevSummary, err
	}

	// Assemble compressed messages
	summaryRole := "assistant"
	if len(head) > 0 && head[len(head)-1].Role == "assistant" {
		summaryRole = "user"
	}

	result := make([]llm.Message, 0, len(head)+1+len(tail))
	result = append(result, head...)
	result = append(result, llm.Message{Role: summaryRole, Content: "[Previous conversation summary]\n" + summary})
	result = append(result, tail...)

	// Sanitize orphaned tool pairs
	result = c.sanitizeToolPairs(result)

	slog.Info("compressor.done", "before", len(messages), "after", len(result), "saved_tokens", totalTokens-estimateTokens(result))
	return result, summary, nil
}

// Replace old tool results (>200 chars) with placeholder.
// Frees 30-50% of context for free (no LLM call).
func (c *Compressor) pruneToolResults(msgs []llm.Message) []llm.Message {
	// Protect last 10 messages
	protectFrom := max(0, len(msgs)-10)
	result := make([]llm.Message, len(msgs))
	copy(result, msgs)

	for i := 0; i < protectFrom; i++ {
		if result[i].Role == "tool" && len(result[i].Content) > 200 {
			result[i].Content = "[Old tool output cleared — " + fmt.Sprintf("%d", len(result[i].Content)) + " chars]"
		}
	}
	return result
}

// Find tail boundary using token budget (not fixed count).
func (c *Compressor) findTailBoundary(msgs []llm.Message, contextWindow int) int {
	budget := int(float64(contextWindow) * c.tailBudgetRatio)
	tokens := 0
	for i := len(msgs) - 1; i >= c.protectFirstN; i-- {
		tokens += estimateMessageTokens(msgs[i])
		if tokens > budget {
			// Align forward to avoid splitting tool call/result groups
			return c.alignBoundary(msgs, i+1)
		}
	}
	return c.protectFirstN
}

// Align boundary to avoid splitting tool call/result groups.
func (c *Compressor) alignBoundary(msgs []llm.Message, idx int) int {
	for idx < len(msgs) && msgs[idx].Role == "tool" {
		idx++ // push past orphaned tool results
	}
	return min(idx, len(msgs))
}

// Generate structured summary.
const summaryTemplate = `Summarize this conversation into a structured format:

## Goal
What the user is trying to accomplish.

## Progress
### Done
What has been completed.
### In Progress
What is currently being worked on.
### Blocked
What is stuck and why.

## Key Decisions
Important choices made during the conversation.

## Relevant Files
Files that were read, written, or discussed.

## Next Steps
What should happen next.

Keep it concise. Preserve all actionable information.`

func (c *Compressor) generateSummary(ctx context.Context, middle []llm.Message, prevSummary string) (string, error) {
	// Serialize middle messages for the summarizer
	var content strings.Builder
	for _, m := range middle {
		role := strings.ToUpper(m.Role)
		text := m.Content
		if len(text) > 500 { text = text[:500] + "..." }
		content.WriteString(fmt.Sprintf("[%s]: %s\n", role, text))
	}

	prompt := summaryTemplate + "\n\nConversation to summarize:\n" + content.String()
	if prevSummary != "" {
		prompt += "\n\nPrevious summary (PRESERVE relevant info, ADD new progress):\n" + prevSummary
	}

	resp, err := c.provider.Chat(ctx, llm.ChatRequest{
		Model:     c.summaryModel,
		Messages:  []llm.Message{{Role: "user", Content: prompt}},
		MaxTokens: 800,
	})
	if err != nil { return "", err }
	return resp.Content, nil
}

// Fix orphaned tool call/result pairs.
func (c *Compressor) sanitizeToolPairs(msgs []llm.Message) []llm.Message {
	// Simple: just remove tool messages that have no preceding assistant message
	result := make([]llm.Message, 0, len(msgs))
	for i, m := range msgs {
		if m.Role == "tool" && (i == 0 || msgs[i-1].Role != "assistant") {
			continue // orphaned tool result — skip
		}
		result = append(result, m)
	}
	return result
}

// --- Helpers ---

func estimateTokens(msgs []llm.Message) int {
	total := 0
	for _, m := range msgs { total += estimateMessageTokens(m) }
	return total
}

func estimateMessageTokens(m llm.Message) int {
	return len(m.Content) / 4 // rough estimate: 4 chars per token
}
