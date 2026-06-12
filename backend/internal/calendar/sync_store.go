// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package calendar

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SyncStore struct{ pool *pgxpool.Pool }

func NewSyncStore(pool *pgxpool.Pool) *SyncStore { return &SyncStore{pool: pool} }

type Sync struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	Scope            string     `json:"scope"`
	ScopeID          *string    `json:"scope_id"`
	OwnerAgentID     *string    `json:"owner_agent_id"`
	Provider         string     `json:"provider"`
	AccountID        string     `json:"account_id"`
	RemoteCalendarID string     `json:"remote_calendar_id"`
	Enabled          bool       `json:"enabled"`
	LastSyncedAt     *time.Time `json:"last_synced_at"`
	Error            string     `json:"error"`
}

// SyncMatchesItem decides whether a sync's scope covers a timeline item, given
// the item's agent and that agent's department. Pure, exported (called by the
// gateway push engine).
func SyncMatchesItem(sScope, sScopeID, sOwner, itemAgent, itemAgentDept string) bool {
	switch sScope {
	case "company":
		return true
	case "private":
		return sOwner != "" && sOwner == itemAgent
	case "department":
		return sScopeID != "" && sScopeID == itemAgentDept
	}
	return false
}

func (s *SyncStore) ListSyncs(ctx context.Context, tenantID string) ([]Sync, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, scope, scope_id, owner_agent_id, provider, account_id, remote_calendar_id, enabled, last_synced_at, error
		 FROM calendar_syncs WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Sync{}
	for rows.Next() {
		var m Sync
		if rows.Scan(&m.ID, &m.TenantID, &m.Scope, &m.ScopeID, &m.OwnerAgentID, &m.Provider, &m.AccountID, &m.RemoteCalendarID, &m.Enabled, &m.LastSyncedAt, &m.Error) != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *SyncStore) CreateSync(ctx context.Context, m Sync) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO calendar_syncs (tenant_id, scope, scope_id, owner_agent_id, provider, account_id, remote_calendar_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		m.TenantID, m.Scope, m.ScopeID, m.OwnerAgentID, m.Provider, m.AccountID, m.RemoteCalendarID).Scan(&id)
	return id, err
}

func (s *SyncStore) DeleteSync(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM calendar_syncs WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	return err
}

func (s *SyncStore) MarkSynced(ctx context.Context, id, errMsg string) error {
	_, err := s.pool.Exec(ctx, `UPDATE calendar_syncs SET last_synced_at=now(), error=$2 WHERE id=$1`, id, errMsg)
	return err
}

func (s *SyncStore) RecordEventPush(ctx context.Context, itemID, syncID, remoteEventID, status, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO calendar_event_remote (item_id, sync_id, remote_event_id, status, error, last_pushed_at)
		 VALUES ($1,$2,$3,$4,$5,now())
		 ON CONFLICT (item_id, sync_id) DO UPDATE SET remote_event_id=EXCLUDED.remote_event_id, status=EXCLUDED.status, error=EXCLUDED.error, last_pushed_at=now()`,
		itemID, syncID, remoteEventID, status, errMsg)
	return err
}

func (s *SyncStore) RemoteEventID(ctx context.Context, itemID, syncID string) string {
	var rid string
	_ = s.pool.QueryRow(ctx, `SELECT remote_event_id FROM calendar_event_remote WHERE item_id=$1 AND sync_id=$2`, itemID, syncID).Scan(&rid)
	return rid
}

// AlreadyPushed reports whether this (item, sync) pair already pushed
// successfully. The sync ticker re-reads all future items every run; without
// this guard a one-way create_event (which has no upsert) would re-create a
// duplicate external event on every tick. We push each item once per sync and
// skip thereafter (one-way OUT — a later edit to the item is not re-pushed; a
// real update_event path is a future enhancement).
func (s *SyncStore) AlreadyPushed(ctx context.Context, itemID, syncID string) bool {
	var n int
	_ = s.pool.QueryRow(ctx,
		`SELECT COUNT(1) FROM calendar_event_remote WHERE item_id=$1 AND sync_id=$2 AND status='ok'`,
		itemID, syncID).Scan(&n)
	return n > 0
}
