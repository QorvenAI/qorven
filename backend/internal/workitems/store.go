// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package workitems

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WorkItem is one durable unit of delegated work.
type WorkItem struct {
	ID            string
	TenantID      string
	Title         string
	Origin        string
	OwnerAgentID  string
	RequestedBy   string
	Status        string
	BlockedOnKind string
	BlockedOnID   string
	ParentID      string // "" when none
	BudgetPlanID  string // "" when none
	UpdatedAt     time.Time
}

// Event is one audit record for a work item.
type Event struct {
	EventType  string    `json:"event_type"`
	ActorID    string    `json:"actor_id"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Detail     string    `json:"detail"`
	CreatedAt  time.Time `json:"created_at"`
}

// Store persists work items and their events.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// nullable returns nil for "" so a TEXT/UUID column is written as NULL.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Create inserts a new work item at status 'open' and logs a 'created' event.
func (s *Store) Create(ctx context.Context, w WorkItem) (string, error) {
	if w.Status == "" {
		w.Status = StatusOpen
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO work_items (tenant_id, title, origin, owner_agent_id, requested_by, status, parent_id, budget_plan_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		w.TenantID, w.Title, w.Origin, w.OwnerAgentID, w.RequestedBy, w.Status, nullable(w.ParentID), nullable(w.BudgetPlanID),
	).Scan(&id)
	if err != nil {
		return "", err
	}
	s.logEvent(ctx, id, Event{EventType: "created", ActorID: w.RequestedBy, ToStatus: w.Status})
	return id, nil
}

// Get returns a work item by id.
func (s *Store) Get(ctx context.Context, id string) (*WorkItem, error) {
	var w WorkItem
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, title, origin, owner_agent_id, requested_by, status,
		        blocked_on_kind, blocked_on_id, COALESCE(parent_id::text,''), COALESCE(budget_plan_id::text,'')
		 FROM work_items WHERE id=$1`, id,
	).Scan(&w.ID, &w.TenantID, &w.Title, &w.Origin, &w.OwnerAgentID, &w.RequestedBy, &w.Status,
		&w.BlockedOnKind, &w.BlockedOnID, &w.ParentID, &w.BudgetPlanID)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// Transition moves a work item to a new status if the move is legal, logging the event.
func (s *Store) Transition(ctx context.Context, id, to, actorID, detail string) error {
	cur, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !CanTransition(cur.Status, to) {
		return fmt.Errorf("illegal transition %s→%s", cur.Status, to)
	}
	closedClause := ""
	if to == StatusDone || to == StatusCancelled {
		closedClause = ", closed_at=now()"
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE work_items SET status=$2, updated_at=now()`+closedClause+` WHERE id=$1 AND status=$3`, id, to, cur.Status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("work item %s was concurrently modified (no longer in %s)", id, cur.Status)
	}
	s.logEvent(ctx, id, Event{EventType: "status_changed", ActorID: actorID, FromStatus: cur.Status, ToStatus: to, Detail: detail})
	return nil
}

// SetBlockedOn marks the item blocked on an approval or another work item.
func (s *Store) SetBlockedOn(ctx context.Context, id, kind, refID, actorID string) error {
	cur, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if cur.Status == StatusDone || cur.Status == StatusCancelled {
		return fmt.Errorf("cannot block a %s work item", cur.Status)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE work_items SET status=$2, blocked_on_kind=$3, blocked_on_id=$4, updated_at=now() WHERE id=$1`,
		id, StatusBlocked, kind, refID); err != nil {
		return err
	}
	s.logEvent(ctx, id, Event{EventType: "blocked", ActorID: actorID, FromStatus: cur.Status, ToStatus: StatusBlocked, Detail: kind + ":" + refID})
	return nil
}

// Unblock clears the block and returns a blocked item to in_progress.
func (s *Store) Unblock(ctx context.Context, id, actorID string) error {
	cur, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if cur.Status != StatusBlocked {
		return fmt.Errorf("work item %s is not blocked (status %s)", id, cur.Status)
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE work_items SET status=$2, blocked_on_kind='', blocked_on_id='', updated_at=now() WHERE id=$1`,
		id, StatusInProgress); err != nil {
		return err
	}
	s.logEvent(ctx, id, Event{EventType: "unblocked", ActorID: actorID, FromStatus: cur.Status, ToStatus: StatusInProgress})
	return nil
}

// ListForOwner returns an owner's work items, optionally filtered by status ("" = all).
func (s *Store) ListForOwner(ctx context.Context, tenantID, ownerAgentID, status string) ([]WorkItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, title, origin, owner_agent_id, requested_by, status,
		        blocked_on_kind, blocked_on_id, COALESCE(parent_id::text,''), COALESCE(budget_plan_id::text,''), updated_at
		 FROM work_items
		 WHERE tenant_id=$1 AND owner_agent_id=$2 AND ($3='' OR status=$3)
		 ORDER BY updated_at DESC`, tenantID, ownerAgentID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkItem{}
	for rows.Next() {
		var w WorkItem
		if err := rows.Scan(&w.ID, &w.TenantID, &w.Title, &w.Origin, &w.OwnerAgentID, &w.RequestedBy, &w.Status,
			&w.BlockedOnKind, &w.BlockedOnID, &w.ParentID, &w.BudgetPlanID, &w.UpdatedAt); err != nil {
			slog.Warn("workitems.list.scan_failed", "err", err)
			continue
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ListForRoom returns the work items that belong to a room, newest first.
// Room membership is encoded in origin as 'room:<roomID>' (how delegation
// writes it); the room_id column is not populated by that path.
func (s *Store) ListForRoom(ctx context.Context, tenantID, roomID, status string) ([]WorkItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, title, origin, owner_agent_id, requested_by, status,
		        blocked_on_kind, blocked_on_id, COALESCE(parent_id::text,''), COALESCE(budget_plan_id::text,''), updated_at
		 FROM work_items
		 WHERE tenant_id=$1 AND origin='room:' || $2 AND ($3='' OR status=$3)
		 ORDER BY created_at DESC`, tenantID, roomID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkItem{}
	for rows.Next() {
		var w WorkItem
		if err := rows.Scan(&w.ID, &w.TenantID, &w.Title, &w.Origin, &w.OwnerAgentID, &w.RequestedBy, &w.Status,
			&w.BlockedOnKind, &w.BlockedOnID, &w.ParentID, &w.BudgetPlanID, &w.UpdatedAt); err != nil {
			slog.Warn("workitems.listforroom.scan_failed", "err", err)
			continue
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// Events returns the audit log for a work item, oldest first.
func (s *Store) Events(ctx context.Context, id string) ([]Event, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT event_type, actor_id, from_status, to_status, detail, created_at
		 FROM work_item_events WHERE work_item_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.EventType, &e.ActorID, &e.FromStatus, &e.ToStatus, &e.Detail, &e.CreatedAt); err != nil {
			slog.Warn("workitems.events.scan_failed", "work_item_id", id, "err", err)
			continue
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// logEvent appends an audit row (best-effort).
func (s *Store) logEvent(ctx context.Context, workItemID string, e Event) {
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO work_item_events (work_item_id, event_type, actor_id, from_status, to_status, detail)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		workItemID, e.EventType, e.ActorID, e.FromStatus, e.ToStatus, e.Detail); err != nil {
		slog.Warn("workitems.logevent.failed", "work_item_id", workItemID, "err", err)
	}
}
