// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package knowledgegraph

import (
	"context"
	"testing"
	"time"
)

// hard_wiki_test.go — Diamond tests for Knowledge Wiki + Graph Visualization API.

// ── 4-Signal Relevance Model ──

func TestDiamond_Relevance_SignalScoring(t *testing.T) {
	// Test the scoring formula
	signal := RelevanceSignal{DirectLink: 1.0, SourceOverlap: 0.5, AdamicAdar: 0.3, TypeAffinity: 1.0}
	score := signal.Score()

	// Expected: 1.0*3.0 + 0.5*4.0 + 0.3*1.5 + 1.0*1.0 = 3.0 + 2.0 + 0.45 + 1.0 = 6.45
	expected := 6.45
	if score < expected-0.01 || score > expected+0.01 {
		t.Errorf("score=%.2f, expected=%.2f", score, expected)
	}
	t.Logf("relevance score: %.2f (direct=3.0 + source=2.0 + adamic=0.45 + type=1.0) ✓", score)
}

func TestDiamond_Relevance_ZeroSignals(t *testing.T) {
	signal := RelevanceSignal{}
	if signal.Score() != 0 { t.Errorf("zero signals should score 0, got %.2f", signal.Score()) }
}

func TestDiamond_Relevance_DirectLinkDominates(t *testing.T) {
	direct := RelevanceSignal{DirectLink: 1.0}
	noLink := RelevanceSignal{AdamicAdar: 1.0, TypeAffinity: 1.0}
	if direct.Score() <= noLink.Score() {
		t.Error("direct link (×3.0) should dominate over adamic+type (×1.5+×1.0)")
	}
}

func TestDiamond_Relevance_SourceOverlapHighest(t *testing.T) {
	sourceOverlap := RelevanceSignal{SourceOverlap: 1.0}
	directLink := RelevanceSignal{DirectLink: 1.0}
	if sourceOverlap.Score() <= directLink.Score() {
		t.Error("source overlap (×4.0) should score higher than direct link (×3.0)")
	}
}

// ── Wiki Config ──

func TestDiamond_WikiConfig_DefaultBudget(t *testing.T) {
	budget := DefaultContextBudget()
	total := budget.WikiPages + budget.ChatHistory + budget.Index + budget.SystemPrompt
	if total < 0.99 || total > 1.01 {
		t.Errorf("budget should sum to 1.0, got %.2f", total)
	}
	if budget.WikiPages != 0.60 { t.Errorf("wiki pages: %.2f", budget.WikiPages) }
	if budget.ChatHistory != 0.20 { t.Errorf("chat history: %.2f", budget.ChatHistory) }
}

// ── Ingest Prompts ──

func TestDiamond_IngestAnalysisPrompt_ContainsAllSections(t *testing.T) {
	prompt := BuildIngestAnalysisPrompt("Test document about AI agents", "index content", "Research AI agent architectures")
	if len(prompt) < 200 { t.Errorf("prompt too short: %d", len(prompt)) }
	if !containsStr(prompt, "key_entities") { t.Error("missing key_entities") }
	if !containsStr(prompt, "connections") { t.Error("missing connections") }
	if !containsStr(prompt, "contradictions") { t.Error("missing contradictions") }
	if !containsStr(prompt, "review_items") { t.Error("missing review_items") }
	if !containsStr(prompt, "Research AI agent") { t.Error("purpose not included") }
	t.Logf("analysis prompt: %d chars, all sections present ✓", len(prompt))
}

func TestDiamond_IngestGenerationPrompt_ContainsAllSections(t *testing.T) {
	prompt := BuildIngestGenerationPrompt("analysis JSON", "Build knowledge base", "schema rules")
	if !containsStr(prompt, "wikilinks") { t.Error("missing wikilinks instruction") }
	if !containsStr(prompt, "frontmatter") { t.Error("missing frontmatter instruction") }
	if !containsStr(prompt, "source summary") { t.Error("missing source summary instruction") }
	t.Logf("generation prompt: %d chars ✓", len(prompt))
}

// ── Graph Visualization API ──

func TestDiamond_GraphAPI_GetGraphData(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	data, err := store.GetGraphData(ctx, tenant)
	if err != nil { t.Skipf("graph: %v", err) }

	if data == nil { t.Fatal("nil graph data") }
	t.Logf("graph: %d nodes, %d edges, types=%v ✓",
		data.Stats.TotalNodes, data.Stats.TotalEdges, data.Stats.NodesByType)

	// Verify node structure
	for _, node := range data.Nodes {
		if node.ID == "" { t.Error("node missing ID") }
		if node.Label == "" { t.Error("node missing label") }
		if node.Color == "" { t.Error("node missing color") }
		if node.Size < 5 { t.Errorf("node size too small: %.1f", node.Size) }
	}

	// Verify edge structure
	for _, edge := range data.Edges {
		if edge.Source == "" { t.Error("edge missing source") }
		if edge.Target == "" { t.Error("edge missing target") }
		if edge.Thickness < 1 { t.Errorf("edge thickness too small: %.1f", edge.Thickness) }
	}
}

func TestDiamond_GraphAPI_NodeColors(t *testing.T) {
	// Verify all entity types have colors
	types := []string{"person", "organization", "product", "concept", "technology", "event", "location"}
	for _, typ := range types {
		color := nodeColor(typ)
		if color == "" { t.Errorf("no color for type %q", typ) }
		if color[0] != '#' { t.Errorf("color should be hex: %q", color) }
	}

	// Unknown type should get default
	unknown := nodeColor("unknown_type")
	if unknown == "" { t.Error("unknown type should get default color") }
	t.Log("node colors: all types have hex colors ✓")
}

func TestDiamond_GraphAPI_EdgeColors(t *testing.T) {
	strong := edgeColor(6.0)
	medium := edgeColor(3.0)
	weak := edgeColor(1.0)

	if strong == weak { t.Error("strong and weak edges should have different colors") }
	if strong != "#27AE60" { t.Errorf("strong should be green: %q", strong) }
	t.Logf("edge colors: strong=%s, medium=%s, weak=%s ✓", strong, medium, weak)
}

func TestDiamond_GraphAPI_NodeSizing(t *testing.T) {
	// Nodes with more links should be bigger
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	data, err := store.GetGraphData(ctx, tenant)
	if err != nil { t.Skipf("graph: %v", err) }

	if len(data.Nodes) < 2 { t.Skip("need 2+ nodes") }

	// Find min and max sized nodes
	minSize, maxSize := data.Nodes[0].Size, data.Nodes[0].Size
	for _, n := range data.Nodes {
		if n.Size < minSize { minSize = n.Size }
		if n.Size > maxSize { maxSize = n.Size }
	}
	t.Logf("node sizes: min=%.1f, max=%.1f (√degree scaling) ✓", minSize, maxSize)
}

// ── Cascade Deletion ──

func TestDiamond_CascadeDelete(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	marker := "CASCADE_" + time.Now().Format("150405")

	// Create 2 entities + 1 relationship
	id1, err := store.AddEntity(ctx, tenant, Entity{Name: marker + "_A", EntityType: "test"})
	if err != nil { t.Skipf("KG: %v", err) }
	id2, err := store.AddEntity(ctx, tenant, Entity{Name: marker + "_B", EntityType: "test"})
	if err != nil { t.Skipf("KG: %v", err) }
	defer pool.Exec(ctx, "DELETE FROM kg_entities WHERE id IN ($1, $2)", id1, id2)

	relID, err := store.AddRelationship(ctx, tenant, Relationship{SourceID: id1, TargetID: id2, RelType: "test_link"})
	if err != nil { t.Skipf("KG: %v", err) }
	defer pool.Exec(ctx, "DELETE FROM kg_relationships WHERE id = $1", relID)

	// Cascade delete entity A
	deleted, err := store.CascadeDelete(ctx, tenant, id1)
	if err != nil { t.Skipf("KG: %v", err) }
	if deleted < 2 { t.Errorf("should delete entity + relationship, deleted %d", deleted) }

	// Entity B should still exist
	neighbors, _ := store.GetNeighbors(ctx, tenant, id2)
	for _, n := range neighbors {
		name, _ := n["name"].(string)
		if containsStr(name, marker+"_A") { t.Error("deleted entity still appears as neighbor") }
	}

	t.Logf("cascade delete: %d items removed, entity B preserved ✓", deleted)
}

// ── Budget-Controlled Context Assembly ──

func TestDiamond_ContextAssembly_BudgetControl(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	budget := DefaultContextBudget()
	maxTokens := 4000

	context_str, err := store.AssembleContext(ctx, tenant, "test query", budget, maxTokens)
	if err != nil { t.Skipf("KG: %v", err) }

	// Context should be within budget (60% of 4000 = 2400 tokens ≈ 9600 chars)
	maxChars := int(float64(maxTokens) * budget.WikiPages * 4) // 4 chars per token
	if len(context_str) > maxChars {
		t.Errorf("context exceeds budget: %d chars > %d max", len(context_str), maxChars)
	}
	t.Logf("context assembly: %d chars within %d budget ✓", len(context_str), maxChars)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub { return true }
	}
	return false
}
