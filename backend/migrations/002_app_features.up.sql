-- 002_app_features: app icons, sidebar/topbar pinning, settings schema

ALTER TABLE apps
  ADD COLUMN IF NOT EXISTS icon             text    NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS pinned_rail      boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS rail_order       int     NOT NULL DEFAULT 999,
  ADD COLUMN IF NOT EXISTS pinned_topbar    boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS topbar_order     int     NOT NULL DEFAULT 999,
  ADD COLUMN IF NOT EXISTS settings_schema  jsonb   NOT NULL DEFAULT '[]';
