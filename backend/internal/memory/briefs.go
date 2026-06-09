// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Brief is a CKO-curated, clearance-tagged knowledge digest for a scope.
type Brief struct {
	ID        string
	Scope     string // company|department|role
	ScopeKey  string // "" for company; dept/role name otherwise
	Clearance Classification
	Content   string
	Version   int
}

// BriefStore persists and serves knowledge briefs for one tenant.
type BriefStore struct {
	pool     *pgxpool.Pool
	tenantID string
}

func NewBriefStore(pool *pgxpool.Pool, tenantID string) *BriefStore {
	return &BriefStore{pool: pool, tenantID: tenantID}
}

// Upsert writes (or replaces) the brief for (tenant, scope, scope_key) and bumps version.
func (s *BriefStore) Upsert(ctx context.Context, b Brief) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO knowledge_briefs (tenant_id, scope, scope_key, clearance, content, version)
		 VALUES ($1,$2,$3,$4,$5,1)
		 ON CONFLICT (tenant_id, scope, scope_key)
		 DO UPDATE SET clearance=$4, content=$5, version=knowledge_briefs.version+1, refreshed_at=now()`,
		s.tenantID, b.Scope, b.ScopeKey, int(b.Clearance), b.Content)
	return err
}

// GetForAgent returns the company brief, the agent's department brief, and the agent's
// role brief — but ONLY those at or below the agent's clearance level.
func (s *BriefStore) GetForAgent(ctx context.Context, orgRole, departmentKey string, clearance Classification) []Brief {
	rows, err := s.pool.Query(ctx,
		`SELECT id, scope, scope_key, clearance, content, version
		 FROM knowledge_briefs
		 WHERE tenant_id=$1
		   AND clearance <= $2
		   AND ( (scope='company')
		      OR (scope='department' AND $3 <> '' AND scope_key=$3)
		      OR (scope='role' AND scope_key=$4) )
		 ORDER BY scope`,
		s.tenantID, int(clearance), departmentKey, orgRole)
	if err != nil {
		slog.Warn("briefstore.get_for_agent.failed", "err", err)
		return nil
	}
	defer rows.Close()
	out := []Brief{}
	for rows.Next() {
		var b Brief
		var cl int
		if err := rows.Scan(&b.ID, &b.Scope, &b.ScopeKey, &cl, &b.Content, &b.Version); err != nil {
			slog.Warn("briefstore.get_for_agent.scan_failed", "err", err)
			continue
		}
		b.Clearance = Classification(cl)
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("briefstore.get_for_agent.rows_err", "err", err)
	}
	return out
}
