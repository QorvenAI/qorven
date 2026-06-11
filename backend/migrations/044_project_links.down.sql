DROP INDEX IF EXISTS idx_rooms_project_brief;
ALTER TABLE rooms DROP COLUMN IF EXISTS project_brief_id;
ALTER TABLE project_briefs DROP COLUMN IF EXISTS github_owner;
ALTER TABLE project_briefs DROP COLUMN IF EXISTS github_repo;
