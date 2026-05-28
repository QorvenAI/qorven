-- 026: Persistent deployments table
CREATE TABLE IF NOT EXISTS deployments (
    id           UUID PRIMARY KEY,
    tenant_id    UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    project_id   TEXT NOT NULL,
    project_name TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending',
    framework    TEXT NOT NULL DEFAULT '',
    url          TEXT,
    dockerfile   TEXT,
    error        TEXT,
    build_log    TEXT[] DEFAULT '{}',
    created_at   TIMESTAMPTZ DEFAULT now(),
    updated_at   TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_deployments_project ON deployments (project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments (status) WHERE status IN ('building', 'pushing', 'live');
