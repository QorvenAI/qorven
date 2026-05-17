// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package discussion_test

import (
	"context"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/discussion"
	"github.com/qorvenai/qorven/internal/testutil"
)

func TestDiscussionStore(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert a real agent row so discussions.agent_id FK is satisfiable.
	var agentID string
	err := pool.QueryRow(ctx,
		`INSERT INTO agents (tenant_id, agent_key, model)
         VALUES ($1, $2, 'default') RETURNING id`,
		tenantID, "test-agent",
	).Scan(&agentID)
	if err != nil {
		t.Fatalf("insert agent: %v", err)
	}

	store := discussion.NewStore(pool)

	t.Run("Create and Get", func(t *testing.T) {
		id, err := store.Create(ctx, discussion.Discussion{
			TenantID: tenantID,
			AgentID:  agentID,
			AILabel:  "Debugging the login flow",
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if id == "" {
			t.Fatal("Create returned empty id")
		}

		d, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if d.AILabel != "Debugging the login flow" {
			t.Errorf("AILabel = %q, want %q", d.AILabel, "Debugging the login flow")
		}
		if d.TenantID != tenantID {
			t.Errorf("TenantID = %q, want %q", d.TenantID, tenantID)
		}
		if d.AgentID != agentID {
			t.Errorf("AgentID = %q, want %q", d.AgentID, agentID)
		}
		if d.UserLabel != nil {
			t.Errorf("UserLabel should be nil on creation, got %q", *d.UserLabel)
		}
		// Label() must fall back to ai_label when user_label is nil.
		if d.Label() != "Debugging the login flow" {
			t.Errorf("Label() = %q, want ai_label", d.Label())
		}
	})

	t.Run("SetUserLabel and Label precedence", func(t *testing.T) {
		id, err := store.Create(ctx, discussion.Discussion{
			TenantID: tenantID,
			AgentID:  agentID,
			AILabel:  "AI generated label",
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if err := store.SetUserLabel(ctx, tenantID, id, "My custom label"); err != nil {
			t.Fatalf("SetUserLabel: %v", err)
		}

		d, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get after SetUserLabel: %v", err)
		}
		if d.UserLabel == nil {
			t.Fatal("UserLabel should be set after SetUserLabel")
		}
		if *d.UserLabel != "My custom label" {
			t.Errorf("UserLabel = %q, want %q", *d.UserLabel, "My custom label")
		}
		// User label must take precedence over ai_label.
		if d.Label() != "My custom label" {
			t.Errorf("Label() = %q, want user label", d.Label())
		}
	})

	t.Run("ListForAgent", func(t *testing.T) {
		// Create two discussions for this agent.
		_, err := store.Create(ctx, discussion.Discussion{
			TenantID: tenantID,
			AgentID:  agentID,
			AILabel:  "First discussion",
		})
		if err != nil {
			t.Fatalf("Create 1: %v", err)
		}
		_, err = store.Create(ctx, discussion.Discussion{
			TenantID: tenantID,
			AgentID:  agentID,
			AILabel:  "Second discussion",
		})
		if err != nil {
			t.Fatalf("Create 2: %v", err)
		}

		list, err := store.ListForAgent(ctx, tenantID, agentID, 100)
		if err != nil {
			t.Fatalf("ListForAgent: %v", err)
		}
		// We created at least 2 here plus potentially more from prior sub-tests;
		// assert at least 2 are returned.
		if len(list) < 2 {
			t.Errorf("ListForAgent returned %d discussions, want >= 2", len(list))
		}
		// All returned rows must belong to our agent.
		for _, d := range list {
			if d.AgentID != agentID {
				t.Errorf("unexpected AgentID %q in list", d.AgentID)
			}
		}
	})

	t.Run("Touch increments message_count", func(t *testing.T) {
		id, err := store.Create(ctx, discussion.Discussion{
			TenantID: tenantID,
			AgentID:  agentID,
			AILabel:  "Touch test discussion",
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		d0, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get before Touch: %v", err)
		}
		if d0.MessageCount != 0 {
			t.Errorf("initial MessageCount = %d, want 0", d0.MessageCount)
		}

		if err := store.Touch(ctx, tenantID, id); err != nil {
			t.Fatalf("Touch 1: %v", err)
		}
		if err := store.Touch(ctx, tenantID, id); err != nil {
			t.Fatalf("Touch 2: %v", err)
		}

		d1, err := store.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get after Touch: %v", err)
		}
		if d1.MessageCount != 2 {
			t.Errorf("MessageCount after 2 Touches = %d, want 2", d1.MessageCount)
		}
	})

	t.Run("ListForAgent_cross_tenant_isolation", func(t *testing.T) {
		// Create a second isolated tenant with its own agent.
		pool2, tenant2ID := testutil.NewIsolatedTenant(t)
		var agent2ID string
		if err := pool2.QueryRow(ctx,
			`INSERT INTO agents (tenant_id, agent_key, model)
             VALUES ($1, $2, 'default') RETURNING id`,
			tenant2ID, "test-agent-2",
		).Scan(&agent2ID); err != nil {
			t.Fatalf("insert agent2: %v", err)
		}

		// Create a discussion under tenant2/agent2.
		store2 := discussion.NewStore(pool2)
		_, err := store2.Create(ctx, discussion.Discussion{
			TenantID: tenant2ID,
			AgentID:  agent2ID,
			AILabel:  "Tenant2 discussion",
		})
		if err != nil {
			t.Fatalf("Create tenant2 discussion: %v", err)
		}

		// Querying with tenant1ID + agent2ID must return nothing (tenant filter blocks it).
		list1, err := store.ListForAgent(ctx, tenantID, agent2ID, 10)
		if err != nil {
			t.Fatalf("ListForAgent(tenant1, agent2): %v", err)
		}
		if len(list1) != 0 {
			t.Errorf("cross-tenant leak: ListForAgent(tenant1, agent2) returned %d rows, want 0", len(list1))
		}

		// Querying with tenant2ID + agent2ID must return the 1 discussion.
		list2, err := store2.ListForAgent(ctx, tenant2ID, agent2ID, 10)
		if err != nil {
			t.Fatalf("ListForAgent(tenant2, agent2): %v", err)
		}
		if len(list2) != 1 {
			t.Errorf("ListForAgent(tenant2, agent2) returned %d rows, want 1", len(list2))
		}
	})
}
