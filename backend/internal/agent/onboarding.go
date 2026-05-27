// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/memory"
)

// OnboardingPipeline runs post-hire provisioning:
// CHRO (creates agent) → CKO (provisions KB + clearance) → CFO (allocates budget).
// The pipeline is synchronous — each stage must succeed before the agent is ready.
type OnboardingPipeline struct {
	pool       *pgxpool.Pool
	grantStore *memory.GrantStore
	tenantID   string
}

func NewOnboardingPipeline(pool *pgxpool.Pool, grantStore *memory.GrantStore, tenantID string) *OnboardingPipeline {
	return &OnboardingPipeline{pool: pool, grantStore: grantStore, tenantID: tenantID}
}

// OnboardingResult captures what each stage provisioned.
type OnboardingResult struct {
	AgentID        string
	ClearanceLevel int
	KBGrants       []string
	BudgetUSD      float64
	Stage          string
}

// Run executes the full onboarding pipeline for a newly created agent.
// Called from the OnHRHireAgent callback after the agent record is created.
func (p *OnboardingPipeline) Run(ctx context.Context, agentID, orgRole, orgLevel string, monthlyBudgetUSD float64) (*OnboardingResult, error) {
	result := &OnboardingResult{AgentID: agentID}

	// Stage 1: CHRO — agent record already created by caller. Mark started.
	if err := p.initOnboarding(ctx, agentID); err != nil {
		return nil, fmt.Errorf("onboarding init: %w", err)
	}
	result.Stage = "chro_done"

	// Stage 2: CKO — provision clearance + knowledge grants
	clearance, grants, err := p.provisionKnowledge(ctx, agentID, orgRole, orgLevel)
	if err != nil {
		p.setStage(ctx, agentID, "cko_failed")
		return result, fmt.Errorf("cko provisioning: %w", err)
	}
	result.ClearanceLevel = int(clearance)
	result.KBGrants = grants
	result.Stage = "cko_done"

	// Stage 3: CFO — allocate budget
	budget, err := p.allocateBudget(ctx, agentID, orgRole, orgLevel, monthlyBudgetUSD)
	if err != nil {
		p.setStage(ctx, agentID, "cfo_failed")
		return result, fmt.Errorf("cfo budget allocation: %w", err)
	}
	result.BudgetUSD = budget
	result.Stage = "completed"

	// Mark fully onboarded
	p.completeOnboarding(ctx, agentID, clearance, grants, budget)

	slog.Info("agent.onboarding.complete",
		"agent_id", agentID, "org_role", orgRole,
		"clearance", clearance.String(), "grants", len(grants),
		"budget_usd", budget)

	return result, nil
}

func (p *OnboardingPipeline) initOnboarding(ctx context.Context, agentID string) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO agent_onboarding (tenant_id, agent_id, stage, chro_completed)
		 VALUES ($1, $2, 'chro_done', true)
		 ON CONFLICT (agent_id) DO UPDATE SET stage='chro_done', chro_completed=true`,
		p.tenantID, agentID)
	return err
}

func (p *OnboardingPipeline) setStage(ctx context.Context, agentID, stage string) {
	p.pool.Exec(ctx, `UPDATE agent_onboarding SET stage=$1 WHERE agent_id=$2`, stage, agentID)
}

// provisionKnowledge sets clearance and grants baseline KB access.
func (p *OnboardingPipeline) provisionKnowledge(ctx context.Context, agentID, orgRole, orgLevel string) (memory.Classification, []string, error) {
	clearance := memory.ClearanceForRole(orgRole)

	// Write clearance to DB (CKO authority)
	ckoSentinel := "00000000-0000-0000-0000-000000000001"
	_, err := p.pool.Exec(ctx,
		`INSERT INTO agent_clearances (agent_id, tenant_id, max_classification, updated_by, reason)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (agent_id) DO UPDATE SET max_classification=$3, updated_by=$4, updated_at=now(), reason=$5`,
		agentID, p.tenantID, int(clearance), "cko:onboarding", fmt.Sprintf("auto-provisioned for role %s", orgRole))
	if err != nil {
		return 0, nil, err
	}

	// Grant baseline knowledge access based on role/level
	grants := []string{}
	grantScopes := baselineKBGrants(orgRole, orgLevel)
	for _, scope := range grantScopes {
		if p.grantStore != nil {
			grantID, err := p.grantStore.Create(ctx, memory.Grant{
				GrantorAgentID: ckoSentinel,
				GranteeAgentID: agentID,
				Scope:          memory.Scope(scope),
				MaxClass:       clearance,
				ReadOnly:       true,
				Purpose:        fmt.Sprintf("onboarding baseline for %s", orgRole),
				GrantedBy:      "cko:onboarding",
			})
			if err == nil {
				grants = append(grants, grantID)
			}
		}
	}

	// Mark CKO stage complete
	p.pool.Exec(ctx,
		`UPDATE agent_onboarding SET cko_completed=true, stage='cko_done',
		 clearance_level=$1, kb_grants=$2 WHERE agent_id=$3`,
		int(clearance), grants, agentID)

	return clearance, grants, nil
}

// allocateBudget provisions the financial layer for the agent.
func (p *OnboardingPipeline) allocateBudget(ctx context.Context, agentID, orgRole, orgLevel string, requestedBudget float64) (float64, error) {
	budget := requestedBudget
	if budget <= 0 {
		budget = defaultBudgetForLevel(orgLevel, orgRole)
	}

	// CFO caps: ensure budget doesn't exceed role limits
	maxBudget := maxBudgetForRole(orgRole, orgLevel)
	if budget > maxBudget {
		budget = maxBudget
	}

	// Write daily budget (monthly / 30 for daily cap)
	dailyBudget := budget / 30.0

	_, err := p.pool.Exec(ctx,
		`INSERT INTO gateway_budgets (tenant_id, agent_id, monthly_usd, daily_usd)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		p.tenantID, agentID, budget, dailyBudget)
	if err != nil {
		return 0, err
	}

	// Also update the agents table monthly_budget_usd for backward compat
	p.pool.Exec(ctx, `UPDATE agents SET monthly_budget_usd=$1 WHERE id=$2`, budget, agentID)

	// Mark CFO stage complete
	p.pool.Exec(ctx,
		`UPDATE agent_onboarding SET cfo_completed=true, stage='cfo_done', budget_usd=$1 WHERE agent_id=$2`,
		budget, agentID)

	return budget, nil
}

func (p *OnboardingPipeline) completeOnboarding(ctx context.Context, agentID string, clearance memory.Classification, grants []string, budget float64) {
	p.pool.Exec(ctx,
		`UPDATE agent_onboarding SET stage='completed', completed_at=now(),
		 clearance_level=$1, kb_grants=$2, budget_usd=$3
		 WHERE agent_id=$4`,
		int(clearance), grants, budget, agentID)
}

// baselineKBGrants returns which scopes a new agent gets read access to.
func baselineKBGrants(orgRole, orgLevel string) []string {
	// All agents get company-public access
	grants := []string{"company:public"}

	switch orgLevel {
	case "l1":
		grants = append(grants, "company:all", "team:all")
	case "l2":
		grants = append(grants, "team:own", "company:internal")
		// C-suite gets cross-team read for their domain
		switch orgRole {
		case "cko":
			grants = append(grants, "company:all", "team:all")
		case "coo", "cfo":
			grants = append(grants, "company:internal", "team:all")
		case "chro":
			grants = append(grants, "team:all")
		case "cto", "cmo", "cso", "cco", "ciso", "caio":
			grants = append(grants, "team:own")
		}
	case "l3":
		grants = append(grants, "team:own")
	}

	return grants
}

// defaultBudgetForLevel returns the default monthly budget.
func defaultBudgetForLevel(level, role string) float64 {
	switch level {
	case "l1":
		return 200.0
	case "l2":
		switch role {
		case "cko", "coo":
			return 100.0
		case "cto", "cfo":
			return 75.0
		default:
			return 50.0
		}
	case "l3":
		return 10.0
	default:
		return 10.0
	}
}

// maxBudgetForRole returns the hard cap a CFO would enforce.
func maxBudgetForRole(role, level string) float64 {
	switch level {
	case "l1":
		return 500.0
	case "l2":
		return 200.0
	case "l3":
		return 50.0
	default:
		return 25.0
	}
}
