// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package budgets

import "testing"

func TestCarvedAllocation_RejectsOverParent(t *testing.T) {
	err := validateCarved(100.0, 80.0, 30.0) // 80+30=110 > 100
	if err == nil {
		t.Fatal("carved over-allocation must be rejected")
	}
}

func TestCarvedAllocation_AllowsWithinParent(t *testing.T) {
	if err := validateCarved(100.0, 80.0, 20.0); err != nil { // exactly 100
		t.Fatalf("within parent must be allowed, got %v", err)
	}
}

func TestFreshAllocation_NotDrawnFromParent(t *testing.T) {
	if err := validateAllocation("fresh", 100.0, 80.0, 999.0); err != nil {
		t.Fatalf("fresh must skip draw-down, got %v", err)
	}
}

func TestCarvedAllocation_ZeroParentMeansUnlimited(t *testing.T) {
	if err := validateAllocation("carved", 0, 0, 500.0); err != nil {
		t.Fatalf("unlimited parent must allow any carved child, got %v", err)
	}
}

func TestReconcile_DeclaredIsBinding(t *testing.T) {
	r := reconcile(50_000_000, 200_000_000) // declared 50, providers 200
	if r.EffectiveUUSD != 50_000_000 || r.Binding != "declared" {
		t.Fatalf("declared should bind: %+v", r)
	}
	if len(r.Warnings) != 0 {
		t.Fatalf("no warning when declared <= providers, got %v", r.Warnings)
	}
}

func TestReconcile_ProvidersAreBinding_Warns(t *testing.T) {
	r := reconcile(200_000_000, 50_000_000) // declared 200, providers only 50
	if r.EffectiveUUSD != 50_000_000 || r.Binding != "providers" {
		t.Fatalf("providers should bind: %+v", r)
	}
	if len(r.Warnings) == 0 {
		t.Fatalf("expected a warning when declared exceeds provider allowance")
	}
}

func TestReconcile_EqualPrefersDeclared(t *testing.T) {
	r := reconcile(50_000_000, 50_000_000)
	if r.EffectiveUUSD != 50_000_000 || r.Binding != "declared" {
		t.Fatalf("equal should report declared binding: %+v", r)
	}
}
