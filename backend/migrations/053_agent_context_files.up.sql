-- 053_agent_context_files: current content of agent persona/context files loaded
-- at runtime into the agent's system prompt.
CREATE TABLE IF NOT EXISTS agent_context_files (
    agent_id    uuid NOT NULL,
    file_name   text NOT NULL,
    content     text NOT NULL DEFAULT '',
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, file_name)
);
