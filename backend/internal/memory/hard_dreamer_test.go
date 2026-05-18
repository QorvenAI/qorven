// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"testing"
	"time"
)

// hard_dreamer_test.go — Tests for background memory consolidation and digests.

func TestHard_Dreamer_RunOnce(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	tenant := "00000000-0000-0000-0000-000000000001"

	dreamer := NewDreamer(store, tenant, 1*time.Hour)
	stats := dreamer.RunOnce(context.Background())

	// Should complete without error
	if stats.LastRunDuration <= 0 { t.Error("duration should be positive") }
	t.Logf("dreamer: decayed=%d, merged=%d, digests=%d, duration=%v ✓",
		stats.MemoriesDecayed, stats.MemoriesMerged, stats.DigestsCreated, stats.LastRunDuration)
}

func TestHard_Dreamer_Stats(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	tenant := "00000000-0000-0000-0000-000000000001"

	dreamer := NewDreamer(store, tenant, 1*time.Hour)
	dreamer.RunOnce(context.Background())

	stats := dreamer.Stats()
	if stats.TotalRuns != 1 { t.Errorf("total runs: %d", stats.TotalRuns) }
}

func TestHard_Dreamer_MultipleRuns(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	tenant := "00000000-0000-0000-0000-000000000001"

	dreamer := NewDreamer(store, tenant, 1*time.Hour)
	dreamer.RunOnce(context.Background())
	dreamer.RunOnce(context.Background())
	dreamer.RunOnce(context.Background())

	stats := dreamer.Stats()
	if stats.TotalRuns != 3 { t.Errorf("total runs: %d", stats.TotalRuns) }
}

// ── Digest Store ──

func TestHard_DigestStore_BuildDigest(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)

	var agentID string
	pool.QueryRow(context.Background(), "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	ds := NewDigestStore(store)
	digest, err := ds.BuildDigest(context.Background(), agentID)
	if err != nil { t.Fatal(err) }

	// Digest may be empty if no high-importance memories exist
	t.Logf("digest: %d chars for agent %s ✓", len(digest), agentID[:8])
}

func TestHard_DigestStore_SearchWithDigest(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(context.Background(), "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	ds := NewDigestStore(store)
	results, digest, err := ds.SearchWithDigest(context.Background(), tenant, agentID, "test", 5)
	if err != nil { t.Fatal(err) }

	t.Logf("search+digest: %d results, digest=%d chars ✓", len(results), len(digest))
}
