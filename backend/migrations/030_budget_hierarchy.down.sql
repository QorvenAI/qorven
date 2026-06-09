DROP INDEX IF EXISTS idx_spend_raw_task;
DROP INDEX IF EXISTS idx_spend_raw_project;
DROP INDEX IF EXISTS idx_spend_raw_department;
ALTER TABLE gateway_spend_raw DROP COLUMN IF EXISTS task_id;
ALTER TABLE gateway_spend_raw DROP COLUMN IF EXISTS project_id;
ALTER TABLE gateway_spend_raw DROP COLUMN IF EXISTS department_id;

DROP INDEX IF EXISTS idx_gateway_budgets_scope;
ALTER TABLE gateway_budgets DROP COLUMN IF EXISTS parent_scope_id;
ALTER TABLE gateway_budgets DROP COLUMN IF EXISTS parent_scope;
ALTER TABLE gateway_budgets DROP COLUMN IF EXISTS allocation_mode;
ALTER TABLE gateway_budgets DROP COLUMN IF EXISTS department_id;
ALTER TABLE gateway_budgets DROP COLUMN IF EXISTS scope;

ALTER TABLE agents DROP COLUMN IF EXISTS department_id;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS departments;
