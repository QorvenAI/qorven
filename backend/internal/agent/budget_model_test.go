// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent_test

import (
	"testing"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/providers"
)

func TestTierForBudget(t *testing.T) {
	cases := []struct {
		budgetCents int64
		taskType    string
		wantTier    string
	}{
		// Under threshold → simple
		{0, "general", providers.TierSimple},
		{500, "coding", providers.TierSimple},
		{999, "research", providers.TierSimple},
		// Mid range → standard
		{1000, "coding", providers.TierStandard},
		{2500, "research", providers.TierStandard},
		{4999, "general", providers.TierStandard},
		// High budget, non-coding → complex
		{5000, "research", providers.TierComplex},
		{10000, "general", providers.TierComplex},
		// High budget, coding task types → coding tier
		{5000, "coding", providers.TierCoding},
		{5000, "code", providers.TierCoding},
		{5000, "developer", providers.TierCoding},
		{20000, "coding", providers.TierCoding},
	}
	for _, c := range cases {
		got := agent.TierForBudget(c.budgetCents, c.taskType)
		if got != c.wantTier {
			t.Errorf("TierForBudget(%d, %q) = %q, want %q", c.budgetCents, c.taskType, got, c.wantTier)
		}
	}
}
