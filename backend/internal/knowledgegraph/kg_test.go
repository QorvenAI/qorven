// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package knowledgegraph

import (
	"path/filepath"
	"runtime"
	"testing"
)

// testdataDir returns the directory this test file lives in,
// so tests work on any machine regardless of absolute path.
func testdataDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Dir(f)
}

func TestAnalyzeGodNodes_Empty(t *testing.T) {
	result := AnalyzeGodNodes(nil, nil, 10)
	if len(result) != 0 { t.Errorf("expected empty, got %d", len(result)) }
}

func TestAnalyzeGodNodes_Basic(t *testing.T) {
	entities := []Entity{
		{ID: "a", Name: "Alpha", EntityType: "concept"},
		{ID: "b", Name: "Beta", EntityType: "concept"},
		{ID: "c", Name: "Gamma", EntityType: "concept"},
	}
	rels := []Relationship{
		{SourceID: "a", TargetID: "b", RelType: "related"},
		{SourceID: "a", TargetID: "c", RelType: "related"},
		{SourceID: "b", TargetID: "c", RelType: "related"},
	}
	gods := AnalyzeGodNodes(entities, rels, 2)
	if len(gods) != 2 { t.Errorf("expected 2, got %d", len(gods)) }
	// All nodes have degree 2, so any 2 are valid
	if gods[0].Degree != 2 { t.Errorf("degree=%d", gods[0].Degree) }
}

func TestAnalyzeGodNodes_TopN(t *testing.T) {
	entities := []Entity{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}, {ID: "c", Name: "C"}}
	rels := []Relationship{
		{SourceID: "a", TargetID: "b"}, {SourceID: "a", TargetID: "c"},
		{SourceID: "b", TargetID: "c"},
	}
	gods := AnalyzeGodNodes(entities, rels, 1)
	if len(gods) != 1 { t.Errorf("expected 1, got %d", len(gods)) }
}

func TestAnalyzeSurprisingConnections_SameCommunity(t *testing.T) {
	entities := []Entity{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}
	rels := []Relationship{{SourceID: "a", TargetID: "b"}}
	communities := map[int][]string{0: {"a", "b"}}
	surprises := AnalyzeSurprisingConnections(entities, rels, communities, 10)
	if len(surprises) != 0 { t.Error("same community should not be surprising") }
}

func TestAnalyzeSurprisingConnections_CrossCommunity(t *testing.T) {
	entities := []Entity{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}
	rels := []Relationship{{SourceID: "a", TargetID: "b"}}
	communities := map[int][]string{0: {"a"}, 1: {"b"}}
	surprises := AnalyzeSurprisingConnections(entities, rels, communities, 10)
	if len(surprises) != 1 { t.Errorf("expected 1 surprise, got %d", len(surprises)) }
	if surprises[0].Score <= 0 { t.Error("score should be positive") }
}

func TestSuggestQuestions(t *testing.T) {
	gods := []GodNode{{Name: "Database", Degree: 15}}
	surprises := []SurprisingConnection{{SourceName: "Auth", TargetName: "Billing"}}
	questions := SuggestQuestions(gods, surprises)
	if len(questions) != 2 { t.Errorf("expected 2 questions, got %d", len(questions)) }
}

func TestSuggestQuestions_LowDegree(t *testing.T) {
	gods := []GodNode{{Name: "Small", Degree: 3}}
	questions := SuggestQuestions(gods, nil)
	if len(questions) != 0 { t.Error("low degree should not generate questions") }
}

func TestDiffGraphs_Empty(t *testing.T) {
	diff := DiffGraphs(nil, nil, nil, nil)
	if !diff.IsEmpty() { t.Error("empty diff should be empty") }
}

func TestDiffGraphs_AddedEntities(t *testing.T) {
	old := []Entity{{ID: "a", Name: "A"}}
	new := []Entity{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}
	diff := DiffGraphs(old, new, nil, nil)
	if len(diff.AddedEntities) != 1 { t.Errorf("added=%d", len(diff.AddedEntities)) }
	if diff.AddedEntities[0].ID != "b" { t.Error("wrong added entity") }
}

func TestDiffGraphs_RemovedEntities(t *testing.T) {
	old := []Entity{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}
	new := []Entity{{ID: "a", Name: "A"}}
	diff := DiffGraphs(old, new, nil, nil)
	if len(diff.RemovedEntities) != 1 { t.Errorf("removed=%d", len(diff.RemovedEntities)) }
}

func TestDiffGraphs_ModifiedEntities(t *testing.T) {
	old := []Entity{{ID: "a", Name: "Alpha", EntityType: "concept"}}
	new := []Entity{{ID: "a", Name: "Alpha Updated", EntityType: "concept"}}
	diff := DiffGraphs(old, new, nil, nil)
	if len(diff.ModifiedEntities) != 1 { t.Errorf("modified=%d", len(diff.ModifiedEntities)) }
	if diff.ModifiedEntities[0].NewValue != "Alpha Updated" { t.Error("wrong new value") }
}

func TestDiffGraphs_AddedRelationships(t *testing.T) {
	oldRels := []Relationship{{SourceID: "a", TargetID: "b", RelType: "related"}}
	newRels := []Relationship{
		{SourceID: "a", TargetID: "b", RelType: "related"},
		{SourceID: "b", TargetID: "c", RelType: "related"},
	}
	diff := DiffGraphs(nil, nil, oldRels, newRels)
	if len(diff.AddedRelationships) != 1 { t.Errorf("added rels=%d", len(diff.AddedRelationships)) }
}

func TestDiffGraphs_Summary(t *testing.T) {
	diff := GraphDiff{
		AddedEntities: []Entity{{ID: "a"}},
		RemovedEntities: []Entity{{ID: "b"}, {ID: "c"}},
	}
	s := diff.Summary()
	if s == "" { t.Error("empty summary") }
}

func TestGraphExport_JSON(t *testing.T) {
	entities := []Entity{{ID: "a", Name: "Alpha", EntityType: "concept"}}
	rels := []Relationship{{SourceID: "a", TargetID: "b", RelType: "related", Confidence: 0.9}}
	err := ExportJSON(entities, rels, "/tmp/test_kg_export.json")
	if err != nil { t.Fatal(err) }
}

func TestGraphExport_GraphML(t *testing.T) {
	entities := []Entity{{ID: "a", Name: "Alpha", EntityType: "concept"}}
	rels := []Relationship{{SourceID: "a", TargetID: "b", RelType: "related", Confidence: 0.9}}
	err := ExportGraphML(entities, rels, "/tmp/test_kg_export.graphml")
	if err != nil { t.Fatal(err) }
}

func TestGraphExport_Cypher(t *testing.T) {
	entities := []Entity{{ID: "a", Name: "Alpha's Test", EntityType: "concept"}}
	rels := []Relationship{{SourceID: "a", TargetID: "b", RelType: "related"}}
	err := ExportCypher(entities, rels, "/tmp/test_kg_export.cypher")
	if err != nil { t.Fatal(err) }
}

func TestSourceExtract_GoFile(t *testing.T) {
	entities, relations := ExtractFromSource([]string{filepath.Join(testdataDir(), "store.go")})
	if len(entities) == 0 { t.Error("should extract entities from Go file") }
	hasFile := false
	for _, e := range entities { if e.Type == "file" { hasFile = true } }
	if !hasFile { t.Error("should have file entity") }
	if len(relations) == 0 { t.Error("should extract relations") }
}

func TestSourceExtract_EmptyPaths(t *testing.T) {
	entities, relations := ExtractFromSource(nil)
	if len(entities) != 0 { t.Error("empty paths should return empty") }
	if len(relations) != 0 { t.Error("empty paths should return empty") }
}

func TestSourceExtract_NonexistentFile(t *testing.T) {
	entities, _ := ExtractFromSource([]string{"/nonexistent/file.go"})
	// File entity is created before Open attempt, so 1 entity expected
	if len(entities) > 1 { t.Errorf("expected at most 1 file entity, got %d", len(entities)) }
}

// === HARD KG TESTS ===

func TestAnalyzeGodNodes_LargeGraph(t *testing.T) {
	entities := make([]Entity, 100)
	for i := range entities { entities[i] = Entity{ID: string(rune('a' + i%26)) + string(rune('0'+i/26)), Name: "Entity" + string(rune('0'+i%10))} }
	rels := make([]Relationship, 500)
	for i := range rels { rels[i] = Relationship{SourceID: entities[i%100].ID, TargetID: entities[(i*7)%100].ID} }
	gods := AnalyzeGodNodes(entities, rels, 5)
	if len(gods) != 5 { t.Errorf("expected 5, got %d", len(gods)) }
	// Should be sorted by degree
	for i := 1; i < len(gods); i++ {
		if gods[i].Degree > gods[i-1].Degree { t.Error("not sorted by degree") }
	}
}

func TestDiffGraphs_LargeChange(t *testing.T) {
	old := make([]Entity, 50)
	for i := range old { old[i] = Entity{ID: string(rune('a'+i%26)), Name: "Old" + string(rune('0'+i%10))} }
	new := make([]Entity, 50)
	for i := range new { new[i] = Entity{ID: string(rune('a'+(i+25)%26)), Name: "New" + string(rune('0'+i%10))} }
	diff := DiffGraphs(old, new, nil, nil)
	if diff.IsEmpty() { t.Error("should detect changes") }
	t.Logf("diff: %s", diff.Summary())
}

func TestExportJSON_LargeGraph(t *testing.T) {
	entities := make([]Entity, 1000)
	for i := range entities { entities[i] = Entity{ID: "e" + string(rune('0'+i%10)), Name: "Entity"} }
	err := ExportJSON(entities, nil, "/tmp/test_large_kg.json")
	if err != nil { t.Fatal(err) }
}

func TestSourceExtract_MultipleFiles(t *testing.T) {
	dir := testdataDir()
	files := []string{
		filepath.Join(dir, "store.go"),
		filepath.Join(dir, "analysis.go"),
		filepath.Join(dir, "export.go"),
	}
	entities, relations := ExtractFromSource(files)
	if len(entities) < 3 { t.Errorf("expected 3+ entities from 3 files, got %d", len(entities)) }
	if len(relations) == 0 { t.Error("should find relations") }
	// Check for file entities
	fileCount := 0
	for _, e := range entities { if e.Type == "file" { fileCount++ } }
	if fileCount < 3 { t.Errorf("expected 3 file entities, got %d", fileCount) }
}
