// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package registry_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/qorvenai/qorven/internal/plugins/registry"
	"github.com/qorvenai/qorven/internal/store"
	"github.com/qorvenai/qorven/internal/testutil"
)

// Tests run against the bypass pool — the Store uses store.FromContext
// which falls back to the pool when no tx is in ctx. RLS enforcement
// is covered separately in TestStore_RLS_BlocksCrossTenantRead.

func TestStore_Upload_InsertsAndGeneratesSHA256(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	s := registry.NewStore(pool)

	bin := []byte("pretend-wasm-bytes")
	p, err := s.Upload(ctx, registry.UploadInput{
		TenantID:    tenantID,
		Name:        "hello",
		Description: "greets the world",
		WasmBinary:  bin,
		Parameters:  json.RawMessage(`{"type":"object"}`),
		CreatedBy:   "admin",
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if p.SHA256 == "" {
		t.Fatalf("sha256 not computed")
	}
	want := sha256.Sum256(bin)
	if p.SHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("sha256 mismatch: got %s want %s", p.SHA256, hex.EncodeToString(want[:]))
	}
	if p.TenantID != tenantID {
		t.Fatalf("tenant: got %s want %s", p.TenantID, tenantID)
	}
	// Postgres JSONB reformats on round-trip (adds spaces, orders keys).
	// Compare semantically, not byte-for-byte.
	var got map[string]any
	if err := json.Unmarshal(p.Parameters, &got); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if got["type"] != "object" {
		t.Fatalf("params[type]=%v, want object", got["type"])
	}
}

// TestStore_Upload_RejectsReservedName is the Phase 6 security gate
// at the Store layer. A tenant (or an AI-generated plugin pipeline)
// attempting to register under a platform-reserved name MUST be
// refused before the DB row lands. The Loader has a second-layer
// check but we reject here so no row is ever written.
func TestStore_Upload_RejectsReservedName(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	s := registry.NewStore(pool)

	// Spot-check a destructive name and an orchestration name — both
	// classes MUST be rejected.
	for _, reservedName := range []string{"exec", "room_post", "gh_push_file"} {
		_, err := s.Upload(ctx, registry.UploadInput{
			TenantID:   tenantID,
			Name:       reservedName,
			WasmBinary: []byte("x"),
		})
		if !errors.Is(err, registry.ErrReservedName) {
			t.Errorf("reserved name %q: got err=%v want ErrReservedName", reservedName, err)
		}
	}
}

func TestStore_Upload_RejectsInvalidName(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	s := registry.NewStore(pool)

	bad := []string{
		"", "1starts_with_digit", "BadCase", "has-dash",
		"has.dot", strings.Repeat("x", 64), // too long
	}
	for _, name := range bad {
		_, err := s.Upload(ctx, registry.UploadInput{
			TenantID: tenantID, Name: name, WasmBinary: []byte("x"),
		})
		if !errors.Is(err, registry.ErrInvalidName) {
			t.Fatalf("name %q: got err=%v want ErrInvalidName", name, err)
		}
	}
}

func TestStore_Upload_SameHashIsNoOp(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	s := registry.NewStore(pool)

	bin := []byte("same-bytes")
	p1, err := s.Upload(ctx, registry.UploadInput{TenantID: tenantID, Name: "t", WasmBinary: bin})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	p2, err := s.Upload(ctx, registry.UploadInput{TenantID: tenantID, Name: "t", WasmBinary: bin})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if p1.ID != p2.ID {
		t.Fatalf("idempotent upload should return same row; got %s vs %s", p1.ID, p2.ID)
	}
}

func TestStore_Upload_NewHashRevokesPrevious(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	s := registry.NewStore(pool)

	p1, _ := s.Upload(ctx, registry.UploadInput{TenantID: tenantID, Name: "t", WasmBinary: []byte("v1")})
	p2, err := s.Upload(ctx, registry.UploadInput{TenantID: tenantID, Name: "t", WasmBinary: []byte("v2")})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if p1.ID == p2.ID {
		t.Fatalf("new binary should produce a new row")
	}

	// p1 must now be revoked; GetActiveByName returns p2.
	active, err := s.GetActiveByName(ctx, tenantID, "t")
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if active.ID != p2.ID {
		t.Fatalf("active is %s, want %s (the new upload)", active.ID, p2.ID)
	}

	// Audit: the old row is still there, revoked_at set.
	var revoked *string
	if err := pool.QueryRow(ctx,
		`SELECT revoked_at::text FROM wasm_plugins WHERE id = $1`, p1.ID,
	).Scan(&revoked); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if revoked == nil || *revoked == "" {
		t.Fatalf("previous row must be revoked; got nil")
	}
}

func TestStore_GetActive_ReturnsNotFoundWhenRevoked(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	s := registry.NewStore(pool)

	_, _ = s.Upload(ctx, registry.UploadInput{TenantID: tenantID, Name: "g", WasmBinary: []byte("b")})
	if err := s.Revoke(ctx, tenantID, "g"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	_, err := s.GetActiveByName(ctx, tenantID, "g")
	if !errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestStore_ListActive_ScopesByTenant(t *testing.T) {
	pool, tenantA := testutil.NewIsolatedTenant(t)
	_, tenantB := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	s := registry.NewStore(pool)

	_, _ = s.Upload(ctx, registry.UploadInput{TenantID: tenantA, Name: "a1", WasmBinary: []byte("a1")})
	_, _ = s.Upload(ctx, registry.UploadInput{TenantID: tenantA, Name: "a2", WasmBinary: []byte("a2")})
	_, _ = s.Upload(ctx, registry.UploadInput{TenantID: tenantB, Name: "b1", WasmBinary: []byte("b1")})

	aPlugins, err := s.ListActive(ctx, tenantA)
	if err != nil {
		t.Fatalf("list A: %v", err)
	}
	if len(aPlugins) != 2 {
		t.Fatalf("tenant A should see 2 plugins, got %d", len(aPlugins))
	}
	for _, p := range aPlugins {
		if p.TenantID != tenantA {
			t.Fatalf("cross-tenant pollution: tenant A list includes %s", p.TenantID)
		}
	}

	bPlugins, _ := s.ListActive(ctx, tenantB)
	if len(bPlugins) != 1 || bPlugins[0].Name != "b1" {
		t.Fatalf("tenant B should see 1 plugin b1, got %+v", bPlugins)
	}
}

// TestStore_RLS_BlocksCrossTenantRead proves the DB-level backstop.
// Even if a bug let tenant A's context try to read tenant B's
// plugins by id, the RLS policy denies.
func TestStore_RLS_BlocksCrossTenantRead(t *testing.T) {
	pool, tenantA := testutil.NewIsolatedTenant(t)
	_, tenantB := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	s := registry.NewStore(pool)
	bPlugin, _ := s.Upload(ctx, registry.UploadInput{
		TenantID: tenantB, Name: "b_only", WasmBinary: []byte("b"),
	})

	// Now connect as the restricted role and scope to tenant A.
	dsn := rlsRestrictedDSN(t)
	db, err := store.NewForMultiTenant(dsn)
	if err != nil {
		t.Fatalf("NewForMultiTenant: %v", err)
	}
	defer db.Close()

	var seen bool
	err = db.WithTenantTx(ctx, tenantA, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id::text FROM wasm_plugins WHERE id = $1::uuid`, bPlugin.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			seen = true
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("WithTenantTx: %v", err)
	}
	if seen {
		t.Fatalf("tenant A saw tenant B's plugin row — RLS is broken")
	}
}

// rlsRestrictedDSN mirrors the helper in internal/store/rls_test.go.
// Kept local so we don't export a testonly helper for this single use.
func rlsRestrictedDSN(t *testing.T) string {
	t.Helper()
	base := testutil.TestDSN
	if i := strings.Index(base, "://"); i >= 0 {
		if j := strings.Index(base[i+3:], "@"); j >= 0 {
			return base[:i+3] + "qorven_app:qorven_app" + base[i+3+j:]
		}
	}
	t.Fatalf("cannot derive restricted DSN")
	return ""
}
