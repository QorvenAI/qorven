// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package budgets

// Feasibility is the CFO's projection of whether a planned spend fits, with the
// full breakdown. All money is integer µUSD (1 USD = 1_000_000 µUSD).
type Feasibility struct {
	BudgetUUSD        int64 `json:"budget_uusd"`         // company monthly budget
	SpentUUSD         int64 `json:"spent_uusd"`          // spent month-to-date
	CommittedUUSD     int64 `json:"committed_uusd"`      // open work items' planned spend
	ProjectedBurnUUSD int64 `json:"projected_burn_uusd"` // daily_burn × days_remaining
	AvailableUUSD     int64 `json:"available_uusd"`      // budget − spent − committed − projected_burn (may be negative)
	PlanUUSD          int64 `json:"plan_uusd"`           // the proposed spend being projected
	Fits              bool  `json:"fits"`                // plan ≤ available
}

// ProjectFeasibility computes whether a planned spend fits, reserving room for
// ongoing daily burn and already-committed work before greenlighting. Pure.
func ProjectFeasibility(budgetUUSD, spentUUSD, committedUUSD, dailyBurnUUSD int64, daysRemaining int, planUUSD int64) Feasibility {
	if daysRemaining < 0 {
		daysRemaining = 0
	}
	projectedBurn := dailyBurnUUSD * int64(daysRemaining)
	available := budgetUUSD - spentUUSD - committedUUSD - projectedBurn
	return Feasibility{
		BudgetUUSD:        budgetUUSD,
		SpentUUSD:         spentUUSD,
		CommittedUUSD:     committedUUSD,
		ProjectedBurnUUSD: projectedBurn,
		AvailableUUSD:     available,
		PlanUUSD:          planUUSD,
		Fits:              planUUSD <= available,
	}
}
