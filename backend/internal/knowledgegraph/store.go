// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package knowledgegraph

import (
	"context"
	"fmt"
	"log/slog"

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
