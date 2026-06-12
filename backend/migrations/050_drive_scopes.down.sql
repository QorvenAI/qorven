DROP INDEX IF EXISTS idx_drive_files_scope;
DROP INDEX IF EXISTS idx_drive_files_agent;
ALTER TABLE drive_files DROP COLUMN IF EXISTS scope;
ALTER TABLE drive_files DROP COLUMN IF EXISTS scope_id;
