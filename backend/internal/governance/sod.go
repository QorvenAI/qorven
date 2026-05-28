// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package governance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SoDRule struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	ActionA  string `json:"action_a"`
	ActionB  string `json:"action_b"`
	Scope    string `json:"scope"`
	Enabled  bool   `json:"enabled"`
}

type SoDStore struct {
	db *pgxpool.Pool
}

func NewSoDStore(db *pgxpool.Pool) *SoDStore {
	return &SoDStore{db: db}
}

func (s *SoDStore) ListRules(ctx context.Context, tenantID string) ([]SoDRule, error) {
	rows, err := s.db.Query(ctx, `SELECT id, tenant_id, name, action_a, action_b, scope, enabled FROM sod_rules WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SoDRule
	for rows.Next() {
		var r SoDRule
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.ActionA, &r.ActionB, &r.Scope, &r.Enabled); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// CheckViolation returns true if the same agent performed both conflicting actions within the scope.
func (s *SoDStore) CheckViolation(ctx context.Context, tenantID, agentID, action string) (bool, string) {
	rows, err := s.db.Query(ctx, `SELECT action_a, action_b, name FROM sod_rules WHERE tenant_id = $1 AND enabled = true AND (action_a = $2 OR action_b = $2)`, tenantID, action)
	if err != nil {
		return false, ""
	}
	defer rows.Close()

	for rows.Next() {
		var actionA, actionB, name string
		if err := rows.Scan(&actionA, &actionB, &name); err != nil {
			continue
		}
		conflicting := actionA
		if conflicting == action {
			conflicting = actionB
		}
		// Check if the same agent performed the conflicting action in last 24h (same_task scope)
		var cnt int
		s.db.QueryRow(ctx, `SELECT COUNT(*) FROM policy_events WHERE tenant_id = $1 AND agent_id = $2 AND trigger_event = $3 AND created_at > $4`,
			tenantID, agentID, conflicting, time.Now().Add(-24*time.Hour)).Scan(&cnt)
		if cnt > 0 {
			return true, name
		}
	}
	return false, ""
}
