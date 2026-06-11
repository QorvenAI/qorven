-- 042_project_events: structured per-project event timeline. The durable spine
-- the Hub/Analytics surfaces read, and the contract 8C's swarm emits into.
CREATE TABLE IF NOT EXISTS project_events (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL,
    project_brief_id uuid NOT NULL,
    task_id          uuid,
    agent_id         uuid,
    type             text NOT NULL CHECK (type IN (
        'task_started','task_progress','pr_opened','pr_merged','blocked','done',
        'gate_decision','budget_warning','agent_spawned','agent_terminated')),
    title            text NOT NULL DEFAULT '',
    payload          jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_project_events_brief_time ON project_events (project_brief_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_project_events_brief_type ON project_events (project_brief_id, type);
