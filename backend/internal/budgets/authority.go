// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package budgets

// AuthorityDecision decides whether a single allocation line should apply
// immediately or wait for user approval, based on the tenant's CFO authority:
//   - "full":      apply any amount (CFO has full autonomy; still validated).
//   - "ask":       always propose (every change needs user sign-off).
//   - "threshold"/"" (default): apply when lineUSD <= thresholdUSD, else propose.
func AuthorityDecision(authority string, thresholdUSD, lineUSD float64) string {
	switch authority {
	case "full":
		return "apply"
	case "ask":
		return "propose"
	default:
		if lineUSD <= thresholdUSD {
			return "apply"
		}
		return "propose"
	}
}
