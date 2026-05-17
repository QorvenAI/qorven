// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tuner

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/providers"
)

// TestCase defines a single evaluation prompt + expected behavior.
type TestCase struct {
	ID             string  `json:"id"`
	AgentID        string  `json:"agent_id"`
	SkillSlug      string  `json:"skill_slug,omitempty"`
	Prompt         string  `json:"prompt"`
	ExpectedOutput string  `json:"expected_output,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

// EvalResult is the outcome of running one test case.
type EvalResult struct {
	ID           string    `json:"id"`
	TestCaseID   string    `json:"test_case_id"`
	AgentID      string    `json:"agent_id"`
	ActualOutput string    `json:"actual_output"`
	Score        float64   `json:"score"`
	JudgeModel   string    `json:"judge_model"`
	Reasoning    string    `json:"reasoning"`
	CreatedAt    time.Time `json:"created_at"`
}

// Evaluator runs test cases against agents and scores with LLM-as-judge.
type Evaluator struct {
	pool     *pgxpool.Pool
	provider providers.Provider
	model    string
}

func NewEvaluator(pool *pgxpool.Pool, provider providers.Provider, judgeModel string) *Evaluator {
	return &Evaluator{pool: pool, provider: provider, model: judgeModel}
}

// Judge scores an agent response using LLM-as-judge.
func (e *Evaluator) Judge(ctx context.Context, tc TestCase, actual string) (*EvalResult, error) {
	prompt := fmt.Sprintf(`You are an evaluation judge. Score the agent's response on a scale of 0.0 to 1.0.

Test prompt: %s
Expected behavior: %s
Actual response: %s

Respond with JSON: {"score": 0.0-1.0, "reasoning": "brief explanation"}`, tc.Prompt, tc.ExpectedOutput, actual)

	resp, err := e.provider.Chat(ctx, providers.ChatRequest{
		Model: e.model,
		Messages: []providers.Message{
			{Role: "user", Content: prompt},
		},
		Options: map[string]any{
			"temperature":     0,
			"response_format": map[string]string{"type": "json_object"},
		},
	})
	if err != nil {
		return nil, err
	}

	var judgeResp struct {
		Score     float64 `json:"score"`
		Reasoning string  `json:"reasoning"`
	}
	json.Unmarshal([]byte(resp.Content), &judgeResp)

	result := &EvalResult{
		ID:           uuid.New().String(),
		TestCaseID:   tc.ID,
		AgentID:      tc.AgentID,
		ActualOutput: actual,
		Score:        judgeResp.Score,
		JudgeModel:   e.model,
		Reasoning:    judgeResp.Reasoning,
		CreatedAt:    time.Now(),
	}

	// Persist
	if e.pool != nil {
		e.pool.Exec(ctx,
			`INSERT INTO evaluations (id, test_case_id, agent_id, actual_output, score, judge_model, reasoning)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			result.ID, result.TestCaseID, result.AgentID, result.ActualOutput,
			result.Score, result.JudgeModel, result.Reasoning)
	}

	return result, nil
}

// RunSuite runs all test cases for an agent and returns aggregate score.
func (e *Evaluator) RunSuite(ctx context.Context, cases []TestCase,
	runAgent func(ctx context.Context, agentID, msg string) (string, error)) ([]EvalResult, float64, error) {

	results := []EvalResult{}
	var totalScore float64

	for _, tc := range cases {
		actual, err := runAgent(ctx, tc.AgentID, tc.Prompt)
		if err != nil {
			continue
		}
		result, err := e.Judge(ctx, tc, actual)
		if err != nil {
			continue
		}
		results = append(results, *result)
		totalScore += result.Score
	}

	avg := 0.0
	if len(results) > 0 {
		avg = totalScore / float64(len(results))
	}
	return results, avg, nil
}
