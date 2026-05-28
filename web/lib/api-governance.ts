// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { request } from './api-core';

export interface Designation {
  id: string;
  tenant_id: string;
  position_name: string;
  department: string;
  org_layer: number;
  nature_of_work: string;
  skill_family: string;
  model_tier: string;
  tool_permissions: string[];
  budget_usd_monthly: number;
  can_create_subagents: boolean;
  can_approve_actions: boolean;
  approval_scope: string[];
  created_at: string;
  updated_at: string;
}

export interface SkillFamily {
  id: string;
  name: string;
  description: string;
  capabilities: string[];
  tool_permissions: string[];
}

export interface ApprovalRule {
  id: string;
  action_type: string;
  threshold_usd: number;
  approver_role: string;
  approver_level: number;
  requires_human: boolean;
  auto_approve_below: number;
  priority: number;
  enabled: boolean;
}

export interface ApprovalRequest {
  id: string;
  action_type: string;
  requester_agent_id: string;
  context: Record<string, any>;
  status: string;
  decided_by: string;
  decided_at: string;
  created_at: string;
}

export interface PolicyEvent {
  id: string;
  policy_id: string;
  policy_name: string;
  trigger_event: string;
  action_taken: string;
  agent_id: string;
  context: Record<string, any>;
  created_at: string;
}

export interface GovException {
  id: string;
  exception_type: string;
  severity: string;
  source_agent_id: string;
  description: string;
  context: Record<string, any>;
  acknowledged: boolean;
  resolved: boolean;
  resolved_by: string;
  resolution: string;
  created_at: string;
}

export interface ExceptionStats {
  total: number;
  critical: number;
  warning: number;
  info: number;
  unresolved: number;
}

export const governanceApi = {
  listDesignations: () => request<{ designations: Designation[]; count: number }>('/governance/designations'),
  getDesignation: (id: string) => request<Designation>(`/governance/designations/${id}`),
  upsertDesignation: (d: Partial<Designation>) => request<{ status: string }>('/governance/designations', { method: 'POST', body: JSON.stringify(d) }),
  deleteDesignation: (id: string) => request<{ status: string }>(`/governance/designations/${id}`, { method: 'DELETE' }),
  listSkillFamilies: () => request<{ skill_families: SkillFamily[] }>('/governance/skill-families'),
  listApprovalRules: () => request<{ rules: ApprovalRule[] }>('/governance/approvals/rules'),
  listPendingApprovals: () => request<{ requests: ApprovalRequest[] }>('/governance/approvals/pending'),
  decideApproval: (id: string, status: string, reason: string) => request<{ status: string }>(`/governance/approvals/${id}/decide`, { method: 'POST', body: JSON.stringify({ status, reason }) }),
  listPolicyEvents: () => request<{ events: PolicyEvent[] }>('/governance/policies/events'),
  listExceptions: () => request<{ exceptions: GovException[]; stats: ExceptionStats }>('/governance/exceptions'),
  resolveException: (id: string, resolution: string) => request<{ status: string }>(`/governance/exceptions/${id}/resolve`, { method: 'POST', body: JSON.stringify({ resolution }) }),
};
