// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package agent

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	OrgLevelUser     = 1
	OrgLevelCSuite   = 2
	OrgLevelWorker   = 3
	OrgLevelSubagent = 4
)

type OrgNode struct {
	TenantID      uuid.UUID   `json:"tenant_id"`
	AgentID       uuid.UUID   `json:"agent_id"`
	ReportsTo     *uuid.UUID  `json:"reports_to"`
	OrgLevel      int         `json:"org_level"`
	OrgRole       string      `json:"org_role"`
	CanDelegateTo []uuid.UUID `json:"can_delegate_to"`
	MaxBudgetUSD  float64     `json:"max_budget_usd"`
}

type OrgChartStore struct {
	db *pgxpool.Pool
}

func NewOrgChartStore(db *pgxpool.Pool) *OrgChartStore {
	return &OrgChartStore{db: db}
}

func (s *OrgChartStore) GetNode(ctx context.Context, tenantID, agentID uuid.UUID) (*OrgNode, error) {
	if s.db == nil {
		return nil, nil
	}
	var node OrgNode
	var reportsTo *uuid.UUID
	var canDelegate []uuid.UUID
	var maxBudget *float64

	err := s.db.QueryRow(ctx, `
		SELECT tenant_id, agent_id, reports_to, org_level, org_role, can_delegate_to, max_budget_usd
		FROM org_hierarchy
		WHERE tenant_id = $1 AND agent_id = $2
	`, tenantID, agentID).Scan(
		&node.TenantID, &node.AgentID, &reportsTo,
		&node.OrgLevel, &node.OrgRole, &canDelegate, &maxBudget,
	)
	if err != nil {
		return nil, nil
	}
	node.ReportsTo = reportsTo
	node.CanDelegateTo = canDelegate
	if maxBudget != nil {
		node.MaxBudgetUSD = *maxBudget
	}
	return &node, nil
}

func (s *OrgChartStore) GetTree(ctx context.Context, tenantID uuid.UUID) ([]OrgNode, error) {
	if s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT tenant_id, agent_id, reports_to, org_level, org_role, can_delegate_to, max_budget_usd
		FROM org_hierarchy
		WHERE tenant_id = $1
		ORDER BY org_level ASC, org_role ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []OrgNode
	for rows.Next() {
		var n OrgNode
		var reportsTo *uuid.UUID
		var canDelegate []uuid.UUID
		var maxBudget *float64
		if err := rows.Scan(&n.TenantID, &n.AgentID, &reportsTo, &n.OrgLevel, &n.OrgRole, &canDelegate, &maxBudget); err != nil {
			continue
		}
		n.ReportsTo = reportsTo
		n.CanDelegateTo = canDelegate
		if maxBudget != nil {
			n.MaxBudgetUSD = *maxBudget
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (s *OrgChartStore) Upsert(ctx context.Context, node OrgNode) error {
	if s.db == nil {
		return fmt.Errorf("database not available")
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO org_hierarchy (tenant_id, agent_id, reports_to, org_level, org_role, can_delegate_to, max_budget_usd)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id, agent_id) DO UPDATE SET
			reports_to = EXCLUDED.reports_to,
			org_level = EXCLUDED.org_level,
			org_role = EXCLUDED.org_role,
			can_delegate_to = EXCLUDED.can_delegate_to,
			max_budget_usd = EXCLUDED.max_budget_usd
	`, node.TenantID, node.AgentID, node.ReportsTo, node.OrgLevel, node.OrgRole, node.CanDelegateTo, node.MaxBudgetUSD)
	return err
}

// SyncFromAgent writes (or updates) the org_hierarchy overlay row for a single agent
// using the values already stored on the agents table. Call this after every hire or
// manager-reassignment so the overlay never drifts from the live reporting tree.
//
// orgLevelStr is the TEXT value stored in agents.org_level ("l1", "l2", "l3", "").
// managerID may be nil for top-of-tree agents (no reports_to).
func (s *OrgChartStore) SyncFromAgent(ctx context.Context, tenantID, agentID uuid.UUID, managerID *uuid.UUID, orgLevelStr, orgRole string, budgetUSD float64) error {
	orgLevel := 0
	switch orgLevelStr {
	case "l1":
		orgLevel = 1
	case "l2":
		orgLevel = 2
	case "l3":
		orgLevel = 3
	}
	return s.Upsert(ctx, OrgNode{
		TenantID:      tenantID,
		AgentID:       agentID,
		ReportsTo:     managerID,
		OrgLevel:      orgLevel,
		OrgRole:       orgRole,
		CanDelegateTo: []uuid.UUID{},
		MaxBudgetUSD:  budgetUSD,
	})
}

// ValidateDelegation checks that delegator can delegate to delegatee per org hierarchy rules.
func (s *OrgChartStore) ValidateDelegation(ctx context.Context, tenantID, delegatorID, delegateeID uuid.UUID) error {
	if s.db == nil {
		return nil
	}

	delegator, _ := s.GetNode(ctx, tenantID, delegatorID)
	delegatee, _ := s.GetNode(ctx, tenantID, delegateeID)

	if delegator == nil || delegatee == nil {
		return nil
	}

	if delegator.OrgLevel >= delegatee.OrgLevel {
		return fmt.Errorf("cannot delegate upward: level %d cannot delegate to level %d", delegator.OrgLevel, delegatee.OrgLevel)
	}

	if len(delegator.CanDelegateTo) > 0 {
		allowed := false
		for _, id := range delegator.CanDelegateTo {
			if id == delegateeID {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("agent %s is not in delegator's allowed delegation targets", delegateeID)
		}
	}

	return nil
}

func (s *OrgChartStore) GetDirectReports(ctx context.Context, tenantID, managerID uuid.UUID) ([]OrgNode, error) {
	if s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT tenant_id, agent_id, reports_to, org_level, org_role, can_delegate_to, max_budget_usd
		FROM org_hierarchy
		WHERE tenant_id = $1 AND reports_to = $2
		ORDER BY org_role ASC
	`, tenantID, managerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []OrgNode
	for rows.Next() {
		var n OrgNode
		var reportsTo *uuid.UUID
		var canDelegate []uuid.UUID
		var maxBudget *float64
		if err := rows.Scan(&n.TenantID, &n.AgentID, &reportsTo, &n.OrgLevel, &n.OrgRole, &canDelegate, &maxBudget); err != nil {
			continue
		}
		n.ReportsTo = reportsTo
		n.CanDelegateTo = canDelegate
		if maxBudget != nil {
			n.MaxBudgetUSD = *maxBudget
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}
