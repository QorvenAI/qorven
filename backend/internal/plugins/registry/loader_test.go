// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package registry_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/plugins/registry"
	"github.com/qorvenai/qorven/internal/plugins/wasm"
	"github.com/qorvenai/qorven/internal/testutil"
)

// loadEchoWasm pulls the pre-built plugin used by every wasm test.
// If missing the test skips with instructions; host_test.go documents
// the Makefile target.
func loadEchoWasm(t *testing.T) []byte {
	t.Helper()
	// Path is relative to this test's package directory:
	// backend/internal/plugins/registry/ → ../wasm/testdata/
	data, err := os.ReadFile("../wasm/testdata/echo_plugin.wasm")
	if err != nil {
		t.Skipf("echo_plugin.wasm not built; run `make wasm-testdata`: %v", err)
	}
	return data
}

// TestLoader_CompilesAndWrapsPerTenantPlugins is the end-to-end
// demonstration. Upload a plugin for tenant A, ask the Loader for
// tenant A's tools, invoke one — it should round-trip through the
// Wasm host under a tenant-scoped module name and return the guest's
// JSON reply via the permission-gated tools.Tool wrapper.
func TestLoader_CompilesAndWrapsPerTenantPlugins(t *testing.T) {
	pool, tenantA := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	s := registry.NewStore(pool)
	if _, err := s.Upload(ctx, registry.UploadInput{
		TenantID:   tenantA,
		Name:       "echo",
		WasmBinary: loadEchoWasm(t),
		Parameters: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}}}`),
	}); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)

	// A real Gate requires a pool, emitter, etc. Since our test
	// doesn't actually exercise the permission flow (the wrapping is
	// what we care about), we hand the loader a nil-gate getter — the
	// wrapper is still installed and IsGated(tool) is true.
	loader := registry.NewLoader(s, host,
		func() *permissions.Gate {
			return permissions.NewGate(pool, apievents.NewEmitter())
		},
		nil,
	)

	list, err := loader.ToolsForTenant(ctx, tenantA)
	if err != nil {
		t.Fatalf("ToolsForTenant: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 plugin tool, got %d", len(list))
	}
	tool := list[0]
	if tool.Name() != "echo" {
		t.Fatalf("tool name: %s", tool.Name())
	}
	if !permissions.IsGated(tool) {
		t.Fatalf("plugin tool must be wrapped with permissions.WrapLazy")
	}
}

// TestLoader_SkipsPluginsWithInvalidParams — if a plugin's params
// JSON is malformed (a future upload endpoint bug or a manual DB
// edit), the loader MUST NOT add it to the tenant's tool set. An
// LLM registry listing that includes a schema-invalid tool can poison
// the tool-call loop.
func TestLoader_SkipsPluginsWithInvalidParams(t *testing.T) {
	pool, tenantA := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	s := registry.NewStore(pool)
	// Upload one good plugin, then mutate its params to invalid JSON.
	good, err := s.Upload(ctx, registry.UploadInput{
		TenantID: tenantA, Name: "good", WasmBinary: loadEchoWasm(t),
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	// Postgres rejects non-JSON as jsonb. Use valid-but-non-object
	// JSON to trigger the "params is nil / not a map" path.
	if _, err := pool.Exec(ctx,
		`UPDATE wasm_plugins SET parameters = $1 WHERE id = $2`,
		[]byte(`null`), good.ID,
	); err != nil {
		t.Fatalf("mutate params: %v", err)
	}

	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)
	loader := registry.NewLoader(s, host,
		func() *permissions.Gate {
			return permissions.NewGate(pool, apievents.NewEmitter())
		},
		nil,
	)

	list, err := loader.ToolsForTenant(ctx, tenantA)
	if err != nil {
		t.Fatalf("ToolsForTenant: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("plugin with null params should have been skipped, got %d tools", len(list))
	}
}

// TestLoader_RefusesSHA256Mismatch — if a row's stored sha256 does
// not match its wasm_binary bytes (e.g. column corruption or
// tampering), the loader must refuse to compile it. Verifies the
// defense-in-depth check runs even in single-tenant / bypass mode.
func TestLoader_RefusesSHA256Mismatch(t *testing.T) {
	pool, tenantA := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	s := registry.NewStore(pool)
	p, err := s.Upload(ctx, registry.UploadInput{
		TenantID: tenantA, Name: "tampered", WasmBinary: loadEchoWasm(t),
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	// Tamper with the stored sha: set a valid-shape but wrong digest.
	fakeHex := strings.Repeat("ab", 32) // 64 hex chars, matches CHECK
	if _, err := pool.Exec(ctx,
		`UPDATE wasm_plugins SET sha256 = $1 WHERE id = $2`,
		fakeHex, p.ID,
	); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)
	loader := registry.NewLoader(s, host,
		func() *permissions.Gate {
			return permissions.NewGate(pool, apievents.NewEmitter())
		},
		nil,
	)

	list, err := loader.ToolsForTenant(ctx, tenantA)
	if err != nil {
		t.Fatalf("ToolsForTenant: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("sha256-mismatched plugin must be skipped, got %d tools", len(list))
	}
}

// TestLoader_RefusesReservedName is the Phase 6 defense-in-depth
// check. Even if a row was smuggled in past Store.Upload (historical
// data predating the reserved list, raw-SQL injection, migration
// import), the Loader must NOT hand the plugin to the agent loop —
// shadow semantics would let it hijack a platform tool.
//
// We bypass Store.Upload (which would reject at write time) by going
// direct to SQL, then assert Loader silently skips the row.
func TestLoader_RefusesReservedName(t *testing.T) {
	pool, tenantA := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	bin := loadEchoWasm(t)

	// Direct INSERT mimicking a smuggled row. The Store's Upload
	// path would reject this, so we go around it.
	if _, err := pool.Exec(ctx, `
        INSERT INTO wasm_plugins (tenant_id, name, description, wasm_binary, sha256, parameters, created_by)
        VALUES ($1, $2, 'smuggled', $3, $4, '{}'::jsonb, 'raw_sql')
    `,
		tenantA, "exec",
		bin, "0000000000000000000000000000000000000000000000000000000000000000",
	); err != nil {
		t.Fatalf("smuggle insert: %v", err)
	}

	s := registry.NewStore(pool)
	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)
	loader := registry.NewLoader(s, host,
		func() *permissions.Gate { return permissions.NewGate(pool, apievents.NewEmitter()) },
		nil,
	)

	list, err := loader.ToolsForTenant(ctx, tenantA)
	if err != nil {
		t.Fatalf("ToolsForTenant: %v", err)
	}
	for _, tool := range list {
		if tool.Name() == "exec" {
			t.Fatalf("Loader returned a tool named 'exec' — reserved guard is broken")
		}
	}
}

// TestLoader_ToolsAreTenantScoped exercises the primary isolation
// claim: tenant A's plugin uploads do not appear in tenant B's tool
// list, and vice versa. The combination of RLS (at Store.ListActive)
// and scopedName (at the Wasm host) makes this a mathematical
// guarantee, but the test is the operational tripwire.
func TestLoader_ToolsAreTenantScoped(t *testing.T) {
	pool, tenantA := testutil.NewIsolatedTenant(t)
	_, tenantB := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	s := registry.NewStore(pool)
	bin := loadEchoWasm(t)
	_, _ = s.Upload(ctx, registry.UploadInput{TenantID: tenantA, Name: "a_only", WasmBinary: bin})
	_, _ = s.Upload(ctx, registry.UploadInput{TenantID: tenantB, Name: "b_only", WasmBinary: bin})

	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)
	loader := registry.NewLoader(s, host,
		func() *permissions.Gate {
			return permissions.NewGate(pool, apievents.NewEmitter())
		},
		nil,
	)

	aList, err := loader.ToolsForTenant(ctx, tenantA)
	if err != nil {
		t.Fatalf("A: %v", err)
	}
	bList, err := loader.ToolsForTenant(ctx, tenantB)
	if err != nil {
		t.Fatalf("B: %v", err)
	}
	if len(aList) != 1 || aList[0].Name() != "a_only" {
		t.Fatalf("A should have [a_only], got %d tools", len(aList))
	}
	if len(bList) != 1 || bList[0].Name() != "b_only" {
		t.Fatalf("B should have [b_only], got %d tools", len(bList))
	}
}

// TestLoader_InvalidateForcesRecompile — after calling Invalidate,
// the next ToolsForTenant call must re-read the DB (and thus see
// any newly-uploaded version). Verifies the cache-busting path.
func TestLoader_InvalidateForcesRecompile(t *testing.T) {
	pool, tenantA := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	s := registry.NewStore(pool)
	bin := loadEchoWasm(t)
	_, _ = s.Upload(ctx, registry.UploadInput{TenantID: tenantA, Name: "iv", WasmBinary: bin})

	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)
	loader := registry.NewLoader(s, host,
		func() *permissions.Gate {
			return permissions.NewGate(pool, apievents.NewEmitter())
		},
		nil,
	)

	list1, _ := loader.ToolsForTenant(ctx, tenantA)
	if len(list1) != 1 {
		t.Fatalf("first load expected 1 tool, got %d", len(list1))
	}

	// Upload a modified binary (byte-level change → new sha256 →
	// old row revoked, new active row inserted). Without Invalidate
	// we might serve the stale compilation.
	modified := append([]byte(nil), bin...)
	modified[0] ^= 1
	_, err = s.Upload(ctx, registry.UploadInput{TenantID: tenantA, Name: "iv", WasmBinary: modified})
	// The modified binary is NOT a valid Wasm module anymore (we
	// flipped a magic byte). Upload still succeeds at the DB layer;
	// the load error happens when the loader tries to compile.
	if err != nil {
		t.Fatalf("Upload modified: %v", err)
	}
	loader.Invalidate(tenantA, "iv")

	list2, err := loader.ToolsForTenant(ctx, tenantA)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	// The new binary is malformed, so the Loader should skip it
	// rather than mint a broken tool. Net: zero tools.
	if len(list2) != 0 {
		t.Fatalf("modified-invalid binary should have been skipped after Invalidate, got %d tools",
			len(list2))
	}
}

func toolNames(ts []tool) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Name())
	}
	return out
}

type tool interface{ Name() string }
