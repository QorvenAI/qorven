// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

// tenantCapAndSpent resolves the overall-budget cap and the spend to compare
// it against, based on the tenant's funding mode. All money is integer µUSD.
//
//   - "prepaid_fixed":   a loaded amount that depletes and never resets →
//     cap = lifetimeUSD, spent = all-time spend.
//   - "monthly_recurring" (or blank, for backward-compat): a monthly budget →
//     cap = monthlyUSD, spent = month-to-date spend.
func tenantCapAndSpent(mode string, monthlyUSD, lifetimeUSD float64, mtdUUSD, allTimeUUSD int64) (capUUSD, spentUUSD int64) {
	if mode == "prepaid_fixed" {
		return int64(lifetimeUSD * float64(uusdPerUSD)), allTimeUUSD
	}
	return int64(monthlyUSD * float64(uusdPerUSD)), mtdUUSD
}
