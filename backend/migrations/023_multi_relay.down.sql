-- 023_multi_relay.down.sql
DROP TABLE IF EXISTS social_account_rules;
DROP INDEX IF EXISTS idx_social_integrations_relay_key;
ALTER TABLE social_integrations
  DROP COLUMN IF EXISTS relay_provider,
  DROP COLUMN IF EXISTS relay_provider_key_id,
  DROP COLUMN IF EXISTS relay_account_id,
  DROP COLUMN IF EXISTS relay_metadata;
DROP TABLE IF EXISTS relay_providers;
