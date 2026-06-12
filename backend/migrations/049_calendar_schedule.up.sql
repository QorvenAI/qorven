-- 049_calendar_schedule: scheduled-run history + one-shot cron jobs.
-- Enables the calendar timeline to show real past executions and run-once tasks.

-- One-shot marker: a job that fires once then disables itself (vs a recurring cron expr).
ALTER TABLE cron_jobs
  ADD COLUMN IF NOT EXISTS one_shot boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS run_count integer NOT NULL DEFAULT 0;

-- Execution history: one row per scheduled run (cron, one-shot, or any source that records).
CREATE TABLE IF NOT EXISTS scheduled_runs (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL,
    agent_id       uuid,
    source         text NOT NULL,
    source_id      text NOT NULL,
    title          text NOT NULL DEFAULT '',
    scheduled_for  timestamptz,
    started_at     timestamptz NOT NULL DEFAULT now(),
    finished_at    timestamptz,
    status         text NOT NULL DEFAULT 'running',
    result_snippet text NOT NULL DEFAULT '',
    tokens         bigint NOT NULL DEFAULT 0,
    cost_cents     bigint NOT NULL DEFAULT 0,
    error          text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_scheduled_runs_tenant_time ON scheduled_runs (tenant_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_scheduled_runs_agent_time  ON scheduled_runs (agent_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_scheduled_runs_source      ON scheduled_runs (source, source_id);
