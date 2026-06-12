-- 056_social_cmo: social is the Marketing department's function — department
-- ownership, campaigns, and post approval state.
ALTER TABLE social_posts
  ADD COLUMN IF NOT EXISTS department_id   uuid,
  ADD COLUMN IF NOT EXISTS campaign_id     uuid,
  ADD COLUMN IF NOT EXISTS approval_status text NOT NULL DEFAULT '';
ALTER TABLE social_integrations
  ADD COLUMN IF NOT EXISTS department_id uuid;

CREATE TABLE IF NOT EXISTS social_campaigns (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL,
    department_id       uuid,
    created_by_agent_id uuid,
    title               text NOT NULL,
    brief               text NOT NULL DEFAULT '',
    target_platforms    text[] NOT NULL DEFAULT '{}',
    window_start        timestamptz,
    window_end          timestamptz,
    status              text NOT NULL DEFAULT 'draft',
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_social_campaigns_dept ON social_campaigns (tenant_id, department_id, status);
-- social_posts has tenant_id — use it in the dept index for efficient tenant-scoped queries
CREATE INDEX IF NOT EXISTS idx_social_posts_dept ON social_posts (tenant_id, department_id, status);
CREATE INDEX IF NOT EXISTS idx_social_posts_campaign ON social_posts (campaign_id);
