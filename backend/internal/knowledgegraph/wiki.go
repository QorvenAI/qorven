// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package knowledgegraph

import (
	"context"
	"fmt"
	"math"
	"strings"
)

// wiki.go — Incremental wiki builder from documents/memory.

// WikiConfig defines the wiki's purpose and structure.
type WikiConfig struct {
	Purpose     string            `json:"purpose"`      // WHY this wiki exists
	KeyQuestions []string         `json:"key_questions"` // what we're trying to answer
	Scope       string            `json:"scope"`         // research boundaries
	PageTypes   []string          `json:"page_types"`    // entity, concept, source, synthesis
	BudgetSplit ContextBudget     `json:"budget_split"`
}

type ContextBudget struct {
	WikiPages    float64 `json:"wiki_pages"`    // 0.60
	ChatHistory  float64 `json:"chat_history"`  // 0.20
	Index        float64 `json:"index"`         // 0.05
	SystemPrompt float64 `json:"system_prompt"` // 0.15
}

func DefaultContextBudget() ContextBudget {
	return ContextBudget{WikiPages: 0.60, ChatHistory: 0.20, Index: 0.05, SystemPrompt: 0.15}
}

// WikiPage represents a page in the knowledge wiki.
type WikiPage struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	PageType    string            `json:"page_type"` // entity, concept, source, synthesis, comparison
	Content     string            `json:"content"`
	Sources     []string          `json:"sources"`    // raw source IDs that contributed
	WikiLinks   []string          `json:"wiki_links"` // [[linked page IDs]]
	Frontmatter map[string]string `json:"frontmatter,omitempty"`
}

// IngestAnalysis is the output of Step 1 (analysis) of two-step ingest.
type IngestAnalysis struct {
	KeyEntities    []string          `json:"key_entities"`
	KeyConcepts    []string          `json:"key_concepts"`
	Arguments      []string          `json:"arguments"`
	Connections    []string          `json:"connections"`    // links to existing wiki content
	Contradictions []string          `json:"contradictions"` // tensions with existing knowledge
	StructureRecs  []string          `json:"structure_recs"` // recommended wiki structure
	ReviewItems    []ReviewItem      `json:"review_items"`
	SearchQueries  []string          `json:"search_queries"` // for deep research
}

type ReviewItem struct {
	Action  string `json:"action"` // create_page, deep_research, skip
	Title   string `json:"title"`
	Reason  string `json:"reason"`
	Query   string `json:"query,omitempty"` // search query for deep research
}

// IngestResult is the output of Step 2 (generation).
type IngestResult struct {
	PagesCreated  []WikiPage   `json:"pages_created"`
	PagesUpdated  []string     `json:"pages_updated"`
	ReviewItems   []ReviewItem `json:"review_items"`
}

// BuildIngestAnalysisPrompt creates the Step 1 prompt for analyzing a source document.
func BuildIngestAnalysisPrompt(source, existingIndex, purpose string) string {
	return fmt.Sprintf(`You are a knowledge analyst. Analyze this source document and produce a structured analysis.

PURPOSE: %s

EXISTING WIKI INDEX:
%s

SOURCE DOCUMENT:
%s

Produce a JSON analysis with:
- key_entities: people, organizations, products mentioned
- key_concepts: theories, methods, techniques discussed
- arguments: main claims and positions
- connections: how this relates to existing wiki content
- contradictions: tensions with existing knowledge
- structure_recs: recommended wiki pages to create/update
- review_items: items needing human judgment (action: create_page|deep_research|skip)
- search_queries: web search queries for knowledge gaps`, purpose, existingIndex, source)
}

// BuildIngestGenerationPrompt creates the Step 2 prompt for generating wiki pages.
func BuildIngestGenerationPrompt(analysis, purpose, schema string) string {
	return fmt.Sprintf(`You are a wiki builder. Based on this analysis, generate wiki pages.

PURPOSE: %s
SCHEMA: %s

ANALYSIS:
%s

Generate wiki pages as JSON array. Each page needs:
- title, page_type (entity/concept/source/synthesis), content (markdown with [[wikilinks]])
- sources: list of source IDs that contributed
- frontmatter: type, title, created date

Rules:
- Use [[wikilinks]] for cross-references
- Include YAML frontmatter on every page
- Create a source summary page
- Update index entries for new pages`, purpose, schema, analysis)
}

// ── 4-Signal Relevance Model for Knowledge Graph ──

type RelevanceSignal struct {
	DirectLink   float64 `json:"direct_link"`    // ×3.0 — pages linked via wikilinks
	SourceOverlap float64 `json:"source_overlap"` // ×4.0 — pages sharing same raw source
	AdamicAdar   float64 `json:"adamic_adar"`    // ×1.5 — shared neighbors weighted by degree
	TypeAffinity float64 `json:"type_affinity"`  // ×1.0 — same page type bonus
}

const (
	WeightDirectLink   = 3.0
	WeightSourceOverlap = 4.0
	WeightAdamicAdar   = 1.5
	WeightTypeAffinity = 1.0
)

func (r RelevanceSignal) Score() float64 {
	return r.DirectLink*WeightDirectLink +
		r.SourceOverlap*WeightSourceOverlap +
		r.AdamicAdar*WeightAdamicAdar +
		r.TypeAffinity*WeightTypeAffinity
}

// ComputeRelevance calculates the 4-signal relevance between two entities.
func (s *Store) ComputeRelevance(ctx context.Context, tenantID, entityA, entityB string) (RelevanceSignal, error) {
	var signal RelevanceSignal

	// Signal 1: Direct link (are they connected by a relationship?)
	var directCount int
	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM kg_relationships
		WHERE tenant_id = $1 AND ((source_id = $2 AND target_id = $3) OR (source_id = $3 AND target_id = $2))`,
		tenantID, entityA, entityB).Scan(&directCount)
	if directCount > 0 { signal.DirectLink = 1.0 }

	// Signal 2: Source overlap (do they share source documents?)
	var overlapCount int
	s.pool.QueryRow(ctx, `SELECT COUNT(DISTINCT a.source_id) FROM
		(SELECT unnest(ARRAY[source]) AS source_id FROM kg_entities WHERE id = $1) a
		JOIN (SELECT unnest(ARRAY[source]) AS source_id FROM kg_entities WHERE id = $2) b
		ON a.source_id = b.source_id`, entityA, entityB).Scan(&overlapCount)
	if overlapCount > 0 { signal.SourceOverlap = float64(overlapCount) / 5.0 } // normalize to ~1.0
	if signal.SourceOverlap > 1.0 { signal.SourceOverlap = 1.0 }

	// Signal 3: Adamic-Adar (shared neighbors weighted by inverse log degree)
	rows, err := s.pool.Query(ctx, `
		WITH neighbors_a AS (SELECT target_id AS nid FROM kg_relationships WHERE source_id = $1 AND tenant_id = $3
			UNION SELECT source_id FROM kg_relationships WHERE target_id = $1 AND tenant_id = $3),
		neighbors_b AS (SELECT target_id AS nid FROM kg_relationships WHERE source_id = $2 AND tenant_id = $3
			UNION SELECT source_id FROM kg_relationships WHERE target_id = $2 AND tenant_id = $3),
		shared AS (SELECT a.nid FROM neighbors_a a JOIN neighbors_b b ON a.nid = b.nid)
		SELECT s.nid, (SELECT COUNT(*) FROM kg_relationships WHERE source_id = s.nid OR target_id = s.nid) AS degree
		FROM shared s`, entityA, entityB, tenantID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var nid string
			var degree int
			rows.Scan(&nid, &degree)
			if degree > 1 { signal.AdamicAdar += 1.0 / math.Log(float64(degree)) }
		}
		if signal.AdamicAdar > 1.0 { signal.AdamicAdar = 1.0 }
	}

	// Signal 4: Type affinity (same entity type?)
	var typeA, typeB string
	s.pool.QueryRow(ctx, "SELECT entity_type FROM kg_entities WHERE id = $1", entityA).Scan(&typeA)
	s.pool.QueryRow(ctx, "SELECT entity_type FROM kg_entities WHERE id = $2", entityB).Scan(&typeB)
	if typeA != "" && typeA == typeB { signal.TypeAffinity = 1.0 }

	return signal, nil
}

// ── Cascade Deletion ──

// CascadeDelete removes an entity and cleans up all references.
func (s *Store) CascadeDelete(ctx context.Context, tenantID, entityID string) (int, error) {
	deleted := 0

	// 1. Delete relationships involving this entity
	tag, _ := s.pool.Exec(ctx, `DELETE FROM kg_relationships WHERE tenant_id = $1 AND (source_id = $2 OR target_id = $2)`, tenantID, entityID)
	deleted += int(tag.RowsAffected())

	// 2. Remove from source_ids arrays of other entities (shared entity preservation)
	s.pool.Exec(ctx, `UPDATE kg_entities SET source_ids = array_remove(source_ids, $1) WHERE tenant_id = $2 AND $1 = ANY(source_ids)`, entityID, tenantID)

	// 3. Delete the entity itself
	tag, _ = s.pool.Exec(ctx, `DELETE FROM kg_entities WHERE id = $1 AND tenant_id = $2`, entityID, tenantID)
	deleted += int(tag.RowsAffected())

	return deleted, nil
}

// ── Budget-Controlled Context Assembly ──

// AssembleContext builds the context for a query using budget-controlled retrieval.
func (s *Store) AssembleContext(ctx context.Context, tenantID, query string, budget ContextBudget, maxTokens int) (string, error) {
	wikiTokens := int(float64(maxTokens) * budget.WikiPages)

	// Search entities by query
	entities, err := s.SearchEntities(ctx, tenantID, query, 20)
	if err != nil { return "", err }

	// Expand via graph (get neighbors of top results)
	expanded := []Entity{}
	for i, e := range entities {
		if i >= 5 { break }
		neighbors, _ := s.GetNeighbors(ctx, tenantID, e.ID)
		for _, n := range neighbors {
			name, _ := n["name"].(string)
			id, _ := n["id"].(string)
			expanded = append(expanded, Entity{ID: id, Name: name})
		}
	}

	// Build context within budget
	var b strings.Builder
	tokensUsed := 0
	for _, e := range append(entities, expanded...) {
		entry := fmt.Sprintf("## %s\nType: %s\n%s\n\n", e.Name, e.EntityType, e.Source)
		entryTokens := len(entry) / 4
		if tokensUsed+entryTokens > wikiTokens { break }
		b.WriteString(entry)
		tokensUsed += entryTokens
	}

	return b.String(), nil
}
