// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { request } from './api-core';

export type PermissionScope = 'auto_approved' | 'ask_first' | 'blocked';

export interface PolicyEntry {
  id: string;
  tenant_id: string;
  user_id: string;
  agent_id?: string;
  tool: string;
  scope: PermissionScope;
  created_at: string;
}

export const agentPermissions = {
  list: (agentId: string) =>
    request<{ policies: PolicyEntry[] }>(`/agents/${agentId}/permissions`)
      .then((r) => r.policies),

  upsert: (agentId: string, tool: string, scope: PermissionScope) =>
    request<{ status: string }>(`/agents/${agentId}/permissions`, {
      method: 'PUT',
      body: JSON.stringify({ tool, scope }),
    }),

  remove: (agentId: string, tool: string) =>
    request<void>(`/agents/${agentId}/permissions/${encodeURIComponent(tool)}`, {
      method: 'DELETE',
    }),
};
