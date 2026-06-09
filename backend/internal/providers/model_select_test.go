// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import "testing"

func makeTestCatalog() *StaticModelCatalog {
	return &StaticModelCatalog{entries: []ModelCatalogEntry{
		{ID: "code-king", Provider: "p1", Tier: "complex", Pricing: CatalogPricing{OutputPerM: 10},
			BenchmarkScores: map[string]float64{"intelligence_index": 50, "coding_index": 99, "math_index": 40}},
		{ID: "math-king", Provider: "p1", Tier: "complex", Pricing: CatalogPricing{OutputPerM: 12},
			BenchmarkScores: map[string]float64{"intelligence_index": 55, "coding_index": 40, "math_index": 99}},
		{ID: "general-king", Provider: "p1", Tier: "complex", Pricing: CatalogPricing{OutputPerM: 11},
			BenchmarkScores: map[string]float64{"intelligence_index": 95, "coding_index": 45, "math_index": 45}},
	}}
}

func TestBestForTierByIndex_PicksTopForIndex(t *testing.T) {
	c := makeTestCatalog()
	avail := []string{"p1"}
	all := map[string]bool{}
	if m := c.BestForTierByIndex("complex", "coding_index", avail, all); m == nil || m.ID != "code-king" {
		t.Fatalf("coding_index should pick code-king, got %v", m)
	}
	if m := c.BestForTierByIndex("complex", "math_index", avail, all); m == nil || m.ID != "math-king" {
		t.Fatalf("math_index should pick math-king, got %v", m)
	}
	if m := c.BestForTierByIndex("complex", "intelligence_index", avail, all); m == nil || m.ID != "general-king" {
		t.Fatalf("intelligence_index should pick general-king, got %v", m)
	}
}

func TestBestForTierByIndex_EnabledFilterSkipsDisabled(t *testing.T) {
	c := makeTestCatalog()
	enabled := map[string]bool{"general-king": true}
	m := c.BestForTierByIndex("complex", "coding_index", []string{"p1"}, enabled)
	if m == nil || m.ID != "general-king" {
		t.Fatalf("enabled filter should restrict to general-king, got %v", m)
	}
}

func TestBestForTierByIndex_NoneEnabledReturnsNil(t *testing.T) {
	c := makeTestCatalog()
	enabled := map[string]bool{"not-in-catalog": true}
	if m := c.BestForTierByIndex("complex", "coding_index", []string{"p1"}, enabled); m != nil {
		t.Fatalf("no enabled candidate should return nil, got %v", m)
	}
}
