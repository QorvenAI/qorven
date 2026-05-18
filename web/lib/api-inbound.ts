// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { request } from './api-core';

export type ActionMode = 'fully_autonomous' | 'draft_and_approve' | 'draft_only' | 'context_only' | 'drop';

export interface InboundConfig {
  agent_id: string;
  default_mode: ActionMode;
  unknown_sender_mode: ActionMode;
  spam_action: 'drop' | 'context_only';
  notification_channel: string;
  notification_target: string;
  briefing_enabled: boolean;
  briefing_time: string;
  briefing_timezone: string;
}

export interface InboundRule {
  id: string;
  priority: number;
  match_type: 'contact' | 'domain' | 'channel' | 'keyword' | 'default';
  match_value: string;
  mode: string;
  status: 'active' | 'pending_confirmation';
  reason: string;
  created_at: string;
}

export interface DraftReply {
  id: string;
  agent_id: string;
  sender_id: string;
  sender_name: string;
  channel: string;
  original_message: string;
  history_summary: string;
  draft_content: string;
  status: string;
  created_at: string;
}

export const getInboundConfig = (agentId: string) =>
  request<InboundConfig>(`/agents/${agentId}/inbound-config`);

export const putInboundConfig = (agentId: string, cfg: Partial<InboundConfig>) =>
  request<{ ok: string }>(`/agents/${agentId}/inbound-config`, {
    method: 'PUT',
    body: JSON.stringify(cfg),
  });

export const listInboundRules = (agentId: string) =>
  request<InboundRule[]>(`/agents/${agentId}/inbound-rules`);

export const createInboundRule = (
  agentId: string,
  rule: Pick<InboundRule, 'priority' | 'match_type' | 'match_value' | 'mode'>
) =>
  request<{ id: string }>(`/agents/${agentId}/inbound-rules`, {
    method: 'POST',
    body: JSON.stringify(rule),
  });

export const updateInboundRule = (
  agentId: string,
  ruleId: string,
  rule: Pick<InboundRule, 'priority' | 'match_type' | 'match_value' | 'mode'>
) =>
  request<{ ok: string }>(`/agents/${agentId}/inbound-rules/${ruleId}`, {
    method: 'PUT',
    body: JSON.stringify(rule),
  });

export const deleteInboundRule = (agentId: string, ruleId: string) =>
  request<{ ok: string }>(`/agents/${agentId}/inbound-rules/${ruleId}`, {
    method: 'DELETE',
  });

export const confirmInboundRule = (agentId: string, ruleId: string) =>
  request<{ status: string }>(`/agents/${agentId}/inbound-rules/${ruleId}/confirm`, { method: 'POST' });

export const discardInboundRule = (agentId: string, ruleId: string) =>
  request<{ status: string }>(`/agents/${agentId}/inbound-rules/${ruleId}/discard`, { method: 'POST' });

export const listDrafts = () => request<DraftReply[]>('/drafts');
export const getDraft = (id: string) => request<DraftReply>(`/drafts/${id}`);
export const sendDraft = (id: string) =>
  request<{ ok: string }>(`/drafts/${id}/send`, { method: 'POST' });
export const discardDraft = (id: string) =>
  request<{ ok: string }>(`/drafts/${id}/discard`, { method: 'POST' });
export const editDraft = (id: string, content: string) =>
  request<{ ok: string }>(`/drafts/${id}/edit`, {
    method: 'POST',
    body: JSON.stringify({ content }),
  });
