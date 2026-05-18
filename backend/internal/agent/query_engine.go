// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// QueryEngine runs a budget-aware LLM conversation loop with tool execution.
// Stops when: no tool calls (final answer), max turns, or token budget exceeded.
type QueryEngine struct {
	provider     providers.Provider
	model        string
	maxTurns     int
	budgetTokens int
	compactAfter int
}

type QueryConfig struct {
	MaxTurns     int `json:"max_turns" toml:"max_turns"`
	BudgetTokens int `json:"budget_tokens" toml:"budget_tokens"`
	CompactAfter int `json:"compact_after" toml:"compact_after"`
}

func DefaultQueryConfig() QueryConfig {
	return QueryConfig{MaxTurns: 8, BudgetTokens: 50000, CompactAfter: 12}
}

func NewQueryEngine(provider providers.Provider, model string, cfg QueryConfig) *QueryEngine {
	return &QueryEngine{
		provider: provider, model: model,
		maxTurns: cfg.MaxTurns, budgetTokens: cfg.BudgetTokens, compactAfter: cfg.CompactAfter,
	}
}

type TurnResult struct {
	Content    string
	ToolCalls  []providers.ToolCall
	Tokens     int
	Elapsed    time.Duration
	TurnNumber int
}

// Run executes the loop: LLM call → tool execution → repeat until done.
func (qe *QueryEngine) Run(ctx context.Context, system, message string, tools []providers.ToolDefinition, execTool func(string, map[string]any) string) ([]TurnResult, error) {
	msgs := []providers.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: message},
	}

	var results []TurnResult
	spent := 0

	for turn := 0; turn < qe.maxTurns; turn++ {
		if spent > qe.budgetTokens {
			slog.Info("query_engine.budget", "spent", spent, "budget", qe.budgetTokens)
			break
		}

		if turn > 0 && turn%qe.compactAfter == 0 && len(msgs) > 10 {
			msgs = append(msgs[:2], msgs[len(msgs)-4:]...)
		}

		start := time.Now()
		resp, err := qe.provider.Chat(ctx, providers.ChatRequest{
			Messages: msgs, Tools: tools, Model: qe.model,
		})
		if err != nil {
			return results, fmt.Errorf("turn %d: %w", turn, err)
		}

		tokens := 0
		if resp.Usage != nil {
			tokens = resp.Usage.TotalTokens
		}
		spent += tokens

		results = append(results, TurnResult{
			Content: resp.Content, ToolCalls: resp.ToolCalls,
			Tokens: tokens, Elapsed: time.Since(start), TurnNumber: turn,
		})

		if len(resp.ToolCalls) == 0 {
			break
		}

		msgs = append(msgs, providers.Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls})
		for _, tc := range resp.ToolCalls {
			result := execTool(tc.Name, tc.Arguments)
			msgs = append(msgs, providers.Message{Role: "tool", Content: result, ToolCallID: tc.ID})
		}
	}

	slog.Info("query_engine.done", "turns", len(results), "tokens", spent)
	return results, nil
}
