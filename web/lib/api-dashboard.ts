// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { request } from './api-core';
import type { WidgetConfig } from '@/components/dashboard/widget-registry';

export interface PinnedTile {
  id: string;
  source_slug: string;
  tool_name: string;
  tool_args: Record<string, unknown>;
  widget_type: 'stat-card' | 'data-table' | 'feed' | 'list' | 'chart';
  label: string;
  position: number;
  refresh_interval_sec: number;
  data?: Record<string, unknown>;
}

export interface DashboardStats {
  cost_this_month_usd: number;
  calls_this_month: number;
  tokens_in: number;
  tokens_out: number;
}

export const dashboardApi = {
  tiles: () => request<PinnedTile[]>('/dashboard/tiles'),
  pin: (t: Omit<PinnedTile, 'id' | 'data'>) =>
    request<PinnedTile>('/dashboard/tiles', { method: 'POST', body: JSON.stringify(t) }),
  unpin: (id: string) =>
    request<void>(`/dashboard/tiles/${id}`, { method: 'DELETE' }),
  stats: () => request<DashboardStats>('/dashboard/stats'),
};

export const dashboardLayout = {
  get: () =>
    request<{
      id: string;
      name: string;
      layout: unknown[];
      widgets: Record<string, WidgetConfig>;
      is_default: boolean;
    }>('/dashboard/layout'),

  save: (name: string, layout: unknown[], widgets: Record<string, WidgetConfig>) =>
    request<{ id: string }>('/dashboard/layout', {
      method: 'PUT',
      body: JSON.stringify({ name, layout, widgets }),
    }),

  list: () =>
    request<Array<{ id: string; name: string; is_default: boolean; updated_at: string }>>(
      '/dashboard/layouts',
    ),

  create: (name: string) =>
    request<{ id: string; name: string }>('/dashboard/layouts', {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),

  setDefault: (id: string) =>
    request<void>(`/dashboard/layouts/${id}/default`, { method: 'PUT' }),

  generateWidget: (prompt: string) =>
    request<WidgetConfig>('/dashboard/generate-widget', {
      method: 'POST',
      body: JSON.stringify({ prompt }),
    }),
};
