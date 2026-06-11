package gateway

import "testing"

func TestEstimateTokens(t *testing.T) {
	if estimateTokens("backend-dev", "mvp") == 0 { t.Error("backend mvp should be > 0") }
	if estimateTokens("backend-dev", "enterprise") <= estimateTokens("backend-dev", "mvp") {
		t.Error("enterprise should estimate more tokens than mvp")
	}
	if estimateTokens("unknown-role", "mvp") == 0 { t.Error("unknown role gets a default > 0") }
}

func TestComputeAgentCostUUSD(t *testing.T) {
	in, out := 1_000_000, 1_000_000
	cost, missing := computeAgentCostUUSD(3.0, 15.0, in, out)
	if missing { t.Error("priced model not missing") }
	if cost != 18_000_000 { t.Errorf("got %d want 18000000", cost) }
	_, missing2 := computeAgentCostUUSD(0, 0, in, out)
	if !missing2 { t.Error("zero price should flag missing") }
}

func TestResourcePlanMarshalRoundtrip(t *testing.T) {
	p := ResourcePlan{
		Agents: []PlannedAgent{{Role: "backend-dev", ModelID: "claude-sonnet-4-6", ProviderID: "anthropic", EstTokensIn: 100, EstTokensOut: 200, CapUUSD: 5_000_000, PricingKnown: true}},
		TotalEstUUSD: 5_000_000, ProjectCapUUSD: 7_500_000, Timeline: "this_week",
	}
	md := p.ToMarkdown()
	if len(md) < 20 { t.Error("markdown too short") }
	parsed, err := ParseResourcePlan(md)
	if err != nil { t.Fatalf("parse: %v", err) }
	if len(parsed.Agents) != 1 || parsed.Agents[0].Role != "backend-dev" { t.Errorf("roundtrip lost data: %+v", parsed) }
	if parsed.ProjectCapUUSD != 7_500_000 { t.Error("cap lost in roundtrip") }
}

func TestBuildResourcePlanFromRoles(t *testing.T) {
	roles := []string{"backend-dev", "frontend-dev"}
	pick := func(role, tier string) (string, string) { return "claude-sonnet-4-6", "anthropic" }
	rates := map[string]ModelPricingLite{"claude-sonnet-4-6": {Input: 3.0, Output: 15.0}}
	p := buildResourcePlan(roles, "mvp", "this_week", 50.0, pick, rates)
	if len(p.Agents) != 2 { t.Fatalf("want 2 agents got %d", len(p.Agents)) }
	if p.TotalEstUUSD <= 0 { t.Error("total should be > 0") }
	if p.ProjectCapUUSD < p.TotalEstUUSD { t.Error("cap should be >= estimate") }
	for _, a := range p.Agents { if a.CapUUSD <= 0 { t.Error("each agent gets a positive cap") } }
}

func TestBuildResourcePlanBudgetCeiling(t *testing.T) {
	roles := []string{"backend-dev"}
	pick := func(role, tier string) (string, string) { return "claude-sonnet-4-6", "anthropic" }
	rates := map[string]ModelPricingLite{"claude-sonnet-4-6": {Input: 3.0, Output: 15.0}}
	p := buildResourcePlan(roles, "mvp", "today", 0.01, pick, rates)
	if p.ProjectCapUUSD != int64(0.01*uusdPerUSD) { t.Errorf("cap should clamp to budget, got %d", p.ProjectCapUUSD) }
}
