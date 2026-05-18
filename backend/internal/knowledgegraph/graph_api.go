// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package knowledgegraph

import (
	"context"
	"fmt"
	"math"
)

// graph_api.go — Knowledge Graph visualization API.
// Exposes graph data for the web UI (ForceAtlas2-compatible).

// GraphData is the complete graph for visualization.
type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
	Stats GraphStats  `json:"stats"`
}

// GraphNode represents an entity in the visualization.
type GraphNode struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	Type        string  `json:"type"`        // entity, concept, source, synthesis
	Color       string  `json:"color"`       // hex color by type
	Size        float64 `json:"size"`        // scaled by link count
	X           float64 `json:"x,omitempty"` // layout position (optional)
	Y           float64 `json:"y,omitempty"`
	LinkCount   int     `json:"link_count"`
	Description string  `json:"description,omitempty"`
}

// GraphEdge represents a relationship in the visualization.
type GraphEdge struct {
	ID        string  `json:"id"`
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Label     string  `json:"label"`      // relationship type
	Weight    float64 `json:"weight"`     // relevance score
	Color     string  `json:"color"`      // green=strong, gray=weak
	Thickness float64 `json:"thickness"`  // scaled by weight
}

// GraphStats provides summary information.
type GraphStats struct {
	TotalNodes    int            `json:"total_nodes"`
	TotalEdges    int            `json:"total_edges"`
	NodesByType   map[string]int `json:"nodes_by_type"`
	AvgDegree     float64        `json:"avg_degree"`
	MaxDegree     int            `json:"max_degree"`
	Components    int            `json:"components"`
}

// Node colors by entity type
var typeColors = map[string]string{
	"person":       "#4A90D9", // blue
	"organization": "#E67E22", // orange
	"product":      "#2ECC71", // green
	"concept":      "#9B59B6", // purple
	"technology":   "#1ABC9C", // teal
	"event":        "#E74C3C", // red
	"location":     "#F39C12", // yellow
	"source":       "#95A5A6", // gray
	"synthesis":    "#3498DB", // light blue
	"comparison":   "#E91E63", // pink
}

// GetGraphData returns the full graph for visualization.
func (s *Store) GetGraphData(ctx context.Context, tenantID string) (*GraphData, error) {
	// Fetch all entities
	rows, err := s.pool.Query(ctx, `SELECT id, name, entity_type, COALESCE(description, '') FROM kg_entities WHERE tenant_id = $1`, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()

	nodeMap := map[string]*GraphNode{}
	nodes := []GraphNode{}
	for rows.Next() {
		var id, name, etype, desc string
		rows.Scan(&id, &name, &etype, &desc)
		node := GraphNode{
			ID: id, Label: name, Type: etype, Description: desc,
			Color: nodeColor(etype), Size: 5, // default, scaled later
		}
		nodes = append(nodes, node)
		nodeMap[id] = &nodes[len(nodes)-1]
	}

	// Fetch all relationships
	relRows, err := s.pool.Query(ctx, `SELECT id, source_id, target_id, rel_type FROM kg_relationships WHERE tenant_id = $1`, tenantID)
	if err != nil { return nil, err }
	defer relRows.Close()

	edges := []GraphEdge{}
	degreeMap := map[string]int{}
	for relRows.Next() {
		var id, src, tgt, relType string
		relRows.Scan(&id, &src, &tgt, &relType)
		degreeMap[src]++
		degreeMap[tgt]++

		weight := 1.0 // default weight
		edges = append(edges, GraphEdge{
			ID: id, Source: src, Target: tgt, Label: relType,
			Weight: weight, Color: edgeColor(weight), Thickness: edgeThickness(weight),
		})
	}

	// Scale node sizes by link count (sqrt scaling)
	maxDegree := 0
	for id, degree := range degreeMap {
		if degree > maxDegree { maxDegree = degree }
		if node, ok := nodeMap[id]; ok {
			node.LinkCount = degree
			node.Size = 5 + math.Sqrt(float64(degree))*3 // min 5, scaled by sqrt
		}
	}

	// Compute stats
	nodesByType := map[string]int{}
	for _, n := range nodes { nodesByType[n.Type]++ }

	avgDegree := 0.0
	if len(nodes) > 0 { avgDegree = float64(len(edges)*2) / float64(len(nodes)) }

	return &GraphData{
		Nodes: nodes,
		Edges: edges,
		Stats: GraphStats{
			TotalNodes:  len(nodes),
			TotalEdges:  len(edges),
			NodesByType: nodesByType,
			AvgDegree:   avgDegree,
			MaxDegree:   maxDegree,
		},
	}, nil
}

// GetNodeNeighborhood returns a subgraph centered on a specific node.
func (s *Store) GetNodeNeighborhood(ctx context.Context, tenantID, nodeID string, depth int) (*GraphData, error) {
	if depth <= 0 { depth = 1 }
	if depth > 3 { depth = 3 }

	entities, rels, err := s.TraverseBFS(ctx, tenantID, nodeID, depth)
	if err != nil { return nil, err }

	nodeMap := map[string]*GraphNode{}
	nodes := []GraphNode{}
	for _, e := range entities {
		node := GraphNode{
			ID: e.ID, Label: e.Name, Type: e.EntityType,
			Color: nodeColor(e.EntityType), Size: 5, Description: e.Source,
		}
		nodes = append(nodes, node)
		nodeMap[e.ID] = &nodes[len(nodes)-1]
	}

	edges := []GraphEdge{}
	for _, r := range rels {
		edges = append(edges, GraphEdge{
			ID: r.ID, Source: r.SourceID, Target: r.TargetID, Label: r.RelType,
			Weight: 1.0, Color: edgeColor(1.0), Thickness: edgeThickness(1.0),
		})
		if n, ok := nodeMap[r.SourceID]; ok { n.LinkCount++ }
		if n, ok := nodeMap[r.TargetID]; ok { n.LinkCount++ }
	}

	// Scale sizes
	for i := range nodes {
		nodes[i].Size = 5 + math.Sqrt(float64(nodes[i].LinkCount))*3
	}

	nodesByType := map[string]int{}
	for _, n := range nodes { nodesByType[n.Type]++ }

	return &GraphData{
		Nodes: nodes, Edges: edges,
		Stats: GraphStats{TotalNodes: len(nodes), TotalEdges: len(edges), NodesByType: nodesByType},
	}, nil
}

// GetRelevanceEdges computes relevance-weighted edges between all entities.
func (s *Store) GetRelevanceEdges(ctx context.Context, tenantID string, topN int) ([]GraphEdge, error) {
	if topN <= 0 { topN = 100 }

	// Get all entity pairs with relationships
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT source_id, target_id FROM kg_relationships WHERE tenant_id = $1 LIMIT $2`,
		tenantID, topN*2)
	if err != nil { return nil, err }
	defer rows.Close()

	edges := []GraphEdge{}
	for rows.Next() {
		var src, tgt string
		rows.Scan(&src, &tgt)

		signal, err := s.ComputeRelevance(ctx, tenantID, src, tgt)
		if err != nil { continue }

		score := signal.Score()
		if score < 0.5 { continue } // skip weak connections

		edges = append(edges, GraphEdge{
			Source: src, Target: tgt, Weight: score,
			Color: edgeColor(score), Thickness: edgeThickness(score),
			Label: fmt.Sprintf("%.1f", score),
		})
	}
	return edges, nil
}

func nodeColor(entityType string) string {
	if c, ok := typeColors[entityType]; ok { return c }
	return "#95A5A6" // default gray
}

func edgeColor(weight float64) string {
	if weight > 5.0 { return "#27AE60" }  // strong = green
	if weight > 2.0 { return "#F39C12" }  // medium = orange
	return "#BDC3C7"                        // weak = gray
}

func edgeThickness(weight float64) float64 {
	if weight > 5.0 { return 3.0 }
	if weight > 2.0 { return 2.0 }
	return 1.0
}

// FindGodNodes returns the most connected entities (from knowledge graph engine).
// Uses kg_entities / kg_relationships (the canonical table names elsewhere in
// this package); older copies of this file referenced `entities` / `relationships`
// which do not exist in any migration and caused every /v1/graph/god-nodes
// call to 500 with "relation does not exist".
func (s *Store) FindGodNodes(ctx context.Context, tenantID string, topN int) ([]GodNode, error) {
	if topN <= 0 { topN = 10 }
	rows, err := s.pool.Query(ctx,
		`SELECT e.id::text, e.name, COUNT(DISTINCT r.id) AS degree
		 FROM kg_entities e
		 LEFT JOIN kg_relationships r ON r.source_id = e.id OR r.target_id = e.id
		 WHERE e.tenant_id = $1
		 GROUP BY e.id, e.name
		 ORDER BY degree DESC LIMIT $2`, tenantID, topN)
	if err != nil { return nil, err }
	defer rows.Close()
	gods := []GodNode{}
	for rows.Next() {
		var g GodNode
		rows.Scan(&g.ID, &g.Name, &g.Degree)
		gods = append(gods, g)
	}
	return gods, nil
}

// ClusterByType groups entities by type (from knowledge graph engine).
// See FindGodNodes for the kg_entities naming fix.
func (s *Store) ClusterByType(ctx context.Context, tenantID string) (map[string]int, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT entity_type, COUNT(*) FROM kg_entities WHERE tenant_id = $1 GROUP BY entity_type ORDER BY COUNT(*) DESC`, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()
	clusters := make(map[string]int)
	for rows.Next() {
		var t string; var c int
		rows.Scan(&t, &c)
		clusters[t] = c
	}
	return clusters, nil
}
