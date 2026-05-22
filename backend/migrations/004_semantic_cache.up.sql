-- Migration 004: Semantic cache — pgvector tier for llm_cache
-- Adds vector embedding column, HNSW index, and hit-count tracking.

-- Ensure pgvector is available (it is on all Qorven deployments).
CREATE EXTENSION IF NOT EXISTS vector;

-- Add vector embedding column to llm_cache.
-- 1536 dimensions = text-embedding-3-small default.
ALTER TABLE llm_cache ADD COLUMN IF NOT EXISTS embedding vector(1536);

-- HNSW index for approximate nearest-neighbour search.
-- Much faster than IVFFlat for online inserts (no need to pre-build lists).
-- ef_construction=64, m=16 — standard balanced settings.
CREATE INDEX IF NOT EXISTS idx_llm_cache_embedding
    ON llm_cache USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
