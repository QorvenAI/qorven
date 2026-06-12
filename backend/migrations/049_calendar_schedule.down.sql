DROP TABLE IF EXISTS scheduled_runs;
ALTER TABLE cron_jobs DROP COLUMN IF EXISTS one_shot;
-- run_count is left in place (the runner UPDATE references it); dropping it would break older binaries.
