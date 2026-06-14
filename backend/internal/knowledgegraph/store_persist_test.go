// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package knowledgegraph

// Integration test — KG store write + read path, tenant isolation, and ON CONFLICT
// upsert idempotency.  Run with:
//
//   go test ./internal/knowledgegraph/ -run TestStore_PersistIsolateUpsert -v
//
// The test pool helper (testPool) calls t.Skipf when no DB is reachable, so this
// test is always safe to include in ./...  runs.

import (
	"context"
	"testing"
)

// testTenantID is the system tenant present on every qorven_dev install (created
// by migration 001).  The FK constraint on kg_entities.tenant_id requires a real
// tenant row, so we reuse the one that always exists rather than inserting a
// synthetic one.
const testTenantID = "00000000-0000-0000-0000-000000000001"

// otherTenantID is a sentinel used to verify that entities written for
// testTenantID are NOT visible when querying under a different tenant.  It
// deliberately does NOT exist in the tenants table — we never write to
// kg_entities with it, so the FK is never exercised for this value.
const otherTenantID = "ffffffff-ffff-ffff-ffff-ffffffffffff"

// TestStore_PersistIsolateUpsert is the canonical integration test for the KG
// store's write + read path.  It covers:
//
//   (a) UpsertEntity          — two entities written, IDs returned (no error)
//   (b) UpsertRelationship    — one edge written between them (no error)
//   (c) SearchEntities        — viz-layer read confirms both entities are found
//       FindGodNodes / ClusterByType — the two methods the viz handlers call
//   (d) Tenant isolation      — querying under otherTenantID returns nothing
//   (e) ON CONFLICT idempotency — re-upserting the same entity leaves count == 1
func TestStore_PersistIsolateUpsert(t *testing.T) {
	pool := testPool(t) // skips if DB not reachable
	store := NewStore(pool)
	ctx := context.Background()

	// Unique entity names so parallel runs / leftover rows don't collide.
	const nameAlice = "Alice_persist_test"
	const nameAcme = "Acme_persist_test"

	// ── Cleanup: remove any leftover rows from a previous failed run. ──────
	cleanup := func() {
		pool.Exec(ctx,
			`DELETE FROM kg_relationships
			  WHERE tenant_id = $1
			    AND source_id IN (
			          SELECT id FROM kg_entities
			           WHERE tenant_id = $1 AND name IN ($2, $3))`,
			testTenantID, nameAlice, nameAcme)
		pool.Exec(ctx,
			`DELETE FROM kg_entities
			  WHERE tenant_id = $1 AND name IN ($2, $3)`,
			testTenantID, nameAlice, nameAcme)
	}
	cleanup()
	t.Cleanup(cleanup)

	// ── (a) UpsertEntity ────────────────────────────────────────────────────
	aliceID, err := store.UpsertEntity(ctx, testTenantID, Entity{
		Name: nameAlice, EntityType: "person", Confidence: 0.9,
	})
	if err != nil {
		t.Fatalf("UpsertEntity Alice: %v", err)
	}
	if aliceID == "" {
		t.Fatal("UpsertEntity Alice returned empty ID")
	}

	acmeID, err := store.UpsertEntity(ctx, testTenantID, Entity{
		Name: nameAcme, EntityType: "organisation", Confidence: 0.8,
	})
	if err != nil {
		t.Fatalf("UpsertEntity Acme: %v", err)
	}
	if acmeID == "" {
		t.Fatal("UpsertEntity Acme returned empty ID")
	}
	t.Logf("entities: alice=%s acme=%s", aliceID, acmeID)

	// ── (b) UpsertRelationship ──────────────────────────────────────────────
	relID, err := store.UpsertRelationship(ctx, testTenantID, Relationship{
		SourceID: aliceID, TargetID: acmeID, RelType: "works_at", Confidence: 1.0,
	})
	if err != nil {
		t.Fatalf("UpsertRelationship works_at: %v", err)
	}
	if relID == "" {
		t.Fatal("UpsertRelationship returned empty ID")
	}
	t.Logf("relationship: %s", relID)

	// ── (c) Read via viz-layer methods ──────────────────────────────────────

	// SearchEntities — the graph-viz full-entity-list call.
	found, err := store.SearchEntities(ctx, testTenantID, "persist_test", 100)
	if err != nil {
		t.Fatalf("SearchEntities: %v", err)
	}
	if len(found) < 2 {
		t.Errorf("SearchEntities: want >= 2 entities, got %d", len(found))
	}
	seenAlice, seenAcme := false, false
	for _, e := range found {
		if e.Name == nameAlice { seenAlice = true }
		if e.Name == nameAcme  { seenAcme = true }
	}
	if !seenAlice { t.Error("SearchEntities: Alice not found") }
	if !seenAcme  { t.Error("SearchEntities: Acme not found") }

	// FindGodNodes — the /v1/graph/god-nodes viz handler call.
	gods, err := store.FindGodNodes(ctx, testTenantID, 100)
	if err != nil {
		t.Fatalf("FindGodNodes: %v", err)
	}
	godNames := map[string]bool{}
	for _, g := range gods { godNames[g.Name] = true }
	if !godNames[nameAlice] { t.Errorf("FindGodNodes: %q not present", nameAlice) }
	if !godNames[nameAcme]  { t.Errorf("FindGodNodes: %q not present", nameAcme) }

	// ClusterByType — the /v1/graph/clusters viz handler call.
	clusters, err := store.ClusterByType(ctx, testTenantID)
	if err != nil {
		t.Fatalf("ClusterByType: %v", err)
	}
	if clusters["person"] < 1 {
		t.Errorf("ClusterByType: person count want >=1, got %d", clusters["person"])
	}
	if clusters["organisation"] < 1 {
		t.Errorf("ClusterByType: organisation count want >=1, got %d", clusters["organisation"])
	}
	t.Logf("clusters: %v", clusters)

	// GetRelationships — confirm the edge is retrievable via alice's ID.
	rels, err := store.GetRelationships(ctx, testTenantID, aliceID)
	if err != nil {
		t.Fatalf("GetRelationships: %v", err)
	}
	if len(rels) < 1 {
		t.Errorf("GetRelationships: want >=1 rel for Alice, got %d", len(rels))
	}
	foundWorksAt := false
	for _, r := range rels {
		if r.RelType == "works_at" { foundWorksAt = true }
	}
	if !foundWorksAt { t.Error("GetRelationships: works_at edge not found") }

	// ── (d) Tenant isolation ────────────────────────────────────────────────
	// otherTenantID has no rows in kg_entities, so all three read paths
	// must return empty results.
	isolatedEnts, err := store.SearchEntities(ctx, otherTenantID, "persist_test", 100)
	if err != nil {
		t.Fatalf("SearchEntities (other tenant): %v", err)
	}
	if len(isolatedEnts) != 0 {
		t.Errorf("tenant isolation: SearchEntities for other tenant returned %d entities, want 0",
			len(isolatedEnts))
	}

	isolatedGods, err := store.FindGodNodes(ctx, otherTenantID, 100)
	if err != nil {
		t.Fatalf("FindGodNodes (other tenant): %v", err)
	}
	for _, g := range isolatedGods {
		if g.Name == nameAlice || g.Name == nameAcme {
			t.Errorf("tenant isolation: FindGodNodes leaked %q under other tenant", g.Name)
		}
	}

	isolatedClusters, err := store.ClusterByType(ctx, otherTenantID)
	if err != nil {
		t.Fatalf("ClusterByType (other tenant): %v", err)
	}
	if len(isolatedClusters) != 0 {
		t.Errorf("tenant isolation: ClusterByType for other tenant returned %v, want empty",
			isolatedClusters)
	}

	// ── (e) Upsert idempotency (ON CONFLICT path) ───────────────────────────
	// Re-upsert Alice with a higher confidence — must not error and must not
	// create a second row; count must stay at 1.
	aliceID2, err := store.UpsertEntity(ctx, testTenantID, Entity{
		Name: nameAlice, EntityType: "person", Confidence: 1.0,
	})
	if err != nil {
		t.Fatalf("UpsertEntity Alice (2nd, idempotency): %v", err)
	}
	if aliceID2 != aliceID {
		t.Errorf("upsert idempotency: expected same ID %q, got %q", aliceID, aliceID2)
	}

	var count int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM kg_entities WHERE tenant_id = $1 AND name = $2 AND entity_type = 'person'`,
		testTenantID, nameAlice).Scan(&count)
	if err != nil {
		t.Fatalf("count Alice after 2nd upsert: %v", err)
	}
	if count != 1 {
		t.Errorf("upsert idempotency: count after re-upsert = %d, want 1 (ON CONFLICT violated)", count)
	}
	t.Logf("upsert idempotency confirmed: count=%d", count)
}
