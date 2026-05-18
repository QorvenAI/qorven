// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package permissions_test

import (
	"context"
	"testing"

	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/testutil"
)

func TestGate_HasPolicy_AgentScoped(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	g := permissions.NewGate(pool, nil)
	ctx := context.Background()

	userID  := testutil.NewIsolatedUser(t, pool, tenantID)
	agentID := testutil.NewIsolatedAgent(t, pool, tenantID)

	if err := g.SetPolicyScoped(ctx, tenantID, userID, agentID, "cron", permissions.ScopeAutoApproved); err != nil {
		t.Fatalf("SetPolicyScoped: %v", err)
	}

	if !g.HasPolicyForAgent(ctx, tenantID, userID, agentID, "cron", permissions.ScopeAutoApproved) {
		t.Fatal("HasPolicyForAgent should return true after SetPolicyScoped")
	}

	otherAgentID := testutil.NewIsolatedAgent(t, pool, tenantID)
	if g.HasPolicyForAgent(ctx, tenantID, userID, otherAgentID, "cron", permissions.ScopeAutoApproved) {
		t.Fatal("HasPolicyForAgent should return false for different agent")
	}
}

func TestGate_ScopeBlocked_ShortCircuitsExecution(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	g := permissions.NewGate(pool, nil)
	ctx := context.Background()

	userID  := testutil.NewIsolatedUser(t, pool, tenantID)
	agentID := testutil.NewIsolatedAgent(t, pool, tenantID)

	if err := g.SetPolicyScoped(ctx, tenantID, userID, agentID, "exec", permissions.ScopeBlocked); err != nil {
		t.Fatalf("SetPolicyScoped: %v", err)
	}

	if !g.IsPolicyBlocked(ctx, tenantID, userID, agentID, "exec") {
		t.Fatal("IsPolicyBlocked should return true after setting ScopeBlocked")
	}
}

func TestGate_ListPolicies(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	g := permissions.NewGate(pool, nil)
	ctx := context.Background()

	userID  := testutil.NewIsolatedUser(t, pool, tenantID)
	agentID := testutil.NewIsolatedAgent(t, pool, tenantID)

	_ = g.SetPolicyScoped(ctx, tenantID, userID, agentID, "cron", permissions.ScopeAutoApproved)
	_ = g.SetPolicyScoped(ctx, tenantID, userID, agentID, "exec", permissions.ScopeBlocked)

	policies, err := g.ListPolicies(ctx, tenantID, agentID)
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	if len(policies) < 2 {
		t.Fatalf("expected at least 2 policies, got %d", len(policies))
	}
}
