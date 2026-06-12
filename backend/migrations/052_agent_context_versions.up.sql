-- 052_agent_context_versions: keep prior versions of agent persona/context files
-- so a bad SOUL.md edit is recoverable.
CREATE TABLE IF NOT EXISTS agent_context_file_versions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id    uuid NOT NULL,
    file_name   text NOT NULL,
    content     text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_acfv_agent_file ON agent_context_file_versions (agent_id, file_name, created_at DESC);
