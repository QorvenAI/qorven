// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package drive

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type MirrorStore struct{ pool *pgxpool.Pool }

func NewMirrorStore(pool *pgxpool.Pool) *MirrorStore { return &MirrorStore{pool: pool} }

type Mirror struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	Scope          string     `json:"scope"`
	ScopeID        *string    `json:"scope_id"`
	OwnerAgentID   *string    `json:"owner_agent_id"`
	Provider       string     `json:"provider"`
	AccountID      string     `json:"account_id"`
	RemoteFolderID string     `json:"remote_folder_id"`
	Enabled        bool       `json:"enabled"`
	LastSyncedAt   *time.Time `json:"last_synced_at"`
	Error          string     `json:"error"`
}

// mirrorMatchesFile decides whether a mirror's scope covers a file. Pure logic.
func mirrorMatchesFile(mScope, mScopeID, mOwner, fScope, fScopeID, fOwner string) bool {
	if mScope != fScope {
		return false
	}
	switch mScope {
	case ScopeCompany:
		return true
	case ScopePrivate:
		return mOwner != "" && mOwner == fOwner
	case ScopeDepartment:
		return mScopeID != "" && mScopeID == fScopeID
	case ScopeCustom:
		return true
	}
	return false
}

func (s *MirrorStore) ListMirrors(ctx context.Context, tenantID string) ([]Mirror, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, scope, scope_id, owner_agent_id, provider, account_id, remote_folder_id, enabled, last_synced_at, error
		 FROM drive_mirrors WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Mirror{}
	for rows.Next() {
		var m Mirror
		if rows.Scan(&m.ID, &m.TenantID, &m.Scope, &m.ScopeID, &m.OwnerAgentID, &m.Provider, &m.AccountID, &m.RemoteFolderID, &m.Enabled, &m.LastSyncedAt, &m.Error) != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *MirrorStore) CreateMirror(ctx context.Context, m Mirror) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO drive_mirrors (tenant_id, scope, scope_id, owner_agent_id, provider, account_id, remote_folder_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		m.TenantID, m.Scope, m.ScopeID, m.OwnerAgentID, m.Provider, m.AccountID, m.RemoteFolderID).Scan(&id)
	return id, err
}

func (s *MirrorStore) DeleteMirror(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM drive_mirrors WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	return err
}

// MirrorsForFile returns enabled mirrors whose scope covers the given file.
func (s *MirrorStore) MirrorsForFile(ctx context.Context, tenantID, fScope, fScopeID, fOwner string) ([]Mirror, error) {
	all, err := s.ListMirrors(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := []Mirror{}
	for _, m := range all {
		if !m.Enabled {
			continue
		}
		mScopeID := ""
		if m.ScopeID != nil {
			mScopeID = *m.ScopeID
		}
		mOwner := ""
		if m.OwnerAgentID != nil {
			mOwner = *m.OwnerAgentID
		}
		if mirrorMatchesFile(m.Scope, mScopeID, mOwner, fScope, fScopeID, fOwner) {
			out = append(out, m)
		}
	}
	return out, nil
}

// RecordPush upserts the remote-id dedup row for a (file, mirror) pair.
func (s *MirrorStore) RecordPush(ctx context.Context, fileID, mirrorID, remoteFileID, status, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO drive_file_remote (file_id, mirror_id, remote_file_id, status, error, last_pushed_at)
		 VALUES ($1,$2,$3,$4,$5,now())
		 ON CONFLICT (file_id, mirror_id) DO UPDATE SET remote_file_id=EXCLUDED.remote_file_id, status=EXCLUDED.status, error=EXCLUDED.error, last_pushed_at=now()`,
		fileID, mirrorID, remoteFileID, status, errMsg)
	return err
}

// RemoteFileID returns the previously-pushed remote id for dedup/update ("" if none).
func (s *MirrorStore) RemoteFileID(ctx context.Context, fileID, mirrorID string) string {
	var rid string
	_ = s.pool.QueryRow(ctx, `SELECT remote_file_id FROM drive_file_remote WHERE file_id=$1 AND mirror_id=$2`, fileID, mirrorID).Scan(&rid)
	return rid
}
