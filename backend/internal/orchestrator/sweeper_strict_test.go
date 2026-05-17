// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package orchestrator_test

import (
	"context"
	"strings"
	"testing"

	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/orchestrator"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/testutil"
)

// TestSweeper_StrictMode_RefusesUnscopedRun proves P3D-05: a sweeper
// constructed with RequireTenantScope=true must refuse to run without
// a concrete TenantScope. This is the multi-tenant safety net — a
// single global sweeper would scan every tenant's plans.
func TestSweeper_StrictMode_RefusesUnscopedRun(t *testing.T) {
	pool, _ := testutil.NewIsolatedTenant(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)
	svc := orchestrator.NewService(ps, as, &scriptedAgent{responses: map[string]string{}}, apievents.NewEmitter(), nil)
	sw := orchestrator.NewSweeper(pool, svc, nil)
	sw.RequireTenantScope = true
	// TenantScope is intentionally empty.

	_, err := sw.Run(context.Background())
	if err == nil {
		t.Fatalf("expected error when RequireTenantScope=true and TenantScope is empty")
	}
	if !strings.Contains(err.Error(), "TenantScope is required") {
		t.Fatalf("expected TenantScope-required error, got: %v", err)
	}
}

// TestSweeper_StrictMode_AllowsExplicitGlobalOverride proves the
// TenantScope="*" admin escape hatch. An operator running a global
// recovery sweep from a CLI must be able to opt in explicitly.
func TestSweeper_StrictMode_AllowsExplicitGlobalOverride(t *testing.T) {
	pool, _ := testutil.NewIsolatedTenant(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)
	svc := orchestrator.NewService(ps, as, &scriptedAgent{responses: map[string]string{}}, apievents.NewEmitter(), nil)
	sw := orchestrator.NewSweeper(pool, svc, nil)
	sw.RequireTenantScope = true
	sw.TenantScope = "*"

	// Expect no TenantScope-required error. Actual resumption count
	// depends on concurrent tests but "no error" is the assertion.
	_, err := sw.Run(context.Background())
	if err != nil {
		t.Fatalf("expected no strict-mode error with TenantScope='*', got: %v", err)
	}
}

// TestSweeper_StrictMode_AllowsTenantScopedRun proves the normal
// multi-tenant operation: one sweeper per tenant.
func TestSweeper_StrictMode_AllowsTenantScopedRun(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)
	svc := orchestrator.NewService(ps, as, &scriptedAgent{responses: map[string]string{}}, apievents.NewEmitter(), nil)
	sw := orchestrator.NewSweeper(pool, svc, nil)
	sw.RequireTenantScope = true
	sw.TenantScope = tenantID

	if _, err := sw.Run(context.Background()); err != nil {
		t.Fatalf("tenant-scoped strict-mode run should succeed: %v", err)
	}
}
