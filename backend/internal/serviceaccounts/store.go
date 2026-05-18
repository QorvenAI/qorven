// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package serviceaccounts persists the allow-list of non-human actors
// that may operate on any session regardless of the owner. Phase 1 kept
// this list hardcoded; Phase 2 moves it to the DB per FU-012.
//
// Roles:
//
//   - "admin"         — full access to every session + admin endpoints
//   - "service"       — programmatic access used by internal infra
//   - "orchestrator"  — used by the plan graph runtime when calling
//                       commands on behalf of a user
//
// The in-memory cache refreshes every RefreshInterval (default 30s) and
// on demand via Invalidate. The cache is safe for concurrent reads.
package serviceaccounts

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qorvenai/qorven/internal/cache"
)

// Role is the service account's privilege class.
type Role string

const (
	RoleAdmin        Role = "admin"
	RoleService      Role = "service"
	RoleOrchestrator Role = "orchestrator"
)

// ServiceAccount is the persisted record.
//
// TenantID is NULL for the legacy infra-global actors seeded by
// migration 037 (system / orchestrator / qoros). Non-NULL when a
// tenant admin creates a per-tenant SA; authorize() then requires the
// resource's tenant_id to match before granting the SA bypass.
type ServiceAccount struct {
	ID          string     `json:"id"`
	Role        Role       `json:"role"`
	Description string     `json:"description,omitempty"`
	CreatedBy   string     `json:"created_by,omitempty"`
	TenantID    string     `json:"tenant_id,omitempty"` // "" == NULL == global
	CreatedAt   time.Time  `json:"created_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

// Store persists and caches service accounts.
//
// The hot-read path uses an atomic.Pointer[snapshot] so lookups are
// wait-free within a process. The shared backend (cache.Cache) provides
// cross-replica invalidation: when a write happens on replica A, it
// deletes the shared key so replica B's next lookup re-queries the DB
// instead of serving a stale snapshot. The default backend is in-memory
// (single-replica safe); set QORVEN_CACHE_BACKEND=redis for HA deploys.
type Store struct {
	pool            *pgxpool.Pool
	RefreshInterval time.Duration

	// local holds the in-process snapshot for wait-free reads.
	local atomic.Pointer[snapshot]

	// shared is the cross-replica cache. Delete("snapshot") to signal
	// other replicas that they should re-query on next lookup.
	shared cache.Cache[map[string]cacheEntry]
}

type snapshot struct {
	loadedAt time.Time
	byID     map[string]cacheEntry
}

// cacheEntry pairs the SA's role with its tenant binding so callers
// that need to compare against resource tenancy don't re-query the DB.
type cacheEntry struct {
	Role     Role   `json:"role"`
	TenantID string `json:"tenant_id"` // "" == global (legacy infra actor)
}

// NewStore constructs a Store. Callers should call Refresh once at
// startup before serving traffic.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:            pool,
		RefreshInterval: 30 * time.Second,
		shared:          cache.NewFromEnv[map[string]cacheEntry]("sa"),
	}
}

// AddInput bundles Add's parameters. TenantID is REQUIRED for the
// default Add() path — a tenant-less SA is a boundary-crossing
// credential and must be created through AddGlobal() so the intent is
// explicit at the call site.
type AddInput struct {
	ID          string
	Role        Role
	Description string
	CreatedBy   string
	TenantID    string // MUST be set for Add(); leave empty only via AddGlobal().
}

// Add inserts a tenant-scoped service account. Returns ErrAlreadyExists
// when the account already exists and is active (not revoked). To update
// an existing active account use Upsert; to reactivate a revoked account
// just call Add again (revoked rows are treated as absent).
func (s *Store) Add(ctx context.Context, in AddInput) (*ServiceAccount, error) {
	if in.TenantID == "" {
		return nil, fmt.Errorf(
			"serviceaccounts: Add requires a non-empty TenantID; use AddGlobal to mint a boundary-crossing infra actor")
	}
	return s.insertStrict(ctx, in)
}

// Upsert inserts or updates a tenant-scoped service account. If the
// account already exists it is updated (role, description) and
// reactivated if revoked. Use this only when --force is explicitly
// requested — the default path is Add, which rejects existing active
// accounts so operators notice unintentional overwrites.
func (s *Store) Upsert(ctx context.Context, in AddInput) (*ServiceAccount, error) {
	if in.TenantID == "" {
		return nil, fmt.Errorf(
			"serviceaccounts: Upsert requires a non-empty TenantID; use AddGlobal for global infra actors")
	}
	return s.insertSA(ctx, in)
}

// AddGlobal creates a tenant-less (NULL) service account. Global SAs
// bypass authorize() for every tenant — reserved for infra actors
// (system/orchestrator/qoros) installed by operators, NEVER by tenant
// admins. Callers must document the reason: a global SA is a standing
// cross-tenant credential. Returns an error when tenant_id is
// non-empty, because a global actor with a tenant is nonsense.
func (s *Store) AddGlobal(ctx context.Context, id string, role Role, description, createdBy string) (*ServiceAccount, error) {
	return s.insertSA(ctx, AddInput{
		ID: id, Role: role, Description: description, CreatedBy: createdBy,
		// TenantID intentionally empty.
	})
}

// insertStrict inserts a new service account and fails if one with the
// same id already exists and is not revoked. Revoked accounts are
// treated as absent — calling Add on a revoked id reactivates it cleanly.
func (s *Store) insertStrict(ctx context.Context, in AddInput) (*ServiceAccount, error) {
	if in.ID == "" {
		return nil, errors.New("serviceaccounts: id required")
	}
	if !validRole(in.Role) {
		return nil, fmt.Errorf("serviceaccounts: invalid role %q", in.Role)
	}
	var tenantArg any
	if in.TenantID != "" {
		tenantArg = in.TenantID
	}
	var sa ServiceAccount
	var tenantScanned *string
	err := s.pool.QueryRow(ctx, `
        INSERT INTO service_accounts (id, role, description, created_by, tenant_id)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (id) DO UPDATE
            SET role = EXCLUDED.role,
                description = EXCLUDED.description,
                tenant_id = EXCLUDED.tenant_id,
                revoked_at = NULL
            WHERE service_accounts.revoked_at IS NOT NULL
        RETURNING id, role, COALESCE(description,''), created_by, tenant_id, created_at, revoked_at
    `, in.ID, in.Role, in.Description, in.CreatedBy, tenantArg).Scan(
		&sa.ID, &sa.Role, &sa.Description, &sa.CreatedBy, &tenantScanned, &sa.CreatedAt, &sa.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAlreadyExists
		}
		return nil, fmt.Errorf("serviceaccounts: insert: %w", err)
	}
	if tenantScanned != nil {
		sa.TenantID = *tenantScanned
	}
	s.Invalidate()
	return &sa, nil
}

// insertSA is the raw upsert used by Upsert + AddGlobal. Not exported —
// callers must go through one of the named entry points so the
// tenant decision is explicit in code review.
func (s *Store) insertSA(ctx context.Context, in AddInput) (*ServiceAccount, error) {
	if in.ID == "" {
		return nil, errors.New("serviceaccounts: id required")
	}
	if !validRole(in.Role) {
		return nil, fmt.Errorf("serviceaccounts: invalid role %q", in.Role)
	}
	var tenantArg any
	if in.TenantID == "" {
		tenantArg = nil // stays NULL — global actor
	} else {
		tenantArg = in.TenantID
	}
	var sa ServiceAccount
	var tenantScanned *string
	err := s.pool.QueryRow(ctx, `
        INSERT INTO service_accounts (id, role, description, created_by, tenant_id)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (id) DO UPDATE
            SET role = EXCLUDED.role,
                description = EXCLUDED.description,
                tenant_id = EXCLUDED.tenant_id,
                revoked_at = NULL
        RETURNING id, role, COALESCE(description,''), created_by, tenant_id, created_at, revoked_at
    `, in.ID, in.Role, in.Description, in.CreatedBy, tenantArg).Scan(
		&sa.ID, &sa.Role, &sa.Description, &sa.CreatedBy, &tenantScanned, &sa.CreatedAt, &sa.RevokedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("serviceaccounts: insert: %w", err)
	}
	if tenantScanned != nil {
		sa.TenantID = *tenantScanned
	}
	s.Invalidate()
	return &sa, nil
}

// Revoke marks the account as no longer valid. Idempotent.
func (s *Store) Revoke(ctx context.Context, id, revokedBy string) error {
	if id == "" {
		return errors.New("serviceaccounts: id required")
	}
	ct, err := s.pool.Exec(ctx, `
        UPDATE service_accounts SET revoked_at = NOW()
        WHERE id = $1 AND revoked_at IS NULL
    `, id)
	if err != nil {
		return fmt.Errorf("serviceaccounts: revoke: %w", err)
	}
	_ = ct // always succeed; idempotency is the point
	s.Invalidate()
	return nil
}

// List returns every service account (including revoked ones) for admin UIs.
func (s *Store) List(ctx context.Context) ([]*ServiceAccount, error) {
	rows, err := s.pool.Query(ctx, `
        SELECT id, role, COALESCE(description,''), created_by, tenant_id, created_at, revoked_at
        FROM service_accounts ORDER BY id
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ServiceAccount{}
	for rows.Next() {
		var sa ServiceAccount
		var tenantScanned *string
		if err := rows.Scan(&sa.ID, &sa.Role, &sa.Description, &sa.CreatedBy, &tenantScanned, &sa.CreatedAt, &sa.RevokedAt); err != nil {
			return nil, err
		}
		if tenantScanned != nil {
			sa.TenantID = *tenantScanned
		}
		out = append(out, &sa)
	}
	return out, rows.Err()
}

// Get returns a single account by id, regardless of revoked state.
func (s *Store) Get(ctx context.Context, id string) (*ServiceAccount, error) {
	var sa ServiceAccount
	var tenantScanned *string
	err := s.pool.QueryRow(ctx, `
        SELECT id, role, COALESCE(description,''), created_by, tenant_id, created_at, revoked_at
        FROM service_accounts WHERE id = $1
    `, id).Scan(&sa.ID, &sa.Role, &sa.Description, &sa.CreatedBy, &tenantScanned, &sa.CreatedAt, &sa.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if tenantScanned != nil {
		sa.TenantID = *tenantScanned
	}
	return &sa, nil
}

// Refresh reloads the active cache from the DB and stores the result in
// both the local atomic snapshot and the shared cross-replica backend.
// Safe to call concurrently with Lookup.
func (s *Store) Refresh(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
        SELECT id, role, tenant_id FROM service_accounts WHERE revoked_at IS NULL
    `)
	if err != nil {
		return fmt.Errorf("serviceaccounts: refresh: %w", err)
	}
	defer rows.Close()
	byID := make(map[string]cacheEntry, 8)
	for rows.Next() {
		var id string
		var role Role
		var tenantScanned *string
		if err := rows.Scan(&id, &role, &tenantScanned); err != nil {
			return err
		}
		entry := cacheEntry{Role: role}
		if tenantScanned != nil {
			entry.TenantID = *tenantScanned
		}
		byID[id] = entry
	}
	if err := rows.Err(); err != nil {
		return err
	}
	s.local.Store(&snapshot{loadedAt: time.Now(), byID: byID})
	// Publish to shared cache so other replicas can pick it up.
	s.shared.Set(ctx, "snapshot", byID, s.RefreshInterval*2)
	return nil
}

// Invalidate drops the local snapshot and removes the shared snapshot
// so all replicas re-query the DB on their next lookup. Safe to call
// after any write that changes the active-SA set.
func (s *Store) Invalidate() {
	s.local.Store(nil)
	s.shared.Delete(context.Background(), "snapshot")
}

// Lookup reports whether the id is an active service account and, if so,
// its role. Uses the cache; refreshes lazily when stale or unset.
// Prefer LookupDetailed for callers that need the tenant binding.
func (s *Store) Lookup(ctx context.Context, id string) (Role, bool) {
	entry, ok := s.lookupEntry(ctx, id)
	if !ok {
		return "", false
	}
	return entry.Role, true
}

// LookupDetailed returns the full cache entry — role plus tenant
// binding — so authorize() can enforce tenant scoping.
func (s *Store) LookupDetailed(ctx context.Context, id string) (Role, string, bool) {
	entry, ok := s.lookupEntry(ctx, id)
	if !ok {
		return "", "", false
	}
	return entry.Role, entry.TenantID, true
}

func (s *Store) lookupEntry(ctx context.Context, id string) (cacheEntry, bool) {
	if id == "" {
		return cacheEntry{}, false
	}
	// Fast path: local in-process snapshot, still fresh.
	loc := s.local.Load()
	if loc != nil && time.Since(loc.loadedAt) <= s.RefreshInterval {
		entry, ok := loc.byID[id]
		return entry, ok
	}

	// Slow path: check the shared backend first (populated by other
	// replicas' Refresh calls). If it has a snapshot, adopt it into the
	// local atomic so subsequent lookups are fast again.
	if byID, ok := s.shared.Get(ctx, "snapshot"); ok {
		s.local.Store(&snapshot{loadedAt: time.Now(), byID: byID})
		entry, ok := byID[id]
		return entry, ok
	}

	// Shared backend is empty — query the DB directly.
	if err := s.Refresh(ctx); err == nil {
		loc = s.local.Load()
	}
	if loc == nil {
		return cacheEntry{}, false
	}
	entry, ok := loc.byID[id]
	return entry, ok
}

// IsServiceAccount is a convenience that reports active membership.
// callers that need tenant enforcement should use
// LookupDetailed and compare TenantID against the resource's tenant.
func (s *Store) IsServiceAccount(ctx context.Context, id string) bool {
	_, ok := s.Lookup(ctx, id)
	return ok
}

// RunBackground periodically refreshes the cache until ctx is done.
// Call this from the gateway bootstrap so admin changes via CLI
// propagate without waiting for the first Lookup.
func (s *Store) RunBackground(ctx context.Context) {
	if s.RefreshInterval <= 0 {
		return
	}
	t := time.NewTicker(s.RefreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = s.Refresh(ctx)
		}
	}
}

// ErrNotFound is returned by Get when no matching account exists.
var ErrNotFound = errors.New("serviceaccounts: not found")

// ErrAlreadyExists is returned by Add when an active (non-revoked)
// account with the same id already exists. Use Upsert (--force) to
// update an existing active account.
var ErrAlreadyExists = errors.New("serviceaccounts: already exists")

func validRole(r Role) bool {
	switch r {
	case RoleAdmin, RoleService, RoleOrchestrator:
		return true
	}
	return false
}

// For internal sync primitives used by tests.
var _ = sync.Mutex{}
