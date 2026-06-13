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
