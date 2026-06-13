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

// RecordAction logs that an agent performed a governed action so that a later
// CheckViolation call can detect a conflicting action by the same agent.
// It reuses policy_events with trigger_event='governed_action' and stores the
// action name in context->>'governed_action'. Failures are silently ignored
// (best-effort audit trail).
func (s *SoDStore) RecordAction(ctx context.Context, tenantID, agentID, action string) {
	if action == "" {
		return
	}
	_, _ = s.db.Exec(ctx, `INSERT INTO policy_events
		(tenant_id, policy_name, agent_id, trigger_event, action_taken, context)
		VALUES ($1, 'sod_action_log', $2, 'governed_action', 'log',
		        jsonb_build_object('governed_action', $3::text))`,
		tenantID, agentID, action)
}

// CheckViolation returns true if the same agent performed both conflicting
// governed actions within the last 24 hours (same_task scope).
// action must be a governed-action vocabulary word (e.g. "write_code"), not a
// raw tool name. RecordAction must have been called with the complementary
// action for a violation to be detected.
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
		// Check if the same agent performed the conflicting governed action in last 24h.
		// RecordAction stores the action in context->>'governed_action'.
		var cnt int
		s.db.QueryRow(ctx, `SELECT COUNT(*) FROM policy_events
			WHERE tenant_id = $1 AND agent_id = $2
			  AND context->>'governed_action' = $3
			  AND created_at > $4`,
			tenantID, agentID, conflicting, time.Now().Add(-24*time.Hour)).Scan(&cnt)
		if cnt > 0 {
			return true, name
		}
	}
	return false, ""
}
