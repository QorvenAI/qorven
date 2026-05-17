// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GraphStore extends Store with embedding-powered hybrid search and knowledge graph operations.
type GraphStore struct {
	pool     *pgxpool.Pool
	embedder *EmbeddingClient
}

func NewGraphStore(pool *pgxpool.Pool, embedder *EmbeddingClient) *GraphStore {
	return &GraphStore{pool: pool, embedder: embedder}
}

// SaveWithEmbedding saves a memory and generates its vector embedding.
func (g *GraphStore) SaveWithEmbedding(ctx context.Context, tenantID string, m Memory) (string, error) {
	// Generate embedding
	vec, err := g.embedder.Embed(ctx, m.Content)
	if err != nil {
		slog.Warn("graph.embedding_failed", "error", err)
		// Fall back to save without embedding
		s := &Store{pool: g.pool}
		return s.Save(ctx, tenantID, m)
	}

	// Save with embedding
	var id string
	err = g.pool.QueryRow(ctx,
		`INSERT INTO memories (agent_id, user_id, memory_type, content, summary, source, source_type,
		                       importance, decay_exempt, tags, embedding)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 RETURNING id`,
		m.AgentID, nilIfEmpty(m.UserID), m.Type, m.Content, nilIfEmpty(m.Summary),
		nilIfEmpty(m.Source), m.SourceType, m.Importance, m.DecayExempt, m.Tags, vec,
	).Scan(&id)
	return id, err
}

// HybridSearch combines vector similarity with full-text search for best results.
// alpha controls the blend: 0.0 = pure text, 1.0 = pure vector, 0.6 = recommended default.
func (g *GraphStore) HybridSearch(ctx context.Context, agentID, query string, maxResults int, alpha float64) ([]SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 10
	}
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}

	// Generate query embedding
	queryVec, err := g.embedder.Embed(ctx, query)
	if err != nil {
		slog.Warn("graph.query_embedding_failed", "error", err)
		// Fall back to text-only search
		s := &Store{pool: g.pool}
		return s.Search(ctx, "", agentID, query, maxResults)
	}

	rows, err := g.pool.Query(ctx,
		`WITH text_results AS (
			SELECT id, agent_id, user_id, memory_type, content, summary, source, source_type,
			       importance, access_count, decay_exempt, tags, created_at,
			       ts_rank_cd(tsv, plainto_tsquery('simple', $1), 32) AS text_score
			FROM memories
			WHERE agent_id = $2 AND tsv @@ plainto_tsquery('simple', $1)
		),
		vec_results AS (
			SELECT id, agent_id, user_id, memory_type, content, summary, source, source_type,
			       importance, access_count, decay_exempt, tags, created_at,
			       1 - (embedding <=> $3::vector) AS vec_score
			FROM memories
			WHERE agent_id = $2 AND embedding IS NOT NULL
			ORDER BY embedding <=> $3::vector
			LIMIT $4 * 2
		),
		combined AS (
			SELECT COALESCE(t.id, v.id) AS id,
			       COALESCE(t.agent_id, v.agent_id) AS agent_id,
			       COALESCE(t.user_id, v.user_id) AS user_id,
			       COALESCE(t.memory_type, v.memory_type) AS memory_type,
			       COALESCE(t.content, v.content) AS content,
			       COALESCE(t.summary, v.summary) AS summary,
			       COALESCE(t.source, v.source) AS source,
			       COALESCE(t.source_type, v.source_type) AS source_type,
			       COALESCE(t.importance, v.importance) AS importance,
			       COALESCE(t.access_count, v.access_count) AS access_count,
			       COALESCE(t.decay_exempt, v.decay_exempt) AS decay_exempt,
			       COALESCE(t.tags, v.tags) AS tags,
			       COALESCE(t.created_at, v.created_at) AS created_at,
			       COALESCE(t.text_score, 0) AS text_score,
			       COALESCE(v.vec_score, 0) AS vec_score
			FROM text_results t
			FULL OUTER JOIN vec_results v ON t.id = v.id
		)
		SELECT id, agent_id, user_id, memory_type, content, summary, source, source_type,
		       importance, access_count, decay_exempt, tags, created_at,
		       (1 - $5) * text_score + $5 * vec_score AS hybrid_score
		FROM combined
		ORDER BY ((1 - $5) * text_score + $5 * vec_score) * importance DESC
		LIMIT $4`,
		query, agentID, queryVec, maxResults, alpha)
	if err != nil {
		return nil, fmt.Errorf("hybrid search: %w", err)
	}
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		var m Memory
		var hybridScore float64
		var userID, summary, source *string
		rows.Scan(&m.ID, &m.AgentID, &userID, &m.Type, &m.Content, &summary, &source,
			&m.SourceType, &m.Importance, &m.AccessCount, &m.DecayExempt, &m.Tags, &m.CreatedAt, &hybridScore)
		if userID != nil {
			m.UserID = *userID
		}
		if summary != nil {
			m.Summary = *summary
		}
		if source != nil {
			m.Source = *source
		}
		score := hybridScore * m.Importance * recencyScore(m.CreatedAt)
		results = append(results, SearchResult{Memory: m, Score: score})
	}
	return results, nil
}

// BackfillEmbeddings generates embeddings for memories that don't have them yet.
func (g *GraphStore) BackfillEmbeddings(ctx context.Context, agentID string, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 50
	}

	rows, err := g.pool.Query(ctx,
		`SELECT id, content FROM memories
		 WHERE agent_id = $1 AND embedding IS NULL
		 ORDER BY created_at DESC LIMIT $2`,
		agentID, batchSize)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	ids := []string{}
	texts := []string{}
	for rows.Next() {
		var id, content string
		rows.Scan(&id, &content)
		ids = append(ids, id)
		texts = append(texts, content)
	}
	if len(texts) == 0 {
		return 0, nil
	}

	embeddings, err := g.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("batch embed: %w", err)
	}

	updated := 0
	for i, id := range ids {
		if i >= len(embeddings) {
			break
		}
		_, err := g.pool.Exec(ctx,
			`UPDATE memories SET embedding = $1 WHERE id = $2`,
			embeddings[i], id)
		if err == nil {
			updated++
		}
	}
	return updated, nil
}
