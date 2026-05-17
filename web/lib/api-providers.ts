// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { request, listRequest, BASE, getToken } from './api-core';
import type { Provider } from '@/types';

export const providers = {
  list: () => listRequest<Provider>('/providers'),
  catalog: () => request<any[]>('/providers/catalog'),
  get: (id: string) => request<Provider>(`/providers/${id}`),
  create: (body: Record<string, unknown>) => request<Provider>('/providers', { method: 'POST', body: JSON.stringify(body) }),
  update: (id: string, body: Record<string, unknown>) => request<void>(`/providers/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  delete: (id: string) => request<void>(`/providers/${id}`, { method: 'DELETE' }),
  verify: (id: string) => request<unknown>(`/providers/${id}/verify`, { method: 'POST' }),
  models: () => listRequest<unknown>('/models'),
  providerModels: (id: string) => listRequest<unknown>(`/providers/${id}/models`),
  listKeys: (providerId: string) => request<any[]>(`/providers/${providerId}/keys`),
  addKey: (providerId: string, body: { label: string; key: string }) =>
    request<any>(`/providers/${providerId}/keys`, { method: 'POST', body: JSON.stringify(body) }),
  verifyKey: (keyId: string) => request<any>(`/providers/keys/${keyId}/verify`, { method: 'POST' }),
  retireKey: (keyId: string) => request<void>(`/providers/keys/${keyId}`, { method: 'DELETE' }),
  usage: (providerId: string) => request<any[]>(`/providers/${providerId}/usage`),
  liveModels: (providerId: string) => request<any>(`/providers/${providerId}/live-models`),
  selectedModels: (providerId?: string) => listRequest<any>(`/models/selected${providerId ? "?provider_id=" + providerId : ""}`),
  selectModel: (providerId: string, modelId: string) => request<void>("/models/select", { method: "POST", body: JSON.stringify({ provider_id: providerId, model_id: modelId }) }),
  deselectModel: (providerId: string, modelId: string) => request<void>("/models/select", { method: "DELETE", body: JSON.stringify({ provider_id: providerId, model_id: modelId }) }),
  setDefaultModel: (providerId: string, modelId: string) => request<void>("/models/default", { method: "POST", body: JSON.stringify({ provider_id: providerId, model_id: modelId }) }),
  probeModels: (apiBase: string, apiKey: string) =>
    request<any>('/providers/probe-models', { method: 'POST', body: JSON.stringify({ api_base: apiBase, api_key: apiKey }) }),
  getPoolConfig: (providerId: string) => request<{ strategy: string; failover_mode: string }>(`/providers/${providerId}/pool`),
  savePoolConfig: (providerId: string, cfg: { strategy: string; failover_mode: string }) =>
    request<void>(`/providers/${providerId}/pool`, { method: 'PUT', body: JSON.stringify(cfg) }),
  setKeyBudget: (keyId: string, budget: { budget_usd_monthly?: number | null; budget_tokens_monthly?: number | null }) =>
    request<void>(`/providers/keys/${keyId}/budget`, { method: 'PUT', body: JSON.stringify(budget) }),
  testKey: (keyId: string) => request<{ key_id: string; ok: boolean; error: string; models: { id: string; name: string }[] }>(`/providers/keys/${keyId}/test`, { method: 'POST' }),
  availableModels: (category?: string) => listRequest<any>(`/models/available${category ? '?category=' + category : ''}`),
  discoveredModels: (unnotifiedOnly?: boolean) => listRequest<any>(`/models/discovered${unnotifiedOnly ? '?unnotified=1' : ''}`),
  actionDiscoveredModel: (id: string, action: 'enable' | 'dismiss') =>
    request<void>(`/models/discovered/${id}/${action}`, { method: 'POST' }),
};

export interface RankedModel {
  rank: number;
  id: string;
  name: string;
  organization: string;
  intelligence_index: number;
  coding_index: number;
  math_index: number;
  speed_tokens_per_sec: number;
  input_price_per_m: number;
  output_price_per_m: number;
}

export interface ModelRankingsResponse {
  models: RankedModel[];
  source: string;
  fetched_at?: string;
  configured: boolean;
  key_url: string;
}

export const routing = {
  categories: () => request<any[]>('/routing/categories'),
  assignments: () => request<Record<string, string[]>>('/routing/assignments'),
  suggestions: (models?: string[]) => {
    const qs = models && models.length ? `?models=${encodeURIComponent(models.join(','))}` : '';
    return request<Record<string, string>>(`/routing/suggestions${qs}`);
  },
  assign: (category: string, model_id: string) => request<void>('/routing/assign', { method: 'POST', body: JSON.stringify({ category, model_id, priority: 0 }) }),
  unassign: (category: string, model_id: string) => request<void>('/routing/assign', { method: 'DELETE', body: JSON.stringify({ category, model_id }) }),
  classify: (query: string) => request<any>('/routing/classify', { method: 'POST', body: JSON.stringify({ query }) }),
  decisions: () => request<any[]>('/routing/decisions'),
  correct: (decision_id: string, model: string, category: string) => request<void>('/routing/correct', { method: 'POST', body: JSON.stringify({ decision_id, model, category }) }),
  modelRankings: () => request<ModelRankingsResponse>('/routing/model-rankings'),
};

export const usage = {
  soul: (soulId: string) => request<any>(`/usage/soul/${soulId}`),
  account: () => request<any>('/usage/account'),
};

export interface TraceRow {
  id: string;
  agent_id?: string;
  session_key?: string;
  start_time: string;
  end_time?: string;
  duration_ms?: number;
  input_tokens: number;
  output_tokens: number;
  cost_cents: number;
  status: string;
  error?: string;
  created_at: string;
}

export interface SpanRow {
  id: string;
  trace_id: string;
  span_type: string;
  name?: string;
  model?: string;
  provider?: string;
  input_tokens?: number;
  output_tokens?: number;
  cost_cents: number;
  start_time?: string;
  end_time?: string;
  duration_ms?: number;
  status: string;
  error?: string;
  created_at: string;
}

export interface TraceSummary {
  agent_id: string;
  traces: number;
  input_tokens: number;
  output_tokens: number;
  cost_cents: number;
}

export const traces = {
  list: (params?: { agent_id?: string; limit?: number; offset?: number }) => {
    const qs = new URLSearchParams();
    if (params?.agent_id) qs.set('agent_id', params.agent_id);
    if (params?.limit)    qs.set('limit',    String(params.limit));
    if (params?.offset)   qs.set('offset',   String(params.offset));
    const q = qs.toString();
    return request<TraceRow[]>(`/traces${q ? '?' + q : ''}`);
  },
  summary: () => request<TraceSummary[]>('/traces/summary'),
  get: (id: string) => request<TraceRow>(`/traces/${id}`),
  spans: (id: string) => request<SpanRow[]>(`/traces/${id}/spans`),
};

export interface RoutingCategory {
  id: string;
  name: string;
  slug: string;
  description?: string;
  icon?: string;
  color?: string;
}

export interface RoutingDecision {
  id: string;
  soul_id?: string;
  query_preview: string;
  category: string;
  confidence?: number;
  model: string;
  override_model?: string;
  override_category?: string;
  was_correct?: boolean;
  created_at: string;
}

export const routingTyped = {
  categories: () => request<RoutingCategory[]>('/routing/categories'),
  decisions: () => request<RoutingDecision[]>('/routing/decisions'),
  correct: (decision_id: string, model: string, category: string) =>
    request<void>('/routing/correct', {
      method: 'POST',
      body: JSON.stringify({ decision_id, model, category }),
    }),
};

export interface ToolMetric {
  name: string;
  call_count: number;
  success_count: number;
  error_count: number;
  avg_latency_ms: number;
  max_latency_ms: number;
  last_call_at?: string;
  last_error?: string;
  success_rate: number;
}

export interface ToolMetricsSummary {
  total_calls: number;
  total_errors: number;
  tool_count: number;
  tools: ToolMetric[];
}

export const toolMetrics = {
  all: () => request<ToolMetricsSummary>('/tools/metrics'),
};

export interface WasmPlugin {
  id: string;
  name: string;
  description?: string;
  sha256: string;
  parameters?: unknown;
  size_bytes: number;
  created_by?: string;
  created_at: string;
  updated_at?: string;
}

export const wasmPlugins = {
  list: () => request<{ plugins: WasmPlugin[]; count: number }>('/wasm-plugins'),
  upload: async (body: { file: File; name: string; description?: string; parameters?: string }) => {
    const fd = new FormData();
    fd.append('wasm', body.file);
    fd.append('name', body.name);
    if (body.description) fd.append('description', body.description);
    if (body.parameters) fd.append('parameters', body.parameters);
    const res = await fetch(
      `${typeof window !== 'undefined' ? '/api/v1' : process.env.NEXT_PUBLIC_API_URL + '/v1'}/wasm-plugins`,
      {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : (process.env.NEXT_PUBLIC_API_TOKEN ?? '')}`,
        },
        body: fd,
      },
    );
    if (!res.ok) throw new Error(`upload ${res.status}: ${await res.text()}`);
    return res.json() as Promise<WasmPlugin>;
  },
  delete: (name: string) =>
    request<{ status: 'revoked'; name: string }>(`/wasm-plugins/${encodeURIComponent(name)}`, {
      method: 'DELETE',
    }),
};

export const systemInfo = {
  get: () => request<any>('/system/info'),
};

export interface NetworkStatus {
  tailscale_installed: boolean;
  tailscale_ip: string;
  tailscale_hostname: string;
  bind_mode: 'public' | 'tailscale' | 'localhost';
  web_listen: string;
  api_listen: string;
}

export const networkApi = {
  status: () => request<NetworkStatus>('/network/status'),
  tailscale: (action: 'install' | 'bind' | 'unbind', auth_key?: string) =>
    request<{ status: NetworkStatus; message?: string; web_listen?: string } | NetworkStatus>(
      '/network/tailscale',
      { method: 'POST', body: JSON.stringify({ action, auth_key: auth_key ?? '' }) }
    ),
};
