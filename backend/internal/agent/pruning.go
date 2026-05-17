// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"fmt"
	"unicode/utf8"

	"github.com/qorvenai/qorven/internal/providers"
)

// Context pruning defaults.
const (
	defaultKeepLastAssistants   = 3
	defaultSoftTrimRatio        = 0.25
	defaultHardClearRatio       = 0.5
	defaultMinPrunableToolChars = 50000
	defaultSoftTrimMaxChars     = 3000
	defaultSoftTrimHeadChars    = 1500
	defaultSoftTrimTailChars    = 1500
	defaultHardClearPlaceholder = "[Old tool result content cleared]"
	charsPerTokenEstimate       = 4
)

// PruningConfig configures context pruning behavior.
type PruningConfig struct {
	Mode               string // "off" to disable
	KeepLastAssistants int
	SoftTrimRatio      float64
	HardClearRatio     float64
	MinPrunableChars   int
	SoftTrimMaxChars   int
	SoftTrimHeadChars  int
	SoftTrimTailChars  int
	HardClearEnabled   bool
	HardClearPlaceholder string
}

// DefaultPruningConfig returns sensible defaults.
func DefaultPruningConfig() *PruningConfig {
	return &PruningConfig{
		KeepLastAssistants:   defaultKeepLastAssistants,
		SoftTrimRatio:        defaultSoftTrimRatio,
		HardClearRatio:       defaultHardClearRatio,
		MinPrunableChars:     defaultMinPrunableToolChars,
		SoftTrimMaxChars:     defaultSoftTrimMaxChars,
		SoftTrimHeadChars:    defaultSoftTrimHeadChars,
		SoftTrimTailChars:    defaultSoftTrimTailChars,
		HardClearEnabled:     true,
		HardClearPlaceholder: defaultHardClearPlaceholder,
	}
}

// PruneContextMessages trims old tool results to reduce context window usage.
// Two-pass approach:
//  1. Soft trim: keep head + tail of long tool results, drop middle.
//  2. Hard clear: replace entire tool result with placeholder.
//
// Only tool results older than keepLastAssistants are eligible.
func PruneContextMessages(msgs []providers.Message, contextWindowTokens int, cfg *PruningConfig) []providers.Message {
	if cfg != nil && cfg.Mode == "off" {
		return msgs
	}
	if contextWindowTokens <= 0 || len(msgs) == 0 {
		return msgs
	}

	if cfg == nil {
		cfg = DefaultPruningConfig()
	}

	charWindow := contextWindowTokens * charsPerTokenEstimate

	// Find cutoff: protect last N assistant messages.
	cutoffIndex := findAssistantCutoff(msgs, cfg.KeepLastAssistants)
	if cutoffIndex < 0 {
		return msgs
	}

	// Find first user message — never prune before it.
	pruneStart := len(msgs)
	for i, m := range msgs {
		if m.Role == "user" {
			pruneStart = i
			break
		}
	}

	// Estimate total chars.
	totalChars := 0
	for _, m := range msgs {
		totalChars += estimateMessageChars(m)
	}

	ratio := float64(totalChars) / float64(charWindow)
	if ratio < cfg.SoftTrimRatio {
		return msgs // context is small enough
	}

	// Collect prunable tool result indexes.
	var prunableIndexes []int
	for i := pruneStart; i < cutoffIndex; i++ {
		if msgs[i].Role == "tool" && msgs[i].Content != "" {
			prunableIndexes = append(prunableIndexes, i)
		}
	}

	if len(prunableIndexes) == 0 {
		return msgs
	}

	// Pass 0: Per-result context guard — force-trim any single tool result
	// exceeding 30% of the context window.
	maxSingleResultChars := charWindow * 3 / 10
	var result []providers.Message
	for _, idx := range prunableIndexes {
		msgChars := estimateMessageChars(msgs[idx])
		if msgChars > maxSingleResultChars {
			if result == nil {
				result = make([]providers.Message, len(msgs))
				copy(result, msgs)
			}
			msg := msgs[idx]
			head := takeHead(msg.Content, maxSingleResultChars*7/10)
			tail := takeTail(msg.Content, maxSingleResultChars*3/10)
			trimmed := fmt.Sprintf("%s\n\n⚠️ [... middle content omitted ...]\n\n%s\n\n[Single tool result trimmed: %d chars exceeded limit of %d chars.]",
				head, tail, msgChars, maxSingleResultChars)
			result[idx] = providers.Message{
				Role:       msg.Role,
				Content:    trimmed,
				ToolCallID: msg.ToolCallID,
			}
			totalChars += len(trimmed) - msgChars
		}
	}
	if result != nil {
		msgs = result
		result = nil
		ratio = float64(totalChars) / float64(charWindow)
		if ratio < cfg.SoftTrimRatio {
			return msgs
		}
	}

	// Pass 1: Soft trim long tool results.
	for _, idx := range prunableIndexes {
		msg := msgs[idx]
		msgChars := estimateMessageChars(msg)

		if msgChars <= cfg.SoftTrimMaxChars {
			continue
		}

		if result == nil {
			result = make([]providers.Message, len(msgs))
			copy(result, msgs)
		}

		// Tail-aware split: if tail has important content, use 70/30 split.
		headChars := cfg.SoftTrimHeadChars
		tailChars := cfg.SoftTrimTailChars
		if hasImportantTail(msg.Content) {
			totalBudget := headChars + tailChars
			headChars = totalBudget * 7 / 10
			tailChars = totalBudget - headChars
		}
		head := takeHead(msg.Content, headChars)
		tail := takeTail(msg.Content, tailChars)
		trimmed := fmt.Sprintf("%s\n...\n%s\n\n[Tool result trimmed: kept first %d chars and last %d chars of %d chars.]",
			head, tail, headChars, tailChars, msgChars)

		result[idx] = providers.Message{
			Role:       msg.Role,
			Content:    trimmed,
			ToolCallID: msg.ToolCallID,
		}
		totalChars += len(trimmed) - msgChars
	}

	output := msgs
	if result != nil {
		output = result
	}

	// Re-check ratio after soft trim.
	ratio = float64(totalChars) / float64(charWindow)
	if ratio < cfg.HardClearRatio || !cfg.HardClearEnabled {
		return output
	}

	// Check min prunable chars threshold.
	prunableChars := 0
	for _, idx := range prunableIndexes {
		prunableChars += estimateMessageChars(output[idx])
	}
	if prunableChars < cfg.MinPrunableChars {
		return output
	}

	// Pass 2: Hard clear — replace entire tool results with placeholder.
	if result == nil {
		result = make([]providers.Message, len(msgs))
		copy(result, msgs)
		output = result
	}

	for _, idx := range prunableIndexes {
		if ratio < cfg.HardClearRatio {
			break
		}
		msg := output[idx]
		beforeChars := estimateMessageChars(msg)

		output[idx] = providers.Message{
			Role:       msg.Role,
			Content:    cfg.HardClearPlaceholder,
			ToolCallID: msg.ToolCallID,
		}
		afterChars := len(cfg.HardClearPlaceholder)
		totalChars += afterChars - beforeChars
		ratio = float64(totalChars) / float64(charWindow)
	}

	return output
}

// findAssistantCutoff returns the index of the Nth-from-last assistant message.
func findAssistantCutoff(msgs []providers.Message, keepLast int) int {
	if keepLast <= 0 {
		return len(msgs)
	}

	remaining := keepLast
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			remaining--
			if remaining == 0 {
				return i
			}
		}
	}
	return -1
}

func estimateMessageChars(m providers.Message) int {
	return utf8.RuneCountInString(m.Content)
}

func takeHead(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

func takeTail(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[len(runes)-n:])
}

// hasImportantTail checks if the tail of content has important info.
func hasImportantTail(content string) bool {
	if len(content) < 500 {
		return false
	}
	tail := content[len(content)-500:]
	// Check for error indicators, summaries, conclusions
	indicators := []string{
		"error", "Error", "ERROR",
		"failed", "Failed", "FAILED",
		"summary", "Summary", "SUMMARY",
		"conclusion", "Conclusion",
		"result", "Result",
		"total", "Total",
	}
	for _, ind := range indicators {
		if contains(tail, ind) {
			return true
		}
	}
	return false
}
