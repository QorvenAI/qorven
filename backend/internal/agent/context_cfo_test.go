// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package agent

import "testing"

func TestIsBudgetMutatingTool(t *testing.T) {
	for _, n := range []string{"set_budget", "propose_allocation", "decide_budget_request"} {
		if !isBudgetMutatingTool(n) {
			t.Fatalf("%s should be budget-mutating", n)
		}
	}
	for _, n := range []string{"reconcile_costs", "effective_budget", "web_search", "cfo_report"} {
		if isBudgetMutatingTool(n) {
			t.Fatalf("%s should NOT be budget-mutating", n)
		}
	}
}

func TestCanUseBudgetTools(t *testing.T) {
	if !canUseBudgetTools("cfo") || !canUseBudgetTools("coo") {
		t.Fatal("cfo and coo may use budget tools")
	}
	if canUseBudgetTools("cto") || canUseBudgetTools("") || canUseBudgetTools("worker") {
		t.Fatal("non-finance roles may not use budget tools")
	}
}
