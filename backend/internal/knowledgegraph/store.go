// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package knowledgegraph

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Entity struct {
	ID         string            `json:"id"`
	TenantID   string            `json:"tenant_id"`
	AgentID    string            `json:"agent_id,omitempty"`
	Name       string            `json:"name"`
	EntityType string            `json:"entity_type"`
	Properties map[string]string `json:"properties"`
	Source     string            `json:"source,omitempty"`
	Confidence float64           `json:"confidence"`
}

type Relationship struct {
	ID         string            `json:"id"`
	SourceID   string            `json:"source_id"`
	TargetID   string            `json:"target_id"`
	RelType    string            `json:"rel_type"`
	Properties map[string]string `json:"properties"`
	Confidence float64           `json:"confidence"`
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) AddEntity(ctx context.Context, tenantID string, e Entity) (string, error) {
	var id string
	// Treat empty agent_id as NULL (column is nullable)
	var agentID any = e.AgentID
	if e.AgentID == "" { agentID = nil }
	// Default properties to empty JSON object if nil
	props := e.Properties
	if props == nil { props = map[string]string{} }
	err := s.pool.QueryRow(ctx, `INSERT INTO kg_entities (tenant_id, agent_id, name, entity_type, properties, source, confidence)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		tenantID, agentID, e.Name, e.EntityType, props, e.Source, e.Confidence).Scan(&id)
	if err != nil { return "", err }
	slog.Debug("kg entity added", "name", e.Name, "type", e.EntityType)
	return id, nil
}

func (s *Store) AddRelationship(ctx context.Context, tenantID string, r Relationship) (string, error) {
	var id string
	props := r.Properties
	if props == nil { props = map[string]string{} }
	err := s.pool.QueryRow(ctx, `INSERT INTO kg_relationships (tenant_id, source_id, target_id, rel_type, properties, confidence)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		tenantID, r.SourceID, r.TargetID, r.RelType, props, r.Confidence).Scan(&id)
	return id, err
}

func (s *Store) SearchEntities(ctx context.Context, tenantID, query string, limit int) ([]Entity, error) {
	if limit <= 0 { limit = 20 }
	rows, err := s.pool.Query(ctx, `SELECT id, tenant_id, agent_id, name, entity_type, source, confidence
		FROM kg_entities WHERE tenant_id=$1 AND name ILIKE '%' || $2 || '%'
		ORDER BY confidence DESC LIMIT $3`, tenantID, query, limit)
	if err != nil { return nil, err }
	defer rows.Close()

	entities := []Entity{}
	for rows.Next() {
		e := Entity{}
		rows.Scan(&e.ID, &e.TenantID, &e.AgentID, &e.Name, &e.EntityType, &e.Source, &e.Confidence)
		entities = append(entities, e)
	}
	return entities, nil
}

func (s *Store) GetRelationships(ctx context.Context, tenantID, entityID string) ([]Relationship, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, source_id, target_id, rel_type, confidence
		FROM kg_relationships WHERE tenant_id=$1 AND (source_id=$2 OR target_id=$2)`, tenantID, entityID)
	if err != nil { return nil, err }
	defer rows.Close()

	rels := []Relationship{}
	for rows.Next() {
		r := Relationship{}
		rows.Scan(&r.ID, &r.SourceID, &r.TargetID, &r.RelType, &r.Confidence)
		rels = append(rels, r)
	}
	return rels, nil
}

// TraverseBFS performs breadth-first traversal from a starting entity.
// Returns all entities reachable within maxDepth hops.
func (s *Store) TraverseBFS(ctx context.Context, tenantID, startEntityID string, maxDepth int) ([]Entity, []Relationship, error) {
	if maxDepth <= 0 { maxDepth = 3 }

	visited := map[string]bool{startEntityID: true}
	queue := []string{startEntityID}
	allEntities := []Entity{}
	allRels := []Relationship{}

	for depth := 0; depth < maxDepth && len(queue) > 0; depth++ {
		nextQueue := []string{}
		for _, entityID := range queue {
			rels, err := s.GetRelationships(ctx, tenantID, entityID)
			if err != nil { continue }
			for _, r := range rels {
				allRels = append(allRels, r)
				targetID := r.TargetID
				if targetID == entityID { targetID = r.SourceID }
				if !visited[targetID] {
					visited[targetID] = true
					nextQueue = append(nextQueue, targetID)
				}
			}
		}
		queue = nextQueue
	}

	// Fetch all visited entities
	for id := range visited {
		rows, err := s.pool.Query(ctx,
			`SELECT id, name, entity_type, COALESCE(source, '') FROM kg_entities WHERE id = $1`, id)
		if err != nil { continue }
		for rows.Next() {
			var e Entity
			rows.Scan(&e.ID, &e.Name, &e.EntityType, &e.Source)
			allEntities = append(allEntities, e)
		}
		rows.Close()
	}

	return allEntities, allRels, nil
}

// FindPath finds the shortest path between two entities using BFS.
func (s *Store) FindPath(ctx context.Context, tenantID, fromID, toID string, maxDepth int) ([]string, error) {
	if maxDepth <= 0 { maxDepth = 5 }

	type node struct {
		id   string
		path []string
	}

	visited := map[string]bool{fromID: true}
	queue := []node{{id: fromID, path: []string{fromID}}}

	for depth := 0; depth < maxDepth && len(queue) > 0; depth++ {
		nextQueue := []node{}
		for _, n := range queue {
			rels, _ := s.GetRelationships(ctx, tenantID, n.id)
			for _, r := range rels {
				targetID := r.TargetID
				if targetID == n.id { targetID = r.SourceID }
				if targetID == toID {
					return append(n.path, toID), nil
				}
				if !visited[targetID] {
					visited[targetID] = true
					nextQueue = append(nextQueue, node{id: targetID, path: append(append([]string{}, n.path...), targetID)})
				}
			}
		}
		queue = nextQueue
	}

	return nil, fmt.Errorf("no path found within %d hops", maxDepth)
}

// MergeDuplicates finds entities with similar names and merges them.
func (s *Store) MergeDuplicates(ctx context.Context, tenantID string) (int, error) {
	// Find entities with same name (case-insensitive)
	rows, err := s.pool.Query(ctx,
		`SELECT LOWER(name), array_agg(id ORDER BY created_at) 
		 FROM kg_entities WHERE tenant_id = $1
		 GROUP BY LOWER(name) HAVING COUNT(*) > 1`, tenantID)
	if err != nil { return 0, err }
	defer rows.Close()

	merged := 0
	for rows.Next() {
		var name string
		ids := []string{}
		if rows.Scan(&name, &ids) != nil || len(ids) < 2 { continue }

		// Keep first (oldest), merge relationships from others
		keepID := ids[0]
		for _, removeID := range ids[1:] {
			// Move relationships to the kept entity
			s.pool.Exec(ctx, `UPDATE kg_relationships SET source_id = $1 WHERE source_id = $2`, keepID, removeID)
			s.pool.Exec(ctx, `UPDATE kg_relationships SET target_id = $1 WHERE target_id = $2`, keepID, removeID)
			// Delete duplicate
			s.pool.Exec(ctx, `DELETE FROM kg_entities WHERE id = $1`, removeID)
			merged++
		}
	}
	return merged, nil
}

// GetNeighbors returns immediate neighbors of an entity with relationship types.
func (s *Store) GetNeighbors(ctx context.Context, tenantID, entityID string) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT e.id, e.name, e.entity_type, r.rel_type, r.confidence
		 FROM kg_relationships r
		 JOIN kg_entities e ON (e.id = r.target_id OR e.id = r.source_id) AND e.id != $1
		 WHERE (r.source_id = $1 OR r.target_id = $1) AND r.tenant_id = $2
		 ORDER BY r.confidence DESC`, entityID, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()

	neighbors := []map[string]any{}
	for rows.Next() {
		var id, name, eType, relType string
		var weight float64
		rows.Scan(&id, &name, &eType, &relType, &weight)
		neighbors = append(neighbors, map[string]any{
			"id": id, "name": name, "type": eType, "relationship": relType, "weight": weight,
		})
	}
	return neighbors, nil
}

// UpsertEntity inserts or updates an entity, deduped on (tenant_id, name,
// entity_type). Returns the entity's UUID (new or existing). Merges properties,
// keeps the higher confidence, and preserves the first agent_id as provenance.
func (s *Store) UpsertEntity(ctx context.Context, tenantID string, e Entity) (string, error) {
	var agentID any = e.AgentID
	if e.AgentID == "" {
		agentID = nil
	}
	props := e.Properties
	if props == nil {
		props = map[string]string{}
	}
	conf := e.Confidence
	if conf == 0 {
		conf = 1.0
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO kg_entities (tenant_id, agent_id, name, entity_type, properties, source, confidence)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (tenant_id, name, entity_type) DO UPDATE SET
		   properties = COALESCE(kg_entities.properties, '{}'::jsonb) || EXCLUDED.properties,
		   confidence = GREATEST(kg_entities.confidence, EXCLUDED.confidence),
		   source = COALESCE(NULLIF(EXCLUDED.source, ''), kg_entities.source),
		   updated_at = now()
		 RETURNING id`,
		tenantID, agentID, e.Name, e.EntityType, props, e.Source, conf,
	).Scan(&id)
	return id, err
}

// UpsertRelationship inserts or updates an edge, deduped on
// (tenant_id, source_id, target_id, rel_type). source_id/target_id MUST be real
// entity UUIDs (the caller remaps LLM names → UUIDs first).
func (s *Store) UpsertRelationship(ctx context.Context, tenantID string, r Relationship) (string, error) {
	props := r.Properties
	if props == nil {
		props = map[string]string{}
	}
	conf := r.Confidence
	if conf == 0 {
		conf = 1.0
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO kg_relationships (tenant_id, source_id, target_id, rel_type, properties, confidence)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tenant_id, source_id, target_id, rel_type) DO UPDATE SET
		   properties = COALESCE(kg_relationships.properties, '{}'::jsonb) || EXCLUDED.properties,
		   confidence = GREATEST(kg_relationships.confidence, EXCLUDED.confidence)
		 RETURNING id`,
		tenantID, r.SourceID, r.TargetID, r.RelType, props, conf,
	).Scan(&id)
	return id, err
}

// RelevantContext finds entities matching the query and their 1-hop relationships,
// tenant-scoped — the data injected into the agent prompt. Returns entities, their
// edges, and a uuid→name map for rendering. maxEntities bounds token cost.
func (s *Store) RelevantContext(ctx context.Context, tenantID, query string, maxEntities int) ([]Entity, []Relationship, map[string]string, error) {
	if maxEntities <= 0 {
		maxEntities = 5
	}
	ents, err := s.SearchEntities(ctx, tenantID, query, maxEntities)
	if err != nil || len(ents) == 0 {
		return ents, nil, nil, err
	}
	nameMap := map[string]string{}
	for _, e := range ents {
		nameMap[e.ID] = e.Name
	}
	rels := []Relationship{}
	for _, e := range ents {
		ers, gerr := s.GetRelationships(ctx, tenantID, e.ID)
		if gerr != nil {
			continue
		}
		rels = append(rels, ers...)
	}
	for _, r := range rels {
		for _, eid := range []string{r.SourceID, r.TargetID} {
			if _, ok := nameMap[eid]; !ok {
				var n string
				if s.pool.QueryRow(ctx, `SELECT name FROM kg_entities WHERE tenant_id=$1 AND id=$2`, tenantID, eid).Scan(&n) == nil {
					nameMap[eid] = n
				}
			}
		}
	}
	return ents, rels, nameMap, nil
}

// FormatForPrompt renders entities + relationships for injection into the agent
// prompt. nameMap resolves relationship endpoint UUIDs → names. Empty input → "".
func FormatForPrompt(entities []Entity, rels []Relationship, nameMap map[string]string) string {
	if len(entities) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Entities\n")
	for _, e := range entities {
		b.WriteString(fmt.Sprintf("- **%s** (%s)\n", e.Name, e.EntityType))
	}
	if len(rels) > 0 {
		b.WriteString("\n### Relationships\n")
		for _, r := range rels {
			src := nameMap[r.SourceID]
			if src == "" {
				src = "?"
			}
			tgt := nameMap[r.TargetID]
			if tgt == "" {
				tgt = "?"
			}
			b.WriteString(fmt.Sprintf("- %s —[%s]→ %s\n", src, r.RelType, tgt))
		}
	}
	return b.String()
}
