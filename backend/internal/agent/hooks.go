// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/memory"
	"strings"
)

// Hook is a pre/post execution interceptor for agent runs.
type Hook interface {
	Name() string
	PreRun(ctx context.Context, req *RunRequest) error
	PostRun(ctx context.Context, req *RunRequest, result *RunResult, dur time.Duration) error
}

// HookChain runs hooks in order. PreRun runs forward, PostRun runs reverse.
type HookChain struct {
	hooks []Hook
}

func NewHookChain(hooks ...Hook) *HookChain { return &HookChain{hooks: hooks} }

func (c *HookChain) Add(h Hook) { c.hooks = append(c.hooks, h) }

func (c *HookChain) RunPre(ctx context.Context, req *RunRequest) error {
	for _, h := range c.hooks {
		if err := h.PreRun(ctx, req); err != nil {
			slog.Warn("hook.pre_run.error", "hook", h.Name(), "error", err)
			return err
		}
	}
	return nil
}

func (c *HookChain) RunPost(ctx context.Context, req *RunRequest, result *RunResult, dur time.Duration) {
	for i := len(c.hooks) - 1; i >= 0; i-- {
		if err := c.hooks[i].PostRun(ctx, req, result, dur); err != nil {
			slog.Warn("hook.post_run.error", "hook", c.hooks[i].Name(), "error", err)
		}
	}
}

// --- Built-in Hooks ---

// LoggingHook logs every agent run with timing and token usage.
type LoggingHook struct{}

func (h *LoggingHook) Name() string { return "logging" }
func (h *LoggingHook) PreRun(_ context.Context, req *RunRequest) error {
	slog.Info("agent.run.start", "agent", req.AgentID, "session", req.SessionID, "channel", req.Channel)
	return nil
}
func (h *LoggingHook) PostRun(_ context.Context, req *RunRequest, res *RunResult, dur time.Duration) error {
	slog.Info("agent.run.done", "agent", req.AgentID, "dur_ms", dur.Milliseconds(),
		"tokens_in", res.InputTokens, "tokens_out", res.OutputTokens,
		"tools", len(res.ToolsUsed), "iterations", res.Iterations)
	return nil
}

// MetricsHook tracks cumulative token usage per agent (in-memory).
type MetricsHook struct {
	Totals map[string]*AgentMetrics
}

type AgentMetrics struct {
	Runs         int64 `json:"runs"`
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	ToolCalls    int64 `json:"tool_calls"`
	TotalMs      int64 `json:"total_ms"`
}

func NewMetricsHook() *MetricsHook { return &MetricsHook{Totals: make(map[string]*AgentMetrics)} }
func (h *MetricsHook) Name() string { return "metrics" }
func (h *MetricsHook) PreRun(_ context.Context, _ *RunRequest) error { return nil }
func (h *MetricsHook) PostRun(_ context.Context, req *RunRequest, res *RunResult, dur time.Duration) error {
	m, ok := h.Totals[req.AgentID]
	if !ok {
		m = &AgentMetrics{}
		h.Totals[req.AgentID] = m
	}
	m.Runs++
	m.InputTokens += int64(res.InputTokens)
	m.OutputTokens += int64(res.OutputTokens)
	m.ToolCalls += int64(len(res.ToolsUsed))
	m.TotalMs += dur.Milliseconds()
	return nil
}

// BudgetHook checks token budget before agent runs.
// If the agent's credit budget is set and would be exceeded, it returns an error.
type BudgetHook struct {
	agentStore *Store
}

func NewBudgetHook(agentStore *Store) *BudgetHook { return &BudgetHook{agentStore: agentStore} }
func (h *BudgetHook) Name() string                { return "budget" }
func (h *BudgetHook) PreRun(ctx context.Context, req *RunRequest) error {
	if h.agentStore == nil {
		return nil
	}
	ag, err := h.agentStore.Get(ctx, req.AgentID)
	if err != nil {
		return nil
	}
	// Skip if no budget set
	if ag.CreditBudgetCents <= 0 {
		return nil
	}
	if ag.CreditUsedCents >= ag.CreditBudgetCents {
		return fmt.Errorf("agent %s has exceeded its credit budget (%d/%d cents)", ag.AgentKey, ag.CreditUsedCents, ag.CreditBudgetCents)
	}
	return nil
}
func (h *BudgetHook) PostRun(ctx context.Context, req *RunRequest, res *RunResult, dur time.Duration) error {
	if h.agentStore == nil {
		return nil
	}
	// Update credit usage based on actual tokens
	costCents := int64(float64(res.InputTokens+res.OutputTokens) / 1000.0 * 0.1)
	if costCents > 0 {
		h.agentStore.Pool().Exec(ctx,
			`UPDATE agents SET credit_used_cents = credit_used_cents + $1 WHERE id = $2`,
			costCents, req.AgentID)
	}
	return nil
}

// KnowledgeHook runs RAG retrieval before agent execution.
// Fires only when the agent has memory_enabled=true and there's a user message to search.
type KnowledgeHook struct {
	enricher interface {
		Enrich(ctx context.Context, agentID, query, systemPrompt string) string
	}
	agentStore   *Store
	enrichedPrompts map[string]string // agentID → enriched prompt (passed via context)
}

func NewKnowledgeHook(enricher interface {
	Enrich(ctx context.Context, agentID, query, systemPrompt string) string
}, agentStore *Store) *KnowledgeHook {
	return &KnowledgeHook{enricher: enricher, agentStore: agentStore, enrichedPrompts: make(map[string]string)}
}

func (h *KnowledgeHook) Name() string { return "knowledge" }
func (h *KnowledgeHook) PreRun(ctx context.Context, req *RunRequest) error {
	if h.enricher == nil || req.UserMessage == "" {
		return nil
	}
	ag, err := h.agentStore.Get(ctx, req.AgentID)
	if err != nil || !ag.MemoryEnabled {
		return nil
	}
	enriched := h.enricher.Enrich(ctx, req.AgentID, req.UserMessage, ag.SystemPrompt)
	if enriched != ag.SystemPrompt {
		h.enrichedPrompts[req.AgentID] = enriched
		slog.Info("knowledge.hook.enriched", "agent", ag.AgentKey, "chunks_injected", true)
	}
	return nil
}
func (h *KnowledgeHook) PostRun(_ context.Context, req *RunRequest, _ *RunResult, _ time.Duration) error {
	delete(h.enrichedPrompts, req.AgentID)
	return nil
}

// GetEnrichedPrompt returns the RAG-enriched prompt if available.
func (h *KnowledgeHook) GetEnrichedPrompt(agentID string) (string, bool) {
	p, ok := h.enrichedPrompts[agentID]
	return p, ok
}

// SkillRetrievalHook — skill retrieval is wired directly into the agent loop
// (see loop.go: "OpenSpace: inject crystallized skills") rather than as a hook,
// because it needs to modify the system prompt before message assembly.

// MemoryNudgeHook periodically checks if important context should be persisted.
// Fires every nudgeInterval messages. Uses cheap model to decide.
type MemoryNudgeHook struct {
	memStore      *memory.Store
	agentStore    *Store
	nudgeInterval int // check every N messages
	counter       map[string]int
}

func NewMemoryNudgeHook(memStore *memory.Store, agentStore *Store) *MemoryNudgeHook {
	return &MemoryNudgeHook{memStore: memStore, agentStore: agentStore, nudgeInterval: 5, counter: make(map[string]int)}
}

func (h *MemoryNudgeHook) Name() string { return "memory_nudge" }

func (h *MemoryNudgeHook) PreRun(ctx context.Context, req *RunRequest) error { return nil }

func (h *MemoryNudgeHook) PostRun(ctx context.Context, req *RunRequest, result *RunResult, dur time.Duration) error {
	if h.memStore == nil || req.UserMessage == "" { return nil }

	h.counter[req.AgentID]++
	if h.counter[req.AgentID] < h.nudgeInterval { return nil }
	h.counter[req.AgentID] = 0

	// Check if the conversation contains memorable information
	combined := req.UserMessage + "\n" + result.Content
	if len(combined) < 100 { return nil } // too short to be memorable

	// Extract key facts worth remembering (run async)
	go func() {
		ag, err := h.agentStore.Get(context.Background(), req.AgentID)
		if err != nil || !ag.MemoryEnabled { return }

		// Simple heuristic: if message contains names, dates, preferences, decisions → save
		keywords := []string{"prefer", "always", "never", "my name", "i work", "i live", "deadline", "important", "remember", "don't forget", "timezone", "project"}
		shouldSave := false
		lower := strings.ToLower(combined)
		for _, kw := range keywords {
			if strings.Contains(lower, kw) { shouldSave = true; break }
		}
		if !shouldSave { return }

		// Save to memory
		h.memStore.Save(context.Background(), ag.TenantID, memory.Memory{AgentID: req.AgentID, Type: "nudge", Content: combined[:min(len(combined), 500)]})
		slog.Info("memory.nudge.saved", "agent", ag.AgentKey, "len", len(combined))
	}()
	return nil
}
