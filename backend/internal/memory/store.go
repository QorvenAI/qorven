// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	TypeFact        = "fact"
	TypePreference  = "preference"
	TypeDecision    = "decision"
	TypeIdentity    = "identity"
	TypeEvent       = "event"
	TypeObservation = "observation"
	TypeGoal        = "goal"
	TypeTodo        = "todo"
)

// 5 edge types for memory graph.
const (
	EdgeRelatedTo   = "related_to"
	EdgeUpdates     = "updates"
	EdgeContradicts = "contradicts"
	EdgeCausedBy    = "caused_by"
	EdgePartOf      = "part_of"
)

// Memory is a typed memory object with importance scoring.
type Memory struct {
	ID          string    `json:"id"`
	AgentID     string    `json:"agent_id"`
	UserID      string    `json:"user_id,omitempty"`
	Type        string    `json:"memory_type"`
	Content     string    `json:"content"`
	Summary     string    `json:"summary,omitempty"`
	Source      string    `json:"source,omitempty"`
	SourceType  string    `json:"source_type"`
	Importance  float64   `json:"importance"`
	AccessCount int       `json:"access_count"`
	DecayExempt bool      `json:"decay_exempt"`
	Embedding   []float32 `json:"-"`
	Tags        []string  `json:"tags,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Edge connects two memories.
type Edge struct {
	ID       string  `json:"id"`
	SourceID string  `json:"source_id"`
	TargetID string  `json:"target_id"`
	Type     string  `json:"edge_type"`
	Weight   float64 `json:"weight"`
}

// SearchResult is a memory with a relevance score.
type SearchResult struct {
	Memory Memory  `json:"memory"`
	Score  float64 `json:"score"`
}

// Store handles typed memory CRUD with graph edges and hybrid search.
type Store struct {
	pool *pgxpool.Pool
	embedClient *EmbeddingClient
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }
// Save creates or updates a memory.
func (s *Store) Save(ctx context.Context, tenantID string, m Memory) (string, error) {
	// Generate embedding if client is available
	if s.embedClient != nil && m.Content != "" {
		if emb, err := s.embedClient.Embed(ctx, m.Content); err == nil {
			m.Embedding = emb
		}
	}
	tags := m.Tags
	if tags == nil { tags = []string{} }
	decayExempt := m.DecayExempt || m.Type == TypeIdentity // identity always exempt

	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO memories (tenant_id, agent_id, user_id, memory_type, content, summary, source, source_type, importance, decay_exempt, tags, embedding)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
		tenantID, m.AgentID, nilIfEmpty(m.UserID), m.Type, m.Content, nilIfEmpty(m.Summary),
		nilIfEmpty(m.Source), m.SourceType, m.Importance, decayExempt, tags, m.Embedding,
	).Scan(&id)
	return id, err
}

// Search performs hybrid BM25 full-text search with importance weighting.
// When pgvector is available, this will also include vector similarity via RRF.
func (s *Store) Search(ctx context.Context, tenantID, agentID, query string, maxResults int) ([]SearchResult, error) {
	if maxResults <= 0 { maxResults = 10 }
	if maxResults > 50 { maxResults = 50 }

	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, user_id, memory_type, content, summary, source, source_type,
		        importance, access_count, decay_exempt, created_at,
		        ts_rank_cd(tsv, plainto_tsquery('simple', $1), 32) AS text_score
		 FROM memories
		 WHERE agent_id = $2 AND tsv @@ plainto_tsquery('simple', $1)
		 ORDER BY ts_rank_cd(tsv, plainto_tsquery('simple', $1), 32) * importance DESC
		 LIMIT $3`,
		query, agentID, maxResults)
	if err != nil { return nil, err }
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		var m Memory
		var textScore float64
		var userID, summary, source *string
		rows.Scan(&m.ID, &m.AgentID, &userID, &m.Type, &m.Content, &summary, &source,
			&m.SourceType, &m.Importance, &m.AccessCount, &m.DecayExempt, &m.CreatedAt, &textScore)
		if userID != nil { m.UserID = *userID }
		if summary != nil { m.Summary = *summary }
		if source != nil { m.Source = *source }

		// Combined score: text relevance * importance * recency
		recency := recencyScore(m.CreatedAt)
		score := textScore * m.Importance * recency
		results = append(results, SearchResult{Memory: m, Score: score})
	}

	// Update access counts for returned memories
	for _, r := range results {
		s.pool.Exec(ctx,
			`UPDATE memories SET access_count = access_count + 1, last_accessed = NOW() WHERE id = $1`, r.Memory.ID)
	}

	return results, nil
}

// SearchByType returns memories of a specific type for an agent.
func (s *Store) SearchByType(ctx context.Context, agentID, memType string, limit int) ([]Memory, error) {
	if limit <= 0 { limit = 20 }
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, user_id, memory_type, content, summary, importance, access_count, decay_exempt, tags, created_at
		 FROM memories WHERE agent_id = $1 AND memory_type = $2
		 ORDER BY importance DESC, created_at DESC LIMIT $3`, agentID, memType, limit)
	if err != nil { return nil, err }
	defer rows.Close()

	memories := []Memory{}
	for rows.Next() {
		var m Memory
		var userID, summary *string
		rows.Scan(&m.ID, &m.AgentID, &userID, &m.Type, &m.Content, &summary,
			&m.Importance, &m.AccessCount, &m.DecayExempt, &m.Tags, &m.CreatedAt)
		if userID != nil { m.UserID = *userID }
		if summary != nil { m.Summary = *summary }
		memories = append(memories, m)
	}
	return memories, nil
}

// ListRecent returns the most recent memories for an agent, ordered by creation time.
func (s *Store) ListRecent(ctx context.Context, agentID string, limit int) ([]SearchResult, error) {
	if limit <= 0 { limit = 20 }
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, user_id, memory_type, content, summary, source, source_type,
		        importance, access_count, decay_exempt, created_at
		 FROM memories WHERE agent_id = $1
		 ORDER BY importance DESC, created_at DESC LIMIT $2`, agentID, limit)
	if err != nil { return nil, err }
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		var m Memory
		var userID, summary, source *string
		rows.Scan(&m.ID, &m.AgentID, &userID, &m.Type, &m.Content, &summary, &source,
			&m.SourceType, &m.Importance, &m.AccessCount, &m.DecayExempt, &m.CreatedAt)
		if userID != nil { m.UserID = *userID }
		if summary != nil { m.Summary = *summary }
		if source != nil { m.Source = *source }
		results = append(results, SearchResult{Memory: m, Score: m.Importance})
	}
	return results, nil
}

// AddEdge creates a graph connection between two memories.
func (s *Store) AddEdge(ctx context.Context, tenantID, sourceID, targetID, edgeType string, weight float64) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO memory_edges (tenant_id, source_id, target_id, edge_type, weight)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (source_id, target_id, edge_type) DO UPDATE SET weight = EXCLUDED.weight`,
		tenantID, sourceID, targetID, edgeType, weight)
	return err
}

// GetRelated returns memories connected to a given memory via graph edges.
func (s *Store) GetRelated(ctx context.Context, memoryID string, maxDepth int) ([]Memory, []Edge, error) {
	if maxDepth <= 0 { maxDepth = 2 }
	if maxDepth > 3 { maxDepth = 3 }

	// Get direct edges
	edgeRows, err := s.pool.Query(ctx,
		`SELECT e.id, e.source_id, e.target_id, e.edge_type, e.weight
		 FROM memory_edges e
		 WHERE e.source_id = $1 OR e.target_id = $1
		 LIMIT 20`, memoryID)
	if err != nil { return nil, nil, err }
	defer edgeRows.Close()

	edges := []Edge{}
	relatedIDs := make(map[string]bool)
	for edgeRows.Next() {
		var e Edge
		edgeRows.Scan(&e.ID, &e.SourceID, &e.TargetID, &e.Type, &e.Weight)
		edges = append(edges, e)
		if e.SourceID != memoryID { relatedIDs[e.SourceID] = true }
		if e.TargetID != memoryID { relatedIDs[e.TargetID] = true }
	}

	if len(relatedIDs) == 0 { return nil, edges, nil }

	// Fetch related memories
	ids := make([]string, 0, len(relatedIDs))
	for id := range relatedIDs { ids = append(ids, id) }

	memRows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, memory_type, content, summary, importance, created_at
		 FROM memories WHERE id = ANY($1)`, ids)
	if err != nil { return nil, edges, err }
	defer memRows.Close()

	memories := []Memory{}
	for memRows.Next() {
		var m Memory
		var summary *string
		memRows.Scan(&m.ID, &m.AgentID, &m.Type, &m.Content, &summary, &m.Importance, &m.CreatedAt)
		if summary != nil { m.Summary = *summary }
		memories = append(memories, m)
	}
	return memories, edges, nil
}

// GenerateBulletin creates a memory bulletin (summary of key memories for context injection).
func (s *Store) GenerateBulletin(ctx context.Context, tenantID, agentID string) (string, error) {
	// Get top memories by importance across all types
	rows, err := s.pool.Query(ctx,
		`SELECT memory_type, content, importance FROM memories
		 WHERE agent_id = $1 ORDER BY importance DESC, access_count DESC LIMIT 20`, agentID)
	if err != nil { return "", err }
	defer rows.Close()

	byType := make(map[string][]string)
	count := 0
	for rows.Next() {
		var mType, content string
		var importance float64
		rows.Scan(&mType, &content, &importance)
		// Truncate long memories for bulletin
		if len(content) > 200 { content = content[:200] + "..." }
		byType[mType] = append(byType[mType], content)
		count++
	}

	if count == 0 { return "", nil }

	// Build structured bulletin
	var b strings.Builder
	b.WriteString("## Memory Bulletin\n\n")

	typeOrder := []string{TypeIdentity, TypeGoal, TypeDecision, TypeFact, TypePreference, TypeObservation, TypeEvent, TypeTodo}
	typeLabels := map[string]string{
		TypeIdentity: "🪪 Identity", TypeGoal: "🎯 Goals", TypeDecision: "⚖️ Decisions",
		TypeFact: "📚 Facts", TypePreference: "💡 Preferences", TypeObservation: "👁 Observations",
		TypeEvent: "📅 Events", TypeTodo: "✅ Todos",
	}

	for _, t := range typeOrder {
		items, ok := byType[t]
		if !ok { continue }
		fmt.Fprintf(&b, "### %s\n", typeLabels[t])
		for _, item := range items {
			fmt.Fprintf(&b, "- %s\n", item)
		}
		b.WriteString("\n")
	}

	bulletin := b.String()

	// Save bulletin to DB
	s.pool.Exec(ctx,
		`INSERT INTO memory_bulletins (tenant_id, agent_id, content, memory_count) VALUES ($1, $2, $3, $4)`,
		tenantID, agentID, bulletin, count)

	return bulletin, nil
}

// GetLatestBulletin returns the most recent bulletin for an agent.
func (s *Store) GetLatestBulletin(ctx context.Context, agentID string) (string, error) {
	var content string
	err := s.pool.QueryRow(ctx,
		`SELECT content FROM memory_bulletins WHERE agent_id = $1 ORDER BY generated_at DESC LIMIT 1`, agentID,
	).Scan(&content)
	if err != nil { return "", nil } // no bulletin yet is fine
	return content, nil
}

// Decay reduces importance of old, infrequently accessed memories.
func (s *Store) Decay(ctx context.Context, agentID string) (int, error) {
	result, err := s.pool.Exec(ctx,
		`UPDATE memories SET importance = importance * 0.95, updated_at = NOW()
		 WHERE agent_id = $1 AND decay_exempt = false
		 AND last_accessed < NOW() - INTERVAL '7 days'
		 AND importance > 0.1`, agentID)
	if err != nil { return 0, err }
	return int(result.RowsAffected()), nil
}

// Stats returns memory statistics for an agent.
func (s *Store) Stats(ctx context.Context, agentID string) (map[string]any, error) {
	var total int
	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM memories WHERE agent_id = $1`, agentID).Scan(&total)

	rows, err := s.pool.Query(ctx,
		`SELECT memory_type, COUNT(*) FROM memories WHERE agent_id = $1 GROUP BY memory_type`, agentID)
	if err != nil { return nil, err }
	defer rows.Close()

	byType := make(map[string]int)
	for rows.Next() {
		var t string
		var c int
		rows.Scan(&t, &c)
		byType[t] = c
	}

	var edgeCount int
	s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM memory_edges e JOIN memories m ON e.source_id = m.id WHERE m.agent_id = $1`, agentID,
	).Scan(&edgeCount)

	return map[string]any{
		"total": total, "by_type": byType, "edges": edgeCount,
	}, nil
}

// --- Helpers ---

func recencyScore(created time.Time) float64 {
	hours := time.Since(created).Hours()
	return math.Max(0.1, 1.0/(1.0+hours/168.0)) // half-life of ~1 week
}

func nilIfEmpty(s string) *string {
	if s == "" { return nil }
	return &s
}

// FormatForContext formats memories for injection into LLM context.
func FormatForContext(memories []SearchResult) string {
	if len(memories) == 0 { return "" }
	var b strings.Builder
	b.WriteString("<memories>\n")
	for _, r := range memories {
		m := r.Memory
		fmt.Fprintf(&b, "  <memory type=\"%s\" importance=\"%.2f\">\n    %s\n  </memory>\n", m.Type, m.Importance, m.Content)
	}
	b.WriteString("</memories>")
	return b.String()
}

// ExtractMemories uses simple heuristics to extract typed memories from conversation text.
// In production, this would use an LLM call for better extraction.
func ExtractMemories(agentID, text, source string) []Memory {
	memories := []Memory{}
	text = strings.ToLower(text)

	// Simple keyword-based extraction (LLM extraction comes in agent loop)
	if strings.Contains(text, "i prefer") || strings.Contains(text, "i like") || strings.Contains(text, "i don't like") {
		memories = append(memories, Memory{AgentID: agentID, Type: TypePreference, Content: text, Source: source, SourceType: "conversation", Importance: 0.6})
	}
	if strings.Contains(text, "my name is") || strings.Contains(text, "i am") {
		memories = append(memories, Memory{AgentID: agentID, Type: TypeIdentity, Content: text, Source: source, SourceType: "conversation", Importance: 0.9, DecayExempt: true})
	}
	if strings.Contains(text, "we decided") || strings.Contains(text, "let's go with") {
		memories = append(memories, Memory{AgentID: agentID, Type: TypeDecision, Content: text, Source: source, SourceType: "conversation", Importance: 0.7})
	}
	if strings.Contains(text, "todo") || strings.Contains(text, "need to") || strings.Contains(text, "don't forget") {
		memories = append(memories, Memory{AgentID: agentID, Type: TypeTodo, Content: text, Source: source, SourceType: "conversation", Importance: 0.8})
	}

	return memories
}

// ImportFromJSON imports memories from a JSON array.
func (s *Store) ImportFromJSON(ctx context.Context, tenantID, agentID string, data []byte) (int, error) {
	memories := []Memory{}
	if err := json.Unmarshal(data, &memories); err != nil {
		return 0, fmt.Errorf("invalid JSON: %w", err)
	}
	count := 0
	for _, m := range memories {
		m.AgentID = agentID
		if m.SourceType == "" { m.SourceType = "import" }
		if m.Importance == 0 { m.Importance = 0.5 }
		if _, err := s.Save(ctx, tenantID, m); err == nil {
			count++
		}
	}
	return count, nil
}

// --- P3.3: Cross-Soul Shared Memory with Privacy ---

// SearchTeam searches memories across ALL agents in the team (shared memory space).
// Respects privacy: only returns memories where source_type != 'private'.
func (s *Store) SearchTeam(ctx context.Context, tenantID string, agentIDs []string, query string, maxResults int) ([]SearchResult, error) {
	if len(agentIDs) == 0 || query == "" {
		return nil, nil
	}
	if maxResults <= 0 { maxResults = 10 }

	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, user_id, memory_type, content, summary, source, source_type,
		        importance, access_count, decay_exempt, created_at,
		        ts_rank_cd(tsv, plainto_tsquery('simple', $1), 32) AS text_score
		 FROM memories
		 WHERE agent_id = ANY($2) AND source_type != 'private'
		 AND tsv @@ plainto_tsquery('simple', $1)
		 ORDER BY ts_rank_cd(tsv, plainto_tsquery('simple', $1), 32) * importance DESC
		 LIMIT $3`,
		query, agentIDs, maxResults)
	if err != nil { return nil, err }
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		var m Memory
		var textScore float64
		var userID, summary, source *string
		rows.Scan(&m.ID, &m.AgentID, &userID, &m.Type, &m.Content, &summary, &source,
			&m.SourceType, &m.Importance, &m.AccessCount, &m.DecayExempt, &m.CreatedAt, &textScore)
		if userID != nil { m.UserID = *userID }
		if summary != nil { m.Summary = *summary }
		if source != nil { m.Source = *source }
		recency := recencyScore(m.CreatedAt)
		results = append(results, SearchResult{Memory: m, Score: textScore * m.Importance * recency})
	}
	return results, nil
}

// SavePrivate saves a memory marked as private (only visible to the owning agent).
func (s *Store) SavePrivate(ctx context.Context, tenantID string, m Memory) (string, error) {
	m.SourceType = "private"
	return s.Save(ctx, tenantID, m)
}

// SaveTeam saves a memory visible to all team members (shared memory space).
func (s *Store) SaveTeam(ctx context.Context, tenantID string, m Memory) (string, error) {
	m.SourceType = "team"
	return s.Save(ctx, tenantID, m)
}

// SearchByTypeQuery searches memories by type prefix and query text.
func (s *Store) SearchByTypeQuery(ctx context.Context, tenantID, typePrefix, query string, maxResults int) ([]SearchResult, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, agent_id, type, content, source, importance, created_at,
		       ts_rank(to_tsvector('english', content), plainto_tsquery('simple', $1)) AS rank
		FROM memories
		WHERE tenant_id = $2 AND type LIKE $3 || '%'
		  AND to_tsvector('english', content) @@ plainto_tsquery('simple', $1)
		ORDER BY rank DESC
		LIMIT $4`, query, tenantID, typePrefix, maxResults)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		var r SearchResult
		var rank float64
		if err := rows.Scan(&r.Memory.ID, &r.Memory.AgentID, &r.Memory.Type, &r.Memory.Content, &r.Memory.Source,
			&r.Memory.Importance, &r.Memory.CreatedAt, &rank); err != nil {
			continue
		}
		r.Score = rank
		results = append(results, r)
	}
	return results, nil
}

// MarkDecayable removes decay exemption for all memories of a given type prefix.
func (s *Store) MarkDecayable(ctx context.Context, tenantID, typePrefix string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE memories SET decay_exempt = false
		WHERE tenant_id = $1 AND type LIKE $2 || '%'`, tenantID, typePrefix)
	return err
}

// HybridSearch combines full-text search with vector similarity (RRF fusion).
// If embedding is provided, uses cosine similarity. Otherwise falls back to text-only.
func (s *Store) HybridSearch(ctx context.Context, tenantID, agentID, query string, embedding []float32, maxResults int) ([]SearchResult, error) {
	if len(embedding) == 0 {
		// Text-only fallback
		return s.Search(ctx, tenantID, agentID, query, maxResults)
	}

	// Reciprocal Rank Fusion: combine text rank + vector similarity
	rows, err := s.pool.Query(ctx, `
		WITH text_results AS (
			SELECT id, agent_id, memory_type, content, source_type, importance, created_at,
			       ts_rank(tsv, plainto_tsquery('simple', $1)) AS text_score,
			       ROW_NUMBER() OVER (ORDER BY ts_rank(tsv, plainto_tsquery('simple', $1)) DESC) AS text_rank
			FROM memories
			WHERE tenant_id = $2 AND ($3 = '' OR agent_id::text = $3)
			  AND tsv @@ plainto_tsquery('simple', $1)
			LIMIT 50
		),
		vector_results AS (
			SELECT id, agent_id, memory_type, content, source_type, importance, created_at,
			       1 - (embedding <=> $4::vector) AS vector_score,
			       ROW_NUMBER() OVER (ORDER BY embedding <=> $4::vector) AS vector_rank
			FROM memories
			WHERE tenant_id = $2 AND ($3 = '' OR agent_id::text = $3)
			  AND embedding IS NOT NULL
			LIMIT 50
		)
		SELECT COALESCE(t.id, v.id) AS id,
		       COALESCE(t.agent_id, v.agent_id) AS agent_id,
		       COALESCE(t.memory_type, v.memory_type) AS memory_type,
		       COALESCE(t.content, v.content) AS content,
		       COALESCE(t.source_type, v.source_type) AS source,
		       COALESCE(t.importance, v.importance) AS importance,
		       COALESCE(t.created_at, v.created_at) AS created_at,
		       (COALESCE(1.0/(60+t.text_rank), 0) + COALESCE(1.0/(60+v.vector_rank), 0)) AS rrf_score
		FROM text_results t
		FULL OUTER JOIN vector_results v ON t.id = v.id
		ORDER BY rrf_score DESC
		LIMIT $5`,
		query, tenantID, agentID, embedding, maxResults)
	if err != nil {
		// Fallback to text-only if vector query fails
		return s.Search(ctx, tenantID, agentID, query, maxResults)
	}
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		var m Memory
		var score float64
		if err := rows.Scan(&m.ID, &m.AgentID, &m.Type, &m.Content, &m.Source,
			&m.Importance, &m.CreatedAt, &score); err != nil {
			continue
		}
		results = append(results, SearchResult{Memory: m, Score: score})
	}
	return results, nil
}

// SetEmbeddingClient enables automatic embedding generation on Save.
func (s *Store) SetEmbeddingClient(c *EmbeddingClient) { s.embedClient = c }
