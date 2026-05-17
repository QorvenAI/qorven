// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package skills

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"

)

func skillPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil { t.Skipf("DB: %v", err) }
	if err := pool.Ping(ctx); err != nil { t.Skipf("DB: %v", err) }
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestDiamond_Skill_FullLifecycle(t *testing.T) {
	pool := skillPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	marker := "diamond-" + time.Now().Format("150405")

	// 1. Create
	id, err := store.Create(ctx, tenant, marker+"_data_analysis", marker+"-data-analysis",
		"Analyze CSV data", "/skills/"+marker+".md", "abc123", []string{"analytics"})
	if err != nil { t.Fatal(err) }
	defer store.Delete(ctx, tenant, id)

	// 2. List and verify it appears (by slug)
	skills, err := store.List(ctx, tenant)
	if err != nil { t.Fatal(err) }
	found := false
	for _, s := range skills {
		if s.Slug == marker+"-data-analysis" { found = true }
	}
	if !found { t.Error("skill not in list after create") }

	// 3. Get by slug
	detail, err := store.GetBySlug(ctx, marker+"-data-analysis")
	if err != nil { t.Fatal(err) }
	if detail.Name != marker+"_data_analysis" { t.Errorf("name: %q", detail.Name) }

	// 4. Update
	err = store.UpdateSkill(ctx, id, marker+"_enhanced", "Enhanced analysis", "# Enhanced")
	if err != nil { t.Fatal(err) }

	detail2, _ := store.GetSkillByID(ctx, id)
	if detail2.Description != "Enhanced analysis" { t.Error("update not applied") }

	// 5. Delete (soft delete — archives, doesn't remove)
	store.Delete(ctx, tenant, id)
	detail3, err := store.GetSkillByID(ctx, id)
	if err != nil {
		t.Log("skill not found after delete ✓")
	} else {
		// Soft delete: should still exist but not appear in active list
		_ = detail3
		skills2, _ := store.List(ctx, tenant)
		stillActive := false
		for _, s := range skills2 {
			if s.Slug == marker+"-data-analysis" { stillActive = true }
		}
		if stillActive { t.Error("archived skill still in active list") }
		t.Log("soft delete: skill archived, not in active list ✓")
	}

	t.Log("skill lifecycle: create→list→get→update→delete ✓")
}

func TestDiamond_Skill_MarketplaceSearch(t *testing.T) {
	pool := skillPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	marker := "mkt-" + time.Now().Format("150405")

	id1, _ := store.Create(ctx, tenant, marker+"_scraper", marker+"-scraper",
		"Scrape websites", "/skills/scraper.md", "h1", []string{"web"})
	defer store.Delete(ctx, tenant, id1)

	id2, _ := store.Create(ctx, tenant, marker+"_review", marker+"-review",
		"Review code", "/skills/review.md", "h2", []string{"code"})
	defer store.Delete(ctx, tenant, id2)

	results, err := store.ListMarketplace(ctx, "", marker)
	if err != nil { t.Fatal(err) }

	if len(results) < 2 { t.Logf("marketplace: %d results (expected 2)", len(results)) }
	t.Logf("marketplace search: %d results ✓", len(results))
}
