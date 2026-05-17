// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package providers

import (
	"strings"
)

// RequestDimensions scores a request across 15 dimensions (0.0-1.0 each).
type RequestDimensions struct {
	Complexity    float64 // simple chat vs multi-step reasoning
	CodeGen       float64 // needs code generation
	Vision        float64 // needs image understanding
	ToolUse       float64 // needs function calling
	ContextLength float64 // how much context needed
	SpeedCritical float64 // latency matters
	CostSensitive float64 // budget constrained
	Creativity    float64 // creative writing vs factual
	Research      float64 // web research needed
	Multilingual  float64 // non-English content
	Safety        float64 // sensitive content handling
	Accuracy      float64 // factual precision needed
	LongOutput    float64 // expects long response
	Structured    float64 // needs structured output (JSON, tables)
	Agentic       float64 // multi-turn agent behavior
}

// ScoreRequest analyzes a message and returns dimension scores.
func ScoreRequest(message string, hasTools bool, hasImages bool, historyLen int) RequestDimensions {
	lower := strings.ToLower(message)
	d := RequestDimensions{}

	// Complexity
	if len(message) > 500 || strings.Count(message, "\n") > 5 { d.Complexity = 0.7 }
	complexWords := []string{"analyze", "compare", "evaluate", "design", "architect"}
	for _, w := range complexWords { if strings.Contains(lower, w) { d.Complexity = 0.9; break } }

	// Code
	codeWords := []string{"code", "function", "bug", "error", "implement", "refactor", "debug", "api", "class", "struct"}
	for _, w := range codeWords { if strings.Contains(lower, w) { d.CodeGen = 0.8; break } }

	// Vision
	if hasImages { d.Vision = 1.0 }

	// Tools
	if hasTools { d.ToolUse = 0.6 }
	toolWords := []string{"search", "fetch", "find", "look up", "check", "weather", "news", "latest"}
	for _, w := range toolWords { if strings.Contains(lower, w) { d.ToolUse = 0.9; break } }

	// Context
	if historyLen > 10 { d.ContextLength = 0.7 }
	if historyLen > 30 { d.ContextLength = 1.0 }

	// Speed — detect latency-sensitive requests from keywords
	d.SpeedCritical = 0.3 // default: not speed-critical
	speedWords := []string{"fast", "quick", "quickly", "real-time", "realtime", "instant", "immediately", "asap", "hurry", "urgent", "low latency"}
	for _, w := range speedWords {
		if strings.Contains(lower, w) { d.SpeedCritical = 0.9; break }
	}

	// Cost
	d.CostSensitive = 0.5 // default medium

	// Research
	researchWords := []string{"research", "find out", "what happened", "latest", "news", "update", "released"}
	for _, w := range researchWords { if strings.Contains(lower, w) { d.Research = 0.9; break } }

	// Creativity
	creativeWords := []string{"write", "story", "poem", "creative", "imagine", "draft", "compose"}
	for _, w := range creativeWords { if strings.Contains(lower, w) { d.Creativity = 0.8; break } }

	// Multilingual
	for _, r := range message {
		if r > 127 { d.Multilingual = 0.7; break }
	}

	// Accuracy
	accuracyWords := []string{"exact", "precise", "correct", "accurate", "verify", "fact"}
	for _, w := range accuracyWords { if strings.Contains(lower, w) { d.Accuracy = 0.9; break } }

	// Long output
	longWords := []string{"detailed", "comprehensive", "full", "complete", "thorough", "explain in detail"}
	for _, w := range longWords { if strings.Contains(lower, w) { d.LongOutput = 0.8; break } }

	// Structured
	structWords := []string{"json", "table", "csv", "list", "format", "structured"}
	for _, w := range structWords { if strings.Contains(lower, w) { d.Structured = 0.8; break } }

	// Agentic
	if hasTools && historyLen > 5 { d.Agentic = 0.7 }

	return d
}

// RoutingTier determines which tier to use based on dimensions.
type RoutingTier string
const (
	TierEco     RoutingTier = "eco"     // cheapest possible
	TierAuto    RoutingTier = "auto"    // balanced (default)
	TierPremium RoutingTier = "premium" // best quality
)

// SelectTier picks the routing tier based on dimension scores.
func SelectTier(d RequestDimensions, profile RoutingTier) RoutingTier {
	if profile != "" { return profile }
	// Auto-select based on complexity
	score := d.Complexity*0.3 + d.CodeGen*0.15 + d.Research*0.15 + d.Accuracy*0.1 +
		d.ToolUse*0.1 + d.Agentic*0.1 + d.LongOutput*0.05 + d.Creativity*0.05
	if score > 0.7 { return TierPremium }
	if score < 0.3 { return TierEco }
	return TierAuto
}

// ModelExclusion tracks models blocked by the user.
type ModelExclusion struct {
	Excluded map[string]bool `json:"excluded"`
}

func NewModelExclusion() *ModelExclusion { return &ModelExclusion{Excluded: make(map[string]bool)} }

func (e *ModelExclusion) Add(model string)    { e.Excluded[strings.ToLower(model)] = true }
func (e *ModelExclusion) Remove(model string) { delete(e.Excluded, strings.ToLower(model)) }
func (e *ModelExclusion) Clear()              { e.Excluded = make(map[string]bool) }
func (e *ModelExclusion) IsExcluded(model string) bool { return e.Excluded[strings.ToLower(model)] }
func (e *ModelExclusion) List() []string {
	var out []string
	for m := range e.Excluded { out = append(out, m) }
	return out
}

// CostTracker tracks per-request costs.
type CostTracker struct {
	TotalRequests int     `json:"total_requests"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	TotalInputTok int     `json:"total_input_tokens"`
	TotalOutputTok int    `json:"total_output_tokens"`
}

func (ct *CostTracker) Record(inputTokens, outputTokens int, inputPrice, outputPrice float64) {
	ct.TotalRequests++
	ct.TotalInputTok += inputTokens
	ct.TotalOutputTok += outputTokens
	ct.TotalCostUSD += float64(inputTokens)/1_000_000*inputPrice + float64(outputTokens)/1_000_000*outputPrice
}

func (ct *CostTracker) AvgCostPerRequest() float64 {
	if ct.TotalRequests == 0 { return 0 }
	return ct.TotalCostUSD / float64(ct.TotalRequests)
}
