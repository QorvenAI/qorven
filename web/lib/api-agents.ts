// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { request, listRequest, BASE, STREAM_BASE, AUTH_BASE, getToken } from './api-core';
import type { Soul, Session, Channel, Skill, CronJob, Notification, Message } from '@/types';

export interface Contact {
  id: string;
  external_id: string;
  channel: string;
  display_name: string;
  company: string;
  notes: string;
  pipeline_stage: string;
  tags: string[];
  email?: string;
  first_seen: string;
  last_seen: string;
  message_count: number;
}

export interface ContactDetail {
  contact: Contact;
  sessions: Array<{
    id: string;
    agent_id: string;
    summary: string;
    updated_at: string;
  }>;
}

export interface ContactAgentPrefs {
  contact_id: string;
  agent_id: string;
  routing_mode: 'inherit' | 'auto' | 'draft' | 'skip';
  trust_level: 'unknown' | 'known' | 'trusted' | 'blocked';
  agent_notes: string;
  updated_at?: string;
}

export const contacts = {
  list: (params?: { stage?: string; search?: string }) => {
    const q = new URLSearchParams();
    if (params?.stage) q.set('stage', params.stage);
    if (params?.search) q.set('search', params.search);
    const qs = q.toString();
    return request<Contact[]>(`/contacts${qs ? '?' + qs : ''}`);
  },
  get: (id: string) => request<ContactDetail>(`/contacts/${id}`),
  create: (body: { external_id: string; channel?: string; display_name?: string; company?: string; notes?: string }) =>
    request<{ id: string }>('/contacts', { method: 'POST', body: JSON.stringify(body) }),
  patch: (id: string, body: { display_name?: string; company?: string; notes?: string; pipeline_stage?: string; tags?: string[] }) =>
    request<void>(`/contacts/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  getPrefs: (contactId: string, agentId: string) =>
    request<ContactAgentPrefs>(`/contacts/${contactId}/prefs/${agentId}`),
  putPrefs: (contactId: string, agentId: string, body: Pick<ContactAgentPrefs, 'routing_mode' | 'trust_level' | 'agent_notes'>) =>
    request<void>(`/contacts/${contactId}/prefs/${agentId}`, { method: 'PUT', body: JSON.stringify(body) }),
};

export interface PendingSender {
  id: string;
  sender_jid: string;
  display_name: string;
  otp_code: string;
  attempts: number;
  locked_until?: string;
  created_at: string;
}

export interface DreamingConfig {
  enabled: boolean;
  interval_hours: number;
  mode: string;
  last_dream_at?: string;
  next_dream_at?: string;
}

export const agents = {
  list: () => listRequest<Soul>('/agents'),
  get: (id: string) => request<Soul>(`/agents/${id}`),
  chief: () => request<Soul>('/agents/chief'),
  create: (body: Partial<Soul>) => request<Soul>('/agents', { method: 'POST', body: JSON.stringify(body) }),
  update: (id: string, body: Partial<Soul>) => request<Soul>(`/agents/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  delete: (id: string) => request<void>(`/agents/${id}`, { method: 'DELETE' }),
  skills: (id: string) => listRequest<Skill>(`/agents/${id}/skills`),
  orgChart: () => request<unknown>('/org-chart'),
  budgets: () => listRequest<unknown>('/budgets'),
  metrics: (id: string) => request<unknown>(`/metrics/${id}`),
  getDreaming: (id: string) => request<DreamingConfig>(`/agents/${id}/dreaming`),
  updateDreaming: (id: string, body: Omit<DreamingConfig, 'last_dream_at' | 'next_dream_at'>) =>
    request<{ status: string }>(`/agents/${id}/dreaming`, { method: 'PUT', body: JSON.stringify(body) }),
  triggerDream: (id: string) =>
    request<{ status: string; agent_id: string }>(`/agents/${id}/dreaming/trigger`, { method: 'POST' }),
  setBudget: (id: string, budgetCents: number) =>
    request<{ status: string }>(`/agents/${id}/budget`, { method: 'PUT', body: JSON.stringify({ budget_cents: budgetCents }) }),
  runtimePause: (id: string) =>
    request<{ status: string }>(`/agents/${id}/runtime/pause`, { method: 'POST' }),
  runtimeResume: (id: string) =>
    request<{ status: string }>(`/agents/${id}/runtime/resume`, { method: 'POST' }),
  runtimeWakeup: (id: string) =>
    request<{ status: string }>(`/agents/${id}/runtime/wakeup`, { method: 'POST' }),
  runtimeOverride: (id: string, message: string) =>
    request<{ status: string }>(`/agents/${id}/runtime/override`, { method: 'POST', body: JSON.stringify({ message }) }),
  generateSoul: (name: string, role: string, description: string) =>
    request<{ soul: string }>('/agents/generate-soul', { method: 'POST', body: JSON.stringify({ name, role, description }) }),
};

export const sessions = {
  list: () => listRequest<Session>('/sessions'),
  get: (id: string) => request<Session>(`/sessions/${id}`),
  messages: (id: string, limit?: number, offset?: number) => {
    const params = new URLSearchParams();
    if (limit !== undefined) params.set('limit', String(limit));
    if (offset !== undefined) params.set('offset', String(offset));
    const qs = params.toString();
    return request<{ messages: Message[]; total: number }>(`/sessions/${id}/messages${qs ? `?${qs}` : ''}`);
  },
  create: (body: { agent_id: string; channel?: string }) => request<Session>('/sessions', { method: 'POST', body: JSON.stringify(body) }),
  delete: (id: string) => request<void>(`/sessions/${id}`, { method: 'DELETE' }),
  search: (q: string) =>
    request<{ sessions: Session[]; count: number; query: string }>(
      `/sessions/search?q=${encodeURIComponent(q)}`,
    ),
  unified: (agentId: string, limit = 100) =>
    request<{
      agent_id: string;
      messages: { session_id: string; channel: string; role: string; content: string; sender_name?: string; timestamp: number }[];
      total: number;
      channels: string[];
    }>(`/sessions/unified?agent_id=${encodeURIComponent(agentId)}&limit=${limit}`),
};

export const chat = {
  send: (body: { session_id: string; agent_id: string; message: string; stream?: boolean; depth?: string }, signal?: AbortSignal) =>
    fetch(`${body.stream ? STREAM_BASE : BASE}/chat/completions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${getToken()}` },
      body: JSON.stringify(body),
      signal,
    }),
};

export const channels = {
  list: () => listRequest<Channel>('/channels'),
  create: (body: Partial<Channel>) => request<Channel>('/channels', { method: 'POST', body: JSON.stringify(body) }),
  update: (id: string, body: { name?: string; config: Record<string, unknown> }) =>
    request<void>(`/channels/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  delete: (id: string) => request<void>(`/channels/${id}`, { method: 'DELETE' }),
  start: (id: string) => request<void>(`/channels/${id}/start`, { method: 'POST' }),
  stop: (id: string) => request<void>(`/channels/${id}/stop`, { method: 'POST' }),
  test: (id: string) => request<{ ok: boolean; error?: string; message?: string }>(`/channels/${id}/test`, { method: 'POST' }),
  whatsapp: {
    qrStreamUrl: (id: string) => `/api/v1/channels/${id}/whatsapp/qr`,
    listPending: (id: string) => request<PendingSender[]>(`/channels/${id}/whatsapp/pending`),
    approve: (channelId: string, pendingId: string) =>
      request<void>(`/channels/${channelId}/whatsapp/pending/${pendingId}/approve`, { method: 'POST' }),
    deny: (channelId: string, pendingId: string) =>
      request<void>(`/channels/${channelId}/whatsapp/pending/${pendingId}/deny`, { method: 'POST' }),
  },
};

export const cron = {
  list: () => listRequest<CronJob>('/cron-jobs'),
  create: (body: { agent_id: string; expression: string; task: string }) =>
    request<CronJob>('/cron-jobs', { method: 'POST', body: JSON.stringify(body) }),
  pause: (id: string) => request<void>(`/cron-jobs/${id}/pause`, { method: 'POST' }),
  resume: (id: string) => request<void>(`/cron-jobs/${id}/resume`, { method: 'POST' }),
  delete: (id: string) => request<void>(`/cron-jobs/${id}`, { method: 'DELETE' }),
};

export const notifications = {
  list: () => listRequest<Notification>('/notifications'),
  markRead: (id: string) => request<void>(`/notifications/${id}/read`, { method: 'POST' }),
  markAllRead: () => request<void>('/notifications/read-all', { method: 'POST' }),
};

export const tools = {
  builtin: () => listRequest<unknown>('/tools/builtin'),
  custom: () => listRequest<unknown>('/tools/custom'),
};

export const mcp = {
  servers: () => listRequest<unknown>('/mcp/servers'),
  tools: () => listRequest<unknown>('/mcp/tools'),
  createServer: (name: string, command: string) =>
    request<{ id: string; name: string }>('/mcp/servers', { method: 'POST', body: JSON.stringify({ name, command }) }),
  deleteServer: (id: string) =>
    request<void>(`/mcp/servers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
};

export interface TerminalSession { id: string; name: string; created_at: string }
export const terminal = {
  list: () => listRequest<TerminalSession>('/terminal/sessions'),
  create: (name?: string) =>
    request<TerminalSession>('/terminal/sessions', { method: 'POST', body: JSON.stringify({ name: name ?? '' }) }),
  delete: (id: string) =>
    request<void>(`/terminal/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  /** Returns the WebSocket URL for the given session ID. */
  wsUrl: (id: string): string => {
    const base = typeof window !== 'undefined' ? window.location.origin : '';
    const token = typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') ?? '') : '';
    const proto = typeof window !== 'undefined' && window.location.protocol === 'https:' ? 'wss' : 'ws';
    return `${proto}://${base.replace(/^https?:\/\//, '')}/api/v1/terminal/sessions/${encodeURIComponent(id)}/ws?token=${encodeURIComponent(token)}`;
  },
};

export interface QorosStatus {
  active: boolean;
  agent_id: string;
  [extra: string]: unknown;
}

export const qoros = {
  start: (agentId: string) =>
    request<{ status: 'started'; agent_id: string }>(`/agents/${encodeURIComponent(agentId)}/qoros/start`, { method: 'POST' }),
  stop: (agentId: string) =>
    request<{ status: 'stopped'; agent_id: string }>(`/agents/${encodeURIComponent(agentId)}/qoros/stop`, { method: 'POST' }),
  status: (agentId: string) =>
    request<QorosStatus>(`/agents/${encodeURIComponent(agentId)}/qoros/status`),
};

export interface HeartbeatConfig {
  agent_id?: string;
  enabled?: boolean;
  interval_hours?: number;
  mode?: string;
  probes?: string[];
  policy?: Record<string, unknown>;
  [extra: string]: unknown;
}

export const heartbeats = {
  get: (agentId: string) =>
    request<HeartbeatConfig>(`/agents/${encodeURIComponent(agentId)}/heartbeat`),
  save: (agentId: string, body: HeartbeatConfig) =>
    request<{ status: 'saved' }>(`/agents/${encodeURIComponent(agentId)}/heartbeat`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
};

export interface MetricsResponse {
  metrics: Record<string, { avg?: number; count?: number; last?: string }>;
}

export const metricsApi = {
  get: (agentId: string) =>
    request<MetricsResponse>(`/metrics/${encodeURIComponent(agentId)}`),
};

export interface MemoryRecord {
  id: string;
  content: string;
  scope?: string;
  agent_id?: string;
  source?: string;
  created_at?: string;
  relevance?: number;
  [extra: string]: unknown;
}

export const memoryApi = {
  search: (agentId: string, q: string, limit = 20) =>
    request<{ memories: MemoryRecord[] } | MemoryRecord[]>('/memory/search', {
      method: 'POST',
      body: JSON.stringify({ scope: 'agent', agent_id: agentId, query: q, limit }),
    }).then((d) => (Array.isArray(d) ? d : (d?.memories ?? []))).catch(() => [] as MemoryRecord[]),
  list: (agentId: string) =>
    request<{ memories: MemoryRecord[] } | MemoryRecord[]>('/memory/search', {
      method: 'POST',
      body: JSON.stringify({ scope: 'agent', agent_id: agentId, query: '', limit: 50 }),
    }).then((d) => (Array.isArray(d) ? d : (d?.memories ?? []))).catch(() => [] as MemoryRecord[]),
};

export const feedback = {
  send: (body: { session_id: string; agent_id: string; content: string; rating: 'like' | 'dislike' }) =>
    request<{ status: 'saved' }>('/feedback', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
};

export const training = {
  exportUrl: (agentId: string, format: 'jsonl' | 'preferences' | 'corrections' = 'jsonl') =>
    `/api/v1/training/export/${encodeURIComponent(agentId)}?format=${format}`,
};

export const orgChart = {
  get: () => request<{ agents: any[] }>('/org-chart'),
};

export interface OrgChartAgent {
  id: string;
  display_name: string;
  agent_key?: string;
  role?: string;
  manager_id?: string;
  [extra: string]: unknown;
}

export const teamsApi = {
  list: () => listRequest<any>('/teams'),
  create: (body: { name: string }) => request<any>('/teams', { method: 'POST', body: JSON.stringify(body) }),
  members: (id: string) => listRequest<any>(`/teams/${id}/members`),
};

export const userApi = {
  me: () => request<any>('/user/me'),
  changePassword: (body: { current_password: string; new_password: string }) =>
    fetch(`${AUTH_BASE}/change-password`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${getToken()}` }, body: JSON.stringify(body) }).then(r => r.json()),
  createApiKey: (name: string) =>
    request<{ key: string; message: string }>('/auth/api-keys', { method: 'POST', body: JSON.stringify({ name }) }),
  listApiKeys: () =>
    request<Array<{ id: string; name: string; created_at: string; last_used_at: string | null }>>('/user/api-keys'),
  revokeApiKey: (id: string) =>
    request<void>(`/user/api-keys/${id}`, { method: 'DELETE' }),
  patchProfile: (body: { email?: string; display_name?: string; avatar_url?: string }) =>
    request<any>('/user/profile', { method: 'PATCH', body: JSON.stringify(body) }),
  listSessions: () =>
    request<Array<{ id: string; user_agent: string; ip_address: string; created_at: string; last_used_at: string | null; expires_at: string }>>('/user/sessions'),
  revokeSession: (id: string) =>
    request<void>(`/user/sessions/${id}`, { method: 'DELETE' }),
};

export const userPrefs = {
  get: () => request<Record<string, any>>('/user/preferences'),
  save: (prefs: Record<string, any>) =>
    request<void>('/user/preferences', { method: 'POST', body: JSON.stringify(prefs) }),
};
