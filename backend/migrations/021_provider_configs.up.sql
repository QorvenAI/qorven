-- provider_configs stores generic provider configuration blobs (search providers,
-- integration settings, OAuth app configs, etc.) keyed by tenant + provider_type.
CREATE TABLE IF NOT EXISTS public.provider_configs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    provider_type TEXT NOT NULL,
    config      JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, provider_type)
);

CREATE INDEX IF NOT EXISTS idx_provider_configs_tenant
    ON public.provider_configs (tenant_id);
