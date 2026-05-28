// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package governance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Asset struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	Name        string         `json:"name"`
	AssetType   string         `json:"asset_type"`
	Category    string         `json:"category"`
	Content     map[string]any `json:"content"`
	Version     int            `json:"version"`
	OwnerAgent  string         `json:"owner_agent"`
	Tags        []string       `json:"tags"`
	UsageCount  int            `json:"usage_count"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type AssetStore struct {
	db *pgxpool.Pool
}

func NewAssetStore(db *pgxpool.Pool) *AssetStore {
	return &AssetStore{db: db}
}

func (s *AssetStore) List(ctx context.Context, tenantID string, limit int) ([]Asset, error) {
	rows, err := s.db.Query(ctx, `SELECT id, tenant_id, name, asset_type, COALESCE(description,''), content, version, COALESCE(created_by::text,''), tags, usage_count, created_at, updated_at FROM asset_library WHERE tenant_id = $1 ORDER BY updated_at DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Name, &a.AssetType, &a.Category, &a.Content, &a.Version, &a.OwnerAgent, &a.Tags, &a.UsageCount, &a.CreatedAt, &a.UpdatedAt); err != nil {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

func (s *AssetStore) Get(ctx context.Context, tenantID, id string) (*Asset, error) {
	var a Asset
	err := s.db.QueryRow(ctx, `SELECT id, tenant_id, name, asset_type, COALESCE(description,''), content, version, COALESCE(created_by::text,''), tags, usage_count, created_at, updated_at FROM asset_library WHERE tenant_id = $1 AND id = $2`, tenantID, id).Scan(&a.ID, &a.TenantID, &a.Name, &a.AssetType, &a.Category, &a.Content, &a.Version, &a.OwnerAgent, &a.Tags, &a.UsageCount, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *AssetStore) Upsert(ctx context.Context, a Asset) error {
	var existingID string
	err := s.db.QueryRow(ctx, `SELECT id FROM asset_library WHERE tenant_id = $1 AND name = $2 AND asset_type = $3`, a.TenantID, a.Name, a.AssetType).Scan(&existingID)
	if err == nil {
		_, err = s.db.Exec(ctx, `UPDATE asset_library SET content = $1, description = $2, tags = $3, version = version + 1, updated_at = NOW() WHERE id = $4`,
			a.Content, a.Category, a.Tags, existingID)
		return err
	}
	var createdBy any
	if a.OwnerAgent != "" {
		createdBy = a.OwnerAgent
	}
	_, err = s.db.Exec(ctx, `INSERT INTO asset_library (tenant_id, name, asset_type, description, content, version, created_by, tags)
		VALUES ($1,$2,$3,$4,$5,1,$6,$7)`,
		a.TenantID, a.Name, a.AssetType, a.Category, a.Content, createdBy, a.Tags)
	return err
}

func (s *AssetStore) IncrementUsage(ctx context.Context, tenantID, id string) {
	s.db.Exec(ctx, `UPDATE asset_library SET usage_count = usage_count + 1 WHERE tenant_id = $1 AND id = $2`, tenantID, id)
}

func (s *AssetStore) Delete(ctx context.Context, tenantID, id string) {
	s.db.Exec(ctx, `DELETE FROM asset_library WHERE tenant_id = $1 AND id = $2`, tenantID, id)
}

func (s *AssetStore) Stats(ctx context.Context, tenantID string) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `SELECT asset_type, COUNT(*) FROM asset_library WHERE tenant_id = $1 GROUP BY asset_type`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var t string
		var c int
		rows.Scan(&t, &c)
		out[t] = c
	}
	return out, nil
}
