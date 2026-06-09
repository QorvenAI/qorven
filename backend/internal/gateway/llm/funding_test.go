// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import "testing"

func TestTenantCapAndSpent_PrepaidFixed_UsesLifetimeAndAllTime(t *testing.T) {
	cap, spent := tenantCapAndSpent("prepaid_fixed", 50.0, 200.0, 10_000_000, 180_000_000)
	if cap != 200_000_000 {
		t.Fatalf("cap want 200000000 µUSD, got %d", cap)
	}
	if spent != 180_000_000 {
		t.Fatalf("spent want all-time 180000000, got %d", spent)
	}
}

func TestTenantCapAndSpent_MonthlyRecurring_UsesMonthlyAndMTD(t *testing.T) {
	cap, spent := tenantCapAndSpent("monthly_recurring", 50.0, 200.0, 10_000_000, 180_000_000)
	if cap != 50_000_000 {
		t.Fatalf("cap want 50000000 µUSD, got %d", cap)
	}
	if spent != 10_000_000 {
		t.Fatalf("spent want MTD 10000000, got %d", spent)
	}
}

func TestTenantCapAndSpent_BlankModeDefaultsMonthly(t *testing.T) {
	cap, spent := tenantCapAndSpent("", 50.0, 200.0, 10_000_000, 180_000_000)
	if cap != 50_000_000 || spent != 10_000_000 {
		t.Fatalf("blank mode must behave as monthly_recurring, got cap=%d spent=%d", cap, spent)
	}
}
