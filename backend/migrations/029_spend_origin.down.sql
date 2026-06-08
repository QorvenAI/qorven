DROP INDEX IF EXISTS idx_spend_raw_origin;
ALTER TABLE gateway_spend_raw DROP COLUMN IF EXISTS origin;
