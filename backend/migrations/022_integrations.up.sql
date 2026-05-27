-- Migration 022: Integration relay support (Pipedream Connect)

-- Extend connector_actions with relay routing columns
ALTER TABLE public.connector_actions
    ADD COLUMN IF NOT EXISTS execution_backend TEXT DEFAULT 'direct',
    ADD COLUMN IF NOT EXISTS pipedream_action_id TEXT;

COMMENT ON COLUMN public.connector_actions.execution_backend IS 'direct = vault HTTP, pipedream = relay API';
COMMENT ON COLUMN public.connector_actions.pipedream_action_id IS 'Pipedream component action ID, e.g. twitter-create-tweet';

-- Connected accounts (relay-managed OAuth tokens)
CREATE TABLE IF NOT EXISTS public.connected_accounts (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID NOT NULL,
    relay_provider       TEXT NOT NULL DEFAULT 'pipedream',
    external_account_id  TEXT NOT NULL,
    platform_id          TEXT NOT NULL,
    display_name         TEXT DEFAULT '',
    authorized_scopes    TEXT[] DEFAULT '{}',
    healthy              BOOLEAN DEFAULT true,
    connected_at         TIMESTAMPTZ DEFAULT now(),
    last_checked_at      TIMESTAMPTZ,
    UNIQUE (tenant_id, external_account_id)
);

CREATE INDEX IF NOT EXISTS idx_connected_accounts_tenant
    ON public.connected_accounts (tenant_id);
CREATE INDEX IF NOT EXISTS idx_connected_accounts_platform
    ON public.connected_accounts (tenant_id, platform_id);

-- Integration action log (audit trail)
CREATE TABLE IF NOT EXISTS public.integration_action_log (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    agent_id      UUID,
    session_id    UUID,
    platform_id   TEXT NOT NULL,
    action_key    TEXT NOT NULL,
    backend_used  TEXT NOT NULL,
    success       BOOLEAN NOT NULL,
    error_message TEXT,
    execution_id  TEXT,
    created_at    TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_action_log_tenant
    ON public.integration_action_log (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_action_log_agent
    ON public.integration_action_log (agent_id, created_at DESC);

-- Integration permissions (per-agent platform access)
CREATE TABLE IF NOT EXISTS public.integration_permissions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    agent_id    UUID NOT NULL,
    platform_id TEXT,
    action_key  TEXT,
    allowed     BOOLEAN DEFAULT true,
    created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_integration_perms_unique
    ON public.integration_permissions (tenant_id, agent_id, COALESCE(platform_id, '__all__'), COALESCE(action_key, '__all__'));

CREATE INDEX IF NOT EXISTS idx_integration_perms_agent
    ON public.integration_permissions (tenant_id, agent_id);

-- Mark migration
INSERT INTO schema_migrations (version, dirty) VALUES (22, false)
ON CONFLICT (version) DO NOTHING;
