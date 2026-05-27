-- 023_multi_relay.up.sql

-- Relay provider keys — MULTIPLE keys per provider allowed (same as LLM provider_keys)
CREATE TABLE IF NOT EXISTS relay_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  provider TEXT NOT NULL,
  label TEXT DEFAULT '',
  api_key BYTEA NOT NULL,
  status TEXT DEFAULT 'active',
  metadata JSONB DEFAULT '{}',
  total_posts BIGINT DEFAULT 0,
  last_used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_relay_providers_tenant ON relay_providers (tenant_id, provider);

-- Add relay routing fields to social_integrations
ALTER TABLE social_integrations
  ADD COLUMN IF NOT EXISTS relay_provider TEXT DEFAULT 'direct',
  ADD COLUMN IF NOT EXISTS relay_provider_key_id UUID,
  ADD COLUMN IF NOT EXISTS relay_account_id TEXT,
  ADD COLUMN IF NOT EXISTS relay_metadata JSONB DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_social_integrations_relay_key ON social_integrations (relay_provider_key_id);

-- Per-agent per-account rules (voice, content policy, KB scope)
CREATE TABLE IF NOT EXISTS social_account_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  agent_id UUID NOT NULL,
  integration_id UUID NOT NULL,
  voice_style TEXT DEFAULT '',
  content_rules TEXT DEFAULT '',
  knowledge_context TEXT DEFAULT '',
  hashtag_sets JSONB DEFAULT '{}',
  posting_guidelines TEXT DEFAULT '',
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE (agent_id, integration_id)
);

CREATE INDEX IF NOT EXISTS idx_social_account_rules_agent ON social_account_rules (agent_id);
