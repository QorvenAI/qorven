// Copyright 2026 Qorven AI. All rights reserved.
package governance

import "testing"

func TestMatchConditions_HasPII(t *testing.T) {
	e := &PolicyEngine{}
	conds := []PolicyCond{{Field: "has_pii", Operator: "equals", Value: "true"}}
	if !e.matchConditions(conds, map[string]any{"has_pii": "true"}) {
		t.Fatal("has_pii=true should match the deny condition")
	}
	if e.matchConditions(conds, map[string]any{"has_pii": "false"}) {
		t.Fatal("has_pii=false should NOT match")
	}
	if e.matchConditions(conds, map[string]any{}) {
		t.Fatal("missing has_pii should NOT match (the original inert bug)")
	}
}

func TestHasBlockingOutputPolicy(t *testing.T) {
	const tid = "tenant-1"

	// No policies → false
	e := &PolicyEngine{}
	if e.HasBlockingOutputPolicy(tid) {
		t.Fatal("empty engine should return false")
	}

	// Enabled deny output_deliver → true
	e.policies = []Policy{
		{TenantID: tid, TriggerEvent: "output_deliver", Action: "deny", Enabled: true},
	}
	if !e.HasBlockingOutputPolicy(tid) {
		t.Fatal("enabled deny output_deliver should return true")
	}

	// Disabled deny → false
	e.policies = []Policy{
		{TenantID: tid, TriggerEvent: "output_deliver", Action: "deny", Enabled: false},
	}
	if e.HasBlockingOutputPolicy(tid) {
		t.Fatal("disabled policy should return false")
	}

	// Wrong tenant → false
	e.policies = []Policy{
		{TenantID: "other-tenant", TriggerEvent: "output_deliver", Action: "deny", Enabled: true},
	}
	if e.HasBlockingOutputPolicy(tid) {
		t.Fatal("policy for different tenant should return false")
	}

	// require_approval also blocks
	e.policies = []Policy{
		{TenantID: tid, TriggerEvent: "output_deliver", Action: "require_approval", Enabled: true},
	}
	if !e.HasBlockingOutputPolicy(tid) {
		t.Fatal("enabled require_approval output_deliver should return true")
	}

	// warn/log/allow do not block
	for _, action := range []string{"allow", "warn", "log", "throttle"} {
		e.policies = []Policy{
			{TenantID: tid, TriggerEvent: "output_deliver", Action: action, Enabled: true},
		}
		if e.HasBlockingOutputPolicy(tid) {
			t.Fatalf("action=%q should not be blocking", action)
		}
	}

	// Wrong trigger event → false
	e.policies = []Policy{
		{TenantID: tid, TriggerEvent: "model_switch", Action: "deny", Enabled: true},
	}
	if e.HasBlockingOutputPolicy(tid) {
		t.Fatal("policy for different trigger event should return false")
	}
}
