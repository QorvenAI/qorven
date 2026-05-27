import { request } from './api-core';

export interface RelayKeyRecord {
  id: string;
  provider: string;
  label: string;
  status: string;
  total_posts: number;
  last_used_at: string | null;
  accounts_count: number;
  created_at: string;
}

export interface PlatformMatrixEntry {
  relays: string[];
  warnings: Record<string, string> | null;
  user_has_keys: string[];
}

export const integrationsApi = {
  status: () =>
    request<{ configured: boolean; provider: string; accounts_count: number }>('/integrations/status'),

  saveRelayKey: (provider: string, apiKey: string) =>
    request<{ status: string }>('/integrations/relay/key', {
      method: 'POST',
      body: JSON.stringify({ provider, api_key: apiKey }),
    }),

  deleteRelayKey: () =>
    request<{ status: string }>('/integrations/relay/key', { method: 'DELETE' }),

  listAccounts: () =>
    request<ConnectedAccount[]>('/integrations/accounts'),

  connectPlatform: (platform: string) =>
    request<{ connect_link_url: string; expires_at: number }>(
      `/integrations/connect/${platform}`, { method: 'POST' }
    ),

  disconnectAccount: (id: string) =>
    request<void>(`/integrations/accounts/${id}`, { method: 'DELETE' }),

  getLog: (limit?: number) =>
    request<ActionLogEntry[]>(`/integrations/log${limit ? `?limit=${limit}` : ''}`),

  listPermissions: () =>
    request<IntegrationPermission[]>('/integrations/permissions'),

  setPermission: (perm: { agent_id: string; platform_id: string; action_key?: string; allowed: boolean }) =>
    request<{ status: string }>('/integrations/permissions', {
      method: 'POST',
      body: JSON.stringify(perm),
    }),

  // Social relay key management
  listRelayKeys: () =>
    request<RelayKeyRecord[]>('/social/relay-providers'),

  addRelayKey: (provider: string, label: string, apiKey: string) =>
    request<{ id: string }>('/social/relay-providers', {
      method: 'POST',
      body: JSON.stringify({ provider, label, api_key: apiKey }),
    }),

  updateRelayKey: (id: string, label: string, status: string) =>
    request<void>(`/social/relay-providers/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ label, status }),
    }),

  deleteRelayKeyById: (id: string) =>
    request<void>(`/social/relay-providers/${id}`, { method: 'DELETE' }),

  testRelayKey: (id: string) =>
    request<{ ok: boolean; error?: string }>(`/social/relay-providers/${id}/test`, { method: 'POST' }),

  getPlatformMatrix: () =>
    request<Record<string, PlatformMatrixEntry>>('/social/platforms'),

  getAccountRules: (integrationId: string, agentId: string) =>
    request<AccountRules>(`/social/integrations/${integrationId}/rules?agent_id=${agentId}`),

  setAccountRules: (integrationId: string, rules: Partial<AccountRules>) =>
    request<{ status: string }>(`/social/integrations/${integrationId}/rules`, {
      method: 'PUT',
      body: JSON.stringify(rules),
    }),

  searchCatalog: (query?: string, limit?: number) =>
    request<{ results: CatalogEntry[]; total: number }>(
      `/connectors/catalog?q=${encodeURIComponent(query || '')}&limit=${limit || 50}`
    ),

  activateCatalog: (slug: string, name: string, categories: string[]) =>
    request<{ status: string; platform_id: string }>('/connectors/catalog/activate', {
      method: 'POST',
      body: JSON.stringify({ slug, name, categories }),
    }),

  connectPlatformOAuth: (platformId: string) =>
    request<{ connect_link_url: string; expires_at: number }>(
      `/integrations/connect/${platformId}`, { method: 'POST' }
    ),

  discoverActions: (platformId: string) =>
    request<{ platform_id: string; actions_stored: number }>('/connectors/catalog/discover', {
      method: 'POST',
      body: JSON.stringify({ platform_id: platformId }),
    }),

  listConnectedAccounts: () =>
    request<ConnectedAccount[]>('/integrations/accounts'),
};

export interface ConnectedAccount {
  id: string;
  relay_provider: string;
  external_account_id: string;
  platform_id: string;
  display_name: string;
  healthy: boolean;
  connected_at: string;
}

export interface ActionLogEntry {
  id: string;
  agent_id: string;
  platform_id: string;
  action_key: string;
  backend_used: 'direct' | 'pipedream';
  success: boolean;
  error_message?: string;
  created_at: string;
}

export interface IntegrationPermission {
  id: string;
  agent_id: string;
  platform_id: string;
  action_key: string;
  allowed: boolean;
}

export interface AccountRules {
  id?: string;
  tenant_id?: string;
  agent_id: string;
  integration_id: string;
  voice_style: string;
  content_rules: string;
  knowledge_context: string;
  hashtag_sets: Record<string, string[]>;
  posting_guidelines: string;
  created_at?: string;
  updated_at?: string;
}

export interface CatalogEntry {
  id: string;
  slug: string;
  name: string;
  img_src: string;
  categories: string[];
  installed: boolean;
}
