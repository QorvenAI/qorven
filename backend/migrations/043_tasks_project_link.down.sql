DROP INDEX IF EXISTS idx_tasks_project_brief;
ALTER TABLE tasks DROP COLUMN IF EXISTS project_brief_id;
