-- Migration 019: Knowledge Governance System
-- Adds data classification, access control grants, PII vault, audit logging,
-- and agent clearance overrides for the zero-leak knowledge isolation architecture.

-- ─── Classification on memories ──────────────────────────────────────────────

ALTER TABLE public.memories
    ADD COLUMN IF NOT EXISTS classification INTEGER DEFAULT 1;
-- 0=public, 1=internal (default), 2=confidential, 3=restricted

CREATE INDEX IF NOT EXISTS idx_memories_classification
    ON public.memories (tenant_id, classification);

-- ─── Knowledge grants (cross-agent access permissions) ───────────────────────

CREATE TABLE IF NOT EXISTS public.knowledge_grants (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL,
    grantor_agent_id  UUID NOT NULL,
    grantee_agent_id  UUID NOT NULL,
    scope             TEXT NOT NULL,
    max_classification INTEGER DEFAULT 1,
    read_only         BOOLEAN DEFAULT true,
    purpose           TEXT,
    granted_by        TEXT NOT NULL,
    created_at        TIMESTAMPTZ DEFAULT now(),
    expires_at        TIMESTAMPTZ,
    revoked           BOOLEAN DEFAULT false,
    revoked_by        TEXT,
    revoked_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_grants_grantee
    ON public.knowledge_grants (tenant_id, grantee_agent_id) WHERE NOT revoked;
CREATE INDEX IF NOT EXISTS idx_grants_grantor
    ON public.knowledge_grants (tenant_id, grantor_agent_id);

-- ─── PII vault (encrypted original values for reversible redaction) ──────────

CREATE TABLE IF NOT EXISTS public.pii_vault (
    id              TEXT PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    agent_id        UUID NOT NULL,
    kind            TEXT NOT NULL,
    ciphertext      BYTEA NOT NULL,
    access_list     TEXT[] NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_pii_vault_agent
    ON public.pii_vault (tenant_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_pii_vault_expiry
    ON public.pii_vault (expires_at);

-- ─── PII access log (audit trail for vault retrievals) ───────────────────────

CREATE TABLE IF NOT EXISTS public.pii_access_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vault_id        TEXT NOT NULL,
    agent_id        UUID NOT NULL,
    purpose         TEXT NOT NULL,
    accessed_at     TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pii_access_vault
    ON public.pii_access_log (vault_id);
CREATE INDEX IF NOT EXISTS idx_pii_access_agent
    ON public.pii_access_log (agent_id, accessed_at);

-- ─── Knowledge access audit log ──────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS public.knowledge_access_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    agent_id        UUID NOT NULL,
    operation       TEXT NOT NULL,
    scope           TEXT,
    classification  INTEGER,
    query_hash      TEXT,
    result_count    INTEGER,
    denied          BOOLEAN DEFAULT false,
    deny_reason     TEXT,
    accessed_at     TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_knowledge_access_agent
    ON public.knowledge_access_log (tenant_id, agent_id, accessed_at);
CREATE INDEX IF NOT EXISTS idx_knowledge_access_time
    ON public.knowledge_access_log (accessed_at);

-- ─── Agent clearance overrides (beyond role defaults) ────────────────────────

CREATE TABLE IF NOT EXISTS public.agent_clearances (
    agent_id          UUID PRIMARY KEY,
    tenant_id         UUID NOT NULL,
    max_classification INTEGER NOT NULL DEFAULT 1,
    updated_by        TEXT NOT NULL,
    updated_at        TIMESTAMPTZ DEFAULT now(),
    reason            TEXT
);

-- ─── Onboarding pipeline tracking ───────────────────────────────────────────

CREATE TABLE IF NOT EXISTS public.agent_onboarding (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    agent_id        UUID NOT NULL,
    stage           TEXT NOT NULL DEFAULT 'created',
    chro_completed  BOOLEAN DEFAULT false,
    cko_completed   BOOLEAN DEFAULT false,
    cfo_completed   BOOLEAN DEFAULT false,
    kb_grants       TEXT[] DEFAULT '{}',
    clearance_level INTEGER DEFAULT 1,
    budget_usd      NUMERIC(10,2) DEFAULT 0,
    created_at      TIMESTAMPTZ DEFAULT now(),
    completed_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_onboarding_agent
    ON public.agent_onboarding (agent_id);
CREATE INDEX IF NOT EXISTS idx_onboarding_stage
    ON public.agent_onboarding (stage) WHERE completed_at IS NULL;

-- ─── Mark migration ─────────────────────────────────────────────────────────

INSERT INTO schema_migrations (version, dirty) VALUES (19, false)
ON CONFLICT (version) DO NOTHING;
