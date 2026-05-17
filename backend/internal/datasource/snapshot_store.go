// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package datasource

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SnapshotStore persists connector data source snapshots to PostgreSQL.
type SnapshotStore struct {
	pool *pgxpool.Pool
}

// NewSnapshotStore creates a SnapshotStore backed by the given pool.
func NewSnapshotStore(pool *pgxpool.Pool) *SnapshotStore {
	return &SnapshotStore{pool: pool}
}

// Insert records a new snapshot for the given tenant/slug pair.
// data must be a valid JSON string.
func (s *SnapshotStore) Insert(ctx context.Context, tenantID, slug, resultKey, data string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO connector_snapshots (tenant_id, source_slug, result_key, data)
		 VALUES ($1, $2, $3, $4::jsonb)`,
		tenantID, slug, resultKey, data,
	)
	return err
}

// Latest returns the most recent snapshot data for a tenant/slug.
// Returns an empty map (not an error) when no rows are found.
func (s *SnapshotStore) Latest(ctx context.Context, tenantID, slug string) (map[string]any, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT data FROM connector_snapshots
		 WHERE tenant_id = $1 AND source_slug = $2
		 ORDER BY created_at DESC LIMIT 1`,
		tenantID, slug,
	).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListSlugs returns the distinct source slugs that have snapshots for a tenant.
func (s *SnapshotStore) ListSlugs(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT source_slug FROM connector_snapshots WHERE tenant_id = $1`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slugs []string
	for rows.Next() {
		var slug string
		if scanErr := rows.Scan(&slug); scanErr != nil {
			slog.Warn("datasource.list_slugs.scan_failed", "err", scanErr)
			continue
		}
		slugs = append(slugs, slug)
	}
	return slugs, rows.Err()
}
