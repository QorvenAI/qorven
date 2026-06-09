// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import "context"

// Origin classifies what triggered an LLM call, for cost attribution and
// reporting. Agent-triggered calls charge the agent; system/maintenance calls
// charge the overhead bucket (hybrid attribution — see budget governance spec).
const (
	OriginAgent        = "agent"        // a user-facing agent's own turn
	OriginPassthrough  = "passthrough"  // agentless /v1/chat/completions
	OriginCouncil      = "council"      // council / debate
	OriginSubconscious = "subconscious"
	OriginWorkflow     = "workflow"
	OriginSubagent     = "subagent"
	OriginResearch     = "research"
	OriginMemory       = "memory"     // compaction/flush/cortex/etc.
	OriginBackground   = "background"  // background tasks, title-gen, classify
	OriginSouldesk     = "souldesk"
	OriginSkills       = "skills"
	OriginSystem       = "system" // catch-all global maintenance
)

// MeterScope is the cost-attribution context carried on every LLM call.
// A blank AgentID means the call is overhead (system/maintenance) charged to
// the tenant's overall budget rather than to a specific agent.
type MeterScope struct {
	TenantID     string
	AgentID      string
	SessionID    string
	Origin       string
	DepartmentID string
	ProjectID    string
	TaskID       string
}

// IsOverhead reports whether this call should be charged to the overall
// overhead bucket rather than to a specific agent.
func (s MeterScope) IsOverhead() bool { return s.AgentID == "" }

type meterScopeKey struct{}

// WithMeterScope returns a context carrying the given scope.
func WithMeterScope(ctx context.Context, s MeterScope) context.Context {
	return context.WithValue(ctx, meterScopeKey{}, s)
}

// MeterScopeFromCtx returns the scope on the context, or the zero value
// (overhead, blank tenant) when none is set.
func MeterScopeFromCtx(ctx context.Context) MeterScope {
	if v, ok := ctx.Value(meterScopeKey{}).(MeterScope); ok {
		return v
	}
	return MeterScope{}
}

type meterBypassKey struct{}

// WithMeterBypass marks a context so the MeteredProvider does NOT enforce or
// record. The gateway pipeline sets this before dispatching, because the
// pipeline already enforces and records with richer data (resolved model,
// provider key id, cache status). Without this flag the pipeline path would
// double-count, since the pipeline dispatches through the metered registry.
func WithMeterBypass(ctx context.Context) context.Context {
	return context.WithValue(ctx, meterBypassKey{}, true)
}

// meterBypassFromCtx reports whether metering should be skipped for this call.
func meterBypassFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(meterBypassKey{}).(bool)
	return v
}

// Enforcer gates an LLM call against budget caps before it is dispatched.
// Implemented by the gateway/llm package; injected into the registry so the
// providers package never imports llm (no import cycle).
type Enforcer interface {
	// Check returns a non-nil error when the scope or any parent scope is over
	// its cap. A nil error means proceed.
	Check(ctx context.Context, scope MeterScope) error
}

// Recorder records the cost of a completed LLM call to the spend ledger.
// Implemented by the gateway/llm CostLedger adapter; injected into the registry.
type Recorder interface {
	RecordScoped(ctx context.Context, scope MeterScope, model, providerID, keyID string, usage Usage)
}
