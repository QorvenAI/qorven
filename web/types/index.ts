// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Agent (Soul) — matches Go backend agent struct
export type SoulStatus = 'active' | 'inactive' | 'error';
export type SoulActivity = 'idle' | 'thinking' | 'running' | 'offline' | 'error';

export interface Soul {
  id: string;
  tenant_id: string;
  agent_key: string;
  display_name: string;
  avatar: string;
  role: string;
  title: string;
  manager_id: string | null;
  provider_id: string | null;
  model: string;
  system_prompt: string;
  temperature: number;
  context_window: number;
  max_tool_iterations: number;
  thinking_level: string; // off | medium | high
  tool_profile: string;
  tools_allowed: string[];
  tools_denied: string[];
  skills: string[];
  memory_enabled: boolean;
  memory_sharing: string;
  auto_compact: boolean;
  web_search_enabled: boolean;
  outbound_approval: string;
  credit_budget_cents: number;
  credit_used_cents: number;
  status: SoulStatus;
  runtime_mode?: string;
  can_delegate?: boolean;
  created_at: string;
  updated_at: string;
}

// Session
export interface Session {
  id: string;
  agent_id: string;
  label: string;
  summary: string;
  channel: string;
  status: string;
  created_at: string;
  messages?: Message[];
}

export interface Message {
  role: 'user' | 'assistant' | 'system' | 'tool';
  content: string;
  thinking?: string;
  model?: string;
  sources?: {index: string; title: string; url: string}[];
  widgets?: any[];
  parts?: any[];
  tool_calls?: ToolCall[];
  timestamp?: string;
  channel?: string;
  sender_name?: string;
}

export interface ToolCall {
  name: string;
  arguments: string;
  result?: string;
}

// WebSocket Events — matches rtHub broadcast types
export type WSEventType =
  | 'new_message'
  | 'soul_activity'
  | 'soul_completed'
  | 'task_updated'
  | 'cron_fired'
  | 'soul_created'
  | 'notification'
  | 'stream_start'
  | 'stream_delta'
  | 'stream_end'
  | 'text_delta'
  | 'thinking_delta'
  | 'tool_start'
  | 'tool_result'
  | 'done'
  | 'error'
  | 'approval_required'
  | 'permission.requested'
  | 'permission.replied'
  | 'room_message'
  | 'room_typing_start'
  | 'room_typing_stop'
  | 'graph.node_started'
  | 'graph.node_completed'
  | 'graph.node_paused'
  | 'graph.node_failed'
  | 'agent.progress'
  | 'room_message'
  | 'browser_frame'          // agent-to-user live Chromium preview
  | 'prompt_guard_blocked'   // prompt-injection gate fired
  | 'ticket_updated'
  | 'ticket_comment'
  | 'project_updated'
  | 'budget_warning'
  | 'service_health'
  | 'github.pr_ready'
  | 'github.commit_pending'
  | 'runtime_state_changed'
  | 'task_progress'
  | 'task_done'
  | 'task_blocked'
  | 'task_iteration_start'
  | 'synthesis_triggered';

export interface WSEvent {
  type: WSEventType;
  data: unknown;
}

export interface LiveEvent {
  id: string;
  type: WSEventType;
  agent_id?: string;
  soul_key?: string;
  detail?: string;
  timestamp: number;
  data: unknown;
}

// Channels
export type ChannelType =
  | 'telegram' | 'discord' | 'slack' | 'email' | 'whatsapp'
  | 'sms' | 'teams' | 'github' | 'webchat' | 'webhook'
  | 'signal' | 'imessage' | 'facebook' | 'line' | 'zalo'
  | 'feishu' | 'dingtalk' | 'wecom' | 'matrix' | 'mattermost';

export interface Channel {
  id: string;
  agent_id: string;
  channel_type: ChannelType;
  name: string;
  config: Record<string, unknown>;
  enabled: boolean;
  status: string;
}

// Provider
export interface Provider {
  id: string;
  name: string;
  provider_type: string;
  api_base: string;
  models: string[];
  status: string;
}

// Skill
export interface Skill {
  id: string;
  name: string;
  slug: string;
  description: string;
  category: string;
  author: string;
  install_count: number;
  rating: number;
}

// CronJob
export interface CronJob {
  id: string;
  agent_id: string;
  expression: string;
  task: string;
  status: string;
  last_run: string;
  next_run: string;
  executor_agent_id: string;
}

// Notification
export interface Notification {
  id: string;
  agent_id: string;
  type: string;
  title: string;
  highlight: string;
  source: string;
  read: boolean;
  created_at: string;
}

// Navigation
export type RailSection =
  | 'home' | 'dashboard'
  | 'souls' | 'code' | 'sessions' | 'live'
  | 'tasks' | 'drive' | 'connectors' | 'workflows'
  | 'teams' | 'org-chart'
  | 'mcp' | 'kg' | 'heartbeat'
  | 'a2a' | 'agents' | 'scenarios' | 'plans' | 'labs'
  | 'models' | 'settings' | 'terminal' | 'social'
  | 'github' | 'analytics'
  | 'apps';

// ─── Work Goals ──────────────────────────────────────────────────────────────

export interface WorkGoal {
  id: string;
  tenant_id: string;
  title: string;
  description: string;
  parent_id: string | null;
  order_index: number;
  status: 'open' | 'done';
  created_at: string;
  updated_at: string;
}

export interface WorkGoalTreeNode extends WorkGoal {
  children: WorkGoalTreeNode[];
  ticket_count: number;
  done_count: number;
}

export type TicketStatus   = 'todo' | 'in_progress' | 'blocked' | 'done';
export type TicketPriority = 'critical' | 'high' | 'normal' | 'low';

export interface Ticket {
  id: string;
  tenant_id: string;
  slug: string;
  title: string;
  description: string;
  status: TicketStatus;
  priority: TicketPriority;
  assigned_agent_id: string | null;
  goal_id: string | null;
  created_at: string;
  updated_at: string;
}

export interface TicketComment {
  id: string;
  ticket_id: string;
  author_type: 'user' | 'agent';
  author_id: string;
  body: string;
  created_at: string;
}

export interface TicketFile {
  id: string;
  ticket_id: string;
  path: string;
  operation: 'created' | 'modified' | 'deleted';
  touched_at: string;
}

// ─── Phase 2 — Project Inception ─────────────────────────────────────────────

export type ProjectQuality = 'mvp' | 'production' | 'enterprise';
export type ProjectBriefStatus = 'intake' | 'proposed' | 'approved' | 'active' | 'done' | 'cancelled';

export interface ProposedAgent {
  role: string;
  display_name: string;
  model: string;
  model_label: string;
  tasks: string[];
  est_min_cents: number;
  est_max_cents: number;
}

export interface ProposedTask {
  title: string;
  role: string;
  priority: string;
  blocked_by: string[];
  est_min_cents: number;
  est_max_cents: number;
}

export interface TeamProposal {
  agents: ProposedAgent[];
  tasks: ProposedTask[];
  est_min_cents: number;
  est_max_cents: number;
  reasoning: string;
}

export interface BriefAgent {
  id: string;
  display_name: string;
  role: string;
  model: string;
  status: string;
  credit_budget_cents?: number;
  credit_used_cents: number;
}

export interface ProjectBrief {
  id: string;
  tenant_id: string;
  title: string;
  idea: string;
  stack: string;
  budget_cents: number;
  timeline: string;
  quality: ProjectQuality;
  status: ProjectBriefStatus;
  proposal?: TeamProposal;
  goal_id?: string | null;
  created_at: string;
  updated_at: string;
}
