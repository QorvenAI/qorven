// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// --- memory_search (pgvector + BM25 hybrid) ---

type MemorySearchTool struct{ pool *pgxpool.Pool }

func NewMemorySearchTool(pool *pgxpool.Pool) *MemorySearchTool { return &MemorySearchTool{pool: pool} }
func (t *MemorySearchTool) Name() string                       { return "memory_search" }
func (t *MemorySearchTool) Description() string {
	return "Search through the agent's long-term memory using keyword matching."
}
func (t *MemorySearchTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"query":       map[string]any{"type": "string", "description": "Search query"},
		"max_results": map[string]any{"type": "integer", "description": "Max results (default 5)"},
	}, "required": []string{"query"}}
}

func (t *MemorySearchTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.pool == nil { return ErrorResult("memory not configured") }
	query, _ := args["query"].(string)
	if query == "" { return ErrorResult("query is required") }
	agentID := AgentIDFromCtx(ctx)
	max := 5
	if n, ok := toInt(args["max_results"]); ok && n > 0 { max = n }
	if max > 20 { max = 20 }

	// BM25 full-text search via tsvector
	rows, err := t.pool.Query(ctx,
		`SELECT COALESCE(source,'memory'), content, ts_rank(tsv, plainto_tsquery('simple', $1)) AS score
		 FROM memory_chunks WHERE agent_id = $2 AND tsv @@ plainto_tsquery('simple', $1)
		 ORDER BY score DESC LIMIT $3`,
		query, agentID, max)
	if err != nil { return ErrorResult(fmt.Sprintf("search failed: %v", err)) }
	defer rows.Close()

	var b strings.Builder
	i := 0
	for rows.Next() {
		var source, content string
		var score float64
		rows.Scan(&source, &content, &score)
		i++
		fmt.Fprintf(&b, "--- %s (score: %.3f) ---\n%s\n\n", source, score, content)
	}
	if i == 0 { return TextResult("no memory matches for: " + query) }
	return TextResult(TruncateOutput(b.String(), MaxToolOutput))
}

// --- memory_get ---

type MemoryGetTool struct{ pool *pgxpool.Pool }

func NewMemoryGetTool(pool *pgxpool.Pool) *MemoryGetTool { return &MemoryGetTool{pool: pool} }
func (t *MemoryGetTool) Name() string                    { return "memory_get" }
func (t *MemoryGetTool) Description() string             { return "Retrieve a specific memory document by path." }
func (t *MemoryGetTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"path": map[string]any{"type": "string", "description": "Document path"},
	}, "required": []string{"path"}}
}

func (t *MemoryGetTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.pool == nil { return ErrorResult("memory not configured") }
	path, _ := args["path"].(string)
	if path == "" { return ErrorResult("path is required") }
	agentID := AgentIDFromCtx(ctx)

	var content string
	err := t.pool.QueryRow(ctx,
		`SELECT content FROM memory_documents WHERE agent_id = $1 AND path = $2`, agentID, path,
	).Scan(&content)
	if err != nil { return ErrorResult("document not found: " + path) }
	return TextResult(TruncateOutput(content, MaxToolOutput))
}

// --- knowledge_graph_search ---

type KnowledgeGraphSearchTool struct{ pool *pgxpool.Pool }

func NewKGSearchTool(pool *pgxpool.Pool) *KnowledgeGraphSearchTool {
	return &KnowledgeGraphSearchTool{pool: pool}
}
func (t *KnowledgeGraphSearchTool) Name() string { return "knowledge_graph_search" }
func (t *KnowledgeGraphSearchTool) Description() string {
	return "Search entities and relationships in the knowledge graph."
}
func (t *KnowledgeGraphSearchTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"query":       map[string]any{"type": "string", "description": "Search term (entity name or type)"},
		"entity_type": map[string]any{"type": "string", "description": "Filter by type (person, project, concept, etc.)"},
	}, "required": []string{"query"}}
}

func (t *KnowledgeGraphSearchTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.pool == nil { return ErrorResult("knowledge graph not configured") }
	query, _ := args["query"].(string)
	if query == "" { return ErrorResult("query is required") }
	agentID := AgentIDFromCtx(ctx)
	entityType, _ := args["entity_type"].(string)

	var b strings.Builder

	// Search entities
	var sqlQuery string
	var sqlArgs []any
	if entityType != "" {
		sqlQuery = `SELECT name, entity_type, properties FROM kg_entities
			 WHERE agent_id = $1 AND entity_type = $2 AND name ILIKE '%' || $3 || '%' LIMIT 10`
		sqlArgs = []any{agentID, entityType, query}
	} else {
		sqlQuery = `SELECT name, entity_type, properties FROM kg_entities
			 WHERE agent_id = $1 AND (name ILIKE '%' || $2 || '%' OR entity_type ILIKE '%' || $2 || '%') LIMIT 10`
		sqlArgs = []any{agentID, query}
	}

	rows, err := t.pool.Query(ctx, sqlQuery, sqlArgs...)
	if err != nil { return ErrorResult(fmt.Sprintf("search failed: %v", err)) }
	defer rows.Close()

	i := 0
	for rows.Next() {
		var name, eType string
		var props []byte
		rows.Scan(&name, &eType, &props)
		i++
		fmt.Fprintf(&b, "Entity: %s [%s]\n  Properties: %s\n\n", name, eType, string(props))
	}

	// Search relationships
	relRows, err := t.pool.Query(ctx,
		`SELECT e1.name, r.rel_type, e2.name FROM kg_relationships r
		 JOIN kg_entities e1 ON r.source_id = e1.id
		 JOIN kg_entities e2 ON r.target_id = e2.id
		 WHERE e1.agent_id = $1 AND (e1.name ILIKE '%' || $2 || '%' OR e2.name ILIKE '%' || $2 || '%')
		 LIMIT 10`, agentID, query)
	if err == nil {
		defer relRows.Close()
		for relRows.Next() {
			var src, rel, tgt string
			relRows.Scan(&src, &rel, &tgt)
			fmt.Fprintf(&b, "Relationship: %s -[%s]-> %s\n", src, rel, tgt)
		}
	}

	if i == 0 && b.Len() == 0 { return TextResult("no knowledge graph matches for: " + query) }
	return TextResult(b.String())
}
