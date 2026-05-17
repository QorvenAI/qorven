// Copyright 2026 Tekky AI Academy LLP. Licensed under FSL-1.1-ALv2.

import { request, listRequest } from './api-core';

export interface GitHubPR {
  number: number;
  title: string;
  html_url: string;
  state: string;
  head: { ref: string; sha: string };
  base: { ref: string };
  user: { login: string; avatar_url: string };
  draft: boolean;
  created_at: string;
  updated_at: string;
  mergeable?: boolean;
  // CI status injected by backend proxy
  ci_status?: 'pending' | 'success' | 'failure' | 'unknown';
  checks?: GitHubCheck[];
}

export interface GitHubCheck {
  id: number;
  name: string;
  status: string;
  conclusion: string | null;
  started_at: string;
  completed_at: string | null;
  html_url: string;
}

export interface GitHubIssue {
  number: number;
  title: string;
  html_url: string;
  state: string;
  body: string;
  user: { login: string; avatar_url: string };
  assignee: { login: string; avatar_url: string } | null;
  labels: { name: string; color: string }[];
  created_at: string;
  updated_at: string;
}

export interface GitHubTask {
  id: string;
  phase: string;
  branch: string;
  pr_number?: number;
  pr_url?: string;
  issue_number?: number;
  agent_id?: string;
  error?: string;
  created_at: string;
  updated_at: string;
}

export interface RepoStatus {
  owner: string;
  repo: string;
  default_branch: string;
  open_prs: number;
  open_issues: number;
  connected: boolean;
}

export interface ConnectRepoResponse {
  webhook_url: string;
  webhook_secret: string;
  owner: string;
  repo: string;
  default_branch: string;
}

export const githubApi = {
  listPRs: (owner: string, repo: string, state: 'open' | 'closed' | 'all' = 'open') =>
    listRequest<GitHubPR>(`/github/${owner}/${repo}/pulls?state=${state}`),

  listIssues: (owner: string, repo: string, state: 'open' | 'closed' | 'all' = 'open') =>
    listRequest<GitHubIssue>(`/github/${owner}/${repo}/issues?state=${state}`),

  listTasks: () =>
    listRequest<GitHubTask>('/github/tasks'),

  getTask: (id: string) =>
    request<GitHubTask>(`/github/tasks/${id}`),

  advanceTask: (id: string, phase: string) =>
    request<GitHubTask>(`/github/tasks/${id}/advance`, {
      method: 'POST',
      body: JSON.stringify({ phase }),
    }),

  blockTask: (id: string, reason: string) =>
    request<GitHubTask>(`/github/tasks/${id}/block`, {
      method: 'POST',
      body: JSON.stringify({ reason }),
    }),

  mergePR: (owner: string, repo: string, prNum: number, method: 'merge' | 'squash' | 'rebase' = 'squash') =>
    request<{ merged: boolean; sha: string; message: string }>(
      `/github/${owner}/${repo}/pulls/${prNum}/merge`,
      { method: 'POST', body: JSON.stringify({ merge_method: method }) },
    ),

  closeIssue: (owner: string, repo: string, num: number) =>
    request<{ number: number; state: string }>(
      `/github/${owner}/${repo}/issues/${num}/close`,
      { method: 'POST' },
    ),

  listPRChecks: (owner: string, repo: string, prNum: number) =>
    listRequest<GitHubCheck>(`/github/${owner}/${repo}/pulls/${prNum}/checks`),

  connectRepo: (projectId: string, owner: string, repo: string, defaultBranch = 'main') =>
    request<ConnectRepoResponse>(`/projects/${projectId}/github/connect`, {
      method: 'POST',
      body: JSON.stringify({ owner, repo, default_branch: defaultBranch }),
    }),

  disconnectRepo: (projectId: string) =>
    request<void>(`/projects/${projectId}/github/connect`, { method: 'DELETE' }),

  getRepoStatus: (projectId: string) =>
    request<RepoStatus>(`/projects/${projectId}/github/status`),
};
