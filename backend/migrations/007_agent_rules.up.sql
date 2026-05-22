-- agent_rules: stores user-stated policies that Prime enforces autonomously.
-- Created by the set_rule tool; backed by the existing cron/daemon infrastructure.
CREATE TABLE IF NOT EXISTS agent_rules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL,
    agent_id     UUID NOT NULL,
    description  TEXT NOT NULL,
    trigger_type TEXT NOT NULL CHECK (trigger_type IN ('cron', 'threshold', 'event')),
    trigger_spec JSONB NOT NULL DEFAULT '{}',
    action_type  TEXT NOT NULL CHECK (action_type IN ('run_tool', 'escalate', 'notify')),
    action_spec  JSONB NOT NULL DEFAULT '{}',
    enabled      BOOLEAN NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_agent_rules_agent   ON agent_rules (tenant_id, agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_rules_enabled ON agent_rules (tenant_id, enabled) WHERE enabled = true;
