-- Migration 017: C-Suite org hierarchy tables and agent org columns
-- Adds org_level, org_role, kb_grants, customer_facing, dept_head_id to agents.
-- Creates org_roster (hire/terminate tracking), org_daily_spend (per-agent cost),
-- and org_delegations (task delegation audit trail).

-- ─── Agent org columns ────────────────────────────────────────────────────────

ALTER TABLE public.agents
    ADD COLUMN IF NOT EXISTS org_level        TEXT DEFAULT 'l3',
    ADD COLUMN IF NOT EXISTS org_role         TEXT DEFAULT '',
    ADD COLUMN IF NOT EXISTS kb_grants        TEXT[] DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS customer_facing  BOOLEAN DEFAULT false,
    ADD COLUMN IF NOT EXISTS dept_head_id     UUID REFERENCES public.agents(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS hired_at         TIMESTAMPTZ DEFAULT now(),
    ADD COLUMN IF NOT EXISTS terminated_at    TIMESTAMPTZ;

-- ─── Org roster ───────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS public.org_roster (
    id                UUID PRIMARY KEY DEFAULT public.uuid_generate_v7(),
    tenant_id         UUID NOT NULL,
    agent_id          UUID REFERENCES public.agents(id) ON DELETE SET NULL,
    org_level         TEXT NOT NULL DEFAULT 'l3',
    org_role          TEXT NOT NULL DEFAULT '',
    display_name      TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'active',
    hired_at          TIMESTAMPTZ DEFAULT now(),
    hired_by          UUID,
    terminated_at     TIMESTAMPTZ,
    terminated_by     UUID,
    termination_reason TEXT,
    total_spend_usd   NUMERIC(14,6) DEFAULT 0,
    total_tokens_in   BIGINT DEFAULT 0,
    total_tokens_out  BIGINT DEFAULT 0,
    metadata          JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_org_roster_tenant    ON public.org_roster (tenant_id);
CREATE INDEX IF NOT EXISTS idx_org_roster_agent     ON public.org_roster (agent_id);
CREATE INDEX IF NOT EXISTS idx_org_roster_status    ON public.org_roster (status);

-- ─── Daily spend per agent ────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS public.org_daily_spend (
    id           UUID PRIMARY KEY DEFAULT public.uuid_generate_v7(),
    tenant_id    UUID NOT NULL,
    agent_id     UUID NOT NULL,
    org_role     TEXT DEFAULT '',
    date         DATE NOT NULL DEFAULT CURRENT_DATE,
    tokens_in    BIGINT DEFAULT 0,
    tokens_out   BIGINT DEFAULT 0,
    cost_usd     NUMERIC(14,6) DEFAULT 0,
    model_used   TEXT DEFAULT '',
    UNIQUE (tenant_id, agent_id, date)
);

CREATE INDEX IF NOT EXISTS idx_org_daily_spend_agent  ON public.org_daily_spend (agent_id, date);
CREATE INDEX IF NOT EXISTS idx_org_daily_spend_tenant ON public.org_daily_spend (tenant_id, date);

-- ─── Delegation audit ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS public.org_delegations (
    id           UUID PRIMARY KEY DEFAULT public.uuid_generate_v7(),
    tenant_id    UUID NOT NULL,
    from_agent   UUID NOT NULL,
    to_agent     UUID NOT NULL,
    task_id      UUID,
    message      TEXT DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending',
    created_at   TIMESTAMPTZ DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_org_delegations_from   ON public.org_delegations (from_agent);
CREATE INDEX IF NOT EXISTS idx_org_delegations_to     ON public.org_delegations (to_agent);
CREATE INDEX IF NOT EXISTS idx_org_delegations_status ON public.org_delegations (status);

-- ─── Mark migration ───────────────────────────────────────────────────────────

INSERT INTO schema_migrations (version, dirty) VALUES (17, false)
ON CONFLICT (version) DO NOTHING;
