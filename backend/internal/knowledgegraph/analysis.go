// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package knowledgegraph

import (
	"math"
	"sort"
)

// analysis.go — Graph analysis: god nodes, surprising connections, suggested questions.

// GodNode represents a highly-connected entity — a core abstraction.
type GodNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Degree    int    `json:"degree"`
	Community int    `json:"community,omitempty"`
}

// SurprisingConnection is a cross-community edge that bridges different clusters.
type SurprisingConnection struct {
	SourceID    string  `json:"source_id"`
	TargetID    string  `json:"target_id"`
	SourceName  string  `json:"source_name"`
	TargetName  string  `json:"target_name"`
	Score       float64 `json:"surprise_score"`
	Reason      string  `json:"reason"`
}

// AnalyzeGodNodes returns the top-N most connected entities.
func AnalyzeGodNodes(entities []Entity, relationships []Relationship, topN int) []GodNode {
	if topN <= 0 { topN = 10 }

	degree := map[string]int{}
	for _, r := range relationships {
		degree[r.SourceID]++
		degree[r.TargetID]++
	}

	entityMap := map[string]Entity{}
	for _, e := range entities { entityMap[e.ID] = e }

	type kv struct{ id string; deg int }
	sorted := []kv{}
	for id, d := range degree { sorted = append(sorted, kv{id, d}) }
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].deg > sorted[j].deg })

	result := []GodNode{}
	for i, kv := range sorted {
		if i >= topN { break }
		e := entityMap[kv.id]
		result = append(result, GodNode{ID: kv.id, Name: e.Name, Type: e.EntityType, Degree: kv.deg})
	}
	return result
}

// AnalyzeSurprisingConnections finds edges that bridge different communities.
func AnalyzeSurprisingConnections(entities []Entity, relationships []Relationship, communities map[int][]string, topN int) []SurprisingConnection {
	if topN <= 0 { topN = 10 }

	nodeCommunity := map[string]int{}
	for cid, members := range communities {
		for _, nid := range members { nodeCommunity[nid] = cid }
	}
	communitySize := map[int]int{}
	for cid, members := range communities { communitySize[cid] = len(members) }

	entityMap := map[string]Entity{}
	for _, e := range entities { entityMap[e.ID] = e }

	surprises := []SurprisingConnection{}
	for _, r := range relationships {
		sc, tc := nodeCommunity[r.SourceID], nodeCommunity[r.TargetID]
		if sc == tc { continue }

		score := surpriseScore(communitySize[sc], communitySize[tc], len(relationships))
		surprises = append(surprises, SurprisingConnection{
			SourceID: r.SourceID, TargetID: r.TargetID,
			SourceName: entityMap[r.SourceID].Name, TargetName: entityMap[r.TargetID].Name,
			Score: score, Reason: "cross-community bridge",
		})
	}

	sort.Slice(surprises, func(i, j int) bool { return surprises[i].Score > surprises[j].Score })
	if len(surprises) > topN { surprises = surprises[:topN] }
	return surprises
}

// SuggestQuestions generates exploration questions based on graph structure.
func SuggestQuestions(gods []GodNode, surprises []SurprisingConnection) []string {
	questions := []string{}
	for _, g := range gods {
		if g.Degree > 10 {
			questions = append(questions, "Why is '"+g.Name+"' connected to so many entities?")
		}
	}
	for _, s := range surprises {
		questions = append(questions, "What's the relationship between '"+s.SourceName+"' and '"+s.TargetName+"'?")
	}
	return questions
}

func surpriseScore(sizeA, sizeB, totalEdges int) float64 {
	if totalEdges == 0 { return 0 }
	expected := float64(sizeA*sizeB) / float64(totalEdges)
	if expected == 0 { return 1.0 }
	return math.Min(1.0/expected, 10.0)
}

// --- Community Detection (from knowledge graph engine) ---

// SimpleLeiden performs community detection using connected-component BFS.
// This is a simplified Leiden: entities in the same connected component
// are in the same community. For sparse knowledge graphs this works well.
func SimpleLeiden(entities []Entity, relationships []Relationship) map[int][]string {
	// Build adjacency list
	adj := map[string][]string{}
	for _, r := range relationships {
		adj[r.SourceID] = append(adj[r.SourceID], r.TargetID)
		adj[r.TargetID] = append(adj[r.TargetID], r.SourceID)
	}

	visited := map[string]bool{}
	communities := map[int][]string{}
	communityID := 0

	for _, e := range entities {
		if visited[e.ID] { continue }

		// BFS from this entity
		queue := []string{e.ID}
		visited[e.ID] = true
		members := []string{}

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			members = append(members, current)

			for _, neighbor := range adj[current] {
				if !visited[neighbor] {
					visited[neighbor] = true
					queue = append(queue, neighbor)
				}
			}
		}

		communities[communityID] = members
		communityID++
	}

	return communities
}

// CohesionScore computes how tightly connected a community is.
// 1.0 = fully connected (every member linked to every other).
// 0.0 = no internal edges.
func CohesionScore(relationships []Relationship, communityMembers []string) float64 {
	n := len(communityMembers)
	if n <= 1 { return 1.0 }

	memberSet := map[string]bool{}
	for _, m := range communityMembers { memberSet[m] = true }

	internalEdges := 0
	for _, r := range relationships {
		if memberSet[r.SourceID] && memberSet[r.TargetID] {
			internalEdges++
		}
	}

	possible := float64(n) * float64(n-1) / 2.0
	if possible == 0 { return 0 }
	return math.Round(float64(internalEdges)/possible*100) / 100
}

// CohesionScoreAll computes cohesion for every community.
func CohesionScoreAll(relationships []Relationship, communities map[int][]string) map[int]float64 {
	result := map[int]float64{}
	for cid, members := range communities {
		result[cid] = CohesionScore(relationships, members)
	}
	return result
}

// FullAnalysis runs the complete graph analysis pipeline.
func FullAnalysis(entities []Entity, relationships []Relationship) GraphAnalysis {
	communities := SimpleLeiden(entities, relationships)
	gods := AnalyzeGodNodes(entities, relationships, 10)
	surprises := AnalyzeSurprisingConnections(entities, relationships, communities, 10)
	cohesion := CohesionScoreAll(relationships, communities)
	questions := SuggestQuestions(gods, surprises)

	// Assign community IDs to god nodes
	nodeCommunity := map[string]int{}
	for cid, members := range communities {
		for _, m := range members { nodeCommunity[m] = cid }
	}
	for i := range gods {
		gods[i].Community = nodeCommunity[gods[i].ID]
	}

	pagerank := PageRank(entities, relationships, 20, 0.85)
	betweenness := BetweennessCentrality(entities, relationships)
	clustering := ClusteringCoefficient(entities, relationships)

	return GraphAnalysis{
		Communities:    communities,
		GodNodes:       gods,
		Surprises:      surprises,
		Cohesion:       cohesion,
		Questions:      questions,
		PageRank:       pagerank,
		Betweenness:    betweenness,
		Clustering:     clustering,
		TotalEntities:  len(entities),
		TotalRelations: len(relationships),
		TotalClusters:  len(communities),
	}
}

// GraphAnalysis is the complete result of graph analysis.
type GraphAnalysis struct {
	Communities    map[int][]string          `json:"communities"`
	GodNodes       []GodNode                 `json:"god_nodes"`
	Surprises      []SurprisingConnection    `json:"surprising_connections"`
	Cohesion       map[int]float64           `json:"cohesion_scores"`
	Questions      []string                  `json:"suggested_questions"`
	PageRank       map[string]float64        `json:"pagerank"`
	Betweenness    map[string]float64        `json:"betweenness"`
	Clustering     map[string]float64        `json:"clustering_coefficient"`
	TotalEntities  int                       `json:"total_entities"`
	TotalRelations int                       `json:"total_relationships"`
	TotalClusters  int                       `json:"total_clusters"`
}

// --- Advanced Graph Algorithms (from knowledge graph engine / NetworkX) ---

// PageRank computes importance scores for entities.
// Entities linked to by many important entities get higher scores.
func PageRank(entities []Entity, relationships []Relationship, iterations int, damping float64) map[string]float64 {
	if iterations <= 0 { iterations = 20 }
	if damping <= 0 { damping = 0.85 }

	n := len(entities)
	if n == 0 { return nil }

	// Build adjacency
	outLinks := map[string][]string{}
	for _, r := range relationships {
		outLinks[r.SourceID] = append(outLinks[r.SourceID], r.TargetID)
		outLinks[r.TargetID] = append(outLinks[r.TargetID], r.SourceID)
	}

	// Initialize scores
	scores := make(map[string]float64, n)
	for _, e := range entities { scores[e.ID] = 1.0 / float64(n) }

	// Iterate
	for i := 0; i < iterations; i++ {
		newScores := make(map[string]float64, n)
		for _, e := range entities {
			newScores[e.ID] = (1 - damping) / float64(n)
		}
		for _, e := range entities {
			links := outLinks[e.ID]
			if len(links) == 0 { continue }
			share := scores[e.ID] / float64(len(links))
			for _, target := range links {
				newScores[target] += damping * share
			}
		}
		scores = newScores
	}
	return scores
}

// BetweennessCentrality measures how often an entity lies on shortest paths.
// High betweenness = bridge between communities.
func BetweennessCentrality(entities []Entity, relationships []Relationship) map[string]float64 {
	// Build adjacency
	adj := map[string][]string{}
	for _, r := range relationships {
		adj[r.SourceID] = append(adj[r.SourceID], r.TargetID)
		adj[r.TargetID] = append(adj[r.TargetID], r.SourceID)
	}

	bc := make(map[string]float64, len(entities))
	for _, e := range entities { bc[e.ID] = 0 }

	// BFS from each node
	for _, source := range entities {
		dist := map[string]int{source.ID: 0}
		paths := map[string]float64{source.ID: 1}
		queue := []string{source.ID}
		order := []string{}

		for len(queue) > 0 {
			v := queue[0]; queue = queue[1:]
			order = append(order, v)
			for _, w := range adj[v] {
				if _, seen := dist[w]; !seen {
					dist[w] = dist[v] + 1
					queue = append(queue, w)
				}
				if dist[w] == dist[v]+1 {
					paths[w] += paths[v]
				}
			}
		}

		delta := make(map[string]float64)
		for i := len(order) - 1; i >= 0; i-- {
			w := order[i]
			for _, v := range adj[w] {
				if dist[v] == dist[w]-1 {
					delta[v] += (paths[v] / paths[w]) * (1 + delta[w])
				}
			}
			if w != source.ID {
				bc[w] += delta[w]
			}
		}
	}

	// Normalize
	n := float64(len(entities))
	if n > 2 {
		for k := range bc { bc[k] /= (n - 1) * (n - 2) }
	}
	return bc
}

// ShortestPath finds the shortest path between two entities using BFS.
func ShortestPath(entities []Entity, relationships []Relationship, sourceID, targetID string) []string {
	adj := map[string][]string{}
	for _, r := range relationships {
		adj[r.SourceID] = append(adj[r.SourceID], r.TargetID)
		adj[r.TargetID] = append(adj[r.TargetID], r.SourceID)
	}

	visited := map[string]bool{sourceID: true}
	parent := map[string]string{}
	queue := []string{sourceID}

	for len(queue) > 0 {
		v := queue[0]; queue = queue[1:]
		if v == targetID {
			// Reconstruct path
			path := []string{targetID}
			for path[len(path)-1] != sourceID {
				path = append(path, parent[path[len(path)-1]])
			}
			// Reverse
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}
		for _, w := range adj[v] {
			if !visited[w] {
				visited[w] = true
				parent[w] = v
				queue = append(queue, w)
			}
		}
	}
	return nil // no path
}

// ClusteringCoefficient measures how tightly connected each entity's neighbors are.
func ClusteringCoefficient(entities []Entity, relationships []Relationship) map[string]float64 {
	adj := map[string]map[string]bool{}
	for _, r := range relationships {
		if adj[r.SourceID] == nil { adj[r.SourceID] = map[string]bool{} }
		if adj[r.TargetID] == nil { adj[r.TargetID] = map[string]bool{} }
		adj[r.SourceID][r.TargetID] = true
		adj[r.TargetID][r.SourceID] = true
	}

	cc := make(map[string]float64, len(entities))
	for _, e := range entities {
		neighbors := adj[e.ID]
		k := len(neighbors)
		if k < 2 { cc[e.ID] = 0; continue }

		// Count edges between neighbors
		triangles := 0
		nlist := make([]string, 0, k)
		for n := range neighbors { nlist = append(nlist, n) }
		for i := 0; i < len(nlist); i++ {
			for j := i + 1; j < len(nlist); j++ {
				if adj[nlist[i]][nlist[j]] { triangles++ }
			}
		}
		cc[e.ID] = 2.0 * float64(triangles) / float64(k*(k-1))
	}
	return cc
}
