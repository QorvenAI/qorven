'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useCallback } from 'react';
import { useStore } from '@/store';
import { BASE, getToken } from '@/lib/api-core';

// ── SSE event types (mirror of INTERFACE-CONTRACT.md §3) ─────────────────────

export interface DaemonAgent {
  id: string;
  name: string;
  provider: string;
  model: string;
  capabilities: string[];
  status: 'idle' | 'working' | 'error';
  current_task_id?: string;
}

export interface DaemonTask {
  id: string;
  title: string;
  description: string;
  owner: string;
  priority: 'high' | 'normal' | 'low';
  status: 'queued' | 'in_progress' | 'done' | 'failed' | 'cancelled';
  depends_on: string[];
  created_by: string;
  files_changed?: string[];
  summary?: string;
  error?: string;
  percent?: number;
}

export interface DaemonPlan {
  id: string;
  title: string;
  description: string;
  proposed_by: string;
  status: 'pending' | 'approved' | 'rejected' | 'executing' | 'done';
  tasks: { id: string; title: string; owner: string; priority: string; estimated_minutes?: number }[];
}

export type AgentEvent =
  | { type: 'agent_snapshot'; data: DaemonAgent[] }
  | { type: 'agent_registered'; data: DaemonAgent }
  | { type: 'agent_status'; data: { id: string; status: DaemonAgent['status']; current_task_id?: string } }
  | { type: 'agent_unregistered'; data: { id: string } }
  | { type: 'task_created'; data: DaemonTask }
  | { type: 'task_assigned'; data: { task_id: string; agent_id: string } }
  | { type: 'task_progress'; data: { task_id: string; agent_id: string; message: string; percent?: number } }
  | { type: 'task_file'; data: { task_id: string; agent_id: string; path: string; action: string } }
  | { type: 'task_done'; data: { task_id: string; agent_id: string; summary: string; files_changed: string[]; duration_ms: number } }
  | { type: 'task_failed'; data: { task_id: string; agent_id: string; error: string; retryable: boolean } }
  | { type: 'plan_proposed'; data: DaemonPlan & { requires_approval: boolean } }
  | { type: 'plan_approved'; data: { plan_id: string; approved_by: string } }
  | { type: 'plan_rejected'; data: { plan_id: string; rejected_by: string; reason: string } };

// ── SSE Hook ─────────────────────────────────────────────────────────────────

export function useAgentsStream() {
  const dispatch = useStore((s) => s.dispatchDaemonEvent);
  const setConnected = useStore((s) => s.setDaemonConnected);
  // Subscribe to WS reconnect signal so daemon state re-hydrates after reconnect
  const catchUpCounter = useStore((s) => s.catchUpCounter);
  const retryRef = useRef(0);
  const esRef = useRef<EventSource | null>(null);
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Track whether the hook is mounted so catchup/connect don't fire after unmount
  const mountedRef = useRef(true);

  // On reconnect: fetch current state to fill any gap missed while disconnected.
  const catchup = useCallback(async () => {
    const token = getToken();
    if (!token || !mountedRef.current) return;
    const headers = { Authorization: `Bearer ${token}` };
    try {
      const [agentsRes, tasksRes, plansRes] = await Promise.all([
        fetch(`${BASE}/daemon/agents`, { headers }),
        fetch(`${BASE}/daemon/tasks`, { headers }),
        fetch(`${BASE}/daemon/plans`, { headers }),
      ]);
      if (!mountedRef.current) return;
      if (agentsRes.ok) {
        const agents: DaemonAgent[] = await agentsRes.json();
        dispatch({ type: 'agent_snapshot', data: agents });
      }
      if (tasksRes.ok) {
        const tasks: DaemonTask[] = await tasksRes.json();
        dispatch({ type: 'task_snapshot', data: tasks });
      }
      if (plansRes.ok) {
        const plans: DaemonPlan[] = await plansRes.json();
        for (const p of plans) dispatch({ type: 'plan_proposed', data: { ...p, requires_approval: p.status === 'pending' } });
      }
    } catch { /* network error during catchup — SSE will deliver deltas */ }
  }, [dispatch]);

  const connect = useCallback(() => {
    if (!mountedRef.current) return;
    const token = getToken();
    if (!token) return;

    const url = `${BASE}/daemon/stream?token=${encodeURIComponent(token)}`;
    const es = new EventSource(url);
    esRef.current = es;

    es.onopen = () => {
      retryRef.current = 0;
      if (mountedRef.current) setConnected(true);
    };

    es.onmessage = (e) => {
      if (!mountedRef.current) return;
      try {
        const event: AgentEvent = JSON.parse(e.data);
        dispatch(event);
      } catch { /* ignore malformed frames */ }
    };

    es.onerror = () => {
      es.close();
      if (!mountedRef.current) return;
      setConnected(false);
      const delay = Math.min(1000 * 2 ** retryRef.current, 30_000);
      retryRef.current++;
      retryTimerRef.current = setTimeout(() => {
        if (!mountedRef.current) return;
        catchup();
        connect();
      }, delay);
    };
  }, [dispatch, setConnected, catchup]);

  useEffect(() => {
    mountedRef.current = true;
    connect();
    return () => {
      mountedRef.current = false;
      esRef.current?.close();
      if (retryTimerRef.current) clearTimeout(retryTimerRef.current);
      setConnected(false);
    };
  }, [connect, setConnected]);

  // Re-hydrate daemon state when the WS reconnects. The global websocket.ts
  // calls store.triggerCatchUp() on reconnect; we respond here so phantom
  // agents/tasks from the previous connection are replaced by fresh snapshots.
  const catchUpRef = useRef(catchUpCounter);
  useEffect(() => {
    if (catchUpCounter === catchUpRef.current) return;
    catchUpRef.current = catchUpCounter;
    catchup();
  }, [catchUpCounter, catchup]);
}
