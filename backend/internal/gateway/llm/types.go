// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Package llm is the AI Gateway — a unified middleware pipeline that every
// agent call routes through. It handles budget enforcement, priority queuing,
// semantic caching, model alias resolution, provider selection with circuit
// breaker, cost tracking, and OAuth provider support.
package llm

import (
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// Priority determines which concurrency tier a request occupies.
// Lower value = higher priority.
type Priority int

const (
	// PriorityInteractive is for user-facing requests that need minimum latency.
	PriorityInteractive Priority = 0
	// PriorityBackground is for autonomous agent tasks running in the background.
	PriorityBackground Priority = 1
	// PriorityBatch is for bulk or scheduled workloads that should yield to interactive/background.
	PriorityBatch Priority = 2
)

// GatewayRequest is the single internal request format used by all callers.
// It wraps providers.ChatRequest and adds gateway-level identity and controls.
type GatewayRequest struct {
	// Identity — populated by the agent loop before dispatching.
	AgentID   string
	TeamID    string
	TenantID  string
	SessionID string
	Priority  Priority

	// Model selection — exact ID or alias ("fast", "smart", "cheap", "vision", "code", "reason").
	Model    string
	Provider string // optional: pin to a specific provider

	// Content — passed through to the provider adapter unchanged.
	Messages   []providers.Message
	Tools      []providers.ToolDefinition
	ToolChoice string
	Options    map[string]any // temperature, max_tokens, reasoning_effort, etc.

	// Gateway controls
	SkipCache  bool    // bypass cache lookup (writes still happen)
	MaxCostUSD float64 // per-request hard cap; 0 = no cap
}

// GatewayResponse is providers.ChatResponse enriched with gateway metadata.
type GatewayResponse struct {
	*providers.ChatResponse

	// Gateway metadata — useful for dashboards and cost attribution.
	ProviderID    string
	KeyID         string
	ModelResolved string // actual model ID used after alias resolution
	CostUSD       float64
	CacheHit      bool
	Latency       time.Duration
}

// StreamChunk extends providers.StreamChunk with gateway metadata.
type StreamChunk struct {
	providers.StreamChunk
	ProviderID string
}

// ErrBudgetExceeded is returned by the budget engine when an agent has
// exceeded its monthly or daily spend cap.
var ErrBudgetExceeded = &gatewayError{"budget exceeded"}

// ErrCircuitOpen is returned when all available keys for a provider
// have open circuit breakers and no healthy key can be selected.
var ErrCircuitOpen = &gatewayError{"all provider keys are temporarily unavailable (circuit open)"}

type gatewayError struct{ msg string }

func (e *gatewayError) Error() string { return e.msg }
