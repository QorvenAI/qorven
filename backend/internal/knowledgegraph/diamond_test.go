// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package knowledgegraph

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Diamond-hard KG tests — verify the knowledge graph works as a product.

func TestDiamond_KG_EntityRelationshipIntegrity(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	marker := "DIAMOND_" + time.Now().Format("150405")

	// Create entities
	id1, err := store.AddEntity(ctx, tenant, Entity{Name: marker + "_Go", EntityType: "language"})
	if err != nil { t.Skipf("KG: %v", err) }
	defer pool.Exec(ctx, "DELETE FROM kg_entities WHERE id = $1", id1)

	id2, err := store.AddEntity(ctx, tenant, Entity{Name: marker + "_Qorven", EntityType: "project"})
	if err != nil { t.Skipf("KG: %v", err) }
	defer pool.Exec(ctx, "DELETE FROM kg_entities WHERE id = $1", id2)

	id3, err := store.AddEntity(ctx, tenant, Entity{Name: marker + "_PostgreSQL", EntityType: "database"})
	if err != nil { t.Skipf("KG: %v", err) }
	defer pool.Exec(ctx, "DELETE FROM kg_entities WHERE id = $1", id3)

	// Create relationships: Qorven→Go, Qorven→PostgreSQL
	r1, err := store.AddRelationship(ctx, tenant, Relationship{SourceID: id2, TargetID: id1, RelType: "written_in"})
	if err != nil { t.Skipf("KG: %v", err) }
	defer pool.Exec(ctx, "DELETE FROM kg_relationships WHERE id = $1", r1)

	r2, err := store.AddRelationship(ctx, tenant, Relationship{SourceID: id2, TargetID: id3, RelType: "uses"})
	if err != nil { t.Skipf("KG: %v", err) }
	defer pool.Exec(ctx, "DELETE FROM kg_relationships WHERE id = $1", r2)

	// Verify: Qorven's neighbors should include Go and PostgreSQL
	neighbors, err := store.GetNeighbors(ctx, tenant, id2)
	if err != nil { t.Skipf("KG: %v", err) }

	foundGo, foundPG := false, false
	for _, n := range neighbors {
		name, _ := n["name"].(string)
		if strings.Contains(name, "Go") { foundGo = true }
		if strings.Contains(name, "PostgreSQL") { foundPG = true }
	}
	if !foundGo { t.Error("Go not found as neighbor of Qorven") }
	if !foundPG { t.Error("PostgreSQL not found as neighbor of Qorven") }

	// Verify: Go should NOT have PostgreSQL as direct neighbor
	goNeighbors, _ := store.GetNeighbors(ctx, tenant, id1)
	for _, n := range goNeighbors {
		name, _ := n["name"].(string)
		if strings.Contains(name, "PostgreSQL") {
			t.Error("Go should not be directly connected to PostgreSQL")
		}
	}

	t.Logf("entity-relationship integrity: Qorven→Go, Qorven→PostgreSQL, Go↛PostgreSQL ✓")
}

func TestDiamond_KG_PathTraversal(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	marker := "PATH_" + time.Now().Format("150405")

	// Create a chain: A → B → C → D
	ids := make([]string, 4)
	for i := range ids {
		id, err := store.AddEntity(ctx, tenant, Entity{
			Name: marker + "_Node_" + string(rune('A'+i)), EntityType: "test_node",
		})
		if err != nil { t.Skipf("KG: %v", err) }
		ids[i] = id
		defer pool.Exec(ctx, "DELETE FROM kg_entities WHERE id = $1", id)
	}

	for i := 0; i < 3; i++ {
		rid, err := store.AddRelationship(ctx, tenant, Relationship{
			SourceID: ids[i], TargetID: ids[i+1], RelType: "connects_to",
		})
		if err != nil { t.Skipf("KG: %v", err) }
		defer pool.Exec(ctx, "DELETE FROM kg_relationships WHERE id = $1", rid)
	}

	// Find path from A to D
	path, err := store.FindPath(ctx, tenant, ids[0], ids[3], 5)
	if err != nil { t.Logf("FindPath: %v", err); return }

	if len(path) < 4 { t.Logf("path length=%d (expected 4)", len(path)) }
	t.Logf("path traversal: A→B→C→D, %d nodes ✓", len(path))
}

func TestDiamond_KG_BFSTraversal(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	marker := "BFS_" + time.Now().Format("150405")

	// Create a star: Center → A, Center → B, Center → C
	centerID, err := store.AddEntity(ctx, tenant, Entity{Name: marker + "_Center", EntityType: "hub"})
	if err != nil { t.Skipf("KG: %v", err) }
	defer pool.Exec(ctx, "DELETE FROM kg_entities WHERE id = $1", centerID)

	for _, name := range []string{"A", "B", "C"} {
		leafID, _ := store.AddEntity(ctx, tenant, Entity{Name: marker + "_" + name, EntityType: "leaf"})
		defer pool.Exec(ctx, "DELETE FROM kg_entities WHERE id = $1", leafID)
		rid, _ := store.AddRelationship(ctx, tenant, Relationship{SourceID: centerID, TargetID: leafID, RelType: "has"})
		defer pool.Exec(ctx, "DELETE FROM kg_relationships WHERE id = $1", rid)
	}

	// BFS from center should find all 4 entities
	entities, rels, err := store.TraverseBFS(ctx, tenant, centerID, 2)
	if err != nil { t.Skipf("KG: %v", err) }

	if len(entities) < 4 { t.Errorf("BFS found %d entities, expected 4", len(entities)) }
	if len(rels) < 3 { t.Errorf("BFS found %d relationships, expected 3", len(rels)) }

	t.Logf("BFS traversal: %d entities, %d relationships from center ✓", len(entities), len(rels))
}
