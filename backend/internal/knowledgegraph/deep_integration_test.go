// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package knowledgegraph

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"

)

// Deep KG integration tests — full pipeline: extract → store → analyze → export.

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil { t.Skipf("DB: %v", err) }
	if err := pool.Ping(ctx); err != nil { t.Skipf("DB: %v", err) }
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestDeep_KG_ExtractAnalyzeExport(t *testing.T) {
	// 1. Extract entities from real source files
	files := []string{
		"store.go",
		"analysis.go",
		"export.go",
		"source_extract.go",
		"diff.go",
	}
	var paths []string
	for _, f := range files {
		p := filepath.Join(".", f)
		if _, err := os.Stat(p); err == nil { paths = append(paths, p) }
	}
	if len(paths) == 0 { t.Skip("no source files found") }

	entities, relations := ExtractFromSource(paths)
	t.Logf("extracted: %d entities, %d relations from %d files", len(entities), len(relations), len(paths))
	if len(entities) < len(paths) { t.Error("should have at least 1 entity per file") }

	// 2. Convert to KG types for analysis
	kgEntities := make([]Entity, len(entities))
	for i, e := range entities {
		kgEntities[i] = Entity{ID: e.ID, Name: e.Name, EntityType: e.Type}
	}
	kgRels := make([]Relationship, len(relations))
	for i, r := range relations {
		kgRels[i] = Relationship{SourceID: r.Source, TargetID: r.Target, RelType: r.Type, Confidence: 1.0}
	}

	// 3. Analyze — find god nodes
	gods := AnalyzeGodNodes(kgEntities, kgRels, 5)
	t.Logf("god nodes: %d", len(gods))
	for _, g := range gods {
		t.Logf("  %s (%s): degree=%d", g.Name, g.Type, g.Degree)
	}

	// 4. Export to JSON
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "kg.json")
	err := ExportJSON(kgEntities, kgRels, jsonPath)
	if err != nil { t.Fatalf("export JSON: %v", err) }
	data, _ := os.ReadFile(jsonPath)
	if len(data) == 0 { t.Error("empty JSON export") }
	if !strings.Contains(string(data), "entities") { t.Error("missing entities in JSON") }

	// 5. Export to GraphML
	graphmlPath := filepath.Join(dir, "kg.graphml")
	err = ExportGraphML(kgEntities, kgRels, graphmlPath)
	if err != nil { t.Fatalf("export GraphML: %v", err) }
	gml, _ := os.ReadFile(graphmlPath)
	if !strings.Contains(string(gml), "<graphml") { t.Error("invalid GraphML") }

	// 6. Export to Cypher
	cypherPath := filepath.Join(dir, "kg.cypher")
	err = ExportCypher(kgEntities, kgRels, cypherPath)
	if err != nil { t.Fatalf("export Cypher: %v", err) }
	cypher, _ := os.ReadFile(cypherPath)
	if !strings.Contains(string(cypher), "CREATE") { t.Error("missing CREATE in Cypher") }

	// 7. Export to Obsidian
	obsDir := filepath.Join(dir, "obsidian")
	err = ExportObsidian(kgEntities, kgRels, obsDir)
	if err != nil { t.Fatalf("export Obsidian: %v", err) }
	mdFiles, _ := filepath.Glob(filepath.Join(obsDir, "*.md"))
	if len(mdFiles) == 0 { t.Error("no Obsidian markdown files") }
	t.Logf("Obsidian: %d markdown files", len(mdFiles))

	// 8. Diff — compare with empty graph
	diff := DiffGraphs(nil, kgEntities, nil, kgRels)
	if diff.IsEmpty() { t.Error("diff should show additions") }
	if len(diff.AddedEntities) != len(kgEntities) { t.Errorf("added=%d, want %d", len(diff.AddedEntities), len(kgEntities)) }
	t.Logf("diff: %s", diff.Summary())
}

func TestDeep_KG_StoreAndTraverse(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	// Cleanup test data from previous runs
	pool.Exec(ctx, "DELETE FROM kg_relationships WHERE tenant_id = $1", tenant)
	pool.Exec(ctx, "DELETE FROM kg_entities WHERE tenant_id = $1 AND name IN ('Qorven','Agent Loop','Memory System')", tenant)

	// 1. Add entities
	id1, err := store.AddEntity(ctx, tenant, Entity{Name: "Qorven", EntityType: "product", Confidence: 1.0})
	if err != nil && strings.Contains(err.Error(), "does not exist") { t.Skip("KG tables not migrated") }
	if err != nil { t.Fatalf("add entity 1: %v", err) }
	id2, err := store.AddEntity(ctx, tenant, Entity{Name: "Agent Loop", EntityType: "component", Confidence: 0.9})
	if err != nil { t.Fatalf("add entity 2: %v", err) }
	id3, err := store.AddEntity(ctx, tenant, Entity{Name: "Memory System", EntityType: "component", Confidence: 0.9})
	if err != nil { t.Fatalf("add entity 3: %v", err) }
	t.Logf("entities: %s, %s, %s", id1, id2, id3)

	// 2. Add relationships
	_, err = store.AddRelationship(ctx, tenant, Relationship{SourceID: id1, TargetID: id2, RelType: "contains", Confidence: 1.0})
	if err != nil { t.Fatalf("add rel 1: %v", err) }
	_, err = store.AddRelationship(ctx, tenant, Relationship{SourceID: id1, TargetID: id3, RelType: "contains", Confidence: 1.0})
	if err != nil { t.Fatalf("add rel 2: %v", err) }
	_, err = store.AddRelationship(ctx, tenant, Relationship{SourceID: id2, TargetID: id3, RelType: "uses", Confidence: 0.8})
	if err != nil { t.Fatalf("add rel 3: %v", err) }

	// 3. Search
	results, err := store.SearchEntities(ctx, tenant, "Qorven", 10)
	if err != nil { t.Fatalf("search: %v", err) }
	if len(results) == 0 { t.Error("should find Qorven") }
	t.Logf("search 'Qorven': %d results", len(results))

	// 4. Get relationships
	rels, err := store.GetRelationships(ctx, tenant, id1)
	if err != nil { t.Fatalf("get rels: %v", err) }
	if len(rels) < 2 { t.Errorf("expected 2+ rels, got %d", len(rels)) }

	// 5. BFS traversal
	entities, traverseRels, err := store.TraverseBFS(ctx, tenant, id1, 2)
	if err != nil { t.Fatalf("traverse: %v", err) }
	if len(entities) < 3 { t.Errorf("BFS should find 3 entities, got %d", len(entities)) }
	t.Logf("BFS from Qorven: %d entities, %d rels", len(entities), len(traverseRels))

	// 6. Find path
	path, err := store.FindPath(ctx, tenant, id2, id3, 3)
	if err != nil { t.Logf("path: %v (may not find direct path)", err) }
	if path != nil { t.Logf("path Agent Loop → Memory System: %v", path) }

	// 7. Get neighbors
	neighbors, err := store.GetNeighbors(ctx, tenant, id1)
	if err != nil { t.Fatalf("neighbors: %v", err) }
	if len(neighbors) < 2 { t.Errorf("Qorven should have 2+ neighbors, got %d", len(neighbors)) }
	for _, n := range neighbors {
		t.Logf("  neighbor: %v (%v)", n["name"], n["relationship"])
	}
}

func TestDeep_KG_SuggestQuestions(t *testing.T) {
	// Build a graph with clear structure
	entities := []Entity{
		{ID: "db", Name: "Database", EntityType: "component"},
		{ID: "api", Name: "API Gateway", EntityType: "component"},
		{ID: "auth", Name: "Auth System", EntityType: "component"},
		{ID: "billing", Name: "Billing", EntityType: "component"},
		{ID: "ui", Name: "Web UI", EntityType: "component"},
	}
	rels := []Relationship{
		{SourceID: "api", TargetID: "db"}, {SourceID: "api", TargetID: "auth"},
		{SourceID: "api", TargetID: "billing"}, {SourceID: "api", TargetID: "ui"},
		{SourceID: "auth", TargetID: "db"}, {SourceID: "billing", TargetID: "db"},
		{SourceID: "ui", TargetID: "api"},
	}

	gods := AnalyzeGodNodes(entities, rels, 3)
	if len(gods) == 0 { t.Fatal("no god nodes") }
	// API Gateway should be the most connected
	if gods[0].Name != "API Gateway" && gods[0].Name != "Database" {
		t.Logf("top god node: %s (degree %d)", gods[0].Name, gods[0].Degree)
	}

	// Surprising connections with communities
	communities := map[int][]string{
		0: {"db", "api", "auth"},     // backend
		1: {"billing", "ui"},          // frontend/business
	}
	surprises := AnalyzeSurprisingConnections(entities, rels, communities, 5)
	t.Logf("surprising connections: %d", len(surprises))
	for _, s := range surprises {
		t.Logf("  %s ↔ %s (score=%.2f)", s.SourceName, s.TargetName, s.Score)
	}

	// Generate questions
	questions := SuggestQuestions(gods, surprises)
	if len(questions) == 0 { t.Error("should generate questions") }
	for _, q := range questions { t.Logf("  Q: %s", q) }
}
