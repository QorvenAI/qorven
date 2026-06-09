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
