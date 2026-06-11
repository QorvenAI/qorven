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
