-- 046_merge_conflict_event: allow the 'merge_conflict' project_events type so the
-- merge queue can surface an unmergeable PR in the project Hub timeline.
ALTER TABLE project_events DROP CONSTRAINT IF EXISTS project_events_type_check;
ALTER TABLE project_events ADD CONSTRAINT project_events_type_check CHECK (type IN (
    'task_started','task_progress','pr_opened','pr_merged','blocked','done',
    'gate_decision','budget_warning','agent_spawned','agent_terminated','merge_conflict'));
