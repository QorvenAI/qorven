-- Migration 027: Provider budget types, key spend attribution, provider-level budgets
-- Adds: budget_type to provider_keys, key_id to gateway_spend_raw,
--        provider_budgets table, pkce_state table for OAuth PKCE flows.

-- ── 1. Budget type on provider_keys ────────────────────────────────────────
-- budget_type: how the user's account with this provider is charged
--   'prepaid'  — user loaded credits; balance depletes; never auto-resets
--   'postpaid' — billed monthly (Bedrock, Azure, Vertex); self-imposed cap; resets monthly
--   'quota'    — OAuth/subscription (Claude Code, Copilot, Gemini); $ invisible; token-quota only
--   'free'     — local model (Ollama, LM Studio); always allowed; track tokens only
ALTER TABLE provider_keys
    ADD COLUMN IF NOT EXISTS budget_type        TEXT    NOT NULL DEFAULT 'prepaid'
        CHECK (budget_type IN ('prepaid','postpaid','quota','free')),
    ADD COLUMN IF NOT EXISTS balance_usd        NUMERIC(10,4),   -- prepaid: declared loaded balance
    ADD COLUMN IF NOT EXISTS token_quota_monthly BIGINT,         -- quota type: monthly token cap (NULL=unlimited)
    ADD COLUMN IF NOT EXISTS budget_reset_day   INT     NOT NULL DEFAULT 1
        CHECK (budget_reset_day BETWEEN 1 AND 28);  -- day-of-month for postpaid/quota resets

-- For local/free providers, set budget_type = 'free' where provider_id matches known local providers
-- (Ollama, LM Studio, Jan — identified by no API key being a real key).
-- This is a hint; users can override.

-- ── 2. key_id on gateway_spend_raw ─────────────────────────────────────────
-- Allows per-key spend attribution (which specific key incurred which cost).
ALTER TABLE gateway_spend_raw
    ADD COLUMN IF NOT EXISTS key_id UUID REFERENCES provider_keys(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_spend_raw_key ON gateway_spend_raw (key_id, created_at)
    WHERE key_id IS NOT NULL;

-- Also add key_id to gateway_spend (daily aggregate) for per-key daily rollup
ALTER TABLE gateway_spend
    ADD COLUMN IF NOT EXISTS key_id UUID REFERENCES provider_keys(id) ON DELETE SET NULL;

-- ── 3. Provider-level budget table ─────────────────────────────────────────
-- User can set an overall cap per provider (e.g. "never spend more than $50/month on OpenAI
-- across all my OpenAI keys combined").
CREATE TABLE IF NOT EXISTS provider_budgets (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID        NOT NULL,
    provider_id     TEXT        NOT NULL,          -- e.g. "openai", "anthropic", "bedrock"
    budget_type     TEXT        NOT NULL DEFAULT 'postpaid'
        CHECK (budget_type IN ('postpaid','prepaid','quota','free')),
    budget_usd      NUMERIC(10,2),                 -- monthly cap (NULL = unlimited)
    token_quota     BIGINT,                        -- monthly token cap for quota type
    spent_usd_month NUMERIC(10,4) NOT NULL DEFAULT 0,
    spent_tokens_month BIGINT    NOT NULL DEFAULT 0,
    reset_day       INT          NOT NULL DEFAULT 1
        CHECK (reset_day BETWEEN 1 AND 28),
    budget_reset_at TIMESTAMPTZ  NOT NULL DEFAULT
        (date_trunc('month', now()) + '1 month'::interval),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, provider_id)
);

CREATE INDEX IF NOT EXISTS idx_provider_budgets_tenant ON provider_budgets (tenant_id);

-- ── 4. PKCE state table for OAuth flows ────────────────────────────────────
-- Stores ephemeral PKCE verifier + state during the OAuth redirect round-trip.
-- Rows expire after 10 minutes (cleaned up by cron or on next start).
CREATE TABLE IF NOT EXISTS oauth_pkce_state (
    state           TEXT        PRIMARY KEY,       -- random hex state param
    provider        TEXT        NOT NULL,
    tenant_id       UUID        NOT NULL,
    code_verifier   TEXT        NOT NULL,           -- PKCE S256 verifier
    redirect_uri    TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT (now() + '10 minutes'::interval)
);

CREATE INDEX IF NOT EXISTS idx_oauth_pkce_expires ON oauth_pkce_state (expires_at);

-- Extend oauth_tokens with PKCE metadata and token type
ALTER TABLE oauth_tokens
    ADD COLUMN IF NOT EXISTS token_type     TEXT    NOT NULL DEFAULT 'bearer',
    ADD COLUMN IF NOT EXISTS provider_type  TEXT,   -- maps to existing ProviderType constants
    ADD COLUMN IF NOT EXISTS pkce_used      BOOL    NOT NULL DEFAULT false;
