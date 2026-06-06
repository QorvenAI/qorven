'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
//
// DashboardDataProvider — reads dashboard widget data from the shared Zustand
// store instead of opening its own WebSocket. The existing websocket.ts
// handler already processes 'dashboard_data' events from the backend and
// writes them to store.dashboardData. This avoids duplicate WS connections.

import React, { createContext, useContext } from 'react';
import { useStore } from '@/store';

// ── Available data sources ──────────────────────────────────────────────────

export const AVAILABLE_DATA_SOURCES: Record<string, string> = {
  agent_status_live:       'Live agent status counts (idle / thinking / running)',
  spend_total_today:       'Total spend today in USD',
  agent_runs_per_hour:     'Agent run counts bucketed by hour',
  session_count_today:     'Number of sessions started today',
  pending_approvals:       'Pending human-approval requests',
  spend_by_provider_30d:   'Spend by AI provider over the last 30 days',
  channel_message_volume:  'Inbound message volume by channel',
  task_completion_rate:    'Task completion rate over time',
  error_rate_by_agent:     'Error rate breakdown by agent',
};

// ── Context ─────────────────────────────────────────────────────────────────

interface DashboardDataContextValue {
  data: Record<string, unknown>;
  connected: boolean;
}

const DashboardDataContext = createContext<DashboardDataContextValue>({
  data: {},
  connected: false,
});

// ── Provider ─────────────────────────────────────────────────────────────────

export function DashboardDataProvider({ children }: { children: React.ReactNode }) {
  // Read directly from the shared store — no extra WS connection needed.
  const dashboardData = useStore(s => s.dashboardData);
  const wsConnected   = useStore(s => s.wsConnected);

  return (
    <DashboardDataContext.Provider value={{ data: dashboardData, connected: wsConnected }}>
      {children}
    </DashboardDataContext.Provider>
  );
}

// ── Hook ─────────────────────────────────────────────────────────────────────

export function useDashboardData(): DashboardDataContextValue {
  return useContext(DashboardDataContext);
}
