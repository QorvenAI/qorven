// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
)

// Evaluator uses a cheap LLM to judge agent outputs.
// Mentor: "Prime should be judging structure, format, and simple correctness,
// not doing deep analysis itself."
type Evaluator struct {
	provider providers.Provider
	model    string // cheap/fast model (Gemini Flash, Haiku-class)
}

// NewEvaluator creates an evaluator with a specific model.
func NewEvaluator(provider providers.Provider, model string) *Evaluator {
	if model == "" {
		model = "deepseek-v3.2" // cheap default
	}
	return &Evaluator{provider: provider, model: model}
}

const evalPrompt = `You are a quality evaluator for an AI agent system. Judge this agent output.

Agent: %s
Task: %s
Output: %s

Evaluate:
1. Is the output relevant to the task? (yes/no)
2. Is it fresh (not stale/outdated)? (yes/no)
3. Is it non-repetitive (not repeating itself)? (yes/no)
4. Are there any errors or issues? (describe briefly)
5. Overall quality: good / degraded / bad
6. Risk level: low / medium / high
7. If degraded/bad, suggest a fix type from: restart_cron, retry_api, clear_cache, switch_model, reset_session, restart_channel (or "none")

Respond in JSON only:
{"quality":"good|degraded|bad","relevant":true,"fresh":true,"repetitive":false,"issues":[],"risk":"low|medium|high","suggested_fix":"none|fix_type","fix_params":{}}`

// Evaluate judges an agent's output using the cheap LLM.
func (e *Evaluator) Evaluate(ctx context.Context, agentID, agentName, task, output string) (*EvalResult, error) {
	if output == "" {
		return &EvalResult{Quality: "bad", Issues: []string{"empty output"}, Risk: RiskMedium}, nil
	}

	// Truncate long outputs — evaluator only needs a sample
	evalOutput := output
	if len(evalOutput) > 2000 {
		evalOutput = evalOutput[:2000] + "... [truncated]"
	}

	prompt := fmt.Sprintf(evalPrompt, agentName, task, evalOutput)

	resp, err := e.provider.Chat(ctx, providers.ChatRequest{
		Model:    e.model,
		Messages: []providers.Message{{Role: "user", Content: prompt}},
		Options:  map[string]any{"max_tokens": 300, "temperature": 0.1},
	})
	if err != nil {
		return nil, fmt.Errorf("evaluator LLM call failed: %w", err)
	}

	return parseEvalResponse(resp.Content)
}

func parseEvalResponse(raw string) (*EvalResult, error) {
	// Extract JSON from response (may have surrounding text)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < 0 || end <= start {
		// Fallback: assume good if we can't parse
		return &EvalResult{Quality: "good", Risk: RiskLow}, nil
	}

	var parsed struct {
		Quality      string   `json:"quality"`
		Issues       []string `json:"issues"`
		Risk         string   `json:"risk"`
		SuggestedFix string   `json:"suggested_fix"`
		FixParams    map[string]any `json:"fix_params"`
	}

	if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err != nil {
		return &EvalResult{Quality: "good", Risk: RiskLow}, nil
	}

	result := &EvalResult{
		Quality: parsed.Quality,
		Issues:  parsed.Issues,
		Risk:    RiskLevel(parsed.Risk),
	}

	if parsed.SuggestedFix != "" && parsed.SuggestedFix != "none" {
		fix := FixType(parsed.SuggestedFix)
		result.SuggestedFix = &fix
		result.FixParams = parsed.FixParams
	}

	// Validate
	if result.Quality == "" {
		result.Quality = "good"
	}
	if result.Risk == "" {
		result.Risk = RiskLow
	}

	return result, nil
}
