// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"testing"
	"time"
)

func TestKeyPool_RateLimitedKeySkipped(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Minute)
	k1 := &KeyRecord{ID: "k1", Status: "verified", BudgetType: "free", RateLimitedUntil: &future}
	k2 := &KeyRecord{ID: "k2", Status: "verified", BudgetType: "free"}
	p := NewKeyPool([]*KeyRecord{k1, k2}, StrategyRoundRobin, "")
	// A rate-limited key must be unavailable; a healthy key available —
	// this is what lets an agent fail over instead of stalling.
	if p.isAvailable(k1, now) {
		t.Fatal("rate-limited key must be unavailable")
	}
	if !p.isAvailable(k2, now) {
		t.Fatal("healthy key must be available — agent fails over to it")
	}
}
