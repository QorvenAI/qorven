DROP INDEX IF EXISTS idx_social_posts_campaign;
DROP INDEX IF EXISTS idx_social_posts_dept;
DROP INDEX IF EXISTS idx_social_campaigns_dept;
DROP TABLE IF EXISTS social_campaigns;
ALTER TABLE social_integrations DROP COLUMN IF EXISTS department_id;
ALTER TABLE social_posts DROP COLUMN IF EXISTS approval_status;
ALTER TABLE social_posts DROP COLUMN IF EXISTS campaign_id;
ALTER TABLE social_posts DROP COLUMN IF EXISTS department_id;
