// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SystemConfig provides per-tenant configuration.
type SystemConfig struct {
	pool *pgxpool.Pool
}

func NewSystemConfig(pool *pgxpool.Pool) *SystemConfig { return &SystemConfig{pool: pool} }

func (sc *SystemConfig) Get(ctx context.Context, tenantID, key string) (map[string]any, error) {
	var val json.RawMessage
	err := sc.pool.QueryRow(ctx,
		`SELECT value FROM system_configs WHERE tenant_id = $1 AND key = $2`, tenantID, key).Scan(&val)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	json.Unmarshal(val, &result)
	return result, nil
}

func (sc *SystemConfig) Set(ctx context.Context, tenantID, key string, value map[string]any) error {
	val, _ := json.Marshal(value)
	_, err := sc.pool.Exec(ctx,
		`INSERT INTO system_configs (tenant_id, key, value) VALUES ($1, $2, $3)
		 ON CONFLICT (tenant_id, key) DO UPDATE SET value = $3, updated_at = now()`,
		tenantID, key, val)
	return err
}

func (sc *SystemConfig) List(ctx context.Context, tenantID string) (map[string]map[string]any, error) {
	rows, err := sc.pool.Query(ctx, `SELECT key, value FROM system_configs WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]map[string]any)
	for rows.Next() {
		var key string
		var val json.RawMessage
		rows.Scan(&key, &val)
		var v map[string]any
		json.Unmarshal(val, &v)
		result[key] = v
	}
	return result, nil
}
