-- 002_app_features rollback

ALTER TABLE apps
  DROP COLUMN IF EXISTS icon,
  DROP COLUMN IF EXISTS pinned_rail,
  DROP COLUMN IF EXISTS rail_order,
  DROP COLUMN IF EXISTS pinned_topbar,
  DROP COLUMN IF EXISTS topbar_order,
  DROP COLUMN IF EXISTS settings_schema;
