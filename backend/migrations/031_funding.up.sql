-- 031_funding: funding mode on the overall (tenant) budget + declared OAuth/
-- usage windows so the key pool can proactively route around an exhausted
-- subscription key. lifetime_usd already exists (migration 024) — S3 starts
-- reading it as the prepaid fixed cap.

-- funding_mode on the caps table. NULL is interpreted as 'monthly_recurring'
-- so every existing row keeps its S2 behavior; the tenant row is set
-- explicitly by SetBudget.
ALTER TABLE gateway_budgets ADD COLUMN IF NOT EXISTS funding_mode TEXT;

CREATE TABLE IF NOT EXISTS provider_usage_windows (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID NOT NULL,
    key_id           UUID,
    provider_id      TEXT,
    window_kind      TEXT NOT NULL,            -- hourly | 5h | daily | weekly | request_count
    limit_count      BIGINT NOT NULL,
    used_count       BIGINT NOT NULL DEFAULT 0,
    window_resets_at TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_usage_windows_tenant_key ON provider_usage_windows (tenant_id, key_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_usage_windows_key ON provider_usage_windows (key_id) WHERE key_id IS NOT NULL;
