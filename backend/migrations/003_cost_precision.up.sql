-- Migration 003: Bank-grade cost precision
-- Adds: gateway_spend_raw (immutable per-call audit log),
--        new token/cost columns on gateway_spend (integer micro-dollar precision).

-- Immutable per-call spend log (append-only, never updated or deleted)
CREATE TABLE IF NOT EXISTS gateway_spend_raw (
    id              UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID    NOT NULL,
    agent_id        UUID,
    session_id      TEXT,
    provider_id     TEXT    NOT NULL DEFAULT '',
    model_id        TEXT    NOT NULL DEFAULT '',
    -- All token counts as reported by provider (never estimated)
    tokens_in          BIGINT  NOT NULL DEFAULT 0,
    tokens_out         BIGINT  NOT NULL DEFAULT 0,
    tokens_thinking    BIGINT  NOT NULL DEFAULT 0,
    tokens_cache_write BIGINT  NOT NULL DEFAULT 0,
    tokens_cache_read  BIGINT  NOT NULL DEFAULT 0,
    -- Cost in integer micro-dollars (1 USD = 1_000_000 µUSD)
    -- Each component tracked separately for reconciliation
    cost_input_uusd     BIGINT  NOT NULL DEFAULT 0,
    cost_output_uusd    BIGINT  NOT NULL DEFAULT 0,
    cost_thinking_uusd  BIGINT  NOT NULL DEFAULT 0,
    cost_cache_w_uusd   BIGINT  NOT NULL DEFAULT 0,
    cost_cache_r_uusd   BIGINT  NOT NULL DEFAULT 0,
    cost_total_uusd     BIGINT  NOT NULL DEFAULT 0,  -- sum of all components
    -- Was this a gateway cache hit? (free — no provider charge)
    cache_hit       BOOL    NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_spend_raw_agent   ON gateway_spend_raw (agent_id, created_at);
CREATE INDEX IF NOT EXISTS idx_spend_raw_tenant  ON gateway_spend_raw (tenant_id, created_at);
CREATE INDEX IF NOT EXISTS idx_spend_raw_session ON gateway_spend_raw (session_id, created_at) WHERE session_id IS NOT NULL;

-- Alter gateway_spend to add micro-dollar columns (integer precision) alongside existing float
ALTER TABLE gateway_spend ADD COLUMN IF NOT EXISTS tokens_thinking    BIGINT NOT NULL DEFAULT 0;
ALTER TABLE gateway_spend ADD COLUMN IF NOT EXISTS tokens_cache_write BIGINT NOT NULL DEFAULT 0;
ALTER TABLE gateway_spend ADD COLUMN IF NOT EXISTS tokens_cache_read  BIGINT NOT NULL DEFAULT 0;
ALTER TABLE gateway_spend ADD COLUMN IF NOT EXISTS cost_total_uusd    BIGINT NOT NULL DEFAULT 0;
