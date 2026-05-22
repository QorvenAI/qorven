DROP INDEX IF EXISTS idx_llm_cache_embedding;
ALTER TABLE llm_cache DROP COLUMN IF EXISTS embedding;
