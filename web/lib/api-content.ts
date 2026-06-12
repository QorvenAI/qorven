// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { request, listRequest, BASE } from './api-core';

// Workflows
export type WorkflowStepType =
  | 'prompt' | 'tool' | 'condition' | 'collect'
  | 'api' | 'delegate' | 'notify' | 'wait';

export interface WorkflowStep {
  id: string;
  type: WorkflowStepType | string;
  prompt?: string;
  tool?: string;
  args?: Record<string, unknown>;
  branches?: Record<string, string>;
  fields?: string[];
  method?: string;
  url?: string;
  body?: Record<string, unknown>;
  soul_key?: string;
  task?: string;
  save_as?: string;
  next?: string;
  parallel?: WorkflowStep[];
}

export interface Workflow {
  id: string;
  tenant_id?: string;
  name: string;
  description?: string;
  agent_id?: string | null;
  trigger_type?: string;
  trigger_config?: unknown;
  steps?: WorkflowStep[] | string | unknown;
  variables?: unknown;
  enabled?: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface WorkflowRun {
  id: string;
  workflow_id: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | string;
  current_step?: number;
  result?: string;
  error?: string;
  started_at: string;
  completed_at?: string;
}

export interface CreateWorkflowInput {
  name: string;
  description?: string;
  agent_id?: string | null;
  trigger_type?: string;
  steps?: WorkflowStep[];
  enabled?: boolean;
}

export const workflows = {
  list: () => listRequest<Workflow>('/workflows'),
  get: (id: string) => request<Workflow>(`/workflows/${id}`),
  create: (input: CreateWorkflowInput) =>
    request<{ id: string }>('/workflows', {
      method: 'POST',
      body: JSON.stringify({ ...input, steps: JSON.stringify(input.steps ?? []) }),
    }),
  update: (id: string, input: Partial<Workflow> & { steps?: WorkflowStep[] | string }) => {
    const body: Record<string, unknown> = { ...input };
    if (Array.isArray(input.steps)) body.steps = JSON.stringify(input.steps);
    return request<{ status: string }>(`/workflows/${id}`, { method: 'PUT', body: JSON.stringify(body) });
  },
  delete: (id: string) => request<{ status: string }>(`/workflows/${id}`, { method: 'DELETE' }),
  run: (id: string) => request<{ run_id: string }>(`/workflows/${id}/run`, { method: 'POST' }),
  runs: (id: string) => listRequest<WorkflowRun>(`/workflows/${id}/runs`),
};

export function parseWorkflowSteps(steps: unknown): WorkflowStep[] {
  if (Array.isArray(steps)) return steps as WorkflowStep[];
  if (typeof steps === 'string') {
    try {
      const parsed = JSON.parse(steps);
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  }
  return [];
}

// Connectors
export interface ConnectorManifest {
  id: string;
  name: string;
  description: string;
  icon?: string;
  category?: string;
  status?: string;
  auth_schema?: {
    type: string;
    fields?: Array<{ name: string; label: string; type: string; required?: boolean; placeholder?: string }>;
  };
  actions?: Array<{ id: string; name: string; description: string }>;
  triggers?: Array<{ id: string; name: string; description: string }>;
}

export interface ConnectorPlatform {
  id: string;
  name: string;
  category: string;
  description: string;
  icon?: string;
  auth_type: string;
  base_url?: string;
  docs_url?: string;
  enabled?: boolean;
}

export const connectors = {
  list: () => listRequest<ConnectorManifest>('/connectors'),
  test: (id: string, credentials: Record<string, string>) =>
    request<unknown>(`/connectors/${id}/test`, { method: 'POST', body: JSON.stringify({ credentials }) }),
  execute: (id: string, body: { action: string; credentials: Record<string, string>; params: Record<string, unknown> }) =>
    request<unknown>(`/connectors/${id}/execute`, { method: 'POST', body: JSON.stringify(body) }),
  platforms: () => listRequest<ConnectorPlatform>('/connectors/platforms'),
  actions: (platformID: string) => listRequest<unknown>(`/connectors/platforms/${platformID}/actions`),
};

// Connections (vault-backed)
export const connections = {
  list: () => request<{ connections: any[] }>('/connections'),
  // config carries per-connection host placeholders (e.g. {site} for WordPress),
  // pinned + SSRF-validated server-side.
  save: (platformId: string, token: string, label?: string, config?: Record<string, string>) =>
    request<any>(`/connections/${platformId}`, {
      method: 'POST',
      body: JSON.stringify({ token, label: label || 'default', config: config || undefined }),
    }),
  delete: (platformId: string) =>
    request<void>(`/connections/${platformId}`, { method: 'DELETE' }),
};

// Pairing
export interface PairingDevice {
  id: string;
  channel_type: string;
  sender_id?: string;
  chat_id?: string;
  sender_name?: string;
  paired_at?: string;
}

export interface PairingRequest {
  code?: string;
  agent_id?: string;
  channel_type?: string;
  sender_id?: string;
  requested_at?: string;
  [extra: string]: unknown;
}

export const pairing = {
  pending: () => request<PairingRequest[]>('/pairing/pending'),
  approve: (code: string) =>
    request<{ status: 'approved' }>('/pairing/approve', {
      method: 'POST',
      body: JSON.stringify({ code }),
    }),
  devices: () => request<PairingDevice[]>('/pairing/devices'),
};

// Mail
export interface MailIdentity {
  id: string;
  tenant_id?: string;
  agent_id?: string | null;
  address: string;
  display_name: string;
  identity_type: 'dedicated' | 'shared' | string;
  is_active?: boolean;
  transport?: 'smtp' | 'forward' | string;
  forward_url?: string;
  reply_to?: string;
  default_importance?: 'low' | 'normal' | 'high' | string;
  signature_html?: string;
  signature_text?: string;
  inbound_secret?: string;
  smtp_host?: string;
  smtp_port?: number;
  smtp_user?: string;
  imap_host?: string;
  imap_port?: number;
  imap_user?: string;
  poll_interval_seconds?: number;
}

export interface MailAlias {
  id: string;
  alias_address: string;
  target_agent_id: string;
  can_send_as: boolean;
  can_receive: boolean;
}

export interface MailAttachment {
  name: string;
  content_type?: string;
  size?: number;
}

export interface MailMessage {
  id: string;
  agent_id?: string;
  from_address: string;
  from_name?: string;
  to_addresses?: string[];
  cc_addresses?: string[];
  bcc_addresses?: string[];
  subject: string;
  body_text?: string;
  body_html?: string;
  direction: 'inbound' | 'outbound';
  status: string;
  send_status?: 'queued' | 'sent' | 'failed' | 'pending_approval' | string;
  importance?: 'low' | 'normal' | 'high' | string;
  agent_decision?: string;
  thread_id?: string;
  folder?: string;
  is_read?: boolean;
  is_starred?: boolean;
  attachments?: MailAttachment[];
  created_at: string;
  updated_at?: string;
}

export interface MailDraft {
  id: string;
  agent_id?: string;
  identity_id?: string;
  to?: string[];
  cc?: string[];
  bcc?: string[];
  subject?: string;
  body_html?: string;
  body_text?: string;
  importance?: 'low' | 'normal' | 'high' | string;
  reply_to_message_id?: string;
  attachments?: MailAttachment[];
  created_at: string;
  updated_at?: string;
}

export const mail = {
  // ── Inbox / folder navigation (folder is always a query param, never a path segment) ──
  inbox: (folder?: string, agentId?: string) => {
    const params = new URLSearchParams();
    if (folder) params.set('folder', folder);
    if (agentId) params.set('agent_id', agentId);
    const qs = params.toString() ? `?${params}` : '';
    return listRequest<MailMessage>(`/mail/inbox${qs}`);
  },
  // folder() is the UI-facing alias used by page.tsx — calls inbox with folder as query param
  // Returns any[] to maintain backward compatibility with page.tsx's local MailMsg type
  folder: (folder: string, agentId?: string): Promise<any[]> => {
    const params = new URLSearchParams();
    params.set('folder', folder);
    if (agentId) params.set('agent_id', agentId);
    return listRequest<any>(`/mail/inbox?${params}`);
  },
  sent: (agentId?: string) => {
    const qs = agentId ? `?agent_id=${encodeURIComponent(agentId)}` : '';
    return listRequest<MailMessage>(`/mail/sent${qs}`);
  },
  get: (id: string) => request<MailMessage>(`/mail/${encodeURIComponent(id)}`),
  /** @deprecated use mail.get(id) */
  getMessage: (id: string) => request<MailMessage>(`/mail/${encodeURIComponent(id)}`),
  thread: (threadId: string) => listRequest<MailMessage>(`/mail/thread/${encodeURIComponent(threadId)}`),
  search: (q: string, agentId?: string) => {
    const params = new URLSearchParams({ q });
    if (agentId) params.set('agent_id', agentId);
    return listRequest<MailMessage>(`/mail/search?${params}`);
  },

  // ── Send ──
  send: (body: {
    to: string[];
    cc?: string[];
    bcc?: string[];
    subject: string;
    body_html?: string;
    body_text?: string;
    /** @deprecated use body_html / body_text */
    body?: string;
    importance?: string;
    reply_to?: string;
    identity_id?: string;
    agent_id?: string;
    attachments?: Array<{ name: string; content_type?: string; data?: string }>;
  }) => request<void>('/mail/send', { method: 'POST', body: JSON.stringify(body) }),

  // ── Message actions ──
  setRead: (id: string, read = true) =>
    request<void>(`/mail/${encodeURIComponent(id)}/read`, { method: 'PUT', body: JSON.stringify({ read }) }),
  setStar: (id: string, starred = true) =>
    request<void>(`/mail/${encodeURIComponent(id)}/star`, { method: 'PUT', body: JSON.stringify({ starred }) }),
  /** @deprecated use mail.setRead */
  markRead: (id: string, read: boolean) =>
    request<void>(`/mail/${encodeURIComponent(id)}/read`, { method: 'PUT', body: JSON.stringify({ read }) }),
  trash: (id: string) =>
    request<void>(`/mail/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  archive: (id: string) =>
    request<void>(`/mail/${encodeURIComponent(id)}/archive`, { method: 'POST' }),
  move: (id: string, folder: string) =>
    request<void>(`/mail/${encodeURIComponent(id)}/move`, { method: 'POST', body: JSON.stringify({ folder }) }),

  // ── Bulk actions ──
  bulk: (ids: string[], action: 'read' | 'star' | 'move' | 'delete', value?: unknown) =>
    request<void>('/mail/bulk', { method: 'POST', body: JSON.stringify({ ids, action, value }) }),

  // ── Attachments ──
  /** Returns the URL string for use in <a> / <img> — does not fetch, just builds path. */
  attachmentUrl: (id: string, name: string) =>
    `${BASE}/mail/${encodeURIComponent(id)}/attachments/${encodeURIComponent(name)}`,

  // ── Drafts ──
  draftsList: (agentId?: string) => {
    const qs = agentId ? `?agent_id=${encodeURIComponent(agentId)}` : '';
    return listRequest<MailDraft>(`/mail/drafts${qs}`);
  },
  draftSave: (body: Partial<MailDraft>) =>
    request<MailDraft>('/mail/drafts', { method: 'POST', body: JSON.stringify(body) }),
  draftUpdate: (id: string, body: Partial<MailDraft>) =>
    request<MailDraft>(`/mail/drafts/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(body) }),
  draftDelete: (id: string) =>
    request<void>(`/mail/drafts/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // ── Identities ──
  identities: () => request<MailIdentity[]>('/mail/identities'),
  createIdentity: (body: {
    agent_id?: string;
    address: string;
    display_name: string;
    identity_type?: string;
    is_active?: boolean;
    transport?: string;
    forward_url?: string;
    reply_to?: string;
    default_importance?: string;
    signature_html?: string;
    signature_text?: string;
    inbound_secret?: string;
    smtp_host?: string;
    smtp_port?: number;
    smtp_user?: string;
    smtp_pass?: string;
    imap_host?: string;
    imap_port?: number;
    imap_user?: string;
    imap_pass?: string;
  }) => request<MailIdentity>('/mail/identities', { method: 'POST', body: JSON.stringify(body) }),
  updateIdentity: (id: string, body: Partial<MailIdentity> & { smtp_pass?: string; imap_pass?: string }) =>
    request<void>(`/mail/identities/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(body) }),

  // ── Aliases ──
  aliases: () => request<MailAlias[]>('/mail/aliases'),
  createAlias: (body: { alias_address: string; target_agent_id: string; can_send_as: boolean; can_receive: boolean }) =>
    request<{ id: string }>('/mail/aliases', { method: 'POST', body: JSON.stringify(body) }),
  deleteAlias: (id: string) =>
    request<void>(`/mail/aliases/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // ── Approvals ──
  approvalsList: () => request<MailApproval[]>('/approvals/mail'),
  approvalApprove: (id: string) =>
    request<{ status: 'approved' }>(`/approvals/mail/${encodeURIComponent(id)}/approve`, { method: 'POST' }),
  approvalReject: (id: string, reason?: string) =>
    request<{ status: 'rejected' }>(`/approvals/mail/${encodeURIComponent(id)}/reject`, { method: 'POST', body: JSON.stringify({ reason: reason ?? '' }) }),
};

// Social
export const social = {
  listPosts: (agentId?: string, status?: string) => {
    const params = new URLSearchParams();
    if (agentId) params.set('agent_id', agentId);
    if (status) params.set('status', status);
    return request<any[]>(`/social/posts?${params}`);
  },
  createPost: (body: Record<string, unknown>) =>
    request<any>('/social/posts', { method: 'POST', body: JSON.stringify(body) }),
  getPost: (id: string) => request<any>(`/social/posts/${id}`),
  deletePost: (id: string) => request<void>(`/social/posts/${id}`, { method: 'DELETE' }),
  publishNow: (id: string) => request<any>(`/social/posts/${id}/publish`, { method: 'POST' }),
  listIntegrations: (agentId?: string) =>
    request<any[]>(`/social/integrations${agentId ? `?agent_id=${agentId}` : ''}`),
  saveIntegration: (body: Record<string, unknown>) =>
    request<any>('/social/integrations', { method: 'POST', body: JSON.stringify(body) }),
  deleteIntegration: (id: string) => request<void>(`/social/integrations/${id}`, { method: 'DELETE' }),
  updateIntegrationSettings: (id: string, body: { nickname?: string; avatar_url?: string; group_name?: string; post_hours?: number[]; post_days?: number[]; paused?: boolean }) =>
    request<any>(`/social/integrations/${id}/settings`, { method: 'PATCH', body: JSON.stringify(body) }),
  listAutoPosts: (agentId?: string) =>
    request<any[]>(`/social/autoposts${agentId ? `?agent_id=${agentId}` : ''}`),
  createAutoPost: (body: Record<string, unknown>) =>
    request<any>('/social/autoposts', { method: 'POST', body: JSON.stringify(body) }),
  deleteAutoPost: (id: string) => request<void>(`/social/autoposts/${id}`, { method: 'DELETE' }),
  toggleAutoPost: (id: string, active: boolean) => request<{ status: string }>(`/social/autoposts/${id}/toggle`, { method: 'PATCH', body: JSON.stringify({ active }) }),
  calendar: (agentId?: string) =>
    request<{ entries: any[]; total: number; stats: Record<string, number> }>(
      `/social/calendar${agentId ? `?agent_id=${agentId}` : ''}`
    ),

  // Post Comments
  listComments: (postId: string) => request<any[]>(`/social/posts/${postId}/comments`),
  createComment: (postId: string, body: { body: string; parent_id?: string; author_name?: string }) =>
    request<any>(`/social/posts/${postId}/comments`, { method: 'POST', body: JSON.stringify(body) }),
  deleteComment: (postId: string, commentId: string) =>
    request<void>(`/social/posts/${postId}/comments/${commentId}`, { method: 'DELETE' }),
  resolveComment: (postId: string, commentId: string, resolved: boolean) =>
    request<any>(`/social/posts/${postId}/comments/${commentId}/resolve`, {
      method: 'PATCH', body: JSON.stringify({ resolved }),
    }),

  // Analytics
  analyticsSummary: (agentId?: string) => {
    const p = new URLSearchParams();
    if (agentId) p.set('agent_id', agentId);
    return request<{ by_platform: any[]; top_posts: any[]; days: number }>(`/social/analytics?${p}`);
  },
  postMetrics: (id: string) => request<any[]>(`/social/posts/${id}/metrics`),

  // Media library
  listMedia: (params?: { agentId?: string; q?: string; type?: string; limit?: number; offset?: number }) => {
    const p = new URLSearchParams();
    if (params?.agentId) p.set('agent_id', params.agentId);
    if (params?.q) p.set('q', params.q);
    if (params?.type) p.set('type', params.type);
    if (params?.limit) p.set('limit', String(params.limit));
    if (params?.offset) p.set('offset', String(params.offset));
    return request<{ assets: any[]; total: number; limit: number; offset: number }>(`/social/media?${p}`);
  },
  uploadMedia: (file: File, agentId: string, altText?: string) => {
    const fd = new FormData();
    fd.append('file', file);
    fd.append('agent_id', agentId);
    if (altText) fd.append('alt_text', altText);
    return request<any>('/social/media', { method: 'POST', body: fd });
  },
  deleteMedia: (id: string) => request<void>(`/social/media/${id}`, { method: 'DELETE' }),
  updateMedia: (id: string, body: { alt_text?: string; tags?: string[] }) =>
    request<any>(`/social/media/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  listWebhooks: (agentId?: string) =>
    request<any[]>(`/social/webhooks${agentId ? `?agent_id=${agentId}` : ''}`),
  createWebhook: (body: { name?: string; url: string; secret?: string; events?: string[]; agent_id?: string }) =>
    request<any>('/social/webhooks', { method: 'POST', body: JSON.stringify(body) }),
  deleteWebhook: (id: string) => request<void>(`/social/webhooks/${id}`, { method: 'DELETE' }),
  toggleWebhook: (id: string) => request<any>(`/social/webhooks/${id}/toggle`, { method: 'PATCH' }),
  testWebhook: (id: string) => request<any>(`/social/webhooks/${id}/test`, { method: 'POST' }),
  listSets: (agentId?: string) => {
    const p = new URLSearchParams();
    if (agentId) p.set('agent_id', agentId);
    return request<any[]>(`/social/sets?${p}`);
  },
  createSet: (body: { agent_id?: string; name: string; description?: string; content: string; platforms?: string[] }) =>
    request<any>('/social/sets', { method: 'POST', body: JSON.stringify(body) }),
  updateSet: (id: string, body: { name?: string; description?: string; content?: string; platforms?: string[] }) =>
    request<any>(`/social/sets/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  deleteSet: (id: string) => request<void>(`/social/sets/${id}`, { method: 'DELETE' }),

  // OAuth app credentials (admin — sets client_id/secret for provider's OAuth app)
  oauthAppsGet: () =>
    request<{ apps: { id: string; name: string; platform: string; has_creds: boolean; pkce: boolean; redirect_uri: string }[] }>('/social/oauth/apps')
      .then(d => (d as any)?.apps ?? []),
  oauthAppSet: (platform: string, clientId: string, clientSecret: string) =>
    request<{ status: string }>(`/social/oauth/apps/${platform}`, { method: 'POST', body: JSON.stringify({ client_id: clientId, client_secret: clientSecret }) }),
  oauthAppDelete: (platform: string) =>
    request<{ status: string }>(`/social/oauth/apps/${platform}`, { method: 'DELETE' }),

  // Relay connect flow
  relayConnect: (relayKeyId: string, platform: string, agentId: string) =>
    request<{ auth_url: string; relay_key_id: string }>('/social/connect', {
      method: 'POST',
      body: JSON.stringify({ relay_key_id: relayKeyId, platform, agent_id: agentId }),
    }),
  relayConnectFinalize: (relayKeyId: string, sessionToken: string, agentId: string) =>
    request<{ status: string }>('/social/connect/finalize', {
      method: 'POST',
      body: JSON.stringify({ relay_key_id: relayKeyId, session_token: sessionToken, agent_id: agentId }),
    }),

  // Campaigns (CMO)
  campaigns: () =>
    request<{ campaigns: any[] }>(`/social/campaigns`),
  createCampaign: (body: { title: string; brief?: string; target_platforms?: string[]; created_by_agent_id?: string }) =>
    request<{ id: string }>(`/social/campaigns`, { method: 'POST', body: JSON.stringify(body) }),
  setCampaignStatus: (id: string, status: string) =>
    request<void>(`/social/campaigns/${id}/status`, { method: 'PUT', body: JSON.stringify({ status }) }),
  delegateCampaign: (id: string, body: { agent_id: string; title?: string; description?: string }) =>
    request<{ task_id: string }>(`/social/campaigns/${id}/delegate`, { method: 'POST', body: JSON.stringify(body) }),

  // Approvals (CMO inbox)
  pendingApprovals: () =>
    request<{ posts: any[] }>(`/social/approvals`),
  approvePost: (id: string) =>
    request<void>(`/social/posts/${id}/approve`, { method: 'POST' }),
  rejectPost: (id: string) =>
    request<void>(`/social/posts/${id}/reject`, { method: 'POST' }),
};

// Research
export type ResearchMode = 'quick' | 'balanced' | 'quality' | string;
export type ResearchStatus = 'running' | 'completed' | 'failed' | string;

export interface ResearchSource { title: string; url: string; text?: string }
export interface ResearchReport { query: string; mode: string; answer: string; sources: ResearchSource[] }
export interface ResearchProgress { step: string; detail: string; sources?: number }
export interface ResearchJob {
  id: string; query: string; mode: string; status: ResearchStatus;
  report?: ResearchReport; progress?: ResearchProgress[]; error?: string;
}

export const research = {
  start: (body: { query: string; mode?: ResearchMode }) =>
    request<{ id: string; status: 'running' }>('/research/start', { method: 'POST', body: JSON.stringify(body) }),
  get: (id: string) => request<ResearchJob>(`/research/${encodeURIComponent(id)}`),
};

// Council
export type CouncilDepth = 'quick' | 'balanced' | 'deep' | 'max';
export interface CouncilDraft { model: string; label: string; response: string; tokens: number; duration: number }
export interface CouncilRanking { ranker: string; ranking: string[]; reason: string }
export interface CouncilResult {
  query: string; stage1: CouncilDraft[]; stage2: CouncilRanking[];
  synthesis: string; gate_skipped: boolean; duration: number; tokens_used: number;
}
export interface CouncilConfig {
  default: { members: string[]; chairman: string; agreement_gate: number; max_tokens: number };
  depths: Record<CouncilDepth, { depth: CouncilDepth; model_tier: string; tools_enabled: boolean; council_enabled: boolean; council_threshold: number; search_passes: number; max_iterations: number; max_tokens: number }>;
}

export const council = {
  config: () => request<CouncilConfig>('/council/config'),
  run: (body: { query: string; depth?: CouncilDepth; members?: string[]; chairman?: string }) =>
    request<CouncilResult>('/council', { method: 'POST', body: JSON.stringify(body) }),
};

// Scenarios
export type ScenarioStatus = 'created' | 'running' | 'completed' | 'failed' | string;
export interface ScenarioAgent { id: string; name: string; role: string; bio: string; stance: string; traits: string }
export interface ScenarioRound { number: number; agent_id: string; agent_name: string; content: string; reply_to?: string; timestamp: string }
export interface ScenarioProject {
  id: string; tenant_id?: string; name: string; seed: string; agent_count: number;
  rounds: number; status: ScenarioStatus; report?: string;
  agents?: ScenarioAgent[]; rounds_data?: ScenarioRound[]; created_at: string; completed_at?: string;
}

export const scenarios = {
  list: () => request<{ scenarios: ScenarioProject[] }>('/scenarios'),
  get: (id: string) => request<ScenarioProject>(`/scenarios/${encodeURIComponent(id)}`),
  create: (body: { name?: string; seed: string; agent_count?: number; rounds?: number }) =>
    request<ScenarioProject>('/scenarios', { method: 'POST', body: JSON.stringify(body) }),
  run: (id: string) =>
    request<{ status: 'started'; id: string }>(`/scenarios/${encodeURIComponent(id)}/run`, { method: 'POST' }),
  inject: (id: string, event: string) =>
    request<{ rounds: ScenarioRound[] }>(`/scenarios/${encodeURIComponent(id)}/inject`, { method: 'POST', body: JSON.stringify({ event }) }),
};

// Code pipeline
export type CodeChangeStatus = 'proposed' | 'validating' | 'validated' | 'applying' | 'applied' | 'rejected' | 'failed' | string;
export type CodeFileAction = 'create' | 'modify' | 'delete';
export interface CodeFileChange { path: string; old_content?: string; new_content: string; action: CodeFileAction }
export interface CodeChange {
  id: string; description: string; files: CodeFileChange[]; risk: 'low' | 'medium' | 'high' | string;
  status: CodeChangeStatus; proposed_by?: string; reviewed_by?: string; created_at: string; applied_at?: string;
  compile_ok?: boolean; compile_error?: string; test_ok?: boolean; test_error?: string;
  tests_passed?: number; tests_failed?: number;
}

export const pipeline = {
  propose: (body: { description: string; files: CodeFileChange[]; risk: CodeChange['risk'] }) =>
    request<CodeChange>('/pipeline/propose', { method: 'POST', body: JSON.stringify(body) }),
  validate: (id: string) => request<CodeChange>(`/pipeline/validate/${encodeURIComponent(id)}`, { method: 'POST' }),
  apply: (id: string) => request<CodeChange>(`/pipeline/apply/${encodeURIComponent(id)}`, { method: 'POST' }),
  changes: () => request<{ changes: CodeChange[] }>('/pipeline/changes'),
  pending: () => request<{ pending: CodeChange[] }>('/pipeline/pending'),
};

// Plans
export type PlanStatus = 'draft' | 'pending_approval' | 'approved' | 'rejected' | 'revision_requested' | 'running' | 'done' | 'failed' | 'cancelled';
export type NodeKind = 'planner' | 'human_feedback' | 'agent_task' | 'review' | 'push' | 'preview';
export type NodeState = 'pending' | 'running' | 'done' | 'failed' | 'blocked' | 'cancelled';
export type EdgeCondition = 'always' | 'approved' | 'rejected' | 'revision' | 'on_success' | 'on_error';

export interface Plan {
  id: string; tenant_id: string; project_id?: string; session_id?: string;
  title: string; status: PlanStatus; spec?: unknown; summary?: string;
  created_by?: string; created_at: string; updated_at: string;
}
export interface PlanNode {
  id: string; plan_id: string; parent_id?: string; kind: NodeKind; title: string;
  state: NodeState; assignee_soul?: string; inputs?: unknown; artifacts?: unknown;
  error?: string; started_at?: string; ended_at?: string; created_at: string; updated_at: string;
}
export interface PlanEdge { plan_id: string; from_node: string; to_node: string; condition: EdgeCondition }

export const plans = {
  list: () => listRequest<Plan>('/plans').catch(() => [] as Plan[]),
  get: (id: string) => request<Plan>(`/plans/${encodeURIComponent(id)}`),
  nodes: (id: string) => request<{ nodes: PlanNode[]; edges: PlanEdge[] }>(`/plans/${encodeURIComponent(id)}/nodes`),
  approve: (id: string, comment?: string) =>
    request<{ plan: Plan }>(`/plans/${encodeURIComponent(id)}/approve`, { method: 'POST', body: JSON.stringify({ comment: comment ?? '' }) }),
  reject: (id: string, comment: string) =>
    request<{ plan: Plan }>(`/plans/${encodeURIComponent(id)}/reject`, { method: 'POST', body: JSON.stringify({ comment }) }),
  revise: (id: string, comment: string) =>
    request<{ plan: Plan }>(`/plans/${encodeURIComponent(id)}/revise`, { method: 'POST', body: JSON.stringify({ comment }) }),
};

// Approvals
export interface ApprovalItem {
  id: string; kind: 'plan' | 'tool' | string; state: 'pending' | 'approved' | 'rejected' | string;
  requested_by?: string; resolved_by?: string; created_at?: string;
  plan_id?: string; node_id?: string; budget?: unknown;
  agent_id?: string; tool_name?: string; tool_args?: unknown; reason?: string; status?: string;
}

export const approvals = {
  list: () => listRequest<ApprovalItem>('/approvals').catch(() => [] as ApprovalItem[]),
  decide: (id: string, decision: 'approve' | 'reject') =>
    request<{ status: string }>(`/approvals/${encodeURIComponent(id)}/decide`, { method: 'POST', body: JSON.stringify({ decision }) }),
};

export const permissions = {
  reply: (id: string, body: { decision: 'allow' | 'allow_always' | 'allow_session' | 'allow_1h' | 'deny'; note?: string }) =>
    request<{ ok: true }>(`/permissions/${id}/reply`, { method: 'POST', body: JSON.stringify(body) }),
};

// Outbound actions
export type OutboundActionKind = 'email_send' | 'telegram_send' | 'social_post' | 'webhook' | string;
export interface OutboundAction {
  id: string; agent_id: string; action_type: OutboundActionKind; payload: unknown;
  status: 'pending' | 'approved' | 'rejected' | 'approved_and_sent' | string;
  approval_mode?: string; requested_at: string; reviewed_by?: string; reviewed_at?: string;
  review_notes?: string; session_id?: string; expires_at?: string;
}
export interface MailApproval {
  id: string; to?: string | string[]; subject?: string; body?: string;
  agent_id?: string; created_at?: string; [extra: string]: unknown;
}

export const outbound = {
  pending: () => request<{ pending: OutboundAction[] }>('/outbound/pending'),
  approve: (id: string, notes?: string) =>
    request<{ status: string; result?: unknown }>(`/outbound/${encodeURIComponent(id)}/approve`, { method: 'POST', body: JSON.stringify({ notes: notes ?? '' }) }),
  reject: (id: string, notes?: string) =>
    request<{ status: 'rejected' }>(`/outbound/${encodeURIComponent(id)}/reject`, { method: 'POST', body: JSON.stringify({ notes: notes ?? '' }) }),
  mailPending: () => request<MailApproval[]>('/approvals/mail'),
  mailApprove: (id: string) =>
    request<{ status: 'approved' }>(`/approvals/mail/${encodeURIComponent(id)}/approve`, { method: 'POST' }),
  mailReject: (id: string, reason?: string) =>
    request<{ status: 'rejected' }>(`/approvals/mail/${encodeURIComponent(id)}/reject`, { method: 'POST', body: JSON.stringify({ reason: reason ?? '' }) }),
};

// Graph
export interface GraphNode { id: string; label: string; type?: string; color?: string; size?: number; x?: number; y?: number; link_count?: number; description?: string }
export interface GraphEdge { id: string; source: string; target: string; label?: string; weight: number; color?: string; thickness?: number }
export interface GraphStats { total_nodes: number; total_edges: number; nodes_by_type: Record<string, number>; avg_degree: number; max_degree: number; components: number }
export interface GraphData { nodes: GraphNode[]; edges: GraphEdge[]; stats: GraphStats }
export interface GodNode { id: string; name: string; type: string; degree: number; community?: number }
export interface SurprisingConnection { source_id: string; target_id: string; source_name: string; target_name: string; surprise_score: number; reason: string }
export interface GraphAnalysis { communities: Record<string, string[]>; god_nodes: GodNode[]; surprising_connections: SurprisingConnection[]; cohesion_scores: Record<string, number>; suggested_questions: string[]; pagerank: Record<string, number>; betweenness: Record<string, number>; clustering_coefficient: Record<string, number>; total_entities: number; total_relationships: number; total_clusters: number }

export const graph = {
  all: () => request<GraphData>('/graph'),
  neighborhood: (nodeId: string, depth = 1) => request<GraphData>(`/graph/${encodeURIComponent(nodeId)}?depth=${depth}`),
  relevance: (nodeId: string) => request<{ node_id: string; edges: GraphEdge[] }>(`/graph/${encodeURIComponent(nodeId)}/relevance`),
  godNodes: () => request<GodNode[]>('/graph/god-nodes'),
  clusters: () => request<Record<string, number>>('/graph/clusters'),
  analysis: () => request<GraphAnalysis>('/graph/analysis'),
};

// Sandbox
export interface SandboxRun { id: string; agent_id: string; command: string; language?: string; code?: string; exit_code: number; output: string; duration_ms: number; status: 'completed' | 'failed' | string; created_at: string }
export interface SandboxArtifact { name: string; path: string; size: number; modified: string }

export const sandbox = {
  run: (body: { agent_id: string; command: string; language?: string; code?: string }) =>
    request<SandboxRun>('/sandbox/run', { method: 'POST', body: JSON.stringify(body) }),
  runs: (agentId?: string) =>
    request<SandboxRun[]>(`/sandbox/runs${agentId ? `?agent_id=${encodeURIComponent(agentId)}` : ''}`),
  run_: (id: string) => request<SandboxRun>(`/sandbox/runs/${encodeURIComponent(id)}`),
  artifacts: (agentId?: string) =>
    request<SandboxArtifact[]>(`/sandbox/artifacts${agentId ? `?agent_id=${encodeURIComponent(agentId)}` : ''}`),
};

// Supervisor
export type SupervisorRisk = 'low' | 'medium' | 'high';
export type SupervisorAgentStatus = 'healthy' | 'degraded' | 'unresponsive' | 'suspended';
export interface SupervisorStatus { total_exchanges?: number; open_exchanges?: number; acked_exchanges?: number; escalated_exchanges?: number; timeout_exchanges?: number; total_messages?: number; pending_escalations?: number; status?: 'not_initialized' }
export interface SupervisorAgentHealth { agent_id: string; agent_name: string; status: SupervisorAgentStatus; last_heartbeat: string; last_status_check: string; consecutive_errors: number; total_errors_7d: number; sampling_rate: number; disagreements: number; suspended_from_ack: boolean }
export interface SupervisorMessage { id: string; from: string; to: string; intent: string; content: string; context?: Record<string, unknown>; risk?: SupervisorRisk; timestamp: string; reply_to?: string; exchange_id?: string; sync_timeout?: number }
export interface SupervisorFix { type: string; description: string; risk: SupervisorRisk }
export interface SupervisorFixHistory { fix_type: string; params?: Record<string, unknown>; success: boolean; error?: string; duration: number; timestamp: string }

export const supervisor = {
  status: () => request<SupervisorStatus>('/supervisor/status'),
  health: () => request<{ agents: SupervisorAgentHealth[] }>('/supervisor/health'),
  auditLog: () => request<{ messages: SupervisorMessage[] }>('/supervisor/audit-log'),
  escalations: () => request<{ escalations: SupervisorMessage[] }>('/supervisor/escalations'),
  approve: (id: string, reason?: string) =>
    request<{ status: 'approved' }>(`/supervisor/escalations/${encodeURIComponent(id)}/approve`, { method: 'POST', body: JSON.stringify({ reason: reason ?? '' }) }),
  reject: (id: string, reason?: string) =>
    request<{ status: 'rejected' }>(`/supervisor/escalations/${encodeURIComponent(id)}/reject`, { method: 'POST', body: JSON.stringify({ reason: reason ?? '' }) }),
  fixes: () => request<{ available: SupervisorFix[]; history: SupervisorFixHistory[] }>('/supervisor/fixes'),
  unsuspend: (agentId: string) =>
    request<{ status: string; agent_id: string }>(`/supervisor/agents/${encodeURIComponent(agentId)}/unsuspend`, { method: 'POST' }),
};

// Voice
export type VoiceKind = 'tts' | 'stt' | 'realtime';
export interface VoiceProviders { tts: string[]; stt: string[]; primary_tts?: string; primary_stt?: string; auto?: string }
export interface VoiceConfig { tts_provider?: string; stt_provider?: string; vad?: string; kokoro?: { url?: string; voice?: string }; whisper?: { model?: string; url?: string }; openai?: { voice?: string }; elevenlabs?: { api_key?: string; voice_id?: string }; edge?: { voice?: string }; auto_tts?: boolean; live_transcribe?: boolean; [extra: string]: unknown }
export interface VoiceCatalogField { name: string; label: string; type: 'password' | 'url' | 'text'; required: boolean; placeholder?: string }
export interface VoiceCatalogEntry { id: string; name: string; kind_supports: VoiceKind[]; hosting: 'cloud' | 'local'; auth: 'api_key' | 'none' | 'oauth'; streaming: boolean; hint?: string; hardware_hint?: string; fields: VoiceCatalogField[]; models?: Record<string, string[]>; default_model?: Record<string, string> }
export interface VoiceProviderRow { id: string; name: string; kind: VoiceKind; driver: string; api_base?: string; api_key?: string; settings: Record<string, unknown>; enabled: boolean; is_default: boolean; created_at?: string }
export interface VoiceProvidersResponse { providers: VoiceProviderRow[]; manager: VoiceProviders }

export const voice = {
  providers: () => request<VoiceProvidersResponse>('/voice/providers'),
  config: () => request<VoiceConfig>('/voice/config'),
  saveConfig: (cfg: VoiceConfig) =>
    request<VoiceConfig>('/voice/config', { method: 'PUT', body: JSON.stringify(cfg) }),
  speech: async (body: { input: string; voice?: string; model?: string }) => {
    const res = await fetch(
      `${typeof window !== 'undefined' ? '/api/v1' : process.env.NEXT_PUBLIC_API_URL + '/v1'}/audio/speech`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : (process.env.NEXT_PUBLIC_API_TOKEN ?? '')}` },
        body: JSON.stringify(body),
      },
    );
    if (!res.ok) throw new Error(`TTS ${res.status}: ${await res.text()}`);
    return res.blob();
  },
  transcribe: async (file: Blob, filename = 'audio.webm') => {
    const fd = new FormData();
    fd.append('file', file, filename);
    const res = await fetch(
      `${typeof window !== 'undefined' ? '/api/v1' : process.env.NEXT_PUBLIC_API_URL + '/v1'}/audio/transcribe`,
      {
        method: 'POST',
        headers: { Authorization: `Bearer ${typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : (process.env.NEXT_PUBLIC_API_TOKEN ?? '')}` },
        body: fd,
      },
    );
    if (!res.ok) throw new Error(`STT ${res.status}: ${await res.text()}`);
    return res.json() as Promise<{ text: string }>;
  },
  catalog: (filters?: { kind?: VoiceKind; hosting?: 'cloud' | 'local' }) => {
    const qs = new URLSearchParams();
    if (filters?.kind) qs.set('kind', filters.kind);
    if (filters?.hosting) qs.set('hosting', filters.hosting);
    return request<{ drivers: VoiceCatalogEntry[]; count: number }>(`/voice/catalog${qs.toString() ? `?${qs}` : ''}`);
  },
  createProvider: (row: Partial<VoiceProviderRow>) =>
    request<VoiceProviderRow>('/voice/providers', { method: 'POST', body: JSON.stringify(row) }),
  updateProvider: (id: string, row: Partial<VoiceProviderRow>) =>
    request<{ status: string }>(`/voice/providers/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(row) }),
  deleteProvider: (id: string) =>
    request<{ status: string }>(`/voice/providers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  setDefault: (id: string) =>
    request<{ status: string }>(`/voice/providers/${encodeURIComponent(id)}/default`, { method: 'POST' }),
  testProvider: (id: string) =>
    request<{ success: boolean; bytes?: number; mime?: string; transcript?: string; error?: string }>(
      `/voice/providers/${encodeURIComponent(id)}/test`, { method: 'POST' }),
};

// Media
export type MediaKind = 'image' | 'video' | 'audio_gen';
export interface MediaCatalogField { name: string; label: string; type: 'text' | 'password'; required: boolean; placeholder?: string }
export interface MediaCatalogEntry { id: string; name: string; kind: MediaKind; hosting: 'cloud' | 'local'; hint?: string; fields: MediaCatalogField[]; default_base?: string; models?: string[] }
export interface MediaProviderRow { id: string; name: string; kind: MediaKind; driver: string; api_base?: string; settings?: Record<string, string>; enabled: boolean; is_default: boolean; fallback_order: number }

export const mediaProviders = {
  catalog: (kind?: MediaKind) =>
    request<{ drivers: MediaCatalogEntry[]; count: number }>(`/media/catalog${kind ? `?kind=${kind}` : ''}`),
  list: () => request<{ providers: MediaProviderRow[]; manager: Record<string, unknown> }>('/media/providers'),
  create: (row: Partial<MediaProviderRow> & { api_key?: string }) =>
    request<MediaProviderRow>('/media/providers', { method: 'POST', body: JSON.stringify(row) }),
  update: (id: string, row: Partial<MediaProviderRow> & { api_key?: string }) =>
    request<{ status: string }>(`/media/providers/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(row) }),
  delete: (id: string) =>
    request<{ status: string }>(`/media/providers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  setDefault: (id: string, kind: MediaKind) =>
    request<{ status: string }>(`/media/providers/${encodeURIComponent(id)}/default`, { method: 'POST', body: JSON.stringify({ kind }) }),
  test: (id: string) =>
    request<{ success: boolean; bytes?: number; url?: string; error?: string }>(
      `/media/providers/${encodeURIComponent(id)}/test`, { method: 'POST' }),
};

// A2A
export interface A2AAgentCard { name: string; description?: string; url: string; version?: string; documentationUrl?: string; capabilities?: Record<string, boolean>; skills?: { id: string; name: string; description?: string; tags?: string[] }[]; defaultInputModes?: string[]; defaultOutputModes?: string[]; authentication?: { schemes?: { scheme: string }[] }; provider?: { organization: string; url?: string }; [extra: string]: unknown }

export const a2a = {
  platformCard: async () => {
    const base = typeof window !== 'undefined' ? '' : (process.env.NEXT_PUBLIC_API_URL ?? '');
    const res = await fetch(`${base}/a2a/.well-known/agent.json`);
    if (!res.ok) throw new Error(`A2A ${res.status}`);
    return res.json() as Promise<A2AAgentCard>;
  },
  agentCardUrl: (key: string) => `/a2a/agents/${encodeURIComponent(key)}/.well-known/agent.json`,
  platformCardUrl: () => `/a2a/.well-known/agent.json`,
};

export const unifiedTimeline = {
  get: (agentId: string, limit = 100) =>
    request<{ agent_id: string; messages: any[]; total: number; channels: string[] }>(
      `/sessions/unified?agent_id=${agentId}&limit=${limit}`
    ),
};

// Content Feed
export interface ContentFeedItem {
  id: string;
  agent_id: string;
  agent_name: string;
  action_type: string;
  content: string;
  platforms: string[];
  channel: string;
  status: string;
  requested_at: string;
  metadata?: Record<string, any>;
}

export interface ContentFeedStats {
  pending: number;
  approved_today: number;
  rejected_today: number;
  total_30d: number;
}

export interface WebsiteProfile {
  id?: string;
  url: string;
  product_name: string;
  tagline: string;
  audience: string;
  competitors: string[];
  brand_voice: string;
  value_props: string[];
  keywords: string[];
  created_at?: string;
  updated_at?: string;
}

export interface AnalyticsOverview {
  content_produced_7d: number;
  content_produced_30d: number;
  approved_7d: number;
  rejected_7d: number;
  published_7d: number;
  approval_rate: number;
  posts_by_platform: Record<string, number>;
  posts_by_agent: { agent_id: string; agent_name: string; count: number }[];
}

export interface AnalyticsSEO {
  connected: boolean;
  clicks?: number;
  impressions?: number;
  ctr?: number;
  position?: number;
  connect_url?: string;
}

export interface AnalyticsTraffic {
  connected: boolean;
  sessions_7d?: number;
  users_7d?: number;
  pageviews_7d?: number;
  top_pages?: { path: string; views: number }[];
  connect_url?: string;
}

export interface TimelineEntry {
  date: string;
  produced: number;
  approved: number;
  published: number;
  rejected: number;
}

export const contentFeed = {
  list: (params?: { status?: string; channel?: string; agent_id?: string; limit?: number }) => {
    const qs = new URLSearchParams();
    if (params?.status) qs.set('status', params.status);
    if (params?.channel) qs.set('channel', params.channel);
    if (params?.agent_id) qs.set('agent_id', params.agent_id);
    if (params?.limit) qs.set('limit', String(params.limit));
    return request<ContentFeedItem[]>(`/content-feed?${qs}`);
  },
  stats: () => request<ContentFeedStats>('/content-feed/stats'),
  approve: (id: string) => request<{ status: string; post_id?: string }>(`/content-feed/${id}/approve`, { method: 'POST' }),
  reject: (id: string, reason?: string) => request<{ status: string }>(`/content-feed/${id}/reject`, { method: 'POST', body: JSON.stringify({ reason }) }),
  edit: (id: string, body: { content: string; platforms?: string[] }) => request<ContentFeedItem>(`/content-feed/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
};

// Onboarding
export const onboarding = {
  analyze: (url: string) => request<WebsiteProfile>('/onboarding/analyze', { method: 'POST', body: JSON.stringify({ url }) }),
  getProfile: () => request<WebsiteProfile>('/onboarding/profile'),
  updateProfile: (body: Partial<WebsiteProfile>) => request<WebsiteProfile>('/onboarding/profile', { method: 'PUT', body: JSON.stringify(body) }),
};

// Analytics
export const analytics = {
  overview: () => request<AnalyticsOverview>('/analytics/overview'),
  seo: () => request<AnalyticsSEO>('/analytics/seo'),
  traffic: () => request<AnalyticsTraffic>('/analytics/traffic'),
  timeline: (days?: number) => request<TimelineEntry[]>(`/analytics/timeline${days ? `?days=${days}` : ''}`),
};

// Admin resets + update
export const admin = {
  reset: (target: string) =>
    request<{ ok: boolean; target: string; deleted_rows: number }>(
      `/admin/reset/${encodeURIComponent(target)}`,
      { method: 'POST' },
    ),
  factoryReset: (password: string, confirm: string) =>
    request<{ ok: boolean }>(
      '/admin/factory-reset',
      { method: 'POST', body: JSON.stringify({ password, confirm }) },
    ),
  checkUpdate: () =>
    request<{ current: string; latest: string; up_to_date: boolean; release_url: string; changelog_url: string }>(
      '/admin/update/check',
    ),
  installUpdate: () =>
    request<{ ok: boolean; from: string; to: string; restart: boolean; message?: string }>(
      '/admin/update/install',
      { method: 'POST' },
    ),
};
