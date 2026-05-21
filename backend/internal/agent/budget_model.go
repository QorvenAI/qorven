// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import "github.com/qorvenai/qorven/internal/providers"

// TierForBudget maps a per-agent budget (in cents) and task type to the
// appropriate model tier for SmartRouter resolution.
//
// Thresholds:
//
//	< 1000 cents  (~$0.10) → simple   — cheapest models (Haiku, Flash)
//	1000–4999              → standard — balanced capability (Sonnet)
//	≥ 5000 cents  (~$0.50) → complex/coding — most capable
//
// Coding tasks at high budget specifically get TierCoding (DeepSeek-Coder,
// Sonnet-code) rather than TierComplex to favour specialised code models.
func TierForBudget(budgetCents int64, taskType string) string {
	switch {
	case budgetCents >= 5000:
		if taskType == "coding" || taskType == "code" || taskType == "developer" {
			return providers.TierCoding
		}
		return providers.TierComplex
	case budgetCents >= 1000:
		return providers.TierStandard
	default:
		return providers.TierSimple
	}
}
