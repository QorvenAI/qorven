-- Rollback migration 002: AI Gateway tables

DROP TABLE IF EXISTS oauth_tokens;
DROP TABLE IF EXISTS llm_cache;
DROP TABLE IF EXISTS model_aliases;
DROP TABLE IF EXISTS gateway_spend;
DROP TABLE IF EXISTS gateway_budgets;
