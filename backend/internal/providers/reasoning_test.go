// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"encoding/json"
	"testing"
)

func TestReasoningCapability_Supports(t *testing.T) {
	cap := &ReasoningCapability{Levels: []string{"low", "medium", "high"}}
	if !cap.Supports("medium") { t.Error("should support medium") }
	if cap.Supports("xhigh") { t.Error("should not support xhigh") }
	if cap.Supports("") { t.Error("should not support empty") }
}

func TestReasoningCapability_Supports_Nil(t *testing.T) {
	var cap *ReasoningCapability
	if cap.Supports("low") { t.Error("nil should not support anything") }
}

func TestLookupReasoningCapability_Known(t *testing.T) {
	cap := LookupReasoningCapability("gpt-5")
	if cap == nil { t.Fatal("gpt-5 should be known") }
	if !cap.Supports("medium") { t.Error("gpt-5 should support medium") }
	if cap.DefaultEffort != "medium" { t.Errorf("default=%q", cap.DefaultEffort) }
}

func TestLookupReasoningCapability_Unknown(t *testing.T) {
	if LookupReasoningCapability("unknown-model") != nil { t.Error("unknown should return nil") }
}

func TestLookupReasoningCapability_CaseInsensitive(t *testing.T) {
	if LookupReasoningCapability("GPT-5") == nil { t.Error("should be case insensitive") }
}

func TestLookupReasoningCapability_WithPrefix(t *testing.T) {
	if LookupReasoningCapability("openai/gpt-5") == nil { t.Error("should strip provider prefix") }
}

func TestNormalizeReasoningEffort(t *testing.T) {
	tests := []struct{ in, want string }{
		{"off", "off"}, {"auto", "auto"}, {"low", "low"}, {"medium", "medium"},
		{"high", "high"}, {"xhigh", "xhigh"}, {"none", "none"}, {"minimal", "minimal"},
		{"invalid", ""}, {"", ""}, {"  HIGH  ", "high"},
	}
	for _, tt := range tests {
		got := NormalizeReasoningEffort(tt.in)
		if got != tt.want { t.Errorf("NormalizeReasoningEffort(%q) = %q, want %q", tt.in, got, tt.want) }
	}
}

func TestNormalizeReasoningFallback(t *testing.T) {
	tests := []struct{ in, want string }{
		{"off", "off"}, {"provider_default", "provider_default"},
		{"downgrade", "downgrade"}, {"invalid", "downgrade"}, {"", "downgrade"},
	}
	for _, tt := range tests {
		got := NormalizeReasoningFallback(tt.in)
		if got != tt.want { t.Errorf("NormalizeReasoningFallback(%q) = %q, want %q", tt.in, got, tt.want) }
	}
}

func TestResolveReasoningDecision_Off(t *testing.T) {
	d := ResolveReasoningDecision(nil, "gpt-5", "off", "", "")
	if d.EffectiveEffort != "off" { t.Errorf("effective=%q", d.EffectiveEffort) }
}

func TestResolveReasoningDecision_Empty(t *testing.T) {
	d := ResolveReasoningDecision(nil, "gpt-5", "", "", "")
	if d.EffectiveEffort != "off" { t.Errorf("empty should default to off: %q", d.EffectiveEffort) }
}

func TestResolveReasoningDecision_KnownModel_Supported(t *testing.T) {
	mock := &mockThinkingProvider{}
	d := ResolveReasoningDecision(mock, "gpt-5", "medium", "downgrade", "reasoning")
	if d.EffectiveEffort != "medium" { t.Errorf("effective=%q, want medium", d.EffectiveEffort) }
	if !d.KnownModel { t.Error("should be known model") }
}

func TestResolveReasoningDecision_KnownModel_Unsupported_Downgrade(t *testing.T) {
	mock := &mockThinkingProvider{}
	d := ResolveReasoningDecision(mock, "gpt-5", "xhigh", "downgrade", "reasoning")
	if d.EffectiveEffort != "high" { t.Errorf("should downgrade to high: %q", d.EffectiveEffort) }
}

func TestResolveReasoningDecision_KnownModel_Unsupported_Disable(t *testing.T) {
	mock := &mockThinkingProvider{}
	d := ResolveReasoningDecision(mock, "gpt-5", "xhigh", "off", "reasoning")
	if d.EffectiveEffort != "off" { t.Errorf("should disable: %q", d.EffectiveEffort) }
}

func TestResolveReasoningDecision_KnownModel_Auto(t *testing.T) {
	mock := &mockThinkingProvider{}
	d := ResolveReasoningDecision(mock, "gpt-5", "auto", "", "reasoning")
	if d.EffectiveEffort != "medium" { t.Errorf("auto should use default: %q", d.EffectiveEffort) }
	if !d.UsedProviderDefault { t.Error("should flag provider default") }
}

func TestResolveReasoningDecision_NoThinkingSupport(t *testing.T) {
	mock := &mockNonThinkingProvider{}
	d := ResolveReasoningDecision(mock, "gpt-5", "high", "", "reasoning")
	if d.EffectiveEffort != "off" { t.Errorf("should be off without thinking: %q", d.EffectiveEffort) }
}

func TestResolveReasoningDecision_UnknownModel_Passthrough(t *testing.T) {
	mock := &mockThinkingProvider{}
	d := ResolveReasoningDecision(mock, "unknown-model-xyz", "high", "", "reasoning")
	if d.EffectiveEffort != "high" { t.Errorf("unknown model should passthrough: %q", d.EffectiveEffort) }
}

func TestReasoningDecision_HasObservation(t *testing.T) {
	d := ReasoningDecision{Source: "reasoning"}
	if !d.HasObservation() { t.Error("should have observation") }
	d2 := ReasoningDecision{Source: "unset"}
	if d2.HasObservation() { t.Error("unset should not have observation") }
}

func TestMergeReasoningMetadata(t *testing.T) {
	d := ReasoningDecision{Source: "reasoning", EffectiveEffort: "high"}
	result := MergeReasoningMetadata(nil, d)
	var m map[string]any
	json.Unmarshal(result, &m)
	if m["reasoning"] == nil { t.Error("should contain reasoning key") }
}

func TestMergeReasoningMetadata_NoObservation(t *testing.T) {
	d := ReasoningDecision{Source: "unset"}
	result := MergeReasoningMetadata([]byte(`{"existing":"data"}`), d)
	var m map[string]any
	json.Unmarshal(result, &m)
	if m["reasoning"] != nil { t.Error("should not add reasoning without observation") }
	if m["existing"] != "data" { t.Error("should preserve existing data") }
}

func TestWithReasoningDecision_Context(t *testing.T) {
	d := ReasoningDecision{Source: "test", EffectiveEffort: "high"}
	ctx := WithReasoningDecision(context.Background(), d)
	got := ReasoningDecisionFromContext(ctx)
	if got == nil { t.Fatal("should retrieve from context") }
	if got.EffectiveEffort != "high" { t.Errorf("effort=%q", got.EffectiveEffort) }
}

func TestReasoningDecisionFromContext_Empty(t *testing.T) {
	got := ReasoningDecisionFromContext(context.Background())
	if got != nil { t.Error("empty context should return nil") }
}

// Mock providers for testing
type mockThinkingProvider struct{}
func (m *mockThinkingProvider) Name() string { return "mock" }
func (m *mockThinkingProvider) DefaultModel() string { return "mock-model" }
func (m *mockThinkingProvider) SupportsThinking() bool { return true }
func (m *mockThinkingProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) { return nil, nil }
func (m *mockThinkingProvider) ChatStream(ctx context.Context, req ChatRequest, fn func(StreamChunk)) (*ChatResponse, error) { return nil, nil }

type mockNonThinkingProvider struct{}
func (m *mockNonThinkingProvider) Name() string { return "mock" }
func (m *mockNonThinkingProvider) DefaultModel() string { return "mock-model" }
func (m *mockNonThinkingProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) { return nil, nil }
func (m *mockNonThinkingProvider) ChatStream(ctx context.Context, req ChatRequest, fn func(StreamChunk)) (*ChatResponse, error) { return nil, nil }
