// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { apiBase as getApiBase, authBase } from '@/lib/api-url';
import type { AgentSummary } from './setup-config';

function tokenHeader(): Record<string, string> {
  const t = typeof window === 'undefined' ? '' : (localStorage.getItem('qorven_token') ?? '');
  return t ? { Authorization: `Bearer ${t}` } : {};
}

export async function api<T = unknown>(path: string, opts: RequestInit = {}): Promise<T> {
  const base = path.startsWith('/auth/') ? authBase() : getApiBase();
  const suffix = path.startsWith('/auth/') ? path.slice(5) : path.startsWith('/v1/') ? path.slice(3) : path;
  const res = await fetch(`${base}${suffix}`, {
    ...opts,
    headers: {
      'Content-Type': 'application/json',
      ...tokenHeader(),
      ...(opts.headers ?? {}),
    },
    credentials: 'include',
  });
  const text = await res.text();
  const body = text ? (() => { try { return JSON.parse(text); } catch { return text; } })() : null;
  if (!res.ok) {
    const msg = (body && typeof body === 'object' && 'error' in body)
      ? String((body as { error: unknown }).error)
      : (typeof body === 'string' ? body : `HTTP ${res.status}`);
    throw new Error(msg);
  }
  return body as T;
}

export async function listAgents(): Promise<AgentSummary[]> {
  const raw = await api<AgentSummary[] | { agents: AgentSummary[] }>('/v1/agents').catch(() => [] as AgentSummary[]);
  if (Array.isArray(raw)) return raw;
  if (raw && typeof raw === 'object' && Array.isArray((raw as any).agents)) return (raw as any).agents;
  return [];
}

export { tokenHeader };
