// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Package budgets owns the budget hierarchy: departments, projects, and the
// per-scope caps with carved-vs-fresh allocation semantics.
package budgets

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrOverAllocated is returned when a carved allocation would exceed the
// parent pool's available budget.
var ErrOverAllocated = errors.New("allocation exceeds the parent budget's available pool")

// validateAllocation enforces the carved-vs-fresh rule. For "fresh" the child
// is additive (no draw-down). For "carved" the sum of carved children must not
// exceed the parent cap; a parent cap of 0 means unlimited.
func validateAllocation(mode string, parentCapUSD, existingCarvedUSD, newCapUSD float64) error {
	if mode == "fresh" {
		return nil
	}
	return validateCarved(parentCapUSD, existingCarvedUSD, newCapUSD)
}

// validateCarved checks a carved child against the parent's available pool.
func validateCarved(parentCapUSD, existingCarvedUSD, newCapUSD float64) error {
	if parentCapUSD <= 0 {
		return nil // unlimited parent
	}
	if existingCarvedUSD+newCapUSD > parentCapUSD {
		return fmt.Errorf("%w: parent cap $%.2f, already allocated $%.2f, requested $%.2f",
			ErrOverAllocated, parentCapUSD, existingCarvedUSD, newCapUSD)
	}
	return nil
}

// Store is the budget-hierarchy data access layer.
type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

// Department / Project DTOs.
type Department struct {
	ID                 string `json:"id"`
	TenantID           string `json:"tenant_id"`
	Name               string `json:"name"`
	HeadAgentID        string `json:"head_agent_id,omitempty"`
	ParentDepartmentID string `json:"parent_department_id,omitempty"`
}
type Project struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	Name         string `json:"name"`
	DepartmentID string `json:"department_id,omitempty"`
	Status       string `json:"status"`
}

// CreateDepartment inserts a department and returns its id.
func (s *Store) CreateDepartment(ctx context.Context, tenantID, name, headAgentID string) (string, error) {
	var id string
	err := s.db.QueryRow(ctx,
		`INSERT INTO departments (tenant_id, name, head_agent_id)
		 VALUES ($1, $2, NULLIF($3,'')::uuid) RETURNING id::text`,
		tenantID, name, headAgentID).Scan(&id)
	return id, err
}

// ListDepartments returns all departments for a tenant.
func (s *Store) ListDepartments(ctx context.Context, tenantID string) ([]Department, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id::text, tenant_id::text, name, COALESCE(head_agent_id::text,''), COALESCE(parent_department_id::text,'')
		 FROM departments WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Department
	for rows.Next() {
		var d Department
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Name, &d.HeadAgentID, &d.ParentDepartmentID); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CreateProject inserts a project under an optional department.
func (s *Store) CreateProject(ctx context.Context, tenantID, name, departmentID string) (string, error) {
	var id string
	err := s.db.QueryRow(ctx,
		`INSERT INTO projects (tenant_id, name, department_id)
		 VALUES ($1, $2, NULLIF($3,'')::uuid) RETURNING id::text`,
		tenantID, name, departmentID).Scan(&id)
	return id, err
}

// ListProjects returns all projects for a tenant.
func (s *Store) ListProjects(ctx context.Context, tenantID string) ([]Project, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id::text, tenant_id::text, name, COALESCE(department_id::text,''), status
		 FROM projects WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.DepartmentID, &p.Status); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// BudgetScope identifies a node in the hierarchy a cap applies to.
type BudgetScope struct {
	Scope          string  `json:"scope"`           // "tenant" | "department" | "project" | "agent"
	ScopeID        string  `json:"scope_id"`        // department/project/agent id; "" for tenant
	MonthlyUSD     float64 `json:"monthly_usd"`
	AllocationMode string  `json:"allocation_mode"` // "carved" | "fresh"
	ParentScope    string  `json:"parent_scope"`    // for carved: the scope this draws from
	ParentScopeID  string  `json:"parent_scope_id"`
	FundingMode    string  `json:"funding_mode"`    // tenant only: prepaid_fixed | monthly_recurring
	LifetimeUSD    float64 `json:"lifetime_usd"`    // tenant prepaid_fixed cap
}

// scopeColumn maps a scope to its id column on gateway_budgets ("" for tenant).
func scopeColumn(scope string) string {
	switch scope {
	case "agent":
		return "agent_id"
	case "department":
		return "department_id"
	case "project":
		return "project_id"
	default:
		return ""
	}
}

// SetBudget upserts a cap for a scope. For carved allocations it validates that
// the sum of carved sibling caps under the same parent plus this cap does not
// exceed the parent cap. Returns ErrOverAllocated on violation.
func (s *Store) SetBudget(ctx context.Context, tenantID string, b BudgetScope) error {
	mode := b.AllocationMode
	if mode == "" {
		mode = "carved"
	}
	if mode == "carved" && b.Scope != "tenant" && b.ParentScope == "" {
		return fmt.Errorf("carved allocation requires a parent_scope")
	}
	if mode == "carved" && b.ParentScope != "" {
		parentCap, _ := s.scopeCapUSD(ctx, tenantID, b.ParentScope, b.ParentScopeID)
		existing, _ := s.carvedChildrenSumUSD(ctx, tenantID, b.ParentScope, b.ParentScopeID, b.Scope, b.ScopeID)
		if err := validateCarved(parentCap, existing, b.MonthlyUSD); err != nil {
			return err
		}
	}
	col := scopeColumn(b.Scope)
	if col == "" {
		// tenant-level row
		var existing string
		_ = s.db.QueryRow(ctx,
			`SELECT id::text FROM gateway_budgets WHERE tenant_id = $1 AND scope = 'tenant' LIMIT 1`,
			tenantID).Scan(&existing)
		if existing != "" {
			_, err := s.db.Exec(ctx,
				`UPDATE gateway_budgets
				 SET monthly_usd = $2, allocation_mode = $3, funding_mode = NULLIF($4,''), lifetime_usd = $5
				 WHERE id = $1::uuid`,
				existing, b.MonthlyUSD, mode, b.FundingMode, b.LifetimeUSD)
			return err
		}
		_, err := s.db.Exec(ctx,
			`INSERT INTO gateway_budgets (tenant_id, scope, monthly_usd, allocation_mode, funding_mode, lifetime_usd)
			 VALUES ($1, 'tenant', $2, $3, NULLIF($4,''), $5)`,
			tenantID, b.MonthlyUSD, mode, b.FundingMode, b.LifetimeUSD)
		return err
	}
	// department/project/agent row — find existing by (tenant, scope, id col)
	var existing string
	_ = s.db.QueryRow(ctx, fmt.Sprintf(
		`SELECT id::text FROM gateway_budgets WHERE tenant_id = $1 AND scope = $2 AND %s = $3::uuid LIMIT 1`, col),
		tenantID, b.Scope, b.ScopeID).Scan(&existing)
	if existing != "" {
		_, err := s.db.Exec(ctx,
			`UPDATE gateway_budgets SET monthly_usd = $2, allocation_mode = $3, parent_scope = NULLIF($4,''), parent_scope_id = NULLIF($5,'')::uuid WHERE id = $1::uuid`,
			existing, b.MonthlyUSD, mode, b.ParentScope, b.ParentScopeID)
		return err
	}
	_, err := s.db.Exec(ctx, fmt.Sprintf(
		`INSERT INTO gateway_budgets (tenant_id, scope, %s, monthly_usd, allocation_mode, parent_scope, parent_scope_id)
		 VALUES ($1, $2, $3::uuid, $4, $5, NULLIF($6,''), NULLIF($7,'')::uuid)`, col),
		tenantID, b.Scope, b.ScopeID, b.MonthlyUSD, mode, b.ParentScope, b.ParentScopeID)
	return err
}

// scopeCapUSD returns the monthly cap (USD) for a scope node, 0 if none.
func (s *Store) scopeCapUSD(ctx context.Context, tenantID, scope, scopeID string) (float64, error) {
	col := scopeColumn(scope)
	var cap *float64
	var err error
	if col == "" {
		err = s.db.QueryRow(ctx,
			`SELECT monthly_usd FROM gateway_budgets WHERE tenant_id=$1 AND scope='tenant' LIMIT 1`,
			tenantID).Scan(&cap)
	} else {
		err = s.db.QueryRow(ctx, fmt.Sprintf(
			`SELECT monthly_usd FROM gateway_budgets WHERE tenant_id=$1 AND scope=$2 AND %s=$3::uuid LIMIT 1`, col),
			tenantID, scope, scopeID).Scan(&cap)
	}
	if err != nil || cap == nil {
		return 0, nil
	}
	return *cap, nil
}

// carvedChildrenSumUSD sums existing carved children caps under a parent,
// EXCLUDING the row being updated (so re-setting a child isn't double-counted).
func (s *Store) carvedChildrenSumUSD(ctx context.Context, tenantID, parentScope, parentScopeID, selfScope, selfScopeID string) (float64, error) {
	var sum *float64
	err := s.db.QueryRow(ctx, `
		SELECT SUM(monthly_usd) FROM gateway_budgets
		WHERE tenant_id = $1 AND allocation_mode = 'carved'
		  AND parent_scope = $2 AND parent_scope_id = NULLIF($3,'')::uuid
		  AND NOT (scope = $4 AND COALESCE(department_id::text, project_id::text, agent_id::text, '') = $5)
	`, tenantID, parentScope, parentScopeID, selfScope, selfScopeID).Scan(&sum)
	if err != nil || sum == nil {
		return 0, nil
	}
	return *sum, nil
}

// EffectiveAvailableResult is the reconciliation of the declared overall
// budget against what the connected provider keys actually allow. All money
// is integer µUSD.
type EffectiveAvailableResult struct {
	DeclaredRemainingUUSD int64    `json:"declared_remaining_uusd"`
	ProviderRemainingUUSD int64    `json:"provider_remaining_uusd"`
	EffectiveUUSD         int64    `json:"effective_uusd"`
	Binding               string   `json:"binding"` // "declared" | "providers"
	Warnings              []string `json:"warnings"`
}

// reconcile computes effective-available = min(declared, providers) and the
// binding constraint. On ties, declared binds. Warns when the declared budget
// exceeds the provider allowance (the keys can't actually fund it).
func reconcile(declaredRemainingUUSD, providerRemainingUUSD int64) EffectiveAvailableResult {
	res := EffectiveAvailableResult{
		DeclaredRemainingUUSD: declaredRemainingUUSD,
		ProviderRemainingUUSD: providerRemainingUUSD,
		Warnings:              []string{},
	}
	if providerRemainingUUSD < declaredRemainingUUSD {
		res.EffectiveUUSD = providerRemainingUUSD
		res.Binding = "providers"
		res.Warnings = append(res.Warnings,
			"declared budget exceeds the available provider-key allowance; effective budget is limited by your provider keys")
	} else {
		res.EffectiveUUSD = declaredRemainingUUSD
		res.Binding = "declared"
	}
	return res
}

// EffectiveAvailable reconciles the declared overall budget against the sum of
// what connected provider keys still allow. Returns µUSD figures + the binding
// constraint. Provider remaining: prepaid → balance−spent; postpaid →
// budget_usd_monthly−spent (when a monthly cap is set); quota/free → no $
// ceiling (excluded). It informs; it does not itself block.
func (s *Store) EffectiveAvailable(ctx context.Context, tenantID string) (EffectiveAvailableResult, error) {
	const uusdPerUSD = 1_000_000.0

	var monthlyUSD, lifetimeUSD *float64
	var fundingMode *string
	_ = s.db.QueryRow(ctx, `
		SELECT monthly_usd, lifetime_usd, funding_mode
		FROM gateway_budgets WHERE tenant_id = $1 AND scope = 'tenant' LIMIT 1
	`, tenantID).Scan(&monthlyUSD, &lifetimeUSD, &fundingMode)

	mode := ""
	if fundingMode != nil {
		mode = *fundingMode
	}
	// Month-to-date and all-time spend in one read; pick per funding mode below.
	var mtd, allTime *int64
	_ = s.db.QueryRow(ctx, `
		SELECT
		  SUM(cost_total_uusd) FILTER (WHERE period >= date_trunc('month', CURRENT_DATE)),
		  SUM(cost_total_uusd)
		FROM gateway_spend WHERE tenant_id = $1
	`, tenantID).Scan(&mtd, &allTime)

	var capUSD float64
	var spentUUSD int64
	if mode == "prepaid_fixed" {
		if lifetimeUSD != nil {
			capUSD = *lifetimeUSD
		}
		if allTime != nil {
			spentUUSD = *allTime
		}
	} else {
		if monthlyUSD != nil {
			capUSD = *monthlyUSD
		}
		if mtd != nil {
			spentUUSD = *mtd
		}
	}
	declaredRemaining := int64(capUSD*uusdPerUSD) - spentUUSD
	if declaredRemaining < 0 {
		declaredRemaining = 0
	}

	var providerRemainingUUSD int64
	rows, err := s.db.Query(ctx, `
		SELECT budget_type, balance_usd, budget_usd_monthly, spent_usd_month
		FROM provider_keys
		WHERE tenant_id = $1 AND status = 'verified'
	`, tenantID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var btype string
			var balance, monthlyCap *float64
			var spent float64
			if rows.Scan(&btype, &balance, &monthlyCap, &spent) != nil {
				continue
			}
			switch btype {
			case "prepaid":
				if balance != nil {
					if rem := *balance - spent; rem > 0 {
						providerRemainingUUSD += int64(rem * uusdPerUSD)
					}
				}
			case "postpaid":
				if monthlyCap != nil {
					if rem := *monthlyCap - spent; rem > 0 {
						providerRemainingUUSD += int64(rem * uusdPerUSD)
					}
				}
			}
		}
	}

	return reconcile(declaredRemaining, providerRemainingUUSD), nil
}
