CREATE TABLE IF NOT EXISTS social_webhooks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    agent_id    UUID,
    name        TEXT NOT NULL DEFAULT '',
    url         TEXT NOT NULL,
    secret      TEXT NOT NULL DEFAULT '',
    events      TEXT[] NOT NULL DEFAULT '{post.published,post.failed}',
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_social_webhooks_tenant ON social_webhooks (tenant_id, active);
CREATE INDEX IF NOT EXISTS idx_social_webhooks_agent  ON social_webhooks (agent_id, active);
