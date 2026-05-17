// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { request } from './api-core';

export interface QorvenApp {
  id: string;
  tenant_id: string;
  slug: string;
  display_name: string;
  description: string;
  version: string;
  author: string;
  icon_url: string;
  install_path: string;
  enabled: boolean;
  scope: 'workspace' | 'agent' | 'team';
  owner_agent_id?: string;
  owner_team_id?: string;
  config: Record<string, unknown>;
  installed_at: string;
  updated_at: string;
}

export interface AppPageDef {
  id: string;
  label: string;
  icon: string;
  path: string;
}

export interface AppTabDef {
  id: string;
  label: string;
  icon: string;
  order: number;
}

export interface AppFrontendEntry {
  app_id: string;
  slug: string;
  display_name: string;
  bundle_url: string;
  pages: AppPageDef[];
  agent_tabs: AppTabDef[];
  setting_tabs: AppTabDef[];
}

export interface AppsListResponse {
  apps: QorvenApp[];
  frontend_manifests: AppFrontendEntry[];
}

export interface ToolResult {
  content: string;       // ForLLM — sent to language model
  user_content: string;  // ForUser — shown in UI
  is_error: boolean;
  widget?: {
    type: string;
    data: Record<string, unknown>;
  } | null;
  widgets?: Array<{
    type: string;
    data: Record<string, unknown>;
  }>;
  media?: Array<{
    path: string;
    mime_type: string;
  }>;
}

export const listApps = () =>
  request<AppsListResponse>('/apps');

export const getApp = (id: string) =>
  request<QorvenApp>(`/apps/${id}`);

export const installApp = (path: string) =>
  request<QorvenApp>('/apps', {
    method: 'POST',
    body: JSON.stringify({ path }),
  });

export const patchApp = (
  id: string,
  body: Partial<{ enabled: boolean; config: Record<string, unknown> }>
) =>
  request<QorvenApp>(`/apps/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(body),
  });

export const uninstallApp = (id: string, dropTables = false) =>
  request<void>(`/apps/${id}?drop_tables=${dropTables}`, { method: 'DELETE' });

export const reloadApp = (id: string) =>
  request<QorvenApp>(`/apps/${id}/reload`, { method: 'POST' });

export const runTool = (
  slug: string,
  toolName: string,
  args: Record<string, unknown> = {}
) =>
  request<ToolResult>(`/apps/${slug}/tools/${toolName}`, {
    method: 'POST',
    body: JSON.stringify({ args }),
  });
