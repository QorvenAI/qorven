-- 029_spend_origin: tag each raw spend row with what triggered the call
-- (agent | memory | background | council | system | ...). Enables hybrid
-- attribution: agent-triggered system work charges the agent; global
-- maintenance charges the overhead bucket (NULL agent_id + origin='system').
ALTER TABLE gateway_spend_raw ADD COLUMN IF NOT EXISTS origin TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_spend_raw_origin ON gateway_spend_raw (tenant_id, origin, created_at);
