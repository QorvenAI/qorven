CREATE TABLE IF NOT EXISTS agent_writers (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id   UUID NOT NULL,
    username   TEXT NOT NULL,          -- Telegram @username (without @)
    user_id    BIGINT,                 -- Telegram numeric user_id (optional, for text_mention entities)
    display_name TEXT NOT NULL DEFAULT '',
    granted_by TEXT NOT NULL DEFAULT '', -- chat_id or admin username who granted
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (agent_id, username)
);

CREATE INDEX IF NOT EXISTS idx_agent_writers_agent ON agent_writers (agent_id);
