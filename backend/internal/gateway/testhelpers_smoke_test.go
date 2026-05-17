// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"testing"

	"github.com/qorvenai/qorven/internal/deployment"
	"github.com/qorvenai/qorven/internal/testutil"
)

// TestMinimalGateway_AuthServiceRoundTrip directly exercises the
// ruling's Step 2 item #3 concern: the auth service wiring MUST be
// covered by the minimal gateway helper, because that's the bootstrap
// path most likely to silently corrupt behavior before buildV1Router
// runs. We construct the helper and verify IssueToken → ValidateToken
// round-trips end-to-end against the helper's configured JWT secret.
func TestMinimalGateway_AuthServiceRoundTrip(t *testing.T) {
	gw, _, tenantID := newMinimalGateway(t, MinimalGatewayOpts{})

	if gw.authSvc == nil {
		t.Fatalf("minimal gateway must configure authSvc")
	}
	if gw.sessions == nil {
		t.Fatalf("minimal gateway must configure sessions")
	}
	if gw.deploymentConfig == nil {
		t.Fatalf("minimal gateway must configure deploymentConfig")
	}

	uniq := testutil.TempID("auth-rt")
	u, err := gw.authSvc.CreateUser(
		context.Background(),
		"auth-"+uniq,
		"pw-"+uniq,
		"auth@example.test",
		"user",
		tenantID,
	)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	token := gw.authSvc.IssueToken(u)
	if token == "" {
		t.Fatalf("IssueToken returned empty")
	}
	back, err := gw.authSvc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if back.ID != u.ID {
		t.Fatalf("roundtrip id: got %s want %s", back.ID, u.ID)
	}
	if back.TenantID != tenantID {
		t.Fatalf("roundtrip tenant: got %s want %s", back.TenantID, tenantID)
	}
}

// TestMinimalGateway_DeploymentModeDefault asserts the single-tenant
// default is preserved (the ruling's core non-negotiable for Step 3).
func TestMinimalGateway_DeploymentModeDefault(t *testing.T) {
	gw, _, _ := newMinimalGateway(t, MinimalGatewayOpts{})
	if gw.deploymentConfig.IsMultiTenant(context.Background()) {
		t.Fatalf("default mode must be single_tenant; got multi")
	}
}

// TestMinimalGateway_DeploymentModeOverride proves the helper can flip
// to multi-tenant for tests that need to exercise strict rules.
func TestMinimalGateway_DeploymentModeOverride(t *testing.T) {
	gw, _, _ := newMinimalGateway(t, MinimalGatewayOpts{
		DeploymentMode: deployment.ModeMultiTenant,
	})
	if !gw.deploymentConfig.IsMultiTenant(context.Background()) {
		t.Fatalf("override failed: still single_tenant")
	}
}
