-- 050_drive_scopes: per-file access scope for Drive sharing.
-- scope: 'private' (owner agent only) | 'company' (whole tenant) |
--        'department' (scope_id = department) | 'custom' (drive_permissions grants).
ALTER TABLE drive_files
  ADD COLUMN IF NOT EXISTS scope    text NOT NULL DEFAULT 'private',
  ADD COLUMN IF NOT EXISTS scope_id uuid;

-- Backfill: existing ownerless (agent_id IS NULL) files become company-shared.
UPDATE drive_files SET scope = 'company' WHERE agent_id IS NULL AND scope = 'private';

CREATE INDEX IF NOT EXISTS idx_drive_files_scope ON drive_files (tenant_id, scope, scope_id);
CREATE INDEX IF NOT EXISTS idx_drive_files_agent ON drive_files (tenant_id, agent_id);
