DROP INDEX IF EXISTS uq_usage_windows_key;
DROP INDEX IF EXISTS idx_usage_windows_tenant_key;
DROP TABLE IF EXISTS provider_usage_windows;
ALTER TABLE gateway_budgets DROP COLUMN IF EXISTS funding_mode;
