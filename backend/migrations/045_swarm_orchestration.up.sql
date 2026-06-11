-- 045_swarm_orchestration: durability columns for long-running task workers,
-- the serialized merge queue, and human-gated release proposals.
ALTER TABLE tasks
  ADD COLUMN IF NOT EXISTS phase          text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS lease_expires  timestamptz,
  ADD COLUMN IF NOT EXISTS max_iterations int NOT NULL DEFAULT 50,
  ADD COLUMN IF NOT EXISTS cancelled      boolean NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS idx_tasks_lease ON tasks (status, lease_expires);

CREATE TABLE IF NOT EXISTS merge_queue (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL,
    project_brief_id uuid NOT NULL,
    task_id          uuid,
    pr_number        int NOT NULL,
    branch           text NOT NULL DEFAULT '',
    base_sha         text NOT NULL DEFAULT '',
    status           text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued','merging','conflict','merged','failed')),
    attempt          int NOT NULL DEFAULT 0,
    detail           text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_merge_queue_project ON merge_queue (project_brief_id, status, created_at);

CREATE TABLE IF NOT EXISTS release_gates (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL,
    project_brief_id uuid NOT NULL,
    version          text NOT NULL DEFAULT '',
    changelog_md     text NOT NULL DEFAULT '',
    status           text NOT NULL DEFAULT 'proposed'
        CHECK (status IN ('proposed','approved','released','rejected')),
    proposed_by      text NOT NULL DEFAULT '',
    approved_by      text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_release_gates_project ON release_gates (project_brief_id, status);
