// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
)

// SpawnTeam creates a complete, budget-aware agent team for a goal.
//
// Team sizing (deterministic):
//   - team_size = clamp(deadline_hours / 4, 1, 8)
//   - per_agent_budget = total_budget / team_size
//   - model_tier = TierForBudget(per_agent_budget, task_type)
//
// All created agents are persistent and pre-seeded with archetype bundles
// via the createAgent callback (which calls OnAgentCreate).
type SpawnTeam struct {
	createAgent  func(ctx context.Context, name, model, role, prompt string) (string, error)
	modelForTier func(tier string) string
}

func NewSpawnTeam(
	createAgent func(ctx context.Context, name, model, role, prompt string) (string, error),
	modelForTier func(tier string) string,
) *SpawnTeam {
	return &SpawnTeam{createAgent: createAgent, modelForTier: modelForTier}
}

func (t *SpawnTeam) Name() string { return "spawn_team" }

func (t *SpawnTeam) Description() string {
	return "Design and create a team of specialist agents sized for a goal, budget, and deadline. Returns a roster of created agents with their roles, models, and individual budgets. All agents are persistent and immediately available for delegation."
}

func (t *SpawnTeam) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"goal": map[string]any{
				"type":        "string",
				"description": "What the team must accomplish — used to select appropriate roles",
			},
			"budget_cents": map[string]any{
				"type":        "number",
				"description": "Total budget in cents for the entire team (e.g. 10000 = $100)",
			},
			"deadline_hours": map[string]any{
				"type":        "number",
				"description": "Hours available to complete the work — determines team size (1 agent per 4h, max 8)",
			},
			"team_name": map[string]any{
				"type":        "string",
				"description": "Optional name prefix for the agents (defaults to first two words of goal)",
			},
		},
		"required": []string{"goal", "budget_cents", "deadline_hours"},
	}
}

func (t *SpawnTeam) Execute(ctx context.Context, args map[string]any) *Result {
	goal, _ := args["goal"].(string)
	budgetCents := int64(spawnToFloat(args["budget_cents"]))
	deadlineHours := spawnToFloat(args["deadline_hours"])
	teamName, _ := args["team_name"].(string)

	if goal == "" {
		return ErrorResult("goal required")
	}
	if budgetCents <= 0 {
		return ErrorResult("budget_cents must be > 0")
	}
	if deadlineHours <= 0 {
		deadlineHours = 4
	}

	// Team size: 1 agent per 4h, min 1, max 8.
	teamSize := int(deadlineHours / 4)
	if teamSize < 1 {
		teamSize = 1
	}
	if teamSize > 8 {
		teamSize = 8
	}

	perAgentBudget := budgetCents / int64(teamSize)
	goalLower := strings.ToLower(goal)
	taskType := spawnClassifyGoal(goalLower)
	tier := spawnTierForBudget(perAgentBudget, taskType)
	model := ""
	if t.modelForTier != nil {
		model = t.modelForTier(tier)
	}

	roles := spawnRolesForGoal(goalLower, teamSize)

	if teamName == "" {
		words := strings.Fields(goal)
		if len(words) > 2 {
			words = words[:2]
		}
		teamName = strings.Join(words, "-")
	}
	teamName = strings.ToLower(strings.ReplaceAll(teamName, " ", "-"))

	type member struct {
		name  string
		role  string
		model string
		id    string
	}
	var roster []member

	for i, role := range roles {
		name := fmt.Sprintf("%s-%s-%d", teamName, role, i+1)
		id, err := t.createAgent(ctx, name, model, role, "")
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to create agent %q: %v", name, err))
		}
		roster = append(roster, member{name: name, role: role, model: model, id: id})
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Team created — %d agent(s), %s tier, $%.2f/agent budget, %.0fh deadline:\n\n",
		teamSize, tier, float64(perAgentBudget)/100, deadlineHours))
	for _, m := range roster {
		modelLabel := m.model
		if modelLabel == "" {
			modelLabel = "auto"
		}
		sb.WriteString(fmt.Sprintf("• %s [%s] — model: %s, id: %s\n", m.name, m.role, modelLabel, m.id))
	}
	sb.WriteString("\nAll agents are ready. Delegate work to them by name using the delegate tool.")
	return TextResult(sb.String())
}

// spawnTierForBudget maps per-agent budget to a model tier.
// Cannot import agent.TierForBudget directly (agent imports tools — cycle).
func spawnTierForBudget(budgetCents int64, taskType string) string {
	switch {
	case budgetCents >= 5000:
		if taskType == "coding" || taskType == "code" || taskType == "developer" {
			return providers.TierCoding
		}
		return providers.TierComplex
	case budgetCents >= 1000:
		return providers.TierStandard
	default:
		return providers.TierSimple
	}
}

// spawnClassifyGoal returns a task type keyword used by spawnTierForBudget.
func spawnClassifyGoal(goal string) string {
	codingKW := []string{"code", "build", "implement", "app", "website", "api", "software", "develop", "program", "script", "scraper"}
	for _, kw := range codingKW {
		if strings.Contains(goal, kw) {
			return "coding"
		}
	}
	researchKW := []string{"research", "analyse", "analyze", "report", "data", "study", "investigate", "market"}
	for _, kw := range researchKW {
		if strings.Contains(goal, kw) {
			return "research"
		}
	}
	return "general"
}

// spawnRolesForGoal returns an ordered slice of role keys sized to teamSize.
func spawnRolesForGoal(goal string, teamSize int) []string {
	coding    := []string{"code", "architect", "reviewer", "qa", "devops", "code", "code", "code"}
	research  := []string{"researcher", "analyst", "writer", "researcher", "analyst", "writer", "researcher", "analyst"}
	marketing := []string{"marketer", "writer", "social", "product", "marketer", "writer", "social", "product"}
	general   := []string{"general", "researcher", "writer", "analyst", "general", "researcher", "writer", "analyst"}

	isCoding := func(g string) bool {
		for _, kw := range []string{"code", "build", "implement", "app", "website", "api", "software", "develop", "program", "script", "scraper"} {
			if strings.Contains(g, kw) {
				return true
			}
		}
		return false
	}
	isMarketing := func(g string) bool {
		for _, kw := range []string{"market", "launch", "campaign", "content", "advertis", "brand", "social"} {
			if strings.Contains(g, kw) {
				return true
			}
		}
		return false
	}
	isResearch := func(g string) bool {
		for _, kw := range []string{"research", "analyse", "analyze", "report", "data", "study", "investigate"} {
			if strings.Contains(g, kw) {
				return true
			}
		}
		return false
	}

	var pool []string
	switch {
	case isCoding(goal):
		pool = coding
	case isMarketing(goal):
		pool = marketing
	case isResearch(goal):
		pool = research
	default:
		pool = general
	}
	if teamSize > len(pool) {
		teamSize = len(pool)
	}
	return pool[:teamSize]
}

func spawnToFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}
