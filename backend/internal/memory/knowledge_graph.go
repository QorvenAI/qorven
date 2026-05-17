// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Entity is a node in the knowledge graph.
type Entity struct {
	ID         string            `json:"id"`
	AgentID    string            `json:"agent_id"`
	Name       string            `json:"name"`
	EntityType string            `json:"entity_type"` // person, project, concept, tool, location, org
	Properties map[string]string `json:"properties,omitempty"`
	Source     string            `json:"source,omitempty"`
	Confidence float64           `json:"confidence"`
	CreatedAt  time.Time         `json:"created_at"`
}

// Relationship is an edge in the knowledge graph.
type Relationship struct {
	ID         string            `json:"id"`
	SourceID   string            `json:"source_id"`
	TargetID   string            `json:"target_id"`
	RelType    string            `json:"rel_type"` // works_on, knows, uses, part_of, manages, created
	Properties map[string]string `json:"properties,omitempty"`
	Confidence float64           `json:"confidence"`
	CreatedAt  time.Time         `json:"created_at"`
}

// KnowledgeGraph provides entity and relationship storage + traversal.
type KnowledgeGraph struct {
	pool *pgxpool.Pool
}

func NewKnowledgeGraph(pool *pgxpool.Pool) *KnowledgeGraph {
	return &KnowledgeGraph{pool: pool}
}

// UpsertEntity creates or updates an entity by name + type (dedup).
func (kg *KnowledgeGraph) UpsertEntity(ctx context.Context, e Entity) (string, error) {
	var id string
	err := kg.pool.QueryRow(ctx,
		`INSERT INTO kg_entities (agent_id, name, entity_type, properties, source, confidence)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (agent_id, name, entity_type) DO UPDATE SET
		   properties = COALESCE(kg_entities.properties, '{}')::jsonb || COALESCE($4, '{}')::jsonb,
		   confidence = GREATEST(kg_entities.confidence, $6),
		   source = COALESCE($5, kg_entities.source)
		 RETURNING id`,
		e.AgentID, e.Name, e.EntityType, e.Properties, nilIfEmpty(e.Source), e.Confidence,
	).Scan(&id)
	return id, err
}

// UpsertRelationship creates or updates a relationship (dedup by source+target+type).
func (kg *KnowledgeGraph) UpsertRelationship(ctx context.Context, r Relationship) (string, error) {
	var id string
	err := kg.pool.QueryRow(ctx,
		`INSERT INTO kg_relationships (source_id, target_id, rel_type, properties, confidence)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (source_id, target_id, rel_type) DO UPDATE SET
		   properties = COALESCE(kg_relationships.properties, '{}')::jsonb || COALESCE($4, '{}')::jsonb,
		   confidence = GREATEST(kg_relationships.confidence, $5)
		 RETURNING id`,
		r.SourceID, r.TargetID, r.RelType, r.Properties, r.Confidence,
	).Scan(&id)
	return id, err
}

// SearchEntities finds entities by name or type.
func (kg *KnowledgeGraph) SearchEntities(ctx context.Context, agentID, query string, limit int) ([]Entity, error) {
	if limit <= 0 { limit = 20 }
	rows, err := kg.pool.Query(ctx,
		`SELECT id, agent_id, name, entity_type, properties, source, confidence, created_at
		 FROM kg_entities
		 WHERE agent_id = $1 AND (name ILIKE '%' || $2 || '%' OR entity_type = $2)
		 ORDER BY confidence DESC, created_at DESC
		 LIMIT $3`,
		agentID, query, limit)
	if err != nil { return nil, err }
	defer rows.Close()

	var entities []Entity
	for rows.Next() {
		var e Entity
		var source *string
		rows.Scan(&e.ID, &e.AgentID, &e.Name, &e.EntityType, &e.Properties, &source, &e.Confidence, &e.CreatedAt)
		if source != nil { e.Source = *source }
		entities = append(entities, e)
	}
	return entities, nil
}

// Traverse finds connected entities up to maxDepth hops from a starting entity.
func (kg *KnowledgeGraph) Traverse(ctx context.Context, entityID string, maxDepth int) ([]Entity, []Relationship, error) {
	if maxDepth <= 0 { maxDepth = 2 }
	if maxDepth > 5 { maxDepth = 5 }

	// Get relationships from this entity
	rows, err := kg.pool.Query(ctx,
		`WITH RECURSIVE graph AS (
			SELECT source_id, target_id, rel_type, properties, confidence, 1 AS depth
			FROM kg_relationships WHERE source_id = $1 OR target_id = $1
			UNION
			SELECT r.source_id, r.target_id, r.rel_type, r.properties, r.confidence, g.depth + 1
			FROM kg_relationships r
			JOIN graph g ON (r.source_id = g.target_id OR r.target_id = g.source_id)
			WHERE g.depth < $2
		)
		SELECT DISTINCT source_id, target_id, rel_type, properties, confidence FROM graph`,
		entityID, maxDepth)
	if err != nil { return nil, nil, err }
	defer rows.Close()

	entityIDs := map[string]bool{entityID: true}
	var rels []Relationship
	for rows.Next() {
		var r Relationship
		rows.Scan(&r.SourceID, &r.TargetID, &r.RelType, &r.Properties, &r.Confidence)
		rels = append(rels, r)
		entityIDs[r.SourceID] = true
		entityIDs[r.TargetID] = true
	}

	// Fetch all referenced entities
	ids := make([]string, 0, len(entityIDs))
	for id := range entityIDs { ids = append(ids, id) }

	eRows, err := kg.pool.Query(ctx,
		`SELECT id, agent_id, name, entity_type, properties, source, confidence, created_at
		 FROM kg_entities WHERE id = ANY($1)`, ids)
	if err != nil { return nil, rels, err }
	defer eRows.Close()

	var entities []Entity
	for eRows.Next() {
		var e Entity
		var source *string
		eRows.Scan(&e.ID, &e.AgentID, &e.Name, &e.EntityType, &e.Properties, &source, &e.Confidence, &e.CreatedAt)
		if source != nil { e.Source = *source }
		entities = append(entities, e)
	}
	return entities, rels, nil
}

// FormatForContext renders KG results as a context string for the LLM.
func FormatKGForContext(entities []Entity, rels []Relationship) string {
	if len(entities) == 0 { return "" }
	result := "## Knowledge Graph Context\n\n"
	result += "### Entities\n"
	for _, e := range entities {
		result += fmt.Sprintf("- **%s** (%s) [confidence: %.0f%%]\n", e.Name, e.EntityType, e.Confidence*100)
	}
	if len(rels) > 0 {
		result += "\n### Relationships\n"
		// Build name lookup
		nameMap := make(map[string]string)
		for _, e := range entities { nameMap[e.ID] = e.Name }
		for _, r := range rels {
			src := nameMap[r.SourceID]
			tgt := nameMap[r.TargetID]
			if src == "" { src = r.SourceID[:8] }
			if tgt == "" { tgt = r.TargetID[:8] }
			result += fmt.Sprintf("- %s —[%s]→ %s\n", src, r.RelType, tgt)
		}
	}
	return result
}
