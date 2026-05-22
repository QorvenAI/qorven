-- Migration 003 rollback: remove gateway_spend_raw and added columns
DROP TABLE IF EXISTS gateway_spend_raw;
ALTER TABLE gateway_spend DROP COLUMN IF EXISTS tokens_thinking;
ALTER TABLE gateway_spend DROP COLUMN IF EXISTS tokens_cache_write;
ALTER TABLE gateway_spend DROP COLUMN IF EXISTS tokens_cache_read;
ALTER TABLE gateway_spend DROP COLUMN IF EXISTS cost_total_uusd;
