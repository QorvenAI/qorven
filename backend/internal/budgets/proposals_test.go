// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package budgets

import "testing"

func TestProposalLineToScope_MapsAllFields(t *testing.T) {
	line := ProposalLine{
		Scope: "department", ScopeID: "d1",
		ProposedMonthlyUSD: 80, ProposedLifetimeUSD: 0,
		AllocationMode: "carved", ParentScope: "tenant", ParentScopeID: "",
		FundingMode: "",
	}
	bs := line.ToBudgetScope()
	if bs.Scope != "department" || bs.ScopeID != "d1" || bs.MonthlyUSD != 80 ||
		bs.AllocationMode != "carved" || bs.ParentScope != "tenant" {
		t.Fatalf("line did not map to BudgetScope correctly: %+v", bs)
	}
}
