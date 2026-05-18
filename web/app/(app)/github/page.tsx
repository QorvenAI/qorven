'use client';

// Copyright 2026 Tekky AI Academy LLP. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useState } from 'react';
import {
  GitPullRequest, GitMerge, AlertCircle, CheckCircle2, XCircle,
  Clock, RefreshCw, Link2, Plus, Tag, Loader2,
  ExternalLink, CheckCheck, GitBranch, Layers, Settings,
  ChevronRight, GitCommit, ArrowRight, Circle, X,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { githubApi, type GitHubPR, type GitHubCheck, type GitHubIssue, type GitHubTask, type RepoStatus } from '@/lib/api-github';
import { request } from '@/lib/api-core';
import { useStore } from '@/store';
import { toast } from 'sonner';

// ─── helpers ────────────────────────────────────────────────────────────────

function relTime(iso: string): string {
  if (!iso) return '—';
  const diff = Date.now() - Date.parse(iso);
  if (!Number.isFinite(diff)) return iso;
  if (diff < 60_000)    return `${Math.round(diff / 1_000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
  return new Date(iso).toLocaleDateString();
}

const CI_META: Record<string, { label: string; icon: typeof CheckCircle2; cls: string }> = {
  success: { label: 'Passed',  icon: CheckCircle2, cls: 'text-emerald-400' },
  failure: { label: 'Failed',  icon: XCircle,      cls: 'text-destructive' },
  pending: { label: 'Running', icon: Loader2,      cls: 'text-amber-400 animate-spin' },
  unknown: { label: 'Unknown', icon: Clock,        cls: 'text-muted-foreground' },
};

const TASK_PHASE_META: Record<string, { label: string; cls: string }> = {
  reading:      { label: 'Reading',      cls: 'text-muted-foreground' },
  branching:    { label: 'Branching',    cls: 'text-blue-400' },
  coding:       { label: 'Coding',       cls: 'text-primary' },
  testing:      { label: 'Testing',      cls: 'text-amber-400' },
  fixing:       { label: 'Fixing',       cls: 'text-orange-400' },
  opening_pr:   { label: 'Opening PR',   cls: 'text-purple-400' },
  awaiting_ci:  { label: 'Awaiting CI',  cls: 'text-cyan-400' },
  complete:     { label: 'Complete',     cls: 'text-emerald-400' },
  blocked:      { label: 'Blocked',      cls: 'text-destructive' },
};

const PR_COLUMNS = [
  { key: 'open',   label: 'Open',      color: 'border-blue-500/40',    filter: (pr: GitHubPR) => pr.state === 'open' && !pr.draft },
  { key: 'review', label: 'In Review', color: 'border-purple-500/40',  filter: (pr: GitHubPR) => pr.state === 'open' && pr.draft },
  { key: 'ci',     label: 'CI Check',  color: 'border-amber-500/40',   filter: (pr: GitHubPR) => pr.state === 'open' && pr.ci_status === 'pending' },
  { key: 'merged', label: 'Merged',    color: 'border-emerald-500/40', filter: (pr: GitHubPR) => pr.state === 'closed' },
];

interface ConnectedProject {
  id: string;
  name: string;
  github_owner: string;
  github_repo: string;
}

// ─── CI badge ────────────────────────────────────────────────────────────────

function CIBadge({ status }: { status?: string }) {
  const meta = CI_META[status ?? 'unknown'] ?? CI_META.unknown!;
  const Icon = meta.icon;
  return (
    <span className={cn('flex items-center gap-1 text-xs', meta.cls)}>
      <Icon className="h-3 w-3" />
      {meta.label}
    </span>
  );
}

// ─── PR Detail Drawer ─────────────────────────────────────────────────────────

interface PRDetailProps {
  pr: GitHubPR;
  owner: string;
  repo: string;
  allPRs: GitHubPR[];
  onClose: () => void;
  onMerged: () => void;
}

function PRDetail({ pr, owner, repo, allPRs, onClose, onMerged }: PRDetailProps) {
  const [checks, setChecks] = useState<GitHubCheck[]>([]);
  const [loadingChecks, setLoadingChecks] = useState(true);
  const [merging, setMerging] = useState(false);
  const [mergeMethod, setMergeMethod] = useState<'squash' | 'merge' | 'rebase'>('squash');

  useEffect(() => {
    setLoadingChecks(true);
    githubApi.listPRChecks(owner, repo, pr.number)
      .then(setChecks)
      .catch(() => setChecks([]))
      .finally(() => setLoadingChecks(false));
  }, [pr.number, owner, repo]);

  const handleMerge = async () => {
    setMerging(true);
    try {
      await githubApi.mergePR(owner, repo, pr.number, mergeMethod);
      toast.success(`PR #${pr.number} merged`);
      onMerged();
      onClose();
    } catch (e: any) {
      toast.error(e.message ?? 'Merge failed');
    } finally {
      setMerging(false);
    }
  };

  // detect stack: PRs whose base branch is this PR's head branch
  const stackChildren = allPRs.filter(
    (p) => p.base.ref === pr.head.ref && p.number !== pr.number && p.state === 'open',
  );
  const stackParent = allPRs.find(
    (p) => p.head.ref === pr.base.ref && p.number !== pr.number && p.state === 'open',
  );

  return (
    // Backdrop
    <div className="fixed inset-0 z-50 flex" onClick={onClose}>
      {/* right-side drawer — stop propagation so clicking inside doesn't close */}
      <div className="ml-auto w-full max-w-xl h-full bg-background border-l border-border overflow-y-auto flex flex-col"
           onClick={(e) => e.stopPropagation()}>

        {/* Header */}
        <div className="flex items-start justify-between gap-3 px-5 py-4 border-b border-border shrink-0">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-xs text-muted-foreground mb-1">
              <span className={cn(
                'inline-flex items-center gap-1 rounded-full px-2 py-0.5 font-medium',
                pr.state === 'open' ? 'bg-emerald-500/10 text-emerald-400' : 'bg-purple-500/10 text-purple-400',
              )}>
                <GitPullRequest className="h-3 w-3" />
                {pr.state === 'open' ? 'Open' : 'Merged'}
              </span>
              <span className="font-mono">#{pr.number}</span>
            </div>
            <h2 className="text-base font-semibold leading-snug">{pr.title}</h2>
            <div className="flex items-center gap-2 mt-1.5 text-xs text-muted-foreground">
              <img src={pr.user.avatar_url} alt="" className="h-4 w-4 rounded-full" />
              <span>{pr.user.login}</span>
              <span>·</span>
              <span>{relTime(pr.updated_at)}</span>
            </div>
          </div>
          <button onClick={onClose} className="shrink-0 p-1 rounded hover:bg-muted transition-colors">
            <X className="h-4 w-4 text-muted-foreground" />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto">
          {/* Branch info */}
          <div className="px-5 py-3 border-b border-border flex items-center gap-2 text-xs text-muted-foreground">
            <GitBranch className="h-3.5 w-3.5 shrink-0" />
            <span className="font-mono text-foreground">{pr.head.ref}</span>
            <ArrowRight className="h-3 w-3 shrink-0" />
            <span className="font-mono">{pr.base.ref}</span>
            <a href={pr.html_url} target="_blank" rel="noopener noreferrer"
               className="ml-auto flex items-center gap-1 hover:text-foreground transition-colors">
              Open on GitHub <ExternalLink className="h-3 w-3" />
            </a>
          </div>

          {/* Stack panel */}
          {(stackParent || stackChildren.length > 0) && (
            <div className="px-5 py-3 border-b border-border">
              <p className="text-xs font-medium text-muted-foreground mb-2">Stack</p>
              <div className="space-y-1 text-xs">
                {stackParent && (
                  <div className="flex items-center gap-2 text-muted-foreground">
                    <Circle className="h-2.5 w-2.5 shrink-0" />
                    <span className="font-mono">#{stackParent.number}</span>
                    <span className="truncate">{stackParent.title}</span>
                  </div>
                )}
                <div className="flex items-center gap-2 text-primary font-medium">
                  <ChevronRight className="h-3 w-3 shrink-0" />
                  <span className="font-mono">#{pr.number}</span>
                  <span className="truncate">{pr.title}</span>
                </div>
                {stackChildren.map((child) => (
                  <div key={child.number} className="flex items-center gap-2 text-muted-foreground ml-4">
                    <Circle className="h-2 w-2 shrink-0" />
                    <span className="font-mono">#{child.number}</span>
                    <span className="truncate">{child.title}</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* CI Checks */}
          <div className="px-5 py-3 border-b border-border">
            <p className="text-xs font-medium text-muted-foreground mb-2">
              Checks {!loadingChecks && checks.length > 0 && `(${checks.length})`}
            </p>
            {loadingChecks ? (
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            ) : checks.length === 0 ? (
              <span className="text-xs text-muted-foreground">No checks</span>
            ) : (
              <div className="space-y-1.5">
                {checks.map((c) => {
                  const passed = c.conclusion === 'success';
                  const failed = c.conclusion === 'failure' || c.conclusion === 'cancelled';
                  const running = c.status !== 'completed';
                  return (
                    <div key={c.id} className="flex items-center gap-2 text-xs">
                      {running  && <Loader2 className="h-3 w-3 animate-spin text-amber-400 shrink-0" />}
                      {!running && passed  && <CheckCircle2 className="h-3 w-3 text-emerald-400 shrink-0" />}
                      {!running && failed  && <XCircle className="h-3 w-3 text-destructive shrink-0" />}
                      {!running && !passed && !failed && <Circle className="h-3 w-3 text-muted-foreground shrink-0" />}
                      <a href={c.html_url} target="_blank" rel="noopener noreferrer"
                         className="hover:underline truncate text-foreground">
                        {c.name}
                      </a>
                      {c.completed_at && (
                        <span className="ml-auto shrink-0 text-muted-foreground">{relTime(c.completed_at)}</span>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>

          {/* Activity: most recent commits (head SHA as reference) */}
          <div className="px-5 py-3">
            <p className="text-xs font-medium text-muted-foreground mb-2">Head</p>
            <div className="flex items-center gap-2 text-xs text-muted-foreground font-mono">
              <GitCommit className="h-3.5 w-3.5 shrink-0" />
              {pr.head.sha.slice(0, 7)}
            </div>
          </div>
        </div>

        {/* Merge panel */}
        {pr.state === 'open' && (
          <div className="px-5 py-4 border-t border-border shrink-0 space-y-2">
            <div className="flex items-center gap-2">
              <select
                value={mergeMethod}
                onChange={(e) => setMergeMethod(e.target.value as any)}
                className="qr-select text-xs flex-1"
              >
                <option value="squash">Squash and merge</option>
                <option value="merge">Create a merge commit</option>
                <option value="rebase">Rebase and merge</option>
              </select>
              <button
                onClick={handleMerge}
                disabled={merging}
                className={cn(
                  'flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors whitespace-nowrap',
                  'bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20 disabled:opacity-50',
                )}>
                {merging ? <Loader2 className="h-3 w-3 animate-spin" /> : <GitMerge className="h-3 w-3" />}
                Merge
              </button>
            </div>
            {pr.ci_status && pr.ci_status !== 'success' && (
              <p className="text-xs text-amber-400">
                CI is {pr.ci_status} — merge anyway at your own risk.
              </p>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// ─── PR card (compact) ───────────────────────────────────────────────────────

function PRCard({ pr, owner, repo, allPRs, onAction }: {
  pr: GitHubPR; owner: string; repo: string; allPRs: GitHubPR[]; onAction: () => void;
}) {
  const [open, setOpen] = useState(false);

  return (
    <>
      <div
        onClick={() => setOpen(true)}
        className="rounded-lg border border-border bg-card p-3 space-y-2 text-sm cursor-pointer hover:border-border/80 hover:bg-card/80 transition-colors"
      >
        <div className="flex items-start justify-between gap-2">
          <span className="font-medium leading-snug line-clamp-2">{pr.title}</span>
          <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground mt-0.5" />
        </div>

        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <GitBranch className="h-3 w-3 shrink-0" />
          <span className="font-mono truncate">{pr.head.ref}</span>
          <span className="shrink-0">→ {pr.base.ref}</span>
        </div>

        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <CIBadge status={pr.ci_status} />
            <span className="text-xs text-muted-foreground">{relTime(pr.updated_at)}</span>
          </div>
          <img src={pr.user.avatar_url} alt={pr.user.login} className="h-5 w-5 rounded-full" />
        </div>
      </div>

      {open && (
        <PRDetail pr={pr} owner={owner} repo={repo} allPRs={allPRs} onClose={() => setOpen(false)} onMerged={onAction} />
      )}
    </>
  );
}

// ─── PR tab strip (Diffkit-style) ─────────────────────────────────────────────

function PRTabStrip({ prs, owner, repo, onRefresh }: {
  prs: GitHubPR[]; owner: string; repo: string; onRefresh: () => void;
}) {
  const [active, setActive] = useState<number | null>(null);
  const openPRs = prs.filter((p) => p.state === 'open').slice(0, 8);

  if (openPRs.length === 0) return null;

  return (
    <div className="flex items-center gap-1 px-4 py-1.5 border-b border-border bg-muted/30 overflow-x-auto shrink-0">
      {openPRs.map((pr) => (
        <button
          key={pr.number}
          onClick={() => setActive(active === pr.number ? null : pr.number)}
          className={cn(
            'flex items-center gap-1.5 rounded px-2.5 py-1 text-xs whitespace-nowrap transition-colors border',
            active === pr.number
              ? 'border-primary/50 bg-primary/10 text-primary'
              : 'border-transparent text-muted-foreground hover:text-foreground hover:bg-muted',
          )}>
          <GitPullRequest className="h-3 w-3 shrink-0" />
          <span className="font-mono">#{pr.number}</span>
          <span className="max-w-[120px] truncate">{pr.title}</span>
          {pr.ci_status === 'success' && <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 shrink-0" />}
          {pr.ci_status === 'failure' && <span className="w-1.5 h-1.5 rounded-full bg-destructive shrink-0" />}
          {pr.ci_status === 'pending' && <span className="w-1.5 h-1.5 rounded-full bg-amber-400 shrink-0 animate-pulse" />}
        </button>
      ))}
      <button onClick={onRefresh} className="ml-auto shrink-0 p-1 rounded hover:bg-muted transition-colors text-muted-foreground">
        <RefreshCw className="h-3.5 w-3.5" />
      </button>

      {/* inline drawer for tab strip selection */}
      {active !== null && (() => {
        const pr = prs.find((p) => p.number === active);
        if (!pr) return null;
        return (
          <PRDetail pr={pr} owner={owner} repo={repo} allPRs={prs}
            onClose={() => setActive(null)} onMerged={() => { setActive(null); onRefresh(); }} />
        );
      })()}
    </div>
  );
}

// ─── PRs tab (kanban) ─────────────────────────────────────────────────────────

function PRsTab({ owner, repo }: { owner: string; repo: string }) {
  const [prs, setPRs] = useState<GitHubPR[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await githubApi.listPRs(owner, repo, 'all');
      setPRs(data);
    } catch { /* ignore */ }
    setLoading(false);
  }, [owner, repo]);

  useEffect(() => { load(); }, [load]);

  if (loading) return (
    <div className="flex items-center justify-center py-20">
      <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
    </div>
  );

  return (
    <div className="flex flex-col h-full">
      <PRTabStrip prs={prs} owner={owner} repo={repo} onRefresh={load} />
      <div className="grid grid-cols-4 gap-4 p-4 flex-1 overflow-auto">
        {PR_COLUMNS.map((col) => {
          const items = prs.filter(col.filter);
          return (
            <div key={col.key} className="space-y-3">
              <div className={cn('flex items-center justify-between rounded-md border-b-2 pb-2', col.color)}>
                <span className="text-sm font-medium">{col.label}</span>
                <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">{items.length}</span>
              </div>
              <div className="space-y-2">
                {items.map((pr) => (
                  <PRCard key={pr.number} pr={pr} owner={owner} repo={repo} allPRs={prs} onAction={load} />
                ))}
                {items.length === 0 && (
                  <div className="py-8 text-center text-xs text-muted-foreground">No PRs here</div>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ─── Issues tab ───────────────────────────────────────────────────────────────

function IssuesTab({ owner, repo, souls, projectId }: { owner: string; repo: string; souls: any[]; projectId: string }) {
  const [issues, setIssues] = useState<GitHubIssue[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try { setIssues(await githubApi.listIssues(owner, repo, 'open')); }
    catch { /* ignore */ }
    setLoading(false);
  }, [owner, repo]);

  useEffect(() => { load(); }, [load]);

  const assignToPrime = async (issue: GitHubIssue) => {
    try {
      const prime = souls.find((s: any) => s.agent_key === 'prime' || s.display_name?.toLowerCase() === 'prime');
      if (!prime) { toast.error('Prime agent not found'); return; }
      if (!projectId) { toast.error('No project selected'); return; }
      await request(`/projects/${encodeURIComponent(projectId)}/tasks`, {
        method: 'POST',
        body: JSON.stringify({
          title: issue.title,
          description: `GitHub issue #${issue.number}: ${issue.html_url}\n\n${issue.body ?? ''}`,
          assigned_agent_id: prime.id,
          github_issue_number: issue.number,
        }),
      });
      toast.success(`Issue #${issue.number} assigned to Prime`);
    } catch (e: any) { toast.error(e.message ?? 'Failed'); }
  };

  const closeIssue = async (issue: GitHubIssue) => {
    try {
      await githubApi.closeIssue(owner, repo, issue.number);
      toast.success(`Issue #${issue.number} closed`);
      setIssues((prev) => prev.filter((i) => i.number !== issue.number));
    } catch (e: any) { toast.error(e.message ?? 'Failed to close issue'); }
  };

  if (loading) return (
    <div className="flex items-center justify-center py-20">
      <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
    </div>
  );

  return (
    <div className="p-4 space-y-2">
      {issues.map((issue) => (
        <div key={issue.number} className="flex items-start gap-3 rounded-lg border border-border bg-card p-3 text-sm">
          <AlertCircle className="h-4 w-4 shrink-0 mt-0.5 text-emerald-400" />
          <div className="flex-1 min-w-0 space-y-1">
            <div className="flex items-start justify-between gap-2">
              <a href={issue.html_url} target="_blank" rel="noopener noreferrer"
                 className="font-medium hover:underline line-clamp-1">
                #{issue.number} {issue.title}
              </a>
              <span className="text-xs text-muted-foreground shrink-0">{relTime(issue.updated_at)}</span>
            </div>
            <div className="flex items-center gap-2 flex-wrap">
              {issue.labels.map((lbl) => (
                <span key={lbl.name} className="flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
                      style={{ backgroundColor: `#${lbl.color}22`, color: `#${lbl.color}` }}>
                  <Tag className="h-2.5 w-2.5" />{lbl.name}
                </span>
              ))}
              {issue.assignee && (
                <span className="flex items-center gap-1 text-xs text-muted-foreground">
                  <img src={issue.assignee.avatar_url} alt="" className="h-4 w-4 rounded-full" />
                  {issue.assignee.login}
                </span>
              )}
            </div>
          </div>
          <div className="flex items-center gap-1.5 shrink-0">
            <button onClick={() => assignToPrime(issue)}
              className="rounded-md px-2 py-1 text-xs bg-primary/10 text-primary hover:bg-primary/20 transition-colors">
              Assign Prime
            </button>
            <button onClick={() => closeIssue(issue)}
              className="rounded-md px-2 py-1 text-xs bg-muted text-muted-foreground hover:bg-muted/80 transition-colors">
              Close
            </button>
          </div>
        </div>
      ))}
      {issues.length === 0 && (
        <div className="py-20 text-center text-sm text-muted-foreground">No open issues</div>
      )}
    </div>
  );
}

// ─── Tasks tab ────────────────────────────────────────────────────────────────

function TasksTab() {
  const [tasks, setTasks] = useState<GitHubTask[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try { const t = await githubApi.listTasks(); setTasks(Array.isArray(t) ? t : []); }
    catch { /* ignore */ }
    setLoading(false);
  }, []);

  useEffect(() => { load(); }, [load]);

  if (loading) return (
    <div className="flex items-center justify-center py-20">
      <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
    </div>
  );

  const phaseOrder = ['reading','branching','coding','testing','fixing','opening_pr','awaiting_ci','complete','blocked'];

  return (
    <div className="p-4 space-y-2">
      {tasks.map((task) => {
        const phaseIdx = phaseOrder.indexOf(task.phase);
        const pct = phaseIdx < 0 ? 0 : Math.round((phaseIdx / (phaseOrder.length - 1)) * 100);
        const meta = TASK_PHASE_META[task.phase] ?? { label: task.phase, cls: 'text-muted-foreground' };
        return (
          <div key={task.id} className="rounded-lg border border-border bg-card p-3 space-y-2 text-sm">
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2 min-w-0">
                <GitBranch className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="font-mono text-xs truncate">{task.branch || task.id.slice(0, 8)}</span>
              </div>
              <span className={cn('text-xs font-medium', meta.cls)}>{meta.label}</span>
            </div>
            <div className="h-1.5 w-full rounded-full bg-muted overflow-hidden">
              <div className="h-full rounded-full bg-primary transition-all" style={{ width: `${pct}%` }} />
            </div>
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <div className="flex items-center gap-2">
                {task.pr_url && (
                  <a href={task.pr_url} target="_blank" rel="noopener noreferrer"
                     className="flex items-center gap-1 hover:text-foreground">
                    <GitPullRequest className="h-3 w-3" />PR #{task.pr_number}
                  </a>
                )}
                {task.issue_number && (
                  <span className="flex items-center gap-1">
                    <AlertCircle className="h-3 w-3" />#{task.issue_number}
                  </span>
                )}
              </div>
              <span>{relTime(task.updated_at)}</span>
            </div>
            {task.error && (
              <div className="rounded-md bg-destructive/10 px-2 py-1 text-xs text-destructive">{task.error}</div>
            )}
          </div>
        );
      })}
      {tasks.length === 0 && (
        <div className="py-20 text-center text-sm text-muted-foreground">No active GitHub tasks</div>
      )}
    </div>
  );
}

// ─── Repos tab ────────────────────────────────────────────────────────────────

function ConnectProjectForm({ onDone }: { onDone: () => void }) {
  const [projects, setProjects] = useState<any[]>([]);
  const [projectId, setProjectId] = useState('');
  const [owner, setOwner] = useState('');
  const [repo, setRepo] = useState('');
  const [branch, setBranch] = useState('main');
  const [saving, setSaving] = useState(false);
  const [result, setResult] = useState<{ webhook_url: string; webhook_secret: string } | null>(null);

  useEffect(() => {
    request<any[]>('/projects').then((d) => {
      const list = Array.isArray(d) ? d : [];
      setProjects(list.filter((p: any) => !p.github_owner));
      if (list.length) setProjectId(list[0].id);
    }).catch(() => {});
  }, []);

  const save = async () => {
    if (!projectId || !owner || !repo) { toast.error('All fields required'); return; }
    setSaving(true);
    try {
      const res = await githubApi.connectRepo(projectId, owner, repo, branch);
      setResult(res);
      toast.success('GitHub connected!');
      onDone();
    } catch (e: any) { toast.error(e.message ?? 'Failed'); }
    setSaving(false);
  };

  if (result) {
    return (
      <div className="space-y-2 text-xs">
        <div className="rounded-md bg-emerald-500/10 p-3 text-emerald-400 font-medium">Connected!</div>
        <div className="space-y-1">
          <p className="text-muted-foreground">Webhook URL (add in GitHub → Settings → Webhooks):</p>
          <code className="block rounded bg-muted px-2 py-1 text-xs break-all">{result.webhook_url}</code>
        </div>
        <div className="space-y-1">
          <p className="text-muted-foreground">Webhook Secret:</p>
          <code className="block rounded bg-muted px-2 py-1 text-xs break-all">{result.webhook_secret}</code>
        </div>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 gap-3 text-sm">
      <label className="col-span-2 space-y-1">
        <span className="text-xs text-muted-foreground">Project</span>
        <select value={projectId} onChange={(e) => setProjectId(e.target.value)}
          className="qr-select">
          {projects.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
      </label>
      <label className="space-y-1">
        <span className="text-xs text-muted-foreground">GitHub Owner</span>
        <input value={owner} onChange={(e) => setOwner(e.target.value)} placeholder="org-or-user"
          className="qr-input" />
      </label>
      <label className="space-y-1">
        <span className="text-xs text-muted-foreground">Repository</span>
        <input value={repo} onChange={(e) => setRepo(e.target.value)} placeholder="repo-name"
          className="qr-input" />
      </label>
      <label className="space-y-1">
        <span className="text-xs text-muted-foreground">Default Branch</span>
        <input value={branch} onChange={(e) => setBranch(e.target.value)} placeholder="main"
          className="qr-input" />
      </label>
      <div className="flex items-end">
        <button onClick={save} disabled={saving}
          className="qr-btn qr-btn-primary qr-btn-sm">
          {saving ? <Loader2 className="h-3 w-3 animate-spin" /> : <Link2 className="h-3 w-3" />}
          Connect
        </button>
      </div>
    </div>
  );
}

function ReposTab() {
  const [projects, setProjects] = useState<ConnectedProject[]>([]);
  const [statuses, setStatuses] = useState<Record<string, RepoStatus>>({});

  const loadProjects = useCallback(async () => {
    try {
      const data: any[] = await request('/projects');
      const list = Array.isArray(data) ? data : [];
      setProjects(list.filter((p: any) => p.github_owner).map((p: any) => ({
        id: p.id, name: p.name, github_owner: p.github_owner, github_repo: p.github_repo,
      })));
      const all = list.filter((p: any) => p.github_owner);
      const pairs = await Promise.allSettled(all.map((p: any) => githubApi.getRepoStatus(p.id).then((s) => [p.id, s] as const)));
      const map: Record<string, RepoStatus> = {};
      for (const res of pairs) {
        if (res.status === 'fulfilled') map[res.value[0]] = res.value[1];
      }
      setStatuses(map);
    } catch { /* ignore */ }
  }, []);

  useEffect(() => { loadProjects(); }, [loadProjects]);

  const disconnect = async (projectId: string) => {
    try {
      await githubApi.disconnectRepo(projectId);
      toast.success('GitHub disconnected');
      setProjects((prev) => prev.filter((p) => p.id !== projectId));
    } catch (e: any) { toast.error(e.message ?? 'Disconnect failed'); }
  };

  return (
    <div className="p-4 space-y-3">
      {projects.map((proj) => {
        const status = statuses[proj.id];
        return (
          <div key={proj.id} className="rounded-lg border border-border bg-card p-4 space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <CheckCheck className="h-4 w-4 text-emerald-400" />
                <span className="font-medium">{proj.name}</span>
                <span className="text-xs text-muted-foreground">{proj.github_owner}/{proj.github_repo}</span>
              </div>
              <div className="flex items-center gap-3">
                {status && (
                  <>
                    <span className="flex items-center gap-1 text-xs text-muted-foreground">
                      <GitPullRequest className="h-3.5 w-3.5" />{status.open_prs} PRs
                    </span>
                    <span className="flex items-center gap-1 text-xs text-muted-foreground">
                      <AlertCircle className="h-3.5 w-3.5" />{status.open_issues} Issues
                    </span>
                  </>
                )}
                <button onClick={() => disconnect(proj.id)}
                  className="text-xs text-destructive/80 hover:text-destructive transition-colors">
                  Disconnect
                </button>
              </div>
            </div>
          </div>
        );
      })}
      <div className="rounded-lg border border-dashed border-border bg-muted/30 p-4 space-y-3">
        <div className="flex items-center gap-2 text-sm font-medium">
          <Plus className="h-4 w-4 text-muted-foreground" />
          Connect a project to GitHub
        </div>
        <p className="text-xs text-muted-foreground">
          Select a project and enter its GitHub details. A webhook secret will be generated.
        </p>
        <ConnectProjectForm onDone={loadProjects} />
      </div>
    </div>
  );
}

// ─── page ────────────────────────────────────────────────────────────────────

const TABS = [
  { id: 'prs',    label: 'Pull Requests', icon: GitPullRequest },
  { id: 'issues', label: 'Issues',        icon: AlertCircle },
  { id: 'tasks',  label: 'Agent Tasks',   icon: Layers },
  { id: 'repos',  label: 'Repositories',  icon: Settings },
] as const;

type Tab = typeof TABS[number]['id'];

export default function GitHubPage() {
  const souls = useStore((s) => s.souls);
  const githubActiveTab = useStore((s) => s.githubActiveTab);
  const setGithubActiveTab = useStore((s) => s.setGithubActiveTab);
  const tab = githubActiveTab as Tab;

  const [connectedProjects, setConnectedProjects] = useState<ConnectedProject[]>([]);
  const [selectedProjectId, setSelectedProjectId] = useState<string>('');

  useEffect(() => {
    request<any[]>('/projects').then((d) => {
      const list = Array.isArray(d) ? d : [];
      const connected = list.filter((p: any) => p.github_owner).map((p: any) => ({
        id: p.id, name: p.name, github_owner: p.github_owner, github_repo: p.github_repo,
      }));
      setConnectedProjects(connected);
      if (connected.length && !selectedProjectId) setSelectedProjectId(connected[0]!.id);
    }).catch(() => {});
  }, []);

  const selected = connectedProjects.find((p) => p.id === selectedProjectId);
  const owner = selected?.github_owner ?? '';
  const repo  = selected?.github_repo ?? '';

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-input px-6 py-3 shrink-0">
        <div className="flex items-center gap-3">
          <GitPullRequest className="h-5 w-5 text-muted-foreground" />
          <h1 className="text-base font-semibold">GitHub</h1>
          {connectedProjects.length > 1 && (
            <select value={selectedProjectId} onChange={(e) => setSelectedProjectId(e.target.value)}
              className="qr-select text-xs">
              {connectedProjects.map((p) => (
                <option key={p.id} value={p.id}>{p.name} — {p.github_owner}/{p.github_repo}</option>
              ))}
            </select>
          )}
          {selected && (
            <a href={`https://github.com/${owner}/${repo}`} target="_blank" rel="noopener noreferrer"
               className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors">
              {owner}/{repo}
              <ExternalLink className="h-3 w-3" />
            </a>
          )}
        </div>
      </div>

      {/* Tabs */}
      <div className="flex items-center gap-1 border-b border-input px-4 shrink-0">
        {TABS.map(({ id, label, icon: Icon }) => (
          <button key={id} onClick={() => setGithubActiveTab(id)}
            className={cn(
              'flex items-center gap-1.5 px-3 py-2.5 text-sm transition-colors border-b-2',
              tab === id
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:text-foreground',
            )}>
            <Icon className="h-4 w-4" />
            {label}
          </button>
        ))}
      </div>

      {/* Content */}
      <div className="flex-1 overflow-auto min-h-0">
        {tab === 'prs'    && owner && repo && <PRsTab owner={owner} repo={repo} />}
        {tab === 'issues' && owner && repo && <IssuesTab owner={owner} repo={repo} souls={souls} projectId={selectedProjectId} />}
        {tab === 'tasks'  && <TasksTab />}
        {tab === 'repos'  && <ReposTab />}
        {(tab === 'prs' || tab === 'issues') && !owner && (
          <div className="flex flex-col items-center justify-center py-24 gap-3 text-sm text-muted-foreground">
            <GitPullRequest className="h-8 w-8" />
            <p>No GitHub repository connected.</p>
            <button onClick={() => setGithubActiveTab('repos')}
              className="flex items-center gap-1.5 rounded-md bg-primary/10 text-primary px-3 py-1.5 text-xs font-medium hover:bg-primary/20 transition-colors">
              <Link2 className="h-3.5 w-3.5" />Connect a Repository
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
