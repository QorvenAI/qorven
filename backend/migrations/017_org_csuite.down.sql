-- Rollback migration 017
DROP TABLE IF EXISTS public.org_delegations;
DROP TABLE IF EXISTS public.org_daily_spend;
DROP TABLE IF EXISTS public.org_roster;

ALTER TABLE public.agents
    DROP COLUMN IF EXISTS terminated_at,
    DROP COLUMN IF EXISTS hired_at,
    DROP COLUMN IF EXISTS dept_head_id,
    DROP COLUMN IF EXISTS customer_facing,
    DROP COLUMN IF EXISTS kb_grants,
    DROP COLUMN IF EXISTS org_role,
    DROP COLUMN IF EXISTS org_level;

DELETE FROM schema_migrations WHERE version = 17;
