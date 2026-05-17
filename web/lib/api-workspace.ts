// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { request, listRequest } from './api-core';
import type { Skill, WorkGoal, WorkGoalTreeNode, Ticket, TicketComment, TicketFile, TicketPriority, ProjectBrief, TeamProposal, ProjectQuality, BriefAgent } from '@/types';

// Skills
export const skills = {
  list: () => listRequest<Skill>('/skills'),
  marketplace: (params?: { category?: string; search?: string }) => {
    const q = new URLSearchParams(params as Record<string, string>).toString();
    return listRequest<Skill>(`/marketplace/skills${q ? `?${q}` : ''}`);
  },
  install: (slug: string, agentId: string) =>
    request<void>(`/marketplace/skills/${slug}/install`, { method: 'POST', body: JSON.stringify({ agent_id: agentId }) }),
  uninstall: (slug: string, agentId: string) =>
    request<{ status: string; slug: string }>(`/marketplace/skills/${encodeURIComponent(slug)}/uninstall`, { method: 'POST', body: JSON.stringify({ agent_id: agentId }) }),
  delete: (id: string) =>
    request<{ status: string }>(`/skills/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  agentSkills: (agentId: string) =>
    request<{ skills: Skill[]; count: number }>(`/agents/${encodeURIComponent(agentId)}/skills`).then(r => r.skills ?? []),
  crystallized: (agentId: string) => listRequest<unknown>(`/skills/crystallized/${agentId}`),
  promote: (id: string, scope: 'shared' | 'marketplace') =>
    request<{ status: string; scope: string }>(`/skills/crystallized/${encodeURIComponent(id)}/promote`, { method: 'POST', body: JSON.stringify({ scope }) }),
  pin: (id: string, pinned: boolean) =>
    request<{ id: string; pinned: boolean }>(`/skills/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify({ pinned }) }),
};

// Tasks
export const tasks = {
  list: (agentId?: string) => listRequest<any>(agentId ? `/tasks?agent_id=${agentId}` : '/tasks'),
  create: (body: Record<string, unknown>) => request<any>('/tasks', { method: 'POST', body: JSON.stringify(body) }),
  update: (id: string, body: Record<string, unknown>) => request<void>(`/tasks/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  updateStatus: (id: string, status: string) => request<void>(`/tasks/${id}/status`, { method: 'PUT', body: JSON.stringify({ status }) }),
  pause: (id: string) =>
    request<{ status: string; task_id: string }>(`/tasks/${id}/pause`, { method: 'POST' }),
  resume: (id: string) =>
    request<{ status: string; task_id: string }>(`/tasks/${id}/resume`, { method: 'POST' }),
  message: (id: string, message: string) =>
    request<{ status: string; task_id: string }>(
      `/tasks/${id}/message`,
      { method: 'POST', body: JSON.stringify({ message }) }
    ),
};

// Calendar
export const calendarApi = {
  list: (start?: string, end?: string, agentId?: string) => {
    const params = new URLSearchParams();
    if (start) params.set('start', start);
    if (end) params.set('end', end);
    if (agentId) params.set('agent_id', agentId);
    return request<any[]>(`/calendar/events?${params}`);
  },
  create: (body: Record<string, unknown>) => request<any>('/calendar/events', { method: 'POST', body: JSON.stringify(body) }),
  update: (id: string, body: Record<string, unknown>) => request<void>(`/calendar/events/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  delete: (id: string) => request<void>(`/calendar/events/${id}`, { method: 'DELETE' }),
};

// Goals
export interface Goal {
  id: string;
  title: string;
  description?: string;
  unit?: string;
  status?: string;
  due_at?: string;
  agent_name?: string;
  agent_id?: string;
  target_value?: number;
  current_value?: number;
}

export const goals = {
  list: () => request<{ goals: Goal[] }>('/goals'),
  create: (body: { agent_id?: string; title: string; description?: string; target_value?: number; unit?: string }) =>
    request<{ id: string }>('/goals', { method: 'POST', body: JSON.stringify(body) }),
};

// Work Goals
export const workGoals = {
  list:   () => request<WorkGoal[]>('/work-goals'),
  tree:   () => request<WorkGoalTreeNode[]>('/work-goals/tree'),
  create: (body: { title: string; description?: string; parent_id?: string | null; order_index?: number }) =>
    request<WorkGoal>('/work-goals', { method: 'POST', body: JSON.stringify(body) }),
  update: (id: string, body: Partial<Pick<WorkGoal, 'title' | 'description' | 'status' | 'order_index'>>) =>
    request<WorkGoal>(`/work-goals/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(body) }),
  delete: (id: string) => request<void>(`/work-goals/${encodeURIComponent(id)}`, { method: 'DELETE' }),
};

// Tickets
const qs = (p?: Record<string, string | undefined>) =>
  p ? '?' + new URLSearchParams(Object.entries(p).filter(([, v]) => !!v) as [string, string][]).toString() : '';

export const tickets = {
  list:     (params?: { status?: string; priority?: string; agent_id?: string; goal_id?: string }) =>
    request<Ticket[]>('/tickets' + qs(params)),
  create:   (body: { title: string; description?: string; priority?: TicketPriority; goal_id?: string | null }) =>
    request<Ticket>('/tickets', { method: 'POST', body: JSON.stringify(body) }),
  update:   (id: string, body: Partial<Pick<Ticket, 'title' | 'description' | 'status' | 'priority' | 'goal_id'>>) =>
    request<Ticket>(`/tickets/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(body) }),
  delete:   (id: string) => request<void>(`/tickets/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  assign:   (id: string, agentId: string) =>
    request<{ ticket: Ticket; heartbeat: string }>(`/tickets/${encodeURIComponent(id)}/assign`, { method: 'POST', body: JSON.stringify({ agent_id: agentId }) }),
  comments: (id: string) => request<TicketComment[]>(`/tickets/${encodeURIComponent(id)}/comments`),
  comment:  (id: string, body: string) =>
    request<TicketComment>(`/tickets/${encodeURIComponent(id)}/comments`, { method: 'POST', body: JSON.stringify({ body }) }),
  files:    (id: string) => request<TicketFile[]>(`/tickets/${encodeURIComponent(id)}/files`),
};

// Project Briefs
export const projectBriefs = {
  list: () => request<ProjectBrief[]>('/project-briefs'),
  create: (body: { title: string; idea: string; stack?: string; budget_cents?: number; timeline?: string; quality?: ProjectQuality }) =>
    request<ProjectBrief>('/project-briefs', { method: 'POST', body: JSON.stringify(body) }),
  update: (id: string, body: Partial<Pick<ProjectBrief, 'title' | 'idea' | 'stack' | 'budget_cents' | 'timeline' | 'quality' | 'status'>>) =>
    request<ProjectBrief>(`/project-briefs/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(body) }),
  propose: (id: string) =>
    request<ProjectBrief>(`/project-briefs/${encodeURIComponent(id)}/propose`, { method: 'POST', body: '{}' }),
  approve: (id: string) =>
    request<{ brief: ProjectBrief; agents: Record<string, string>; tickets: Record<string, string> }>(
      `/project-briefs/${encodeURIComponent(id)}/approve`, { method: 'POST', body: '{}' }),
  team: (id: string) =>
    request<{ agents: BriefAgent[] }>(`/project-briefs/${encodeURIComponent(id)}/team`),
};

// Workspaces / Templates
export const workspaces = {
  listTemplates: () => request<any[]>('/templates'),
  listInstalled: () => request<any[]>('/templates/installed'),
  install: (templateId: string) =>
    request<any>('/templates/install', { method: 'POST', body: JSON.stringify({ template_id: templateId }) }),
  getDashboard: (templateId: string) => request<any>(`/templates/${templateId}/dashboard`),
  selfBuild: (description: string, templateId?: string) =>
    request<any>('/templates/self-build', { method: 'POST', body: JSON.stringify({ description, template_id: templateId }) }),
  exportTemplate: (id: string) => request<any>(`/templates/${id}/export`),
};

// Rooms
export const rooms = {
  list: () => request<any>('/rooms'),
  get: (id: string) => request<any>(`/rooms/${id}`),
  create: (body: { name: string; display_name: string; description?: string; members?: string[] }) => request<{ id: string }>('/rooms', { method: 'POST', body: JSON.stringify(body) }),
  delete: (id: string) => request<void>(`/rooms/${id}`, { method: 'DELETE' }),
  messages: (id: string) => request<any>(`/rooms/${id}/messages`),
  sendMessage: (id: string, content: string, messageType?: string) =>
    request<any>(`/rooms/${id}/messages`, { method: 'POST', body: JSON.stringify({
      content, sender_type: 'user', sender_id: 'user', message_type: messageType || 'text'
    }) }),
  addMember: (id: string, agentId: string) => request<void>(`/rooms/${id}/members`, { method: 'POST', body: JSON.stringify({ agent_id: agentId }) }),
  removeMember: (id: string, agentId: string) => request<void>(`/rooms/${id}/members/${encodeURIComponent(agentId)}`, { method: 'DELETE' }),
  decisions: (id: string) => request<any>(`/rooms/${id}/decisions`),
  pinDecision: (id: string, content: string, decidedBy?: string) =>
    request<any>(`/rooms/${id}/decisions`, { method: 'POST', body: JSON.stringify({ content, decided_by: decidedBy }) }),
  minutes: (id: string) => request<any>(`/rooms/${id}/minutes`),
  generateMinutes: (id: string) => request<any>(`/rooms/${id}/minutes/generate`, { method: 'POST' }),
  typing: (id: string, agentId: string, isTyping: boolean) =>
    request<void>(`/rooms/${id}/typing`, { method: 'POST', body: JSON.stringify({ agent_id: agentId, typing: isTyping }) }),
  tasks: (id: string) => request<any>(`/rooms/${id}/tasks`),
  createTask: (id: string, body: { title: string; assigned_to: string; description?: string; due_at?: string }) =>
    request<any>(`/rooms/${id}/tasks`, { method: 'POST', body: JSON.stringify(body) }),
  updateTask: (id: string, taskId: string, status: string) =>
    request<any>(`/rooms/${id}/tasks/${taskId}`, { method: 'PUT', body: JSON.stringify({ status }) }),
  org: (id: string) => request<any>(`/rooms/${id}/org`),
};
