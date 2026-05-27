-- Rollback migration 019: Knowledge Governance System

DROP TABLE IF EXISTS public.agent_onboarding;
DROP TABLE IF EXISTS public.agent_clearances;
DROP TABLE IF EXISTS public.knowledge_access_log;
DROP TABLE IF EXISTS public.pii_access_log;
DROP TABLE IF EXISTS public.pii_vault;
DROP TABLE IF EXISTS public.knowledge_grants;
ALTER TABLE public.memories DROP COLUMN IF EXISTS classification;

DELETE FROM schema_migrations WHERE version = 19;
