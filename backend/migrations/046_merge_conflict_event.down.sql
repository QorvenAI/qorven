ALTER TABLE project_events DROP CONSTRAINT IF EXISTS project_events_type_check;
ALTER TABLE project_events ADD CONSTRAINT project_events_type_check CHECK (type IN (
    'task_started','task_progress','pr_opened','pr_merged','blocked','done',
    'gate_decision','budget_warning','agent_spawned','agent_terminated'));
