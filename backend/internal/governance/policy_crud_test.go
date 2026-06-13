// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
package governance

import (
	"sync"
	"testing"
)

// No DB pool harness exists in this package; integration coverage is provided
// by the live smoke test. This file covers the pure-logic helper.

func TestValidPolicyAction(t *testing.T) {
	for _, a := range []string{"allow", "deny", "warn", "require_approval", "throttle", "log", "escalate"} {
		if !ValidPolicyAction(a) {
			t.Errorf("%q should be valid", a)
		}
	}
	if ValidPolicyAction("bogus") {
		t.Error("bogus should be invalid")
	}
}

func TestValidTriggerEvent(t *testing.T) {
	valid := []string{"tool_call", "model_switch", "output_deliver", "memory_write", "agent_spawn", "budget_spend", "external_action", "build_approve"}
	for _, ev := range valid {
		if !ValidTriggerEvent(ev) {
			t.Errorf("%q should be a valid trigger event", ev)
		}
	}
	for _, bad := range []string{"", "bogus", "output_deliver2", "TOOL_CALL"} {
		if ValidTriggerEvent(bad) {
			t.Errorf("%q should not be a valid trigger event", bad)
		}
	}
}

// TestPolicyEngine_ConcurrentReloadAndEvaluate verifies that concurrent
// LoadPolicies-style writes and Evaluate/HasBlockingOutputPolicy reads do not
// race. Run with -race to catch unsynchronised access.
func TestPolicyEngine_ConcurrentReloadAndEvaluate(t *testing.T) {
	e := &PolicyEngine{}

	const goroutines = 8
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	// Writers: simulate what LoadPolicies does (lock + assign).
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			for n := 0; n < iterations; n++ {
				batch := []Policy{
					{TenantID: "t1", TriggerEvent: "output_deliver", Action: "deny", Enabled: true},
					{TenantID: "t1", TriggerEvent: "tool_call", Action: "log", Enabled: true},
				}
				e.mu.Lock()
				e.policies = batch
				e.mu.Unlock()
			}
		}(i)
	}

	// Readers via Evaluate.
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for n := 0; n < iterations; n++ {
				e.Evaluate(nil, "output_deliver", "agent-1", 1, map[string]any{"has_pii": "true"}) //nolint:staticcheck
			}
		}()
	}

	// Readers via HasBlockingOutputPolicy.
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for n := 0; n < iterations; n++ {
				e.HasBlockingOutputPolicy("t1")
			}
		}()
	}

	wg.Wait()
}
