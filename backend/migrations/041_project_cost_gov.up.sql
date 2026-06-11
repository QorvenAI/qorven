-- 041_project_cost_gov: circuit-breaker pause state for /code projects. When the
-- project hits its µUSD cap, paused=true and the swarm suspends at safe points.
ALTER TABLE project_briefs
  ADD COLUMN IF NOT EXISTS paused boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS pause_reason text NOT NULL DEFAULT '';
