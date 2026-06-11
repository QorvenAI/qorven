-- 040_project_artifacts: typed, versioned, gated documents for the /code Org-mode
-- pipeline (PRD/Spec/Design/ResourcePlan). Extends project_briefs with a stage
-- machine + mode (vibe|org). Each artifact must be approved before the next
-- stage unlocks. DB is the source of truth; approved docs are mirrored to the repo.

ALTER TABLE project_briefs
  ADD COLUMN IF NOT EXISTS stage TEXT NOT NULL DEFAULT 'intake',
  ADD COLUMN IF NOT EXISTS mode  TEXT NOT NULL DEFAULT 'org';

-- Widen the status CHECK to include the pipeline stages used as status too.
ALTER TABLE project_briefs DROP CONSTRAINT IF EXISTS project_briefs_status_check;
ALTER TABLE project_briefs ADD CONSTRAINT project_briefs_status_check
  CHECK (status = ANY (ARRAY['intake','proposed','approved','active','done','cancelled']));

CREATE TABLE IF NOT EXISTS project_artifacts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    brief_id    UUID NOT NULL REFERENCES project_briefs(id) ON DELETE CASCADE,
    type        TEXT NOT NULL CHECK (type = ANY (ARRAY['prd','spec','design','resource_plan'])),
    version     INT  NOT NULL DEFAULT 1,
    content_md  TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'draft'
                CHECK (status = ANY (ARRAY['draft','in_review','approved','needs_review','superseded'])),
    repo_committed BOOLEAN NOT NULL DEFAULT false,
    created_by  TEXT NOT NULL DEFAULT 'cto',
    approved_by TEXT,
    approved_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One active (non-superseded) row per (brief, type).
CREATE UNIQUE INDEX IF NOT EXISTS uq_project_artifacts_active
    ON project_artifacts (brief_id, type) WHERE status <> 'superseded';
CREATE INDEX IF NOT EXISTS idx_project_artifacts_brief ON project_artifacts (brief_id, type, version);
