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
