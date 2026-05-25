-- Social media asset library
CREATE TABLE IF NOT EXISTS social_media_assets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    TEXT NOT NULL DEFAULT 'default',
    agent_id     TEXT NOT NULL,
    name         TEXT NOT NULL,
    original_name TEXT NOT NULL,
    path         TEXT NOT NULL,
    mime_type    TEXT NOT NULL,
    size         BIGINT NOT NULL DEFAULT 0,
    width        INT,
    height       INT,
    alt_text     TEXT NOT NULL DEFAULT '',
    tags         TEXT[] NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_social_media_assets_agent ON social_media_assets (agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_social_media_assets_tenant ON social_media_assets (tenant_id, created_at DESC);
