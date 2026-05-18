// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package registry is the tenant-scoped Wasm plugin store.
//
// This package handles persistence + lookup of plugin metadata and
// the raw .wasm bytes. It does NOT compile or execute plugins — that
// lives in internal/plugins/wasm. A plugins.Loader (defined
// separately) glues Store.List output to wasm.Host.LoadPlugin at plan
// execution time.
//
// RLS is the primary protection boundary: every query here goes
// through store.FromContext so that in multi-tenant mode the
// TenantScopeMiddleware's transaction pins the query to the caller's
// tenant_id. A misconfigured handler that forgets to pass ctx down
// would see EMPTY result sets (not another tenant's data) because
// RLS filters at the server. The belt to this suspender is that
// every CRUD function also asserts tenantID on inputs — so a caller
// in single-tenant mode (bypass=on) still gets tenant isolation.
package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qorvenai/qorven/internal/store"
	"github.com/qorvenai/qorven/internal/tools"
)

// ErrNotFound is returned by Get when no row matches.
var ErrNotFound = errors.New("registry: plugin not found")

// ErrInvalidName is returned when a plugin name violates the
// constraint. Mirrored here so HTTP handlers can surface a 400 with
// a specific code instead of a generic DB error.
var ErrInvalidName = errors.New("registry: plugin name must match ^[a-z][a-z0-9_]{0,62}$")

// ErrReservedName is returned when a plugin name collides with a
// platform-reserved tool (see tools.ReservedCoreToolNames). Shadow
// semantics in the agent loop would otherwise let the plugin
// intercept a built-in destructive or orchestration tool — a major
// security footgun. Phase 6 blocker closure.
var ErrReservedName = errors.New("registry: plugin name collides with a platform-reserved tool")

// nameRE mirrors the CHECK constraint in migration 043. We validate
// in Go BEFORE hitting Postgres so the error is structured and can
// be distinguished from a concurrent-rename race.
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// Plugin is the persisted row.
type Plugin struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	WasmBinary  []byte          `json:"-"` // never serialized — it's MB of opaque bytes
	SHA256      string          `json:"sha256"`
	Parameters  json.RawMessage `json:"parameters"`
	CreatedBy   string          `json:"created_by,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	RevokedAt   *time.Time      `json:"revoked_at,omitempty"`
}

// UploadInput bundles the arguments to Upload. Pass the raw wasm
// bytes; the store computes + stores the sha256.
type UploadInput struct {
	TenantID    string
	Name        string
	Description string
	WasmBinary  []byte
	Parameters  json.RawMessage // tool parameter schema (JSON Schema fragment)
	CreatedBy   string
}

// Store persists Wasm plugin rows.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store. pool may be nil for tests that wire
// through FromContext; production callers pass a real pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// q routes queries through the ctx-tx in multi-tenant mode, else the
// raw pool. Mirrors the AGENTS.md §1.1 rule.
func (s *Store) q(ctx context.Context) store.Queryable {
	return store.FromContext(ctx, s.pool)
}

// Upload inserts a new plugin row (or re-activates an existing
// same-name row under the same tenant). Idempotent upload: if the
// sha256 matches the current row, nothing changes and the existing
// row is returned. If it differs, the old row is revoked (soft
// delete) and a new active row takes its place — so the sha256
// history is preserved for audit.
func (s *Store) Upload(ctx context.Context, in UploadInput) (*Plugin, error) {
	if in.TenantID == "" {
		return nil, errors.New("registry: tenant_id required")
	}
	if !nameRE.MatchString(in.Name) {
		return nil, ErrInvalidName
	}
	// Phase 6 security gate: reject uploads whose name would shadow
	// a reserved core tool. First of three enforcement points (Store
	// upload → HTTP handler surface → Loader runtime); layered so a
	// refactor that bypasses one still catches the collision at the
	// next layer.
	if tools.IsReservedCoreToolName(in.Name) {
		return nil, ErrReservedName
	}
	if len(in.WasmBinary) == 0 {
		return nil, errors.New("registry: wasm_binary required")
	}
	sum := sha256.Sum256(in.WasmBinary)
	digest := hex.EncodeToString(sum[:])

	params := in.Parameters
	if len(params) == 0 {
		params = json.RawMessage("{}")
	}

	// Fast path: same sha256 already active — no-op.
	existing, err := s.GetActiveByName(ctx, in.TenantID, in.Name)
	if err == nil && existing != nil && existing.SHA256 == digest {
		return existing, nil
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// If a different binary is active, revoke it first so the
	// partial-unique-index (WHERE revoked_at IS NULL) lets the new
	// row in.
	if existing != nil {
		if err := s.revokeByID(ctx, in.TenantID, existing.ID); err != nil {
			return nil, fmt.Errorf("registry: revoke previous %q: %w", in.Name, err)
		}
	}

	// Insert the new active row.
	var p Plugin
	var paramsOut []byte
	err = s.q(ctx).QueryRow(ctx, `
        INSERT INTO wasm_plugins
            (tenant_id, name, description, wasm_binary, sha256, parameters, created_by)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id, tenant_id::text, name, description, wasm_binary, sha256,
                  parameters, created_by, created_at, updated_at, revoked_at
    `,
		in.TenantID, in.Name, in.Description, in.WasmBinary, digest,
		params, in.CreatedBy,
	).Scan(
		&p.ID, &p.TenantID, &p.Name, &p.Description, &p.WasmBinary, &p.SHA256,
		&paramsOut, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.RevokedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("registry: insert: %w", err)
	}
	p.Parameters = paramsOut
	return &p, nil
}

// GetActiveByName returns the currently-active plugin (revoked_at IS
// NULL) for a (tenant, name) pair. Returns ErrNotFound if no active
// row exists — a revoked row is not visible through this path.
func (s *Store) GetActiveByName(ctx context.Context, tenantID, name string) (*Plugin, error) {
	if tenantID == "" || name == "" {
		return nil, errors.New("registry: tenant_id and name required")
	}
	var p Plugin
	var paramsOut []byte
	err := s.q(ctx).QueryRow(ctx, `
        SELECT id, tenant_id::text, name, description, wasm_binary, sha256,
               parameters, created_by, created_at, updated_at, revoked_at
          FROM wasm_plugins
         WHERE tenant_id = $1 AND name = $2 AND revoked_at IS NULL
    `, tenantID, name).Scan(
		&p.ID, &p.TenantID, &p.Name, &p.Description, &p.WasmBinary, &p.SHA256,
		&paramsOut, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("registry: get active: %w", err)
	}
	p.Parameters = paramsOut
	return &p, nil
}

// ListActive returns every active plugin for a tenant. Binary bytes
// ARE included — callers (the loader) need them to compile. HTTP
// handlers that list plugins to the UI should strip the binary
// before serializing; see the handler's projection.
func (s *Store) ListActive(ctx context.Context, tenantID string) ([]*Plugin, error) {
	if tenantID == "" {
		return nil, errors.New("registry: tenant_id required")
	}
	rows, err := s.q(ctx).Query(ctx, `
        SELECT id, tenant_id::text, name, description, wasm_binary, sha256,
               parameters, created_by, created_at, updated_at, revoked_at
          FROM wasm_plugins
         WHERE tenant_id = $1 AND revoked_at IS NULL
         ORDER BY name ASC
    `, tenantID)
	if err != nil {
		return nil, fmt.Errorf("registry: list: %w", err)
	}
	defer rows.Close()
	var out []*Plugin
	for rows.Next() {
		var p Plugin
		var paramsOut []byte
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.Name, &p.Description, &p.WasmBinary, &p.SHA256,
			&paramsOut, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.RevokedAt,
		); err != nil {
			return nil, err
		}
		p.Parameters = paramsOut
		out = append(out, &p)
	}
	return out, rows.Err()
}

// Revoke soft-deletes the currently-active plugin with the given
// name in the tenant. Idempotent: revoking an already-revoked plugin
// returns ErrNotFound (the active row is gone) — callers that want
// a "revoke or silently succeed" semantic can ignore ErrNotFound.
func (s *Store) Revoke(ctx context.Context, tenantID, name string) error {
	if tenantID == "" || !nameRE.MatchString(name) {
		return ErrInvalidName
	}
	ct, err := s.q(ctx).Exec(ctx, `
        UPDATE wasm_plugins
           SET revoked_at = NOW(), updated_at = NOW()
         WHERE tenant_id = $1 AND name = $2 AND revoked_at IS NULL
    `, tenantID, name)
	if err != nil {
		return fmt.Errorf("registry: revoke: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// revokeByID is the internal variant Upload uses to retire a
// previous row when a new binary lands under the same (tenant, name).
func (s *Store) revokeByID(ctx context.Context, tenantID, id string) error {
	_, err := s.q(ctx).Exec(ctx, `
        UPDATE wasm_plugins
           SET revoked_at = NOW(), updated_at = NOW()
         WHERE tenant_id = $1 AND id = $2 AND revoked_at IS NULL
    `, tenantID, id)
	return err
}
