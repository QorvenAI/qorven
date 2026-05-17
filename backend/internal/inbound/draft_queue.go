package inbound

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DraftReply is a pending reply waiting for user approval.
type DraftReply struct {
	ID              string     `json:"id"`
	TenantID        uuid.UUID  `json:"tenant_id"`
	AgentID         string     `json:"agent_id"`
	SessionID       string     `json:"session_id,omitempty"`
	SenderID        string     `json:"sender_id"`
	SenderName      string     `json:"sender_name,omitempty"`
	Channel         string     `json:"channel"`
	OriginalMessage string     `json:"original_message"`
	HistorySummary  string     `json:"history_summary,omitempty"`
	DraftContent    string     `json:"draft_content"`
	Status          string     `json:"status"`
	ApprovalMsgID   string     `json:"approval_msg_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	DecidedAt       *time.Time `json:"decided_at,omitempty"`
	DecidedBy       string     `json:"decided_by,omitempty"`
}

// DraftQueue persists and manages draft reply state.
type DraftQueue struct {
	pool *pgxpool.Pool
}

// Save persists a draft and returns its ID.
func (q *DraftQueue) Save(ctx context.Context, d DraftReply) string {
	if q.pool == nil {
		return ""
	}
	var id string
	err := q.pool.QueryRow(ctx,
		`INSERT INTO draft_replies
		    (tenant_id, agent_id, sender_id, sender_name, channel,
		     original_message, history_summary, draft_content, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 RETURNING id`,
		d.TenantID, d.AgentID, d.SenderID, d.SenderName, d.Channel,
		d.OriginalMessage, d.HistorySummary, d.DraftContent, d.Status,
	).Scan(&id)
	if err != nil {
		slog.Warn("inbound.draft.save_failed", "err", err)
	}
	return id
}

// List returns pending drafts for a tenant.
func (q *DraftQueue) List(ctx context.Context, tenantID uuid.UUID) ([]DraftReply, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, agent_id, sender_id, sender_name, channel,
		        original_message, history_summary, draft_content, status, created_at
		 FROM draft_replies
		 WHERE tenant_id = $1 AND status = 'pending'
		 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var drafts []DraftReply
	for rows.Next() {
		var d DraftReply
		if err := rows.Scan(&d.ID, &d.AgentID, &d.SenderID, &d.SenderName, &d.Channel,
			&d.OriginalMessage, &d.HistorySummary, &d.DraftContent, &d.Status, &d.CreatedAt); err != nil {
			continue
		}
		drafts = append(drafts, d)
	}
	return drafts, nil
}

// Transition changes a draft's status, scoped to tenant to prevent cross-tenant mutation.
func (q *DraftQueue) Transition(ctx context.Context, tenantID uuid.UUID, id, status, decidedBy string) error {
	now := time.Now()
	_, err := q.pool.Exec(ctx,
		`UPDATE draft_replies SET status=$1, decided_at=$2, decided_by=$3 WHERE id=$4 AND tenant_id=$5`,
		status, now, decidedBy, id, tenantID)
	return err
}

// UpdateContent replaces the draft content, scoped to tenant.
func (q *DraftQueue) UpdateContent(ctx context.Context, tenantID uuid.UUID, id, newContent string) error {
	_, err := q.pool.Exec(ctx,
		`UPDATE draft_replies SET draft_content=$1, status='pending' WHERE id=$2 AND tenant_id=$3`,
		newContent, id, tenantID)
	return err
}
