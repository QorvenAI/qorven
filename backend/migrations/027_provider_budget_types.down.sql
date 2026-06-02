-- Rollback migration 027
DROP TABLE IF EXISTS oauth_pkce_state;
DROP TABLE IF EXISTS provider_budgets;

ALTER TABLE oauth_tokens
    DROP COLUMN IF EXISTS token_type,
    DROP COLUMN IF EXISTS provider_type,
    DROP COLUMN IF EXISTS pkce_used;

ALTER TABLE gateway_spend
    DROP COLUMN IF EXISTS key_id;

ALTER TABLE gateway_spend_raw
    DROP COLUMN IF EXISTS key_id;

ALTER TABLE provider_keys
    DROP COLUMN IF EXISTS budget_type,
    DROP COLUMN IF EXISTS balance_usd,
    DROP COLUMN IF EXISTS token_quota_monthly,
    DROP COLUMN IF EXISTS budget_reset_day;
