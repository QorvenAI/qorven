'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import React, { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';

// --- Available data sources ---

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

// --- Context types ---

interface DashboardDataContextValue {
  /** Keyed by data-source slug → latest payload received from the server. */
  data: Record<string, unknown>;
  /** True when the dashboard WS subscription is active. */
  connected: boolean;
}

const DashboardDataContext = createContext<DashboardDataContextValue>({
  data: {},
  connected: false,
});

// --- Provider ---

export function DashboardDataProvider({ children }: { children: React.ReactNode }) {
  const [data, setData] = useState<Record<string, unknown>>({});
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mountedRef = useRef(true);

  const getWsUrl = useCallback((): string => {
    if (typeof window === 'undefined') return '';
    const envUrl = process.env.NEXT_PUBLIC_API_URL;
    if (envUrl) {
      return envUrl.replace(/^http/, 'ws').replace(/\/$/, '') + '/ws/realtime';
    }
    const { protocol, host } = window.location;
    const wsProtocol = protocol === 'https:' ? 'wss:' : 'ws:';
    return `${wsProtocol}//${host}/ws/realtime`;
  }, []);

  const connect = useCallback(() => {
    if (!mountedRef.current) return;
    if (wsRef.current && wsRef.current.readyState <= WebSocket.OPEN) return;

    const url = getWsUrl();
    if (!url) return;

    const token =
      typeof window !== 'undefined'
        ? (localStorage.getItem('qorven_token') ?? process.env.NEXT_PUBLIC_API_TOKEN ?? '')
        : '';

    const fullUrl = token ? `${url}${url.includes('?') ? '&' : '?'}token=${encodeURIComponent(token)}` : url;

    let ws: WebSocket;
    try {
      ws = new WebSocket(fullUrl);
    } catch {
      return;
    }

    wsRef.current = ws;

    ws.onopen = () => {
      if (!mountedRef.current) { ws.close(); return; }
      setConnected(true);
    };

    ws.onmessage = (event) => {
      if (!mountedRef.current) return;
      try {
        const msg = JSON.parse(event.data as string);
        if (msg?.type === 'dashboard_data' && typeof msg.source === 'string') {
          setData((prev) => ({ ...prev, [msg.source as string]: msg.payload }));
        }
      } catch {
        // ignore malformed frames
      }
    };

    ws.onerror = () => {
      setConnected(false);
    };

    ws.onclose = () => {
      if (!mountedRef.current) return;
      setConnected(false);
      wsRef.current = null;
      // Reconnect after 5 s
      timerRef.current = setTimeout(() => { connect(); }, 5000);
    };
  }, [getWsUrl]);

  useEffect(() => {
    mountedRef.current = true;
    connect();
    return () => {
      mountedRef.current = false;
      if (timerRef.current) clearTimeout(timerRef.current);
      if (wsRef.current) {
        wsRef.current.onclose = null;
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [connect]);

  return (
    <DashboardDataContext.Provider value={{ data, connected }}>
      {children}
    </DashboardDataContext.Provider>
  );
}

// --- Hook ---

export function useDashboardData(): DashboardDataContextValue {
  return useContext(DashboardDataContext);
}
