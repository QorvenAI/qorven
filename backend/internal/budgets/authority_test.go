// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package budgets

import "testing"

func TestAuthorityDecision_Full_AlwaysApply(t *testing.T) {
	if got := AuthorityDecision("full", 25, 1000); got != "apply" {
		t.Fatalf("full must apply any amount, got %q", got)
	}
}
func TestAuthorityDecision_Ask_AlwaysPropose(t *testing.T) {
	if got := AuthorityDecision("ask", 25, 1); got != "propose" {
		t.Fatalf("ask must propose any amount, got %q", got)
	}
}
func TestAuthorityDecision_Threshold_UnderApplies(t *testing.T) {
	if got := AuthorityDecision("threshold", 25, 20); got != "apply" {
		t.Fatalf("threshold under-limit must apply, got %q", got)
	}
}
func TestAuthorityDecision_Threshold_AtLimitApplies(t *testing.T) {
	if got := AuthorityDecision("threshold", 25, 25); got != "apply" {
		t.Fatalf("threshold at-limit must apply, got %q", got)
	}
}
func TestAuthorityDecision_Threshold_OverProposes(t *testing.T) {
	if got := AuthorityDecision("threshold", 25, 25.01); got != "propose" {
		t.Fatalf("threshold over-limit must propose, got %q", got)
	}
}
func TestAuthorityDecision_BlankDefaultsThreshold(t *testing.T) {
	if got := AuthorityDecision("", 25, 100); got != "propose" {
		t.Fatalf("blank authority must behave as threshold, got %q", got)
	}
}
