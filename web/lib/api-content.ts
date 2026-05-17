// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { request, listRequest } from './api-core';

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
  save: (platformId: string, token: string, label?: string) =>
    request<any>(`/connections/${platformId}`, {
      method: 'POST',
      body: JSON.stringify({ token, label: label || 'default' }),
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

export interface MailMessage {
  id: string;
  agent_id?: string;
  from_address: string;
  from_name?: string;
  to_addresses?: string[];
  subject: string;
  body_text?: string;
  body_html?: string;
  direction: 'inbound' | 'outbound';
  status: string;
  agent_decision?: string;
  thread_id?: string;
  is_read?: boolean;
  created_at: string;
  updated_at?: string;
}

export const mail = {
  inbox: (agentId?: string) => {
    const qs = agentId ? `?agent_id=${encodeURIComponent(agentId)}` : '';
    return listRequest<MailMessage>(`/mail/inbox${qs}`);
  },
  sent: (agentId?: string) => {
    const qs = agentId ? `?agent_id=${encodeURIComponent(agentId)}` : '';
    return listRequest<MailMessage>(`/mail/sent${qs}`);
  },
  folder: (folder: string) => listRequest<any>(`/mail/${folder}`),
  send: (body: { to: string[]; subject: string; body: string }) =>
    request<void>('/mail/send', { method: 'POST', body: JSON.stringify(body) }),
  thread: (threadId: string) => listRequest<MailMessage>(`/mail/thread/${threadId}`),
  getMessage: (id: string) => request<MailMessage>(`/mail/messages/${id}`),
  markRead: (id: string, read: boolean) =>
    request<void>(`/mail/messages/${id}/read`, { method: 'POST', body: JSON.stringify({ read }) }),
  identities: () => request<MailIdentity[]>('/mail/identities'),
  createIdentity: (body: { agent_id?: string; address: string; display_name: string; identity_type?: string }) =>
    request<MailIdentity>('/mail/identities', { method: 'POST', body: JSON.stringify(body) }),
  updateIdentity: (id: string, body: Partial<MailIdentity> & { smtp_pass?: string; imap_pass?: string }) =>
    request<void>(`/mail/identities/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  aliases: () => request<MailAlias[]>('/mail/aliases'),
  createAlias: (body: { alias_address: string; target_agent_id: string; can_send_as: boolean; can_receive: boolean }) =>
    request<{ id: string }>('/mail/aliases', { method: 'POST', body: JSON.stringify(body) }),
  deleteAlias: (id: string) =>
    request<void>(`/mail/aliases/${id}`, { method: 'DELETE' }),
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
  listAutoPosts: (agentId?: string) =>
    request<any[]>(`/social/autoposts${agentId ? `?agent_id=${agentId}` : ''}`),
  createAutoPost: (body: Record<string, unknown>) =>
    request<any>('/social/autoposts', { method: 'POST', body: JSON.stringify(body) }),
  deleteAutoPost: (id: string) => request<void>(`/social/autoposts/${id}`, { method: 'DELETE' }),
  calendar: (agentId?: string) =>
    request<{ entries: any[]; total: number; stats: Record<string, number> }>(
      `/social/calendar${agentId ? `?agent_id=${agentId}` : ''}`
    ),
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
