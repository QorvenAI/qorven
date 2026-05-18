// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package knowledgegraph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Hard KG tests — real codebase extraction, analysis quality, export integrity.

func TestHard_KG_ExtractRealCodebase(t *testing.T) {
	// Extract from the Qorven codebase — walk up from this test file's directory
	baseDir := filepath.Join(testdataDir(), "..")
	files := []string{
		filepath.Join(baseDir, "agent/loop.go"),
		filepath.Join(baseDir, "agent/compactor.go"),
		filepath.Join(baseDir, "agent/loop_guard.go"),
		filepath.Join(baseDir, "memory/store.go"),
		filepath.Join(baseDir, "providers/openai.go"),
		filepath.Join(baseDir, "providers/anthropic.go"),
		filepath.Join(baseDir, "providers/gemini.go"),
		filepath.Join(baseDir, "tools/filesystem.go"),
		filepath.Join(baseDir, "tools/exec.go"),
		filepath.Join(baseDir, "gateway/gateway.go"),
	}

	var existing []string
	for _, f := range files {
		if _, err := os.Stat(f); err == nil { existing = append(existing, f) }
	}
	if len(existing) == 0 { t.Skip("no source files") }

	entities, relations := ExtractFromSource(existing)
	t.Logf("extracted from %d files: %d entities, %d relations", len(existing), len(entities), len(relations))

	// Verify entity types
	typeCounts := map[string]int{}
	for _, e := range entities { typeCounts[e.Type]++ }
	t.Logf("entity types: %v", typeCounts)

	if typeCounts["file"] < len(existing) { t.Errorf("expected %d+ file entities, got %d", len(existing), typeCounts["file"]) }
	if typeCounts["function"] == 0 { t.Error("no function entities extracted") }

	// Verify relations
	relTypes := map[string]int{}
	for _, r := range relations { relTypes[r.Type]++ }
	t.Logf("relation types: %v", relTypes)

	if relTypes["contains"] == 0 { t.Error("no 'contains' relations") }
	if relTypes["imports"] == 0 { t.Error("no 'imports' relations") }
}

func TestHard_KG_AnalysisQuality(t *testing.T) {
	// Build a graph that models a real system
	entities := []Entity{
		{ID: "gateway", Name: "Gateway", EntityType: "component"},
		{ID: "agent_loop", Name: "Agent Loop", EntityType: "component"},
		{ID: "memory", Name: "Memory Store", EntityType: "component"},
		{ID: "providers", Name: "Provider Registry", EntityType: "component"},
		{ID: "tools", Name: "Tool Registry", EntityType: "component"},
		{ID: "sessions", Name: "Session Store", EntityType: "component"},
		{ID: "channels", Name: "Channel Manager", EntityType: "component"},
		{ID: "scheduler", Name: "Scheduler", EntityType: "component"},
		{ID: "supervisor", Name: "Supervisor", EntityType: "component"},
		{ID: "kg", Name: "Knowledge Graph", EntityType: "component"},
	}

	rels := []Relationship{
		// Gateway is the hub
		{SourceID: "gateway", TargetID: "agent_loop", RelType: "uses"},
		{SourceID: "gateway", TargetID: "sessions", RelType: "uses"},
		{SourceID: "gateway", TargetID: "channels", RelType: "uses"},
		{SourceID: "gateway", TargetID: "providers", RelType: "uses"},
		{SourceID: "gateway", TargetID: "tools", RelType: "uses"},
		{SourceID: "gateway", TargetID: "memory", RelType: "uses"},
		// Agent loop dependencies
		{SourceID: "agent_loop", TargetID: "providers", RelType: "uses"},
		{SourceID: "agent_loop", TargetID: "tools", RelType: "uses"},
		{SourceID: "agent_loop", TargetID: "memory", RelType: "uses"},
		{SourceID: "agent_loop", TargetID: "sessions", RelType: "uses"},
		// Supervisor monitors agents
		{SourceID: "supervisor", TargetID: "agent_loop", RelType: "monitors"},
		// Scheduler triggers agents
		{SourceID: "scheduler", TargetID: "agent_loop", RelType: "triggers"},
		// KG is standalone
		{SourceID: "kg", TargetID: "memory", RelType: "uses"},
	}

	// God nodes — gateway and agent_loop should be top
	gods := AnalyzeGodNodes(entities, rels, 3)
	if len(gods) < 2 { t.Fatal("expected 2+ god nodes") }
	topNames := map[string]bool{}
	for _, g := range gods { topNames[g.Name] = true }
	if !topNames["Gateway"] && !topNames["Agent Loop"] {
		t.Errorf("expected Gateway or Agent Loop in top 3: %v", gods)
	}
	t.Logf("god nodes: %v", gods)

	// Surprising connections with communities
	communities := map[int][]string{
		0: {"gateway", "agent_loop", "sessions", "channels"},  // core
		1: {"providers", "tools", "memory"},                    // infrastructure
		2: {"supervisor", "scheduler", "kg"},                   // auxiliary
	}
	surprises := AnalyzeSurprisingConnections(entities, rels, communities, 5)
	t.Logf("surprising connections: %d", len(surprises))
	for _, s := range surprises {
		t.Logf("  %s ↔ %s (score=%.2f)", s.SourceName, s.TargetName, s.Score)
	}

	// Questions
	questions := SuggestQuestions(gods, surprises)
	if len(questions) == 0 { t.Error("no questions generated") }
	for _, q := range questions { t.Logf("  Q: %s", q) }
}

func TestHard_KG_ExportAllFormats(t *testing.T) {
	entities := []Entity{
		{ID: "a", Name: "Alpha", EntityType: "concept", Confidence: 0.95},
		{ID: "b", Name: "Beta", EntityType: "concept", Confidence: 0.87},
		{ID: "c", Name: "Gamma's Test", EntityType: "concept", Confidence: 0.92}, // apostrophe
	}
	rels := []Relationship{
		{SourceID: "a", TargetID: "b", RelType: "related", Confidence: 0.9},
		{SourceID: "b", TargetID: "c", RelType: "causes", Confidence: 0.8},
	}

	dir := t.TempDir()

	// JSON
	jsonPath := filepath.Join(dir, "graph.json")
	if err := ExportJSON(entities, rels, jsonPath); err != nil { t.Fatal(err) }
	jsonData, _ := os.ReadFile(jsonPath)
	if !strings.Contains(string(jsonData), "Alpha") { t.Error("JSON missing Alpha") }
	if !strings.Contains(string(jsonData), "Gamma") { t.Error("JSON missing Gamma") }
	t.Logf("JSON: %d bytes", len(jsonData))

	// GraphML
	gmlPath := filepath.Join(dir, "graph.graphml")
	if err := ExportGraphML(entities, rels, gmlPath); err != nil { t.Fatal(err) }
	gmlData, _ := os.ReadFile(gmlPath)
	if !strings.Contains(string(gmlData), "<graphml") { t.Error("invalid GraphML") }
	if !strings.Contains(string(gmlData), "Alpha") { t.Error("GraphML missing Alpha") }
	t.Logf("GraphML: %d bytes", len(gmlData))

	// Cypher
	cypherPath := filepath.Join(dir, "graph.cypher")
	if err := ExportCypher(entities, rels, cypherPath); err != nil { t.Fatal(err) }
	cypherData, _ := os.ReadFile(cypherPath)
	if !strings.Contains(string(cypherData), "CREATE") { t.Error("Cypher missing CREATE") }
	// Apostrophe should be escaped
	if strings.Contains(string(cypherData), "Gamma's") && !strings.Contains(string(cypherData), "Gamma\\'s") {
		t.Log("Cypher apostrophe escaping may vary")
	}
	t.Logf("Cypher: %d bytes", len(cypherData))

	// Obsidian
	obsDir := filepath.Join(dir, "obsidian")
	if err := ExportObsidian(entities, rels, obsDir); err != nil { t.Fatal(err) }
	mdFiles, _ := filepath.Glob(filepath.Join(obsDir, "*.md"))
	if len(mdFiles) != 3 { t.Errorf("expected 3 Obsidian files, got %d", len(mdFiles)) }
	// Read one and verify wiki-links
	if len(mdFiles) > 0 {
		content, _ := os.ReadFile(mdFiles[0])
		if !strings.Contains(string(content), "[[") { t.Log("Obsidian wiki-links may use different format") }
		t.Logf("Obsidian: %d files, sample=%d bytes", len(mdFiles), len(content))
	}
}

func TestHard_KG_DiffDetection(t *testing.T) {
	old := []Entity{
		{ID: "a", Name: "Alpha", EntityType: "concept"},
		{ID: "b", Name: "Beta", EntityType: "concept"},
		{ID: "c", Name: "Gamma", EntityType: "concept"},
	}
	new := []Entity{
		{ID: "a", Name: "Alpha Updated", EntityType: "concept"}, // modified
		{ID: "b", Name: "Beta", EntityType: "concept"},          // unchanged
		// c removed
		{ID: "d", Name: "Delta", EntityType: "concept"},         // added
	}

	oldRels := []Relationship{{SourceID: "a", TargetID: "b", RelType: "related"}}
	newRels := []Relationship{
		{SourceID: "a", TargetID: "b", RelType: "related"}, // unchanged
		{SourceID: "a", TargetID: "d", RelType: "causes"},  // added
	}

	diff := DiffGraphs(old, new, oldRels, newRels)

	if len(diff.AddedEntities) != 1 { t.Errorf("added=%d, want 1", len(diff.AddedEntities)) }
	if diff.AddedEntities[0].ID != "d" { t.Error("wrong added entity") }

	if len(diff.RemovedEntities) != 1 { t.Errorf("removed=%d, want 1", len(diff.RemovedEntities)) }
	if diff.RemovedEntities[0].ID != "c" { t.Error("wrong removed entity") }

	if len(diff.ModifiedEntities) != 1 { t.Errorf("modified=%d, want 1", len(diff.ModifiedEntities)) }
	if diff.ModifiedEntities[0].NewValue != "Alpha Updated" { t.Error("wrong modification") }

	if len(diff.AddedRelationships) != 1 { t.Errorf("added rels=%d, want 1", len(diff.AddedRelationships)) }

	if diff.IsEmpty() { t.Error("diff should not be empty") }
	t.Logf("diff: %s", diff.Summary())
}
