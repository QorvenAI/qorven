-- Migration 006: Track calls where model pricing was unknown at call time.
-- These rows have correct token counts but zero cost — flagged for backfill
-- when pricing data is later added.

ALTER TABLE gateway_spend_raw
    ADD COLUMN IF NOT EXISTS pricing_missing BOOL NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_spend_raw_pricing_missing
    ON gateway_spend_raw (pricing_missing, created_at)
    WHERE pricing_missing = true;
