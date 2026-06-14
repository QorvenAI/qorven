DROP INDEX IF EXISTS idx_tasks_project;
ALTER TABLE tasks DROP COLUMN IF EXISTS project_id;
-- The monthly_usd/lifetime_usd 0->NULL backfill is intentionally not reversed.
