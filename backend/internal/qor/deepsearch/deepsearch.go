// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package deepsearch

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// deepsearch.go — Graph-based deep search: decompose complex queries into sub-queries,
// search in parallel, synthesize results. Inspired by MindSearch's graph decomposition.

// SearchNode represents a node in the search graph.
type SearchNode struct {
	ID       string `json:"id"`
	Query    string `json:"query"`
	Topic    string `json:"topic,omitempty"`
	Result   string `json:"result,omitempty"`
	Status   string `json:"status"` // pending, searching, done, failed
	ParentID string `json:"parent_id,omitempty"`
}

// SearchGraph manages the decomposition of a complex query into sub-queries.
type SearchGraph struct {
	mu       sync.Mutex
	nodes    map[string]*SearchNode
	children map[string][]string // parentID → child IDs
	root     string
}

func NewSearchGraph() *SearchGraph {
	return &SearchGraph{
		nodes:    make(map[string]*SearchNode),
		children: make(map[string][]string),
	}
}

func (g *SearchGraph) AddRoot(query string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	id := "root"
	g.nodes[id] = &SearchNode{ID: id, Query: query, Status: "pending"}
	g.root = id
	return id
}

func (g *SearchGraph) AddSubQuery(parentID, query, topic string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	id := fmt.Sprintf("q%d", len(g.nodes))
	g.nodes[id] = &SearchNode{ID: id, Query: query, Topic: topic, Status: "pending", ParentID: parentID}
	g.children[parentID] = append(g.children[parentID], id)
	return id
}

func (g *SearchGraph) SetResult(nodeID, result string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if n, ok := g.nodes[nodeID]; ok {
		n.Result = result
		n.Status = "done"
	}
}

func (g *SearchGraph) SetFailed(nodeID, err string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if n, ok := g.nodes[nodeID]; ok {
		n.Result = err
		n.Status = "failed"
	}
}

func (g *SearchGraph) AllResults() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var sb strings.Builder
	for _, n := range g.nodes {
		if n.Result != "" && n.Status == "done" {
			sb.WriteString(fmt.Sprintf("## %s\n%s\n\n", n.Topic, n.Result))
		}
	}
	return sb.String()
}

// SearchFunc is the function that performs actual web search for a query.
type SearchFunc func(ctx context.Context, query string) (string, error)

// DecomposeFunc uses an LLM to break a complex query into sub-queries.
type DecomposeFunc func(ctx context.Context, query string) ([]SubQuery, error)

// SynthesizeFunc uses an LLM to combine sub-results into a final answer.
type SynthesizeFunc func(ctx context.Context, query string, subResults string) (string, error)

type SubQuery struct {
	Query string `json:"query"`
	Topic string `json:"topic"`
}

// Engine orchestrates the deep search process.
type Engine struct {
	search     SearchFunc
	decompose  DecomposeFunc
	synthesize SynthesizeFunc
	maxDepth   int
	maxParallel int
}

func NewEngine(search SearchFunc, decompose DecomposeFunc, synthesize SynthesizeFunc) *Engine {
	return &Engine{
		search:      search,
		decompose:   decompose,
		synthesize:  synthesize,
		maxDepth:    2,
		maxParallel: 5,
	}
}

// Search performs a deep search: decompose → parallel search → synthesize.
func (e *Engine) Search(ctx context.Context, query string) (string, error) {
	graph := NewSearchGraph()
	rootID := graph.AddRoot(query)

	// Step 1: Decompose the query
	subQueries, err := e.decompose(ctx, query)
	if err != nil {
		// Fallback: search the original query directly
		slog.Warn("deepsearch: decompose failed, searching directly", "err", err)
		result, err := e.search(ctx, query)
		if err != nil { return "", err }
		return result, nil
	}

	if len(subQueries) == 0 {
		// Simple query — search directly
		result, err := e.search(ctx, query)
		if err != nil { return "", err }
		return result, nil
	}

	// Step 2: Search sub-queries in parallel
	var wg sync.WaitGroup
	sem := make(chan struct{}, e.maxParallel)

	for _, sq := range subQueries {
		nodeID := graph.AddSubQuery(rootID, sq.Query, sq.Topic)
		wg.Add(1)
		go func(nid string, q string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := e.search(ctx, q)
			if err != nil {
				graph.SetFailed(nid, err.Error())
				slog.Warn("deepsearch: sub-query failed", "query", q, "err", err)
				return
			}
			graph.SetResult(nid, result)
		}(nodeID, sq.Query)
	}
	wg.Wait()

	// Step 3: Synthesize results
	allResults := graph.AllResults()
	if allResults == "" {
		return "", fmt.Errorf("all sub-queries failed")
	}

	answer, err := e.synthesize(ctx, query, allResults)
	if err != nil {
		// Fallback: return raw results
		return allResults, nil
	}

	slog.Info("deepsearch: complete", "query", query, "sub_queries", len(subQueries))
	return answer, nil
}
