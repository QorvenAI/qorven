DROP INDEX IF EXISTS idx_spend_raw_pricing_missing;
ALTER TABLE gateway_spend_raw DROP COLUMN IF EXISTS pricing_missing;
