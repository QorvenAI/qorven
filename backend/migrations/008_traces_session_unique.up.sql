-- Migration 008: unique index on traces(tenant_id, session_key) for upsert support
CREATE UNIQUE INDEX IF NOT EXISTS idx_traces_tenant_session
    ON traces (tenant_id, session_key)
    WHERE session_key IS NOT NULL;
