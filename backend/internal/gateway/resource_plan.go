// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"encoding/json"
	"fmt"
	"strings"
)

// uusdPerUSD — micro-dollars per dollar. The ledger's canonical unit.
const uusdPerUSD = 1_000_000

// PlannedAgent is one role in the CFO resource plan.
type PlannedAgent struct {
	Role         string `json:"role"`
	ModelID      string `json:"model_id"`
	ProviderID   string `json:"provider_id"`
	EstTokensIn  int    `json:"est_tokens_in"`
	EstTokensOut int    `json:"est_tokens_out"`
	CapUUSD      int64  `json:"cap_uusd"`
	PricingKnown bool   `json:"pricing_known"`
}

// ResourcePlan is the CFO's structured output for the resource_plan artifact.
type ResourcePlan struct {
	Agents         []PlannedAgent `json:"agents"`
	TotalEstUUSD   int64          `json:"total_est_uusd"`
	ProjectCapUUSD int64          `json:"project_cap_uusd"`
	Timeline       string         `json:"timeline"`
	Notes          []string       `json:"notes,omitempty"`
}

// estimateTokens returns a rough input-token estimate for a role at a quality
// tier. Heuristic; the plan persists the snapshot so estimates are reproducible.
func estimateTokens(role, quality string) int {
	base := map[string]int{
		"backend-dev": 600_000, "frontend-dev": 500_000, "devops": 300_000,
		"reviewer": 250_000, "qa": 250_000, "writer": 200_000, "architect": 200_000,
	}
	b, ok := base[role]
	if !ok {
		b = 400_000
	}
	mult := map[string]float64{"mvp": 1.0, "production": 1.8, "enterprise": 3.0}
	m, ok := mult[quality]
	if !ok {
		m = 1.0
	}
	return int(float64(b) * m)
}

// computeAgentCostUUSD prices a token budget at the given per-1M USD rates.
// Returns (cost in µUSD, pricingMissing). Zero rates flag missing (cost 0).
func computeAgentCostUUSD(inputPer1M, outputPer1M float64, tokensIn, tokensOut int) (int64, bool) {
	if inputPer1M <= 0 && outputPer1M <= 0 {
		return 0, true
	}
	usd := (float64(tokensIn)/1_000_000)*inputPer1M + (float64(tokensOut)/1_000_000)*outputPer1M
	return int64(usd * uusdPerUSD), false
}

// ToMarkdown renders a human-readable plan table plus a fenced JSON block that
// ParseResourcePlan reads back (single source, diffable in the artifact row).
func (p ResourcePlan) ToMarkdown() string {
	var sb strings.Builder
	sb.WriteString("# Resource Plan\n\n")
	sb.WriteString("| Role | Model | Provider | Est. tokens (in/out) | Cap |\n|---|---|---|---|---|\n")
	for _, a := range p.Agents {
		capStr := fmt.Sprintf("$%.2f", float64(a.CapUUSD)/uusdPerUSD)
		if !a.PricingKnown {
			capStr = "pricing unknown"
		}
		fmt.Fprintf(&sb, "| %s | %s | %s | %d / %d | %s |\n", a.Role, a.ModelID, a.ProviderID, a.EstTokensIn, a.EstTokensOut, capStr)
	}
	fmt.Fprintf(&sb, "\n**Total estimate:** $%.2f  \n**Project cap:** $%.2f  \n**Timeline:** %s\n",
		float64(p.TotalEstUUSD)/uusdPerUSD, float64(p.ProjectCapUUSD)/uusdPerUSD, p.Timeline)
	for _, n := range p.Notes {
		fmt.Fprintf(&sb, "\n> ⚠ %s\n", n)
	}
	b, _ := json.Marshal(p)
	fmt.Fprintf(&sb, "\n<!-- RESOURCE_PLAN_JSON\n%s\n-->\n", string(b))
	return sb.String()
}

// ParseResourcePlan extracts the structured plan from the fenced JSON in md.
func ParseResourcePlan(md string) (ResourcePlan, error) {
	const openTag, closeTag = "<!-- RESOURCE_PLAN_JSON", "-->"
	i := strings.Index(md, openTag)
	if i < 0 {
		return ResourcePlan{}, fmt.Errorf("no plan json block")
	}
	rest := md[i+len(openTag):]
	j := strings.Index(rest, closeTag)
	if j < 0 {
		return ResourcePlan{}, fmt.Errorf("unterminated plan json block")
	}
	var p ResourcePlan
	if err := json.Unmarshal([]byte(strings.TrimSpace(rest[:j])), &p); err != nil {
		return ResourcePlan{}, err
	}
	return p, nil
}
