ALTER TABLE social_integrations
  DROP COLUMN IF EXISTS nickname,
  DROP COLUMN IF EXISTS avatar_url,
  DROP COLUMN IF EXISTS post_hours,
  DROP COLUMN IF EXISTS post_days,
  DROP COLUMN IF EXISTS group_name,
  DROP COLUMN IF EXISTS paused;
