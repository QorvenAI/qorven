-- Link a task to a project so agent spend on the task can be attributed to and
-- capped by that project (the project-level budget cap).
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS project_id uuid;
CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id);

-- Zero-cap semantic flip (paired with the enforcer change in a later task): a
-- stored monthly_usd of 0 will now mean "block all spend". Historically a 0 was
-- treated as "unlimited", so preserve that intent for any existing 0 rows by
-- nulling them BEFORE the enforcer starts treating 0 as a hard block.
UPDATE gateway_budgets SET monthly_usd = NULL WHERE monthly_usd = 0;
UPDATE gateway_budgets SET lifetime_usd = NULL WHERE lifetime_usd = 0;
