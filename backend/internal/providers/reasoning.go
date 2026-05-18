// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"encoding/json"
	"strings"
)

// reasoning.go — Reasoning effort resolution for models that support thinking/reasoning.
// Handles capability detection, effort level negotiation, and fallback policies.

const (
	ReasoningFallbackDowngrade       = "downgrade"
	ReasoningFallbackDisable         = "off"
	ReasoningFallbackProviderDefault = "provider_default"
	ReasoningMetadataKey             = "reasoning"
)

// ReasoningCapability describes supported reasoning levels for a model.
type ReasoningCapability struct {
	Levels        []string `json:"levels,omitempty"`
	DefaultEffort string   `json:"default_effort,omitempty"`
}

func (c *ReasoningCapability) Supports(level string) bool {
	if c == nil || level == "" { return false }
	for _, s := range c.Levels { if s == level { return true } }
	return false
}

// ReasoningDecision records how reasoning effort was resolved for a request.
type ReasoningDecision struct {
	Source              string   `json:"source,omitempty"`
	RequestedEffort     string   `json:"requested_effort,omitempty"`
	EffectiveEffort     string   `json:"effective_effort,omitempty"`
	Fallback            string   `json:"fallback,omitempty"`
	Reason              string   `json:"reason,omitempty"`
	KnownModel          bool     `json:"known_model,omitempty"`
	SupportedLevels     []string `json:"supported_levels,omitempty"`
	UsedProviderDefault bool     `json:"used_provider_default,omitempty"`
}

func (d ReasoningDecision) HasObservation() bool { return d.Source != "" && d.Source != "unset" }


// ResolveReasoningDecision determines the effective reasoning effort for a request.
func ResolveReasoningDecision(provider Provider, model, requestedEffort, fallback, source string) ReasoningDecision {
	d := ReasoningDecision{
		Source:          normalizeReasoningSource(source),
		RequestedEffort: NormalizeReasoningEffort(requestedEffort),
		Fallback:        NormalizeReasoningFallback(fallback),
	}
	if d.RequestedEffort == "" { d.RequestedEffort = "off" }
	if d.RequestedEffort == "off" { d.EffectiveEffort = "off"; return d }

	tc, ok := provider.(ThinkingCapable)
	if !ok || !tc.SupportsThinking() {
		d.EffectiveEffort = "off"
		d.Reason = "provider does not support reasoning controls"
		return d
	}

	cap := LookupReasoningCapability(model)
	if cap == nil {
		if d.RequestedEffort == "auto" {
			d.UsedProviderDefault = true
			d.Reason = "unknown model; leaving provider default reasoning"
			return d
		}
		d.EffectiveEffort = d.RequestedEffort
		d.Reason = "unknown model; passing requested effort through"
		return d
	}

	d.KnownModel = true
	d.SupportedLevels = append([]string(nil), cap.Levels...)

	if d.RequestedEffort == "auto" {
		d.EffectiveEffort = cap.DefaultEffort
		d.UsedProviderDefault = true
		d.Reason = "auto uses model default"
		return d
	}
	if cap.Supports(d.RequestedEffort) {
		d.EffectiveEffort = d.RequestedEffort
		return d
	}

	switch d.Fallback {
	case ReasoningFallbackDisable:
		d.EffectiveEffort = "off"
		d.Reason = "unsupported effort; disabled by fallback policy"
	case ReasoningFallbackProviderDefault:
		d.EffectiveEffort = cap.DefaultEffort
		d.UsedProviderDefault = true
		d.Reason = "unsupported effort; using model default"
	default:
		d.EffectiveEffort = downgradeReasoningLevel(d.RequestedEffort, cap.Levels)
		if d.EffectiveEffort == "" {
			d.EffectiveEffort = "off"
			d.Reason = "unsupported effort; no lower level available"
		} else {
			d.Reason = "downgraded from " + d.RequestedEffort + " to " + d.EffectiveEffort
		}
	}
	return d
}

// LookupReasoningCapability returns the reasoning capability for a known model.
func LookupReasoningCapability(model string) *ReasoningCapability {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if idx := strings.LastIndex(normalized, "/"); idx >= 0 { normalized = normalized[idx+1:] }
	for _, e := range reasoningModels {
		if normalized == e.id { cap := e.cap; cap.Levels = append([]string(nil), cap.Levels...); return &cap }
	}
	return nil
}

type reasoningEntry struct {
	id  string
	cap ReasoningCapability
}

var reasoningModels = []reasoningEntry{
	// ── OpenAI ──────────────────────────────────────────────────────────────
	{"gpt-5.4-mini", ReasoningCapability{[]string{"none", "low", "medium", "high", "xhigh"}, "none"}},
	{"gpt-5-mini", ReasoningCapability{[]string{"none", "low", "medium", "high", "xhigh"}, "none"}},
	{"gpt-5.4", ReasoningCapability{[]string{"none", "low", "medium", "high", "xhigh"}, "none"}},
	{"gpt-5.3-codex-spark", ReasoningCapability{[]string{"low", "medium", "high", "xhigh"}, "medium"}},
	{"gpt-5.3-codex", ReasoningCapability{[]string{"low", "medium", "high", "xhigh"}, "medium"}},
	{"gpt-5.2-codex", ReasoningCapability{[]string{"low", "medium", "high", "xhigh"}, "medium"}},
	{"gpt-5.2", ReasoningCapability{[]string{"none", "low", "medium", "high", "xhigh"}, "none"}},
	{"gpt-5.1-codex-max", ReasoningCapability{[]string{"none", "medium", "high", "xhigh"}, "none"}},
	{"gpt-5.1-codex-mini", ReasoningCapability{[]string{"low", "medium", "high"}, "medium"}},
	{"gpt-5.1-codex", ReasoningCapability{[]string{"low", "medium", "high"}, "medium"}},
	{"gpt-5.1", ReasoningCapability{[]string{"none", "low", "medium", "high"}, "none"}},
	{"gpt-5-codex-mini", ReasoningCapability{[]string{"low", "medium", "high"}, "medium"}},
	{"gpt-5-codex", ReasoningCapability{[]string{"low", "medium", "high"}, "medium"}},
	{"gpt-5", ReasoningCapability{[]string{"minimal", "low", "medium", "high"}, "medium"}},
	// ── Anthropic Claude (extended thinking via budget_tokens) ────────────
	{"claude-opus-4-7", ReasoningCapability{[]string{"none", "low", "medium", "high", "xhigh"}, "none"}},
	{"claude-opus-4-5", ReasoningCapability{[]string{"none", "low", "medium", "high", "xhigh"}, "none"}},
	{"claude-opus-4-1", ReasoningCapability{[]string{"none", "low", "medium", "high", "xhigh"}, "none"}},
	{"claude-sonnet-4-6", ReasoningCapability{[]string{"none", "low", "medium", "high"}, "none"}},
	{"claude-sonnet-4-5", ReasoningCapability{[]string{"none", "low", "medium", "high"}, "none"}},
	{"claude-haiku-4-5", ReasoningCapability{[]string{"none", "low", "medium"}, "none"}},
	{"claude-3-7-sonnet-20250219", ReasoningCapability{[]string{"none", "low", "medium", "high"}, "none"}},
	{"claude-3-5-sonnet-20241022", ReasoningCapability{[]string{"none", "low", "medium", "high"}, "none"}},
	// ── Anthropic via Bedrock (cross-region inference profiles) ──────────
	{"anthropic.claude-opus-4-7-20251101-v1:0", ReasoningCapability{[]string{"none", "low", "medium", "high", "xhigh"}, "none"}},
	{"anthropic.claude-opus-4-5-20251001-v1:0", ReasoningCapability{[]string{"none", "low", "medium", "high", "xhigh"}, "none"}},
	{"anthropic.claude-3-7-sonnet-20250219-v1:0", ReasoningCapability{[]string{"none", "low", "medium", "high"}, "none"}},
	{"anthropic.claude-3-5-sonnet-20241022-v2:0", ReasoningCapability{[]string{"none", "low", "medium", "high"}, "none"}},
	// ── Google Gemini (thinking_config budget) ────────────────────────────
	{"gemini-2.5-pro-preview-05-06", ReasoningCapability{[]string{"none", "low", "medium", "high", "xhigh"}, "none"}},
	{"gemini-2.5-flash-preview-04-17", ReasoningCapability{[]string{"none", "low", "medium", "high"}, "none"}},
	{"gemini-2.0-flash", ReasoningCapability{[]string{"none", "low", "medium"}, "none"}},
}

// Context helpers for passing reasoning decisions through the request pipeline.
type reasoningDecisionKey struct{}

func WithReasoningDecision(ctx context.Context, d ReasoningDecision) context.Context {
	return context.WithValue(ctx, reasoningDecisionKey{}, &d)
}

func ReasoningDecisionFromContext(ctx context.Context) *ReasoningDecision {
	d, _ := ctx.Value(reasoningDecisionKey{}).(*ReasoningDecision)
	return d
}

func MergeReasoningMetadata(existing json.RawMessage, d ReasoningDecision) json.RawMessage {
	if !d.HasObservation() { return existing }
	payload := map[string]any{}
	if len(existing) > 0 { json.Unmarshal(existing, &payload) }
	payload[ReasoningMetadataKey] = d
	data, _ := json.Marshal(payload)
	return json.RawMessage(data)
}

func NormalizeReasoningEffort(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "auto", "none", "minimal", "low", "medium", "high", "xhigh":
		return strings.ToLower(strings.TrimSpace(v))
	default: return ""
	}
}

func NormalizeReasoningFallback(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case ReasoningFallbackDisable, ReasoningFallbackProviderDefault:
		return strings.ToLower(strings.TrimSpace(v))
	default: return ReasoningFallbackDowngrade
	}
}

func normalizeReasoningSource(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "reasoning", "thinking_level", "provider_default":
		return strings.ToLower(strings.TrimSpace(v))
	default: return "unset"
	}
}

func downgradeReasoningLevel(requested string, supported []string) string {
	order := map[string]int{"none": 0, "minimal": 1, "low": 2, "medium": 3, "high": 4, "xhigh": 5}
	reqRank, ok := order[requested]
	if !ok || len(supported) == 0 { return "" }
	bestLevel, bestRank := "", -1
	for _, level := range supported {
		rank, ok := order[level]
		if ok && rank <= reqRank && rank > bestRank { bestLevel = level; bestRank = rank }
	}
	return bestLevel
}
