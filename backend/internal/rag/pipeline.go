// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/knowledgegraph"
	"github.com/qorvenai/qorven/internal/memory"
)

// Chunk is a retrieved piece of context.
type Chunk struct {
	Content   string  `json:"content"`
	Source    string  `json:"source"` // "memory", "kg", "document"
	SourceID string  `json:"source_id,omitempty"`
	Relevance float64 `json:"relevance"`
}

// DocumentChunk stored in pgvector.
type DocumentChunk struct {
	ID         string    `json:"id"`
	AgentID    string    `json:"agent_id,omitempty"`
	SourceType string    `json:"source_type"`
	SourceName string    `json:"source_name"`
	Content    string    `json:"content"`
	TokenCount int       `json:"token_count"`
	CreatedAt  time.Time `json:"created_at"`
}

// Pipeline implements retrieve → rank → inject for agent context enrichment.
type Pipeline struct {
	pool     *pgxpool.Pool
	memStore *memory.Store
	kgStore  *knowledgegraph.Store
	tenantID string
	topK     int
	embeddingURL string // Provider API base for embeddings
}

func NewPipeline(pool *pgxpool.Pool, memStore *memory.Store, kgStore *knowledgegraph.Store, tenantID, embeddingURL string) *Pipeline {
	return &Pipeline{pool: pool, memStore: memStore, kgStore: kgStore, tenantID: tenantID, topK: 5, embeddingURL: embeddingURL}
}

// Ingest chunks a document and stores embeddings.
func (p *Pipeline) Ingest(ctx context.Context, agentID, sourceType, sourceID, sourceName, content string) (int, error) {
	chunks := chunkText(content, 512, 64) // 512 tokens per chunk, 64 overlap
	count := 0
	for i, chunk := range chunks {
		emb, err := p.embed(ctx, chunk)
		if err != nil {
			slog.Warn("rag.embed_failed", "chunk", i, "error", err)
			continue
		}
		_, err = p.pool.Exec(ctx,
			`INSERT INTO document_chunks (tenant_id, agent_id, source_type, source_id, source_name, chunk_index, content, token_count, embedding)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			p.tenantID, nilIfEmpty(agentID), sourceType, sourceID, sourceName, i, chunk, len(strings.Fields(chunk)), embeddingToString(emb))
		if err != nil {
			slog.Warn("rag.store_failed", "chunk", i, "error", err)
			continue
		}
		count++
	}
	slog.Info("rag.ingested", "source", sourceName, "chunks", count)
	return count, nil
}

// Retrieve searches all knowledge sources and returns ranked chunks.
func (p *Pipeline) Retrieve(ctx context.Context, agentID, query string) []Chunk {
	chunks := []Chunk{}

	// 1. Vector search on document_chunks
	if p.pool != nil {
		emb, err := p.embed(ctx, query)
		if err == nil {
			rows, err := p.pool.Query(ctx,
				`SELECT id, content, source_name, 1 - (embedding <=> $1::vector) as similarity
				 FROM document_chunks
				 WHERE tenant_id = $2 AND ($3 = '' OR agent_id::text = $3)
				 ORDER BY embedding <=> $1::vector LIMIT $4`,
				embeddingToString(emb), p.tenantID, agentID, p.topK)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var id, content, source string
					var sim float64
					rows.Scan(&id, &content, &source, &sim)
					chunks = append(chunks, Chunk{Content: content, Source: "document:" + source, SourceID: id, Relevance: sim})
				}
			}
		}
	}

	// 2. Keyword search (BM25-style via ts_rank)
	if p.pool != nil {
		rows, _ := p.pool.Query(ctx,
			`SELECT id, content, source_name, ts_rank(to_tsvector('english', content), plainto_tsquery('english', $1)) as rank
			 FROM document_chunks
			 WHERE tenant_id = $2 AND ($3 = '' OR agent_id::text = $3)
			   AND to_tsvector('english', content) @@ plainto_tsquery('english', $1)
			 ORDER BY rank DESC LIMIT $4`,
			query, p.tenantID, agentID, p.topK)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var id, content, source string
				var rank float64
				rows.Scan(&id, &content, &source, &rank)
				// Avoid duplicates from vector search
				dup := false
				for _, c := range chunks { if c.SourceID == id { dup = true; break } }
				if !dup {
					chunks = append(chunks, Chunk{Content: content, Source: "keyword:" + source, SourceID: id, Relevance: rank * 0.5})
				}
			}
		}
	}

	// 3. Search memories
	if p.memStore != nil {
		results, _ := p.memStore.Search(ctx, p.tenantID, agentID, query, p.topK)
		for _, r := range results {
			chunks = append(chunks, Chunk{Content: r.Memory.Content, Source: "memory", Relevance: r.Score})
		}
	}

	// 4. Search knowledge graph
	if p.kgStore != nil {
		entities, _ := p.kgStore.SearchEntities(ctx, p.tenantID, query, p.topK)
		for _, e := range entities {
			chunks = append(chunks, Chunk{
				Content:   fmt.Sprintf("%s (%s): %v", e.Name, e.EntityType, e.Properties),
				Source:    "kg",
				Relevance: e.Confidence,
			})
		}
	}

	// 5. Rank by relevance, take top-K
	sort.Slice(chunks, func(i, j int) bool { return chunks[i].Relevance > chunks[j].Relevance })
	if len(chunks) > p.topK {
		chunks = chunks[:p.topK]
	}
	return chunks
}

// Enrich injects retrieved context into the system prompt.
func (p *Pipeline) Enrich(ctx context.Context, agentID, query, systemPrompt string) string {
	chunks := p.Retrieve(ctx, agentID, query)
	if len(chunks) == 0 {
		return systemPrompt
	}
	var sb strings.Builder
	sb.WriteString(systemPrompt)
	sb.WriteString("\n\n## Relevant Context\n")
	for i, c := range chunks {
		sb.WriteString(fmt.Sprintf("[%d] (%s) %s\n", i+1, c.Source, c.Content))
	}
	return sb.String()
}

// DeleteBySource removes all chunks for a source.
func (p *Pipeline) DeleteBySource(ctx context.Context, sourceType, sourceID string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM document_chunks WHERE source_type = $1 AND source_id = $2`, sourceType, sourceID)
	return err
}

// embed calls provider /v1/embeddings endpoint.
func (p *Pipeline) embed(ctx context.Context, text string) ([]float32, error) {
	if p.embeddingURL == "" {
		return nil, fmt.Errorf("no embedding endpoint configured")
	}
	body := fmt.Sprintf(`{"model":"text-embedding-3-small","input":"%s"}`, strings.ReplaceAll(text, `"`, `\"`))
	req, _ := http.NewRequestWithContext(ctx, "POST", p.embeddingURL+"/v1/embeddings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result struct {
		Data []struct { Embedding []float32 `json:"embedding"` } `json:"data"`
	}
	json.Unmarshal(b, &result)
	if len(result.Data) == 0 { return nil, fmt.Errorf("no embedding returned") }
	return result.Data[0].Embedding, nil
}

func embeddingToString(emb []float32) string {
	parts := make([]string, len(emb))
	for i, v := range emb { parts[i] = fmt.Sprintf("%f", v) }
	return "[" + strings.Join(parts, ",") + "]"
}

func nilIfEmpty(s string) *string {
	if s == "" { return nil }
	return &s
}

// chunkText splits text into overlapping chunks by approximate word count.
func chunkText(text string, chunkSize, overlap int) []string {
	words := strings.Fields(text)
	if len(words) <= chunkSize {
		return []string{text}
	}
	chunks := []string{}
	for i := 0; i < len(words); i += chunkSize - overlap {
		end := i + chunkSize
		if end > len(words) { end = len(words) }
		chunks = append(chunks, strings.Join(words[i:end], " "))
		if end == len(words) { break }
	}
	return chunks
}
