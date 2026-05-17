// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/plugins/wasm"
	"github.com/qorvenai/qorven/internal/tools"
)

// Loader bridges the persistent plugin registry (Postgres) with the
// live Wasm host (in-memory, shared across the process). At plan
// execution time:
//
//   1. Orchestrator asks the Loader for the tenant's active plugins.
//   2. Loader reads the DB via Store.ListActive (ctx-scoped; RLS
//      filters cross-tenant rows).
//   3. For each plugin, the Loader verifies the stored sha256 still
//      matches the row's wasm_binary (defense in depth against
//      column-level corruption / audit-log tampering).
//   4. If the plugin hasn't been compiled into the Wasm host under
//      its tenant-scoped name, Loader compiles it now. Re-compiling
//      is cheap when the sha already matches an existing compilation.
//   5. Each compiled plugin is wrapped as a permissions-gated
//      tools.Tool and returned for registration.
//
// ## Why tenant-scoped compilation names
//
// Wasm plugins live under a tenant/tenant-name composite key in the
// Wasm host (`<tenant>:<name>`). Two tenants can upload a plugin
// called "search" without collision, and Invoke attributes the call
// to the correct compiled module. This also means sha256 differences
// between tenants are always distinguished at the Host layer — a
// compromised tenant cannot trick the host into running tenant B's
// binary.
//
// ## Plugin gating
//
// Every plugin tool is wrapped with permissions.WrapLazy. Users
// cannot upload a plugin that skips the permission gate. This is
// non-negotiable — Wasm sandboxes the CPU/memory surface, but the
// plugin could still trigger a tool that makes external calls via
// the orchestrator's context. The gate ensures the user sees and
// approves the invocation.
type Loader struct {
	store *Store
	host  *wasm.Host
	gate  func() *permissions.Gate
	log   *slog.Logger

	// compiled tracks which tenant-scoped module names have been
	// loaded into host already, along with their sha256. A mismatch
	// triggers re-compile; a match is a fast no-op.
	mu       sync.Mutex
	compiled map[string]string // scoped_name → sha256
}

// NewLoader wires the loader to an existing Store + Wasm host + a
// permission-gate resolver.
//
// gateGetter is a closure so the loader works correctly when the
// permission.Gate is built AFTER the loader (matches the lazy wiring
// the gateway uses for the built-in tools).
func NewLoader(store *Store, host *wasm.Host, gateGetter func() *permissions.Gate, logger *slog.Logger) *Loader {
	if logger == nil {
		logger = slog.Default()
	}
	return &Loader{
		store:    store,
		host:     host,
		gate:     gateGetter,
		log:      logger,
		compiled: map[string]string{},
	}
}

// ToolsForTenant returns one tools.Tool per active plugin in the
// tenant. Caller registers these on a scratch Registry passed to the
// tool runner — NOT the gateway's shared Registry — so plugins from
// tenant A cannot leak into tenant B's execution context.
//
// The returned slice is owned by the caller; calling ToolsForTenant
// twice for the same tenant produces two independent slices (but the
// underlying Wasm compilation is shared — see scopedName).
func (l *Loader) ToolsForTenant(ctx context.Context, tenantID string) ([]tools.Tool, error) {
	if l == nil {
		return nil, nil
	}
	if tenantID == "" {
		return nil, errors.New("registry.Loader: tenant_id required")
	}

	plugins, err := l.store.ListActive(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("registry.Loader: list: %w", err)
	}
	if len(plugins) == 0 {
		return nil, nil
	}

	out := make([]tools.Tool, 0, len(plugins))
	for _, p := range plugins {
		// Phase 6 security gate. Belt-and-braces against rows that
		// predate the reserved list: Upload now refuses them, but a
		// historical row is still in the DB. Refuse to load it and
		// record the skip — the operator should revoke + rename.
		if tools.IsReservedCoreToolName(p.Name) {
			l.log.Warn("registry.Loader: refusing to load plugin whose name shadows a reserved core tool",
				"tenant_id", p.TenantID, "plugin", p.Name)
			continue
		}
		scoped := scopedName(p.TenantID, p.Name)
		if err := l.ensureCompiled(ctx, scoped, p); err != nil {
			l.log.Error("registry.Loader: compile failed",
				"tenant_id", p.TenantID, "plugin", p.Name, "err", err)
			// A broken plugin should not block other plugins or the
			// overall plan run. Skip it and continue.
			continue
		}

		// Parse the parameters JSON into the shape wasm.ToolDescriptor
		// expects. Malformed params cause the tool to be skipped — an
		// uploadable-only operator should not be able to mint a plugin
		// that crashes the LLM registry listing.
		var params map[string]any
		if err := json.Unmarshal(p.Parameters, &params); err != nil || params == nil {
			l.log.Warn("registry.Loader: plugin has invalid params; skipping",
				"tenant_id", p.TenantID, "plugin", p.Name, "err", err)
			continue
		}

		desc := wasm.ToolDescriptor{
			Name:        p.Name,
			Description: p.Description,
			Parameters:  params,
		}
		bridge := wasm.NewBridgeTool(l.host, scoped, desc)

		// Gate every plugin tool. This is the single-line invariant
		// that prevents a plugin from skipping permission checks
		// regardless of what a malicious upload tries to claim.
		wrapped := permissions.WrapLazy(l.gate, bridge, permissions.GatedToolOptions{
			Reason:      "Executes user-uploaded Wasm plugin " + p.Name,
			RequestedBy: "agent",
		})
		out = append(out, wrapped)
	}
	return out, nil
}

// ensureCompiled compiles the plugin into the Wasm host if needed.
// If the stored plugin's sha256 doesn't match its bytes, the load
// is refused — defense in depth against a sha/column desync.
func (l *Loader) ensureCompiled(ctx context.Context, scoped string, p *Plugin) error {
	// Verify sha256 vs bytes. A mismatch is a data-integrity issue
	// we must NOT silently paper over — a compromised admin could
	// tamper with wasm_binary directly and this catches it.
	actual := sha256.Sum256(p.WasmBinary)
	actualHex := hex.EncodeToString(actual[:])
	if actualHex != p.SHA256 {
		return fmt.Errorf("sha256 mismatch: stored=%s actual=%s", p.SHA256, actualHex)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if existing, ok := l.compiled[scoped]; ok && existing == p.SHA256 {
		return nil // already loaded, same hash — fast path
	}

	// LoadPlugin under the tenant-scoped name. If the name already
	// exists in the host under a different sha, LoadPlugin replaces
	// the compilation (see wasm.Host.LoadPlugin — idempotent replace).
	if err := l.host.LoadPlugin(ctx, scoped, p.WasmBinary); err != nil {
		return err
	}
	l.compiled[scoped] = p.SHA256
	return nil
}

// Invalidate drops the local compilation cache for a tenant+name
// pair. HTTP handlers call this after Revoke / Upload so the next
// ToolsForTenant run re-reads the DB instead of serving a stale
// compilation.
//
// Note: this does NOT remove the module from the Wasm host. The host
// keeps the compiled artifact until UnloadPlugin is called or the
// process exits. That's intentional — two tenants with the same
// upload hash will share the in-host compilation. Per-tenant
// UnloadPlugin would require a refcount; out of scope here.
func (l *Loader) Invalidate(tenantID, name string) {
	if l == nil || tenantID == "" || name == "" {
		return
	}
	scoped := scopedName(tenantID, name)
	l.mu.Lock()
	delete(l.compiled, scoped)
	l.mu.Unlock()
}

// scopedName is the tenant-qualified plugin name the Wasm host knows.
// Keep this a pure function — the tenant UUID goes in as-is so naming
// collisions are literally impossible across tenants.
func scopedName(tenantID, name string) string {
	return tenantID + ":" + name
}
