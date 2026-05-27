-- Fix ON CONFLICT (agent_id, platform, account_id) in social_integrations store.
-- The store uses a three-column conflict target but only a partial unique index existed.
CREATE UNIQUE INDEX IF NOT EXISTS uq_social_integrations_agent_platform_account
    ON public.social_integrations (agent_id, platform, account_id);
