-- Migration 002: AI Gateway tables
-- Adds: gateway_budgets, gateway_spend, model_aliases, llm_cache, oauth_tokens

-- Per-agent / per-team spend caps
CREATE TABLE IF NOT EXISTS gateway_budgets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    agent_id     UUID,           -- NULL = team-level budget
    team_id      UUID,
    monthly_usd  NUMERIC(10,2),  -- NULL = unlimited
    daily_usd    NUMERIC(10,4),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_gateway_budgets_agent  ON gateway_budgets (tenant_id, agent_id)  WHERE agent_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_gateway_budgets_team   ON gateway_budgets (tenant_id, team_id)   WHERE team_id  IS NOT NULL;

-- Daily spend tracking (upserted per agent per calendar day)
CREATE TABLE IF NOT EXISTS gateway_spend (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    agent_id     UUID NOT NULL,
    model_id     TEXT,
    provider_id  TEXT,
    tokens_in    BIGINT NOT NULL DEFAULT 0,
    tokens_out   BIGINT NOT NULL DEFAULT 0,
    cost_usd     NUMERIC(12,6)   NOT NULL DEFAULT 0,
    period       DATE            NOT NULL DEFAULT CURRENT_DATE,
    created_at   TIMESTAMPTZ     NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, agent_id, period)
);

CREATE INDEX IF NOT EXISTS idx_gateway_spend_agent ON gateway_spend (agent_id, period);
CREATE INDEX IF NOT EXISTS idx_gateway_spend_tenant ON gateway_spend (tenant_id, period);

-- Admin-overridable model aliases ("fast", "smart", "cheap", "vision", "code", "reason")
CREATE TABLE IF NOT EXISTS model_aliases (
    tenant_id TEXT NOT NULL,
    alias     TEXT NOT NULL,
    model_id  TEXT NOT NULL,
    priority  INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, alias, model_id)
);

-- Semantic + exact LLM response cache (tier-2 uses pgvector if extension available)
CREATE TABLE IF NOT EXISTS llm_cache (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    cache_key   TEXT NOT NULL,           -- SHA-256(model+messages) for exact tier
    prompt_hash TEXT NOT NULL,
    response    JSONB NOT NULL,
    model       TEXT,
    tokens_saved INT  NOT NULL DEFAULT 0,
    hit_count   INT  NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_cache_key    ON llm_cache (tenant_id, cache_key);
CREATE INDEX        IF NOT EXISTS idx_llm_cache_expiry ON llm_cache (expires_at) WHERE expires_at IS NOT NULL;

-- OAuth tokens for providers that use OAuth 2.0 (Claude Code, GitHub Copilot, Google Vertex AI)
CREATE TABLE IF NOT EXISTS oauth_tokens (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    provider      TEXT NOT NULL,        -- "claude_code" | "github_copilot" | "google_vertex"
    access_token  BYTEA NOT NULL,       -- AES-256-GCM encrypted
    refresh_token BYTEA,               -- AES-256-GCM encrypted; NULL if provider doesn't issue one
    expires_at    TIMESTAMPTZ,
    scopes        TEXT[],
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, provider)
);
