// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package governance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ApprovalRule struct {
	ID                string  `json:"id"`
	TenantID          string  `json:"tenant_id"`
	ActionType        string  `json:"action_type"`
	ThresholdUSD      float64 `json:"threshold_usd"`
	ApproverRole      string  `json:"approver_role"`
	ApproverLevel     int     `json:"approver_level"`
	RequiresHuman     bool    `json:"requires_human"`
	AutoApproveBelow  float64 `json:"auto_approve_below"`
	EscalationTimeout int     `json:"escalation_timeout_min"`
	EscalationTo      string  `json:"escalation_to"`
	Enabled           bool    `json:"enabled"`
	Priority          int     `json:"priority"`
}

type ApprovalRequest struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	ActionType     string     `json:"action_type"`
	RequestorID    string     `json:"requestor_id"`
	RequestorKey   string     `json:"requestor_key"`
	ApproverRole   string     `json:"approver_role"`
	ApproverID     string     `json:"approver_id,omitempty"`
	MatrixRuleID   string     `json:"matrix_rule_id,omitempty"`
	Context        map[string]any `json:"context"`
	Status         string     `json:"status"`
	DecisionBy     string     `json:"decision_by,omitempty"`
	DecisionAt     *time.Time `json:"decision_at,omitempty"`
	DecisionReason string     `json:"decision_reason,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type ApprovalStore struct {
	db *pgxpool.Pool
}

func NewApprovalStore(db *pgxpool.Pool) *ApprovalStore {
	return &ApprovalStore{db: db}
}

// CheckRequiresApproval evaluates the approval matrix for a given action.
// Returns the matching rule if approval is needed, nil if auto-approved.
func (s *ApprovalStore) CheckRequiresApproval(ctx context.Context, tenantID, actionType string, costUSD float64) (*ApprovalRule, error) {
	var r ApprovalRule
	err := s.db.QueryRow(ctx, `
		SELECT id, tenant_id, action_type, COALESCE(threshold_usd,0), approver_role, approver_level,
		       requires_human, COALESCE(auto_approve_below,0), COALESCE(escalation_timeout_min,60),
		       COALESCE(escalation_to,''), enabled, priority
		FROM approval_matrix
		WHERE tenant_id = $1 AND action_type = $2 AND enabled = true
		  AND (threshold_usd = 0 OR threshold_usd <= $3)
		ORDER BY priority ASC
		LIMIT 1
	`, tenantID, actionType, costUSD).Scan(&r.ID, &r.TenantID, &r.ActionType, &r.ThresholdUSD,
		&r.ApproverRole, &r.ApproverLevel, &r.RequiresHuman, &r.AutoApproveBelow,
		&r.EscalationTimeout, &r.EscalationTo, &r.Enabled, &r.Priority)
	if err != nil {
		return nil, nil // no rule = no approval needed
	}
	if r.AutoApproveBelow > 0 && costUSD < r.AutoApproveBelow {
		return nil, nil // below auto-approve threshold
	}
	return &r, nil
}

func (s *ApprovalStore) CreateRequest(ctx context.Context, req ApprovalRequest) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO approval_requests (tenant_id, action_type, requestor_id, requestor_key, approver_role, approver_id, matrix_rule_id, context, status, expires_at)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,NULLIF($7,'')::uuid,$8,$9,$10)
	`, req.TenantID, req.ActionType, req.RequestorID, req.RequestorKey, req.ApproverRole,
		req.ApproverID, req.MatrixRuleID, req.Context, req.Status, req.ExpiresAt)
	return err
}

func (s *ApprovalStore) Decide(ctx context.Context, tenantID, requestID, decisionBy, status, reason string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE approval_requests SET status=$1, decision_by=$2::uuid, decision_at=now(), decision_reason=$3
		WHERE tenant_id=$4 AND id=$5 AND status='pending'
	`, status, decisionBy, reason, tenantID, requestID)
	return err
}

func (s *ApprovalStore) ListPending(ctx context.Context, tenantID string) ([]ApprovalRequest, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, tenant_id, action_type, requestor_id, COALESCE(requestor_key,''), approver_role,
		       COALESCE(approver_id::text,''), COALESCE(matrix_rule_id::text,''), COALESCE(context,'{}'),
		       status, COALESCE(decision_by::text,''), decision_at, COALESCE(decision_reason,''), expires_at, created_at
		FROM approval_requests WHERE tenant_id = $1 AND status = 'pending'
		ORDER BY created_at ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ApprovalRequest
	for rows.Next() {
		var r ApprovalRequest
		if err := rows.Scan(&r.ID, &r.TenantID, &r.ActionType, &r.RequestorID, &r.RequestorKey,
			&r.ApproverRole, &r.ApproverID, &r.MatrixRuleID, &r.Context, &r.Status,
			&r.DecisionBy, &r.DecisionAt, &r.DecisionReason, &r.ExpiresAt, &r.CreatedAt); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// Get returns a single ApprovalRequest by tenant and id. Returns an error if not found.
func (s *ApprovalStore) Get(ctx context.Context, tenantID, id string) (ApprovalRequest, error) {
	var r ApprovalRequest
	err := s.db.QueryRow(ctx, `
		SELECT id, tenant_id, action_type, requestor_id, COALESCE(requestor_key,''), approver_role,
		       COALESCE(approver_id::text,''), COALESCE(matrix_rule_id::text,''), COALESCE(context,'{}'),
		       status, COALESCE(decision_by::text,''), decision_at, COALESCE(decision_reason,''), expires_at, created_at
		FROM approval_requests WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(&r.ID, &r.TenantID, &r.ActionType, &r.RequestorID, &r.RequestorKey,
		&r.ApproverRole, &r.ApproverID, &r.MatrixRuleID, &r.Context, &r.Status,
		&r.DecisionBy, &r.DecisionAt, &r.DecisionReason, &r.ExpiresAt, &r.CreatedAt)
	if err != nil {
		return ApprovalRequest{}, err
	}
	return r, nil
}

func (s *ApprovalStore) ListRules(ctx context.Context, tenantID string) ([]ApprovalRule, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, tenant_id, action_type, COALESCE(threshold_usd,0), approver_role, approver_level,
		       requires_human, COALESCE(auto_approve_below,0), COALESCE(escalation_timeout_min,60),
		       COALESCE(escalation_to,''), enabled, priority
		FROM approval_matrix WHERE tenant_id = $1 ORDER BY priority ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ApprovalRule
	for rows.Next() {
		var r ApprovalRule
		if err := rows.Scan(&r.ID, &r.TenantID, &r.ActionType, &r.ThresholdUSD, &r.ApproverRole,
			&r.ApproverLevel, &r.RequiresHuman, &r.AutoApproveBelow, &r.EscalationTimeout,
			&r.EscalationTo, &r.Enabled, &r.Priority); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}
