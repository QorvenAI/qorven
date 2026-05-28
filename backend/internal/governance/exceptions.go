// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package governance

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Exception struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	Category     string         `json:"category"`
	Severity     string         `json:"severity"`
	AgentID      string         `json:"agent_id"`
	AgentKey     string         `json:"agent_key"`
	Description  string         `json:"description"`
	Context      map[string]any `json:"context"`
	Resolution   string         `json:"resolution,omitempty"`
	ResolvedBy   string         `json:"resolved_by,omitempty"`
	ResolvedAt   *time.Time     `json:"resolved_at,omitempty"`
	Acknowledged bool           `json:"acknowledged"`
	CreatedAt    time.Time      `json:"created_at"`
}

type ExceptionStore struct {
	db *pgxpool.Pool
}

func NewExceptionStore(db *pgxpool.Pool) *ExceptionStore {
	return &ExceptionStore{db: db}
}

func (s *ExceptionStore) Record(ctx context.Context, e Exception) error {
	if s.db == nil {
		return nil
	}
	ctxJSON, _ := json.Marshal(e.Context)
	_, err := s.db.Exec(ctx, `
		INSERT INTO exceptions (tenant_id, category, severity, agent_id, agent_key, description, context)
		VALUES ($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7)
	`, e.TenantID, e.Category, e.Severity, e.AgentID, e.AgentKey, e.Description, ctxJSON)
	return err
}

func (s *ExceptionStore) ListUnresolved(ctx context.Context, tenantID string, limit int) ([]Exception, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, tenant_id, category, severity, COALESCE(agent_id::text,''), COALESCE(agent_key,''),
		       description, COALESCE(context,'{}'), COALESCE(resolution,''),
		       COALESCE(resolved_by::text,''), resolved_at, acknowledged, created_at
		FROM exceptions WHERE tenant_id = $1 AND resolved_at IS NULL
		ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, created_at DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Exception
	for rows.Next() {
		var e Exception
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Category, &e.Severity, &e.AgentID, &e.AgentKey,
			&e.Description, &e.Context, &e.Resolution, &e.ResolvedBy, &e.ResolvedAt, &e.Acknowledged, &e.CreatedAt); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *ExceptionStore) Resolve(ctx context.Context, tenantID, id, resolvedBy, resolution string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE exceptions SET resolution=$1, resolved_by=$2::uuid, resolved_at=now()
		WHERE tenant_id=$3 AND id=$4 AND resolved_at IS NULL
	`, resolution, resolvedBy, tenantID, id)
	return err
}

func (s *ExceptionStore) Acknowledge(ctx context.Context, tenantID, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE exceptions SET acknowledged=true WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	return err
}

func (s *ExceptionStore) Stats(ctx context.Context, tenantID string) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `
		SELECT category, COUNT(*) FROM exceptions
		WHERE tenant_id = $1 AND resolved_at IS NULL
		GROUP BY category
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var cat string
		var count int
		if rows.Scan(&cat, &count) == nil {
			stats[cat] = count
		}
	}
	return stats, nil
}
