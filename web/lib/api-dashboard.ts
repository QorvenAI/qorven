// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { request } from './api-core';

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

export const dashboardApi = {
  tiles: () => request<PinnedTile[]>('/dashboard/tiles'),
  pin: (t: Omit<PinnedTile, 'id' | 'data'>) =>
    request<PinnedTile>('/dashboard/tiles', { method: 'POST', body: JSON.stringify(t) }),
  unpin: (id: string) =>
    request<void>(`/dashboard/tiles/${id}`, { method: 'DELETE' }),
};
