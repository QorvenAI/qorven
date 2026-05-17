// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package apps

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AppStore handles DB CRUD for the apps table.
type AppStore struct {
	pool *pgxpool.Pool
}

// NewStore creates an AppStore.
func NewStore(pool *pgxpool.Pool) *AppStore {
	return &AppStore{pool: pool}
}

// Create inserts a new app row and returns the created App.
func (s *AppStore) Create(ctx context.Context, a App) (App, error) {
	cfgJSON, _ := json.Marshal(a.Config)
	scope := a.Scope
	if scope == "" {
		scope = "workspace"
	}
	var created App
	var cfgRaw []byte
	err := s.pool.QueryRow(ctx,
		`INSERT INTO apps (tenant_id, slug, display_name, description, version, author, icon_url, install_path, enabled, config, scope, owner_agent_id, owner_team_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		 RETURNING id, tenant_id, slug, display_name, description, version, author, icon_url, install_path, enabled, config, installed_at, updated_at, scope, owner_agent_id, owner_team_id`,
		a.TenantID, a.Slug, a.DisplayName, a.Description, a.Version, a.Author, a.IconURL, a.InstallPath, a.Enabled, cfgJSON, scope, a.OwnerAgentID, a.OwnerTeamID,
	).Scan(&created.ID, &created.TenantID, &created.Slug, &created.DisplayName,
		&created.Description, &created.Version, &created.Author, &created.IconURL,
		&created.InstallPath, &created.Enabled, &cfgRaw, &created.InstalledAt, &created.UpdatedAt,
		&created.Scope, &created.OwnerAgentID, &created.OwnerTeamID)
	if err != nil {
		return App{}, err
	}
	json.Unmarshal(cfgRaw, &created.Config)
	return created, nil
}

// Get returns an app by ID scoped to tenantID.
func (s *AppStore) Get(ctx context.Context, tenantID, id string) (App, error) {
	return s.scanOne(ctx,
		`SELECT id, tenant_id, slug, display_name, description, version, author, icon_url, install_path, enabled, config, installed_at, updated_at, scope, owner_agent_id, owner_team_id
		 FROM apps WHERE id = $1 AND tenant_id = $2`, id, tenantID)
}

// GetBySlug returns an app by slug scoped to tenantID.
func (s *AppStore) GetBySlug(ctx context.Context, tenantID, slug string) (App, error) {
	return s.scanOne(ctx,
		`SELECT id, tenant_id, slug, display_name, description, version, author, icon_url, install_path, enabled, config, installed_at, updated_at, scope, owner_agent_id, owner_team_id
		 FROM apps WHERE slug = $1 AND tenant_id = $2`, slug, tenantID)
}

// List returns all apps for a tenant ordered by display_name (admin view — unscoped).
func (s *AppStore) List(ctx context.Context, tenantID string) ([]App, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, slug, display_name, description, version, author, icon_url, install_path, enabled, config, installed_at, updated_at, scope, owner_agent_id, owner_team_id
		 FROM apps WHERE tenant_id = $1 ORDER BY display_name ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []App
	for rows.Next() {
		a, err := s.scanRow(rows)
		if err != nil {
			continue
		}
		result = append(result, a)
	}
	if result == nil {
		result = []App{}
	}
	return result, nil
}

// ListScoped returns enabled apps visible to the given agentID and teamID.
// workspace-scoped apps are always included. agent-scoped apps require a
// matching agentID; team-scoped apps require a matching teamID. An empty
// agentID or teamID means those scoped apps are never returned.
func (s *AppStore) ListScoped(ctx context.Context, tenantID, agentID, teamID string) ([]App, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, slug, display_name, description, version, author, icon_url, install_path, enabled, config, installed_at, updated_at, scope, owner_agent_id, owner_team_id
		 FROM apps
		 WHERE tenant_id = $1 AND enabled = true
		   AND (
		     scope = 'workspace'
		     OR (scope = 'agent' AND owner_agent_id = $2)
		     OR (scope = 'team'  AND owner_team_id  = $3)
		   )
		 ORDER BY display_name ASC`, tenantID, agentID, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []App
	for rows.Next() {
		a, err := s.scanRow(rows)
		if err != nil {
			continue
		}
		result = append(result, a)
	}
	if result == nil {
		result = []App{}
	}
	return result, nil
}

// SetEnabled toggles the enabled flag.
func (s *AppStore) SetEnabled(ctx context.Context, tenantID, id string, enabled bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE apps SET enabled=$1, updated_at=$2 WHERE id=$3 AND tenant_id=$4`,
		enabled, time.Now(), id, tenantID)
	return err
}

// SetConfig merges config fields (full replace).
func (s *AppStore) SetConfig(ctx context.Context, tenantID, id string, cfg map[string]any) error {
	cfgJSON, _ := json.Marshal(cfg)
	_, err := s.pool.Exec(ctx,
		`UPDATE apps SET config=$1, updated_at=$2 WHERE id=$3 AND tenant_id=$4`,
		cfgJSON, time.Now(), id, tenantID)
	return err
}

// Delete removes an app row.
func (s *AppStore) Delete(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM apps WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	return err
}

// --- helpers ---

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *AppStore) scanOne(ctx context.Context, query string, args ...any) (App, error) {
	row := s.pool.QueryRow(ctx, query, args...)
	return s.scanRow(row)
}

func (s *AppStore) scanRow(r rowScanner) (App, error) {
	var a App
	var cfgRaw []byte
	if err := r.Scan(&a.ID, &a.TenantID, &a.Slug, &a.DisplayName, &a.Description,
		&a.Version, &a.Author, &a.IconURL, &a.InstallPath, &a.Enabled,
		&cfgRaw, &a.InstalledAt, &a.UpdatedAt,
		&a.Scope, &a.OwnerAgentID, &a.OwnerTeamID); err != nil {
		return App{}, err
	}
	if len(cfgRaw) > 0 {
		json.Unmarshal(cfgRaw, &a.Config)
	}
	if a.Config == nil {
		a.Config = map[string]any{}
	}
	if a.Scope == "" {
		a.Scope = "workspace"
	}
	return a, nil
}
