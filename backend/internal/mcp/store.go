// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DBServerConfig extends ServerConfig with DB fields.
type DBServerConfig struct {
	ServerConfig
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Create(ctx context.Context, cfg DBServerConfig) (string, error) {
	argsJSON, _ := json.Marshal(cfg.Args)
	envJSON, _ := json.Marshal(cfg.Env)
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO mcp_servers (name, transport, command, args, env, url, enabled, tenant_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW()) RETURNING id`,
		cfg.Name, cfg.Transport, cfg.Command, argsJSON, envJSON, cfg.URL, cfg.Enabled, cfg.TenantID).Scan(&id)
	if err != nil { return "", fmt.Errorf("mcp.create: %w", err) }
	return id, nil
}

func (s *Store) Get(ctx context.Context, serverID string) (*DBServerConfig, error) {
	var cfg DBServerConfig
	var argsJSON, envJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, transport, command, args, env, COALESCE(url,''), enabled, tenant_id, created_at
		 FROM mcp_servers WHERE id = $1`, serverID).Scan(
		&cfg.ID, &cfg.Name, &cfg.Transport, &cfg.Command, &argsJSON, &envJSON, &cfg.URL, &cfg.Enabled, &cfg.TenantID, &cfg.CreatedAt)
	if err != nil { return nil, err }
	json.Unmarshal(argsJSON, &cfg.Args)
	json.Unmarshal(envJSON, &cfg.Env)
	return &cfg, nil
}

func (s *Store) List(ctx context.Context, tenantID string) ([]DBServerConfig, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, transport, command, args, env, COALESCE(url,''), enabled, tenant_id, created_at
		 FROM mcp_servers WHERE tenant_id = $1 OR $1 = '' ORDER BY name`, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()
	configs := []DBServerConfig{}
	for rows.Next() {
		var cfg DBServerConfig
		var argsJSON, envJSON []byte
		rows.Scan(&cfg.ID, &cfg.Name, &cfg.Transport, &cfg.Command, &argsJSON, &envJSON, &cfg.URL, &cfg.Enabled, &cfg.TenantID, &cfg.CreatedAt)
		json.Unmarshal(argsJSON, &cfg.Args)
		json.Unmarshal(envJSON, &cfg.Env)
		configs = append(configs, cfg)
	}
	return configs, nil
}

func (s *Store) Delete(ctx context.Context, serverID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE id = $1`, serverID)
	return err
}

func (s *Store) Toggle(ctx context.Context, serverID string, enabled bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE mcp_servers SET enabled = $1 WHERE id = $2`, enabled, serverID)
	return err
}

func (s *Store) UpdateCommand(ctx context.Context, serverID, command string, args []string) error {
	argsJSON, _ := json.Marshal(args)
	_, err := s.pool.Exec(ctx, `UPDATE mcp_servers SET command = $1, args = $2 WHERE id = $3`, command, argsJSON, serverID)
	return err
}
