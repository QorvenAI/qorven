-- 033_knowledge_briefs: CKO-curated, clearance-tagged knowledge briefs.
-- One row per (tenant, scope, scope_key). Curated by the CKO on a schedule
-- and on demand; injected (clearance-filtered) into each agent's system prompt.

CREATE TABLE IF NOT EXISTS knowledge_briefs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    scope        TEXT NOT NULL,                       -- company|department|role
    scope_key    TEXT NOT NULL DEFAULT '',            -- '' for company; dept/role name otherwise
    clearance    INT NOT NULL DEFAULT 1,              -- max classification contained (0..3)
    content      TEXT NOT NULL DEFAULT '',
    sources      JSONB NOT NULL DEFAULT '[]',         -- provenance: memory/doc/work_item/research ids
    version      INT NOT NULL DEFAULT 1,
    refreshed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    refreshed_by UUID,
    UNIQUE (tenant_id, scope, scope_key)
);
CREATE INDEX IF NOT EXISTS idx_knowledge_briefs_tenant_scope
    ON knowledge_briefs (tenant_id, scope, scope_key);
