// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package dashboard

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PinnedTile represents a tile pinned to the dashboard.
type PinnedTile struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenant_id"`
	SourceSlug         string         `json:"source_slug"`
	ToolName           string         `json:"tool_name"`
	ToolArgs           map[string]any `json:"tool_args"`
	WidgetType         string         `json:"widget_type"`
	Label              string         `json:"label"`
	Position           int            `json:"position"`
	RefreshIntervalSec int            `json:"refresh_interval_sec"`
}

// TileStore persists pinned dashboard tiles to PostgreSQL.
type TileStore struct {
	pool *pgxpool.Pool
}

// NewTileStore returns a TileStore backed by the given pool.
func NewTileStore(pool *pgxpool.Pool) *TileStore {
	return &TileStore{pool: pool}
}

// List returns all pinned tiles for a tenant, ordered by position then created_at.
func (s *TileStore) List(ctx context.Context, tenantID string) ([]PinnedTile, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, source_slug, tool_name, tool_args, widget_type, label, position, refresh_interval_sec
		 FROM pinned_tiles
		 WHERE tenant_id = $1
		 ORDER BY position ASC, created_at ASC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tiles []PinnedTile
	for rows.Next() {
		var t PinnedTile
		var argsRaw []byte
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.SourceSlug, &t.ToolName,
			&argsRaw, &t.WidgetType, &t.Label, &t.Position, &t.RefreshIntervalSec,
		); err != nil {
			return nil, err
		}
		if len(argsRaw) > 0 {
			if err := json.Unmarshal(argsRaw, &t.ToolArgs); err != nil {
				t.ToolArgs = map[string]any{}
			}
		} else {
			t.ToolArgs = map[string]any{}
		}
		tiles = append(tiles, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if tiles == nil {
		tiles = []PinnedTile{}
	}
	return tiles, nil
}

// Create inserts a new pinned tile and returns the tile with its generated ID.
func (s *TileStore) Create(ctx context.Context, t PinnedTile) (*PinnedTile, error) {
	argsJSON, err := json.Marshal(t.ToolArgs)
	if err != nil {
		argsJSON = []byte("{}")
	}

	var id string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO pinned_tiles (tenant_id, source_slug, tool_name, tool_args, widget_type, label, position, refresh_interval_sec)
		 VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8)
		 RETURNING id`,
		t.TenantID, t.SourceSlug, t.ToolName, string(argsJSON),
		t.WidgetType, t.Label, t.Position, t.RefreshIntervalSec,
	).Scan(&id)
	if err != nil {
		return nil, err
	}
	t.ID = id
	return &t, nil
}

// Delete removes a pinned tile by ID, scoped to the tenant.
func (s *TileStore) Delete(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM pinned_tiles WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return err
}
