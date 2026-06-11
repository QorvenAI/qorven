-- 047_deploy_fixloop: deploy lineage + target, fix-loop bookkeeping on tasks,
-- and new project_events types for deploy + fix-loop telemetry.
ALTER TABLE deployments
  ADD COLUMN IF NOT EXISTS release_id   uuid,
  ADD COLUMN IF NOT EXISTS target       text NOT NULL DEFAULT 'local',
  ADD COLUMN IF NOT EXISTS deployed_url text NOT NULL DEFAULT '';
ALTER TABLE tasks
  ADD COLUMN IF NOT EXISTS fix_attempt int  NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS fix_source  text NOT NULL DEFAULT '';
ALTER TABLE project_events DROP CONSTRAINT IF EXISTS project_events_type_check;
ALTER TABLE project_events ADD CONSTRAINT project_events_type_check CHECK (type IN (
    'task_started','task_progress','pr_opened','pr_merged','blocked','done',
    'gate_decision','budget_warning','agent_spawned','agent_terminated','merge_conflict',
    'deploy_started','deploy_live','deploy_failed','fix_triggered'));
