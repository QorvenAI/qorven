-- 043_tasks_project_link: scope tasks directly to a project brief (was only
-- transitive via agents.project_brief_id). Backfills from the agent link.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS project_brief_id uuid;
CREATE INDEX IF NOT EXISTS idx_tasks_project_brief ON tasks (project_brief_id);
UPDATE tasks t SET project_brief_id = a.project_brief_id
  FROM agents a
  WHERE t.assigned_to = a.id AND a.project_brief_id IS NOT NULL AND t.project_brief_id IS NULL;
