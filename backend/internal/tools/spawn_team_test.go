// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/tools"
)

func makeSpawnTeam(created *[]string) *tools.SpawnTeam {
	return tools.NewSpawnTeam(
		func(ctx context.Context, name, model, role, prompt string) (string, error) {
			*created = append(*created, role)
			return "agent-" + role, nil
		},
		func(tier string) string { return "test-model-" + tier },
	)
}

func TestSpawnTeam_8hDeadlineGives2Agents(t *testing.T) {
	var created []string
	st := makeSpawnTeam(&created)

	result := st.Execute(context.Background(), map[string]any{
		"goal":           "build a web app",
		"budget_cents":   float64(10000),
		"deadline_hours": float64(8),
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if len(created) != 2 {
		t.Errorf("8h deadline: want 2 agents, got %d", len(created))
	}
}

func TestSpawnTeam_1hDeadlineGives1Agent(t *testing.T) {
	var created []string
	st := makeSpawnTeam(&created)

	result := st.Execute(context.Background(), map[string]any{
		"goal":           "summarise this document",
		"budget_cents":   float64(2000),
		"deadline_hours": float64(1),
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if len(created) != 1 {
		t.Errorf("1h deadline: want 1 agent, got %d", len(created))
	}
}

func TestSpawnTeam_32hDeadlineCapsAt8(t *testing.T) {
	var created []string
	st := makeSpawnTeam(&created)

	result := st.Execute(context.Background(), map[string]any{
		"goal":           "research market trends",
		"budget_cents":   float64(100000),
		"deadline_hours": float64(32),
	})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if len(created) != 8 {
		t.Errorf("32h deadline: want 8 agents (capped), got %d", len(created))
	}
}

func TestSpawnTeam_CodingGoalGivesCodeRoles(t *testing.T) {
	var created []string
	st := makeSpawnTeam(&created)

	st.Execute(context.Background(), map[string]any{
		"goal":           "build a REST API",
		"budget_cents":   float64(20000),
		"deadline_hours": float64(16),
	})
	// 16h → 4 agents; coding → [code, architect, reviewer, qa]
	if len(created) != 4 {
		t.Fatalf("want 4 agents, got %d", len(created))
	}
	if created[0] != "code" {
		t.Errorf("first agent for coding goal should be 'code', got %q", created[0])
	}
}

func TestSpawnTeam_OutputMentionsRoster(t *testing.T) {
	var created []string
	st := makeSpawnTeam(&created)

	result := st.Execute(context.Background(), map[string]any{
		"goal":           "write a marketing campaign",
		"budget_cents":   float64(5000),
		"deadline_hours": float64(4),
	})
	if !strings.Contains(result.ForLLM, "marketer") {
		t.Errorf("output should mention marketer role; got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "delegate") {
		t.Errorf("output should mention delegate tool; got: %s", result.ForLLM)
	}
}

func TestSpawnTeam_MissingGoalErrors(t *testing.T) {
	var created []string
	st := makeSpawnTeam(&created)

	result := st.Execute(context.Background(), map[string]any{
		"budget_cents":   float64(5000),
		"deadline_hours": float64(4),
	})
	if !result.IsError {
		t.Error("should return error when goal is missing")
	}
}

func TestSpawnTeam_ZeroBudgetErrors(t *testing.T) {
	var created []string
	st := makeSpawnTeam(&created)

	result := st.Execute(context.Background(), map[string]any{
		"goal":           "do something",
		"budget_cents":   float64(0),
		"deadline_hours": float64(4),
	})
	if !result.IsError {
		t.Error("should return error when budget_cents is 0")
	}
}
