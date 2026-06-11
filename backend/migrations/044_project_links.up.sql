-- 044_project_links: link a room to a project brief (project-scoped Hub) and
-- carry the connected GitHub repo on the brief for analytics joins.
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS project_brief_id uuid;
CREATE INDEX IF NOT EXISTS idx_rooms_project_brief ON rooms (project_brief_id);
ALTER TABLE project_briefs
  ADD COLUMN IF NOT EXISTS github_owner text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS github_repo  text NOT NULL DEFAULT '';
