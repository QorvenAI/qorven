-- 039_app_external: per-app "published externally" flag for external-facing apps.
-- When true (and a tunnel is up), the app's public-flagged pages/tools are
-- reachable on the restricted public mux at /a/{slug}. Default false (default-deny).
ALTER TABLE apps
  ADD COLUMN IF NOT EXISTS external_enabled boolean NOT NULL DEFAULT false;
