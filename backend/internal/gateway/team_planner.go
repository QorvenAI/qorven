// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
// team_planner.go — budget-aware agent + task graph generator.
// Pure function: no DB, no HTTP. Called by handleApproveInception.
package gateway

import "fmt"

// ModelSpec describes one LLM model available for agent assignment.
type ModelSpec struct {
	ID            string  // model ID passed to agentLoop
	Label         string  // display name
	InputCentsPerM  float64 // cost per million input tokens
	OutputCentsPerM float64 // cost per million output tokens
	ContextK      int     // context window in thousands of tokens
	Tier          string  // "budget", "mid", "premium"
	Strengths     []string
}

// MODEL_CATALOG is the static catalog. Update pricing as needed.
var MODEL_CATALOG = []ModelSpec{
	{
		ID: "claude-haiku-4-5-20251001", Label: "Haiku 4.5",
		InputCentsPerM: 80, OutputCentsPerM: 400, ContextK: 200,
		Tier: "budget", Strengths: []string{"coordination", "docs", "review"},
	},
	{
		ID: "deepseek/deepseek-v3", Label: "DeepSeek V3",
		InputCentsPerM: 27, OutputCentsPerM: 110, ContextK: 128,
		Tier: "budget", Strengths: []string{"coding", "boilerplate"},
	},
	{
		ID: "claude-sonnet-4-6", Label: "Sonnet 4.6",
		InputCentsPerM: 300, OutputCentsPerM: 1500, ContextK: 200,
		Tier: "mid", Strengths: []string{"coding", "architecture", "testing"},
	},
	{
		ID: "claude-opus-4-7", Label: "Opus 4.7",
		InputCentsPerM: 1500, OutputCentsPerM: 7500, ContextK: 200,
		Tier: "premium", Strengths: []string{"architecture", "coordination", "review"},
	},
}

// ProposedAgent is one agent in the team proposal.
type ProposedAgent struct {
	Role        string  `json:"role"`
	DisplayName string  `json:"display_name"`
	Model       string  `json:"model"`
	ModelLabel  string  `json:"model_label"`
	Tasks       []string `json:"tasks"` // task titles this agent owns
	EstMinCents int     `json:"est_min_cents"`
	EstMaxCents int     `json:"est_max_cents"`
}

// ProposedTask is one task node in the dependency graph.
type ProposedTask struct {
	Title      string   `json:"title"`
	Role       string   `json:"role"`    // which ProposedAgent.Role handles this
	Priority   string   `json:"priority"`
	BlockedBy  []string `json:"blocked_by"` // titles of tasks that must complete first
	EstMinCents int     `json:"est_min_cents"`
	EstMaxCents int     `json:"est_max_cents"`
}

// TeamProposal is what Prime sends to the user for approval.
type TeamProposal struct {
	Agents      []ProposedAgent `json:"agents"`
	Tasks       []ProposedTask  `json:"tasks"`
	EstMinCents int             `json:"est_min_cents"`
	EstMaxCents int             `json:"est_max_cents"`
	Reasoning   string          `json:"reasoning"`
}

// PlanTeam generates a budget-aware TeamProposal for a given brief.
// budget is in USD cents (e.g. 2000 = $20).
// quality is "mvp", "production", or "enterprise".
func PlanTeam(idea, stack, quality string, budgetCents int) TeamProposal {
	tier := budgetTier(budgetCents)

	devModel  := pickModel(tier, "coding")
	testModel := pickModel("budget", "review")
	coordModel := pickModel(tier, "coordination")

	reasoning := fmt.Sprintf(
		"Budget $%d → tier=%s. Developer on %s (%s), Tester on %s (%s), Prime on %s (%s).",
		budgetCents/100, tier,
		devModel.Label, devModel.ID,
		testModel.Label, testModel.ID,
		coordModel.Label, coordModel.ID,
	)

	agents := []ProposedAgent{
		{
			Role: "developer", DisplayName: "Dev",
			Model: devModel.ID, ModelLabel: devModel.Label,
			Tasks:       []string{"Design schema", "Implement API handlers", "Implement auth"},
			EstMinCents: 800, EstMaxCents: 1200,
		},
		{
			Role: "tester", DisplayName: "Tester",
			Model: testModel.ID, ModelLabel: testModel.Label,
			Tasks:       []string{"Write unit tests", "Write integration tests"},
			EstMinCents: 100, EstMaxCents: 200,
		},
	}

	// Add reviewer for production/enterprise
	if quality != "mvp" {
		revModel := pickModel(tier, "review")
		agents = append(agents, ProposedAgent{
			Role: "reviewer", DisplayName: "Reviewer",
			Model: revModel.ID, ModelLabel: revModel.Label,
			Tasks:       []string{"Code review", "Security audit"},
			EstMinCents: 200, EstMaxCents: 400,
		})
	}

	// Add docs writer for enterprise
	if quality == "enterprise" {
		agents = append(agents, ProposedAgent{
			Role: "writer", DisplayName: "Writer",
			Model: testModel.ID, ModelLabel: testModel.Label,
			Tasks:       []string{"Write README", "Write API docs"},
			EstMinCents: 50, EstMaxCents: 100,
		})
	}

	tasks := buildTaskGraph(agents, quality)

	totalMin, totalMax := 0, 0
	for _, a := range agents {
		totalMin += a.EstMinCents
		totalMax += a.EstMaxCents
	}

	return TeamProposal{
		Agents:      agents,
		Tasks:       tasks,
		EstMinCents: totalMin,
		EstMaxCents: totalMax,
		Reasoning:   reasoning,
	}
}

func budgetTier(cents int) string {
	switch {
	case cents <= 500:   return "budget"  // $0–$5
	case cents <= 5000:  return "mid"     // $5–$50
	default:             return "premium" // $50+
	}
}

func pickModel(tier, strength string) ModelSpec {
	// Try to find a model in the right tier with the matching strength
	for _, m := range MODEL_CATALOG {
		if m.Tier == tier {
			for _, s := range m.Strengths {
				if s == strength {
					return m
				}
			}
		}
	}
	// Fallback: first model in tier
	for _, m := range MODEL_CATALOG {
		if m.Tier == tier {
			return m
		}
	}
	return MODEL_CATALOG[0]
}

func buildTaskGraph(agents []ProposedAgent, quality string) []ProposedTask {
	tasks := []ProposedTask{
		{Title: "Design schema",           Role: "developer", Priority: "high",   BlockedBy: nil},
		{Title: "Implement API handlers",  Role: "developer", Priority: "high",   BlockedBy: []string{"Design schema"}},
		{Title: "Implement auth",          Role: "developer", Priority: "high",   BlockedBy: []string{"Design schema"}},
		{Title: "Write unit tests",        Role: "tester",    Priority: "normal", BlockedBy: []string{"Design schema"}},
		{Title: "Write integration tests", Role: "tester",    Priority: "normal", BlockedBy: []string{"Implement API handlers", "Implement auth"}},
	}

	hasRole := func(role string) bool {
		for _, a := range agents { if a.Role == role { return true } }
		return false
	}

	if hasRole("reviewer") {
		tasks = append(tasks,
			ProposedTask{Title: "Code review",    Role: "reviewer", Priority: "normal",
				BlockedBy: []string{"Write integration tests"}},
			ProposedTask{Title: "Security audit", Role: "reviewer", Priority: "normal",
				BlockedBy: []string{"Code review"}},
		)
	}

	if hasRole("writer") {
		finalTask := "Write integration tests"
		if hasRole("reviewer") { finalTask = "Security audit" }
		tasks = append(tasks,
			ProposedTask{Title: "Write README",   Role: "writer", Priority: "low", BlockedBy: []string{finalTask}},
			ProposedTask{Title: "Write API docs", Role: "writer", Priority: "low", BlockedBy: []string{"Write README"}},
		)
	}

	return tasks
}
