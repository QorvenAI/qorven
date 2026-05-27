-- Migration 022 down: Remove integration relay tables

DROP TABLE IF EXISTS public.integration_permissions;
DROP TABLE IF EXISTS public.integration_action_log;
DROP TABLE IF EXISTS public.connected_accounts;

ALTER TABLE public.connector_actions
    DROP COLUMN IF EXISTS execution_backend,
    DROP COLUMN IF EXISTS pipedream_action_id;

DELETE FROM schema_migrations WHERE version = 22;
