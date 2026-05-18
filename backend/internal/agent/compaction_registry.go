// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
)

// CompactionStrategy defines how context window overflow is handled.
type CompactionStrategy interface {
	Name() string
	Compact(ctx context.Context, messages []providers.Message, targetTokens int) []providers.Message
}

// CompactionRegistry holds available compaction strategies.
type CompactionRegistry struct {
	strategies map[string]CompactionStrategy
	defaultKey string
}

func NewCompactionRegistry() *CompactionRegistry {
	r := &CompactionRegistry{
		strategies: make(map[string]CompactionStrategy),
		defaultKey: "hybrid",
	}
	r.Register(&TruncateStrategy{})
	r.Register(&SummarizeStrategy{})
	r.Register(&HybridStrategy{})
	return r
}

func (r *CompactionRegistry) Register(s CompactionStrategy) { r.strategies[s.Name()] = s }
func (r *CompactionRegistry) Get(name string) CompactionStrategy {
	if s, ok := r.strategies[name]; ok { return s }
	return r.strategies[r.defaultKey]
}
func (r *CompactionRegistry) SetDefault(name string) { r.defaultKey = name }

// ── Strategy 1: Truncate ──
// Simply drops oldest messages, keeping system + recent.
type TruncateStrategy struct{}

func (s *TruncateStrategy) Name() string { return "truncate" }
func (s *TruncateStrategy) Compact(_ context.Context, messages []providers.Message, targetTokens int) []providers.Message {
	if len(messages) <= 4 { return messages }

	// Keep system prompt + last N messages
	var system []providers.Message
	var rest []providers.Message
	for _, m := range messages {
		if m.Role == "system" { system = append(system, m) } else { rest = append(rest, m) }
	}

	// Estimate tokens and drop from front
	kept := rest
	for estimateTokens(append(system, kept...)) > targetTokens && len(kept) > 2 {
		kept = kept[1:]
	}
	return append(system, kept...)
}

// ── Strategy 2: Summarize ──
// Summarizes older messages into a single context message.
type SummarizeStrategy struct {
	Provider providers.Provider
	Model    string
}

func (s *SummarizeStrategy) Name() string { return "summarize" }
func (s *SummarizeStrategy) Compact(ctx context.Context, messages []providers.Message, targetTokens int) []providers.Message {
	if len(messages) <= 6 { return messages }

	// Split: system + old + recent
	var system []providers.Message
	var rest []providers.Message
	for _, m := range messages {
		if m.Role == "system" { system = append(system, m) } else { rest = append(rest, m) }
	}

	keepRecent := 6
	if len(rest) <= keepRecent { return messages }

	older := rest[:len(rest)-keepRecent]
	recent := rest[len(rest)-keepRecent:]

	// Build summary of older messages
	var summary strings.Builder
	summary.WriteString("[Earlier conversation summary]\n")
	for _, m := range older {
		preview := m.Content
		if len(preview) > 120 { preview = preview[:120] + "..." }
		summary.WriteString("• " + m.Role + ": " + preview + "\n")
	}

	// If we have a provider, use LLM to summarize
	if s.Provider != nil {
		llmSummary := CompactWithLLM(ctx, s.Provider, s.Model, older, keepRecent)
		if len(llmSummary) > 0 {
			return append(system, append(llmSummary, recent...)...)
		}
	}

	contextMsg := providers.Message{Role: "system", Content: summary.String()}
	return append(system, append([]providers.Message{contextMsg}, recent...)...)
}

// ── Strategy 3: Hybrid ──
// Truncates tool results first, then summarizes if still over limit.
type HybridStrategy struct {
	Provider providers.Provider
	Model    string
}

func (s *HybridStrategy) Name() string { return "hybrid" }
func (s *HybridStrategy) Compact(ctx context.Context, messages []providers.Message, targetTokens int) []providers.Message {
	c := NewCompactor(targetTokens)

	// Prune tool results
	pruned, _ := c.PruneTools(messages)
	if estimateTokens(pruned) <= targetTokens { return pruned }

	// Summarize
	summarizer := &SummarizeStrategy{Provider: s.Provider, Model: s.Model}
	return summarizer.Compact(ctx, pruned, targetTokens)
}
