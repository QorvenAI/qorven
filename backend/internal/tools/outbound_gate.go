// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboundGate checks approval settings before executing outbound actions.
// Outbound tools: email_send, social_post, webhook_send, sms_send
type OutboundGate struct {
	pool       *pgxpool.Pool
	agentStore interface{ Get(ctx context.Context, id string) (interface{ GetOutboundApproval() string }, error) }
}

// OutboundAction represents a queued outbound action awaiting approval.
type OutboundAction struct {
	ID           string          `json:"id"`
	AgentID      string          `json:"agent_id"`
	ActionType   string          `json:"action_type"`
	Payload      json.RawMessage `json:"payload"`
	Status       string          `json:"status"` // pending, approved, rejected, expired
	ApprovalMode string          `json:"approval_mode"`
	RequestedAt  time.Time       `json:"requested_at"`
	ReviewedBy   string          `json:"reviewed_by,omitempty"`
	ReviewedAt   *time.Time      `json:"reviewed_at,omitempty"`
	ReviewNotes  string          `json:"review_notes,omitempty"`
	SessionID    string          `json:"session_id,omitempty"`
	ExpiresAt    time.Time       `json:"expires_at"`
}

// IsOutboundTool returns true if the tool sends data outside the system.
func IsOutboundTool(toolName string) bool {
	switch toolName {
	case "email_send", "social_post", "webhook_send", "sms_send",
		"slack_send", "telegram_send", "discord_send", "whatsapp_send":
		return true
	}
	return false
}

// CheckApproval checks if an outbound action needs approval and queues it if so.
// Returns (proceed bool, queuedID string, error).
func CheckApproval(ctx context.Context, pool *pgxpool.Pool, agentID, toolName, sessionID string, args map[string]any) (bool, string, error) {
	if pool == nil {
		return true, "", nil // no DB = no approval
	}

	// Get agent's approval setting
	var approvalMode string
	err := pool.QueryRow(ctx,
		`SELECT COALESCE(outbound_approval, 'supervisor') FROM agents WHERE id = $1`, agentID,
	).Scan(&approvalMode)
	if err != nil {
		slog.Warn("outbound.approval.agent_not_found", "agent", agentID, "error", err)
		return true, "", nil // can't find agent = allow (fail open for now)
	}

	if approvalMode == "none" || approvalMode == "auto" {
		return true, "", nil // no approval needed — proceed immediately
	}

	// Queue the action
	payload, _ := json.Marshal(args)
	var queueID string
	err = pool.QueryRow(ctx,
		`INSERT INTO outbound_queue (agent_id, action_type, payload, approval_mode, session_id)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		agentID, toolName, payload, approvalMode, sessionID,
	).Scan(&queueID)
	if err != nil {
		slog.Error("outbound.queue.insert_failed", "error", err)
		return true, "", err // fail open
	}

	if OnApprovalQueued != nil { OnApprovalQueued(queueID, agentID, toolName, args) }
	slog.Info("outbound.queued", "id", queueID, "agent", agentID[:8], "tool", toolName, "mode", approvalMode)
	return false, queueID, nil
}

// ApproveAction approves a queued outbound action.
func ApproveAction(ctx context.Context, pool *pgxpool.Pool, queueID, reviewedBy, notes string) error {
	_, err := pool.Exec(ctx,
		`UPDATE outbound_queue SET status = 'approved', reviewed_by = $1, reviewed_at = NOW(), review_notes = $2 WHERE id = $3 AND status = 'pending'`,
		reviewedBy, notes, queueID)
	if err != nil {
		return err
	}
	slog.Info("outbound.approved", "id", queueID, "by", reviewedBy)
	return nil
}

// RejectAction rejects a queued outbound action.
func RejectAction(ctx context.Context, pool *pgxpool.Pool, queueID, reviewedBy, notes string) error {
	_, err := pool.Exec(ctx,
		`UPDATE outbound_queue SET status = 'rejected', reviewed_by = $1, reviewed_at = NOW(), review_notes = $2 WHERE id = $3 AND status = 'pending'`,
		reviewedBy, notes, queueID)
	slog.Info("outbound.rejected", "id", queueID, "by", reviewedBy)
	return err
}

// ListPending returns pending outbound actions for a tenant.
func ListPending(ctx context.Context, pool *pgxpool.Pool) ([]OutboundAction, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, agent_id, action_type, payload, status, approval_mode, requested_at, session_id, expires_at
		 FROM outbound_queue WHERE status = 'pending' AND expires_at > NOW()
		 ORDER BY requested_at DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var actions []OutboundAction
	for rows.Next() {
		var a OutboundAction
		rows.Scan(&a.ID, &a.AgentID, &a.ActionType, &a.Payload, &a.Status, &a.ApprovalMode, &a.RequestedAt, &a.SessionID, &a.ExpiresAt)
		actions = append(actions, a)
	}
	return actions, nil
}

// OnApprovalQueued is called when an outbound action is queued for approval.
// Set this to broadcast notifications to connected clients.
var OnApprovalQueued func(queueID, agentID, toolName string, args map[string]any)
