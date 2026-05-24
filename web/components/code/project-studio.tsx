'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useCallback, useEffect, useMemo } from 'react';
import {
  CheckCircle2, Loader2, ChevronDown, ChevronRight,
  Rocket, Users, ListChecks, FileText, Zap, Bot, RefreshCw,
  Clock, AlertCircle, File, DollarSign, CheckCheck, Activity,
} from 'lucide-react';
import { projectBriefs as api } from '@/lib/api';
import { cn } from '@/lib/utils';
import { useStore } from '@/store';
import type { DaemonTask } from '@/hooks/use-agents-stream';
import type { ProjectBrief, ProposedAgent, ProposedTask, BriefAgent } from '@/types';

interface Props {
  brief: ProjectBrief;
  onBriefUpdate: (b: ProjectBrief) => void;
}

export function ProjectStudio({ brief, onBriefUpdate }: Props) {
  if (brief.status === 'intake') return (
    <div className="h-full overflow-y-auto">
      <IntakeCanvas brief={brief} onBriefUpdate={onBriefUpdate} />
    </div>
  );
  if (brief.status === 'proposed') return (
    <div className="h-full overflow-y-auto">
      <ApprovalCanvas brief={brief} onBriefUpdate={onBriefUpdate} />
    </div>
  );
  return <ExecutionCanvas brief={brief} onBriefUpdate={onBriefUpdate} />;
}

// ── IntakeCanvas ──────────────────────────────────────────────────────────────

function IntakeCanvas({ brief, onBriefUpdate }: Props) {
  const [budget, setBudget] = useState(brief.budget_cents ? String(brief.budget_cents / 100) : '');
  const [timeline, setTimeline] = useState(brief.timeline ?? '');
  const [quality, setQuality] = useState<'mvp' | 'production' | 'enterprise'>(brief.quality ?? 'mvp');
  const [stack, setStack] = useState(brief.stack ?? '');
  const [proposing, setProposing] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const propose = async () => {
    setProposing(true);
    setErr(null);
    try {
      const updated = await api.update(brief.id, {
        budget_cents: Math.round(parseFloat(budget || '0') * 100),
        timeline,
        quality,
        stack,
      });
      onBriefUpdate(updated);
      const proposed = await api.propose(brief.id);
      onBriefUpdate(proposed);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Failed to generate proposal');
    } finally {
      setProposing(false);
    }
  };

  return (
    <div className="flex flex-col items-center justify-center min-h-full py-12 px-6">
      <div className="w-full max-w-xl space-y-6">
        <div className="text-center space-y-2">
          <div className="inline-flex h-12 w-12 items-center justify-center rounded-xl bg-primary/10">
            <Rocket className="h-6 w-6 text-primary" />
          </div>
          <h2 className="text-lg font-semibold">{brief.title || 'New Project'}</h2>
          <p className="text-sm text-muted-foreground">
            Fill in the project details below, or describe your project in the chat — Prime will fill these in for you.
          </p>
        </div>

        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs font-medium text-muted-foreground">Budget ($)</label>
              <input
                value={budget}
                onChange={e => setBudget(e.target.value)}
                placeholder="e.g. 50"
                className="mt-1 qr-input"
              />
            </div>
            <div>
              <label className="text-xs font-medium text-muted-foreground">Timeline</label>
              <input
                value={timeline}
                onChange={e => setTimeline(e.target.value)}
                placeholder="e.g. 2 weeks"
                className="mt-1 qr-input"
              />
            </div>
          </div>

          <div>
            <label className="text-xs font-medium text-muted-foreground">Quality tier</label>
            <select value={quality} onChange={e => setQuality(e.target.value as 'mvp' | 'production' | 'enterprise')} className="mt-1 qr-select">
              <option value="mvp">MVP — fast, functional, ship it</option>
              <option value="production">Production — stable, tested, scalable</option>
              <option value="enterprise">Enterprise — compliance, auditing, SLAs</option>
            </select>
          </div>

          <div>
            <label className="text-xs font-medium text-muted-foreground">Preferred stack (optional)</label>
            <input
              value={stack}
              onChange={e => setStack(e.target.value)}
              placeholder="e.g. Next.js + Go + PostgreSQL"
              className="mt-1 qr-input"
            />
          </div>
        </div>

        {err && <p className="text-xs text-destructive">{err}</p>}

        <button
          onClick={propose}
          disabled={proposing || (!budget && !brief.idea)}
          className="qr-btn qr-btn-primary qr-btn-lg w-full justify-center"
        >
          {proposing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Zap className="h-4 w-4" />}
          {proposing ? 'Generating proposal…' : 'Generate Team Proposal'}
        </button>

        <p className="text-center text-xs text-muted-foreground">
          Prime will analyse the requirements and propose the right team, models, and cost estimate.
        </p>
      </div>
    </div>
  );
}

// ── ApprovalCanvas ────────────────────────────────────────────────────────────

function ApprovalCanvas({ brief, onBriefUpdate }: Props) {
  const [specOk, setSpecOk] = useState(false);
  const [teamOk, setTeamOk] = useState(false);
  const [tasksOk, setTasksOk] = useState(false);
  const [launching, setLaunching] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const proposal = brief.proposal!;

  const launch = async () => {
    setLaunching(true);
    setErr(null);
    try {
      const result = await api.approve(brief.id);
      onBriefUpdate(result.brief ?? (result as unknown as ProjectBrief));
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Launch failed');
    } finally {
      setLaunching(false);
    }
  };

  const allApproved = specOk && teamOk && tasksOk;

  return (
    <div className="max-w-2xl mx-auto py-8 px-4 space-y-4">
      <StepperHeader steps={[
        { label: 'Spec', done: specOk },
        { label: 'Team', done: teamOk },
        { label: 'Tasks', done: tasksOk },
        { label: 'Launch', done: brief.status === 'approved' || brief.status === 'active' },
      ]} />

      <ApprovalSection
        step={1}
        title="Spec & Design"
        icon={FileText}
        approved={specOk}
        onApprove={() => setSpecOk(true)}
        onRevise={() => setSpecOk(false)}
      >
        <div className="space-y-3 text-sm">
          <div className="grid grid-cols-3 gap-3">
            <Chip label="Budget" value={`$${(brief.budget_cents / 100).toFixed(0)}`} />
            <Chip label="Timeline" value={brief.timeline || '—'} />
            <Chip label="Quality" value={brief.quality} />
          </div>
          {brief.stack && <Chip label="Stack" value={brief.stack} wide />}
          {proposal.reasoning && (
            <div className="rounded-xl border border-border/60 bg-muted/20 p-3 text-xs text-muted-foreground leading-relaxed">
              {proposal.reasoning}
            </div>
          )}
          <div className="text-xs text-muted-foreground">
            Estimated cost:{' '}
            <span className="font-medium text-foreground">
              ${(proposal.est_min_cents / 100).toFixed(0)}–${(proposal.est_max_cents / 100).toFixed(0)}
            </span>
          </div>
        </div>
      </ApprovalSection>

      {specOk && (
        <ApprovalSection
          step={2}
          title="Team Composition"
          icon={Users}
          approved={teamOk}
          onApprove={() => setTeamOk(true)}
          onRevise={() => setTeamOk(false)}
        >
          <div className="space-y-2">
            {proposal.agents.map(a => <ProposedAgentRow key={a.role} agent={a} />)}
          </div>
        </ApprovalSection>
      )}

      {specOk && teamOk && (
        <ApprovalSection
          step={3}
          title="Task Breakdown"
          icon={ListChecks}
          approved={tasksOk}
          onApprove={() => setTasksOk(true)}
          onRevise={() => setTasksOk(false)}
        >
          <div className="space-y-1">
            {proposal.tasks.map((t, i) => <ProposedTaskRow key={i} task={t} />)}
          </div>
        </ApprovalSection>
      )}

      {allApproved && (
        <div className="qr-card p-4 space-y-3 border-primary/30">
          <p className="text-sm font-medium">All sections approved — ready to launch.</p>
          <p className="text-xs text-muted-foreground">
            Prime will spawn the team, assign tasks, and begin working autonomously.
          </p>
          {err && <p className="text-xs text-destructive">{err}</p>}
          <button
            onClick={launch}
            disabled={launching}
            className="qr-btn qr-btn-primary qr-btn-lg w-full justify-center"
          >
            {launching
              ? <><Loader2 className="h-4 w-4 animate-spin" /> Launching…</>
              : <><Rocket className="h-4 w-4" /> Give Prime Full Power</>
            }
          </button>
        </div>
      )}
    </div>
  );
}

// ── ExecutionCanvas ───────────────────────────────────────────────────────────

function ExecutionCanvas({ brief, onBriefUpdate: _onBriefUpdate }: Props) {
  const [team, setTeam] = useState<BriefAgent[]>([]);
  const [teamLoading, setTeamLoading] = useState(true);
  const [taskFilter, setTaskFilter] = useState<'all' | 'active' | 'done' | 'failed'>('all');

  // Pull live daemon tasks from SSE store
  const allDaemonTasks = useStore(s => s.daemonTasks);
  const daemonAgents = useStore(s => s.daemonAgents);

  const refresh = useCallback(async () => {
    try {
      const res = await api.team(brief.id);
      setTeam(res.agents ?? []);
    } catch {
      // non-fatal
    } finally {
      setTeamLoading(false);
    }
  }, [brief.id]);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, 10_000);
    return () => clearInterval(id);
  }, [refresh]);

  // Live tasks from SSE (DaemonTask[]) sorted by status priority
  const liveTasks = useMemo(() => {
    const STATUS_ORDER: Record<string, number> = { in_progress: 0, queued: 1, failed: 2, done: 3, cancelled: 4 };
    return Object.values(allDaemonTasks).sort((a, b) =>
      (STATUS_ORDER[a.status] ?? 5) - (STATUS_ORDER[b.status] ?? 5)
    );
  }, [allDaemonTasks]);

  const filteredTasks = useMemo(() => {
    if (taskFilter === 'all') return liveTasks;
    if (taskFilter === 'active') return liveTasks.filter(t => t.status === 'in_progress' || t.status === 'queued');
    if (taskFilter === 'done') return liveTasks.filter(t => t.status === 'done');
    if (taskFilter === 'failed') return liveTasks.filter(t => t.status === 'failed');
    return liveTasks;
  }, [liveTasks, taskFilter]);

  // Stats derived from live data
  const taskDone = liveTasks.filter(t => t.status === 'done').length;
  const taskTotal = liveTasks.length;
  const taskActive = liveTasks.filter(t => t.status === 'in_progress').length;
  const taskPct = taskTotal > 0 ? Math.round((taskDone / taskTotal) * 100) : 0;

  const budgetUsed = team.reduce((s, a) => s + (a.credit_used_cents ?? 0), 0);
  const budgetTotal = team.reduce((s, a) => s + (a.credit_budget_cents ?? 0), 0) || brief.budget_cents;
  const budgetPct = budgetTotal > 0 ? Math.min(100, Math.round((budgetUsed / budgetTotal) * 100)) : 0;

  const agentDone = team.filter(a => a.status === 'done').length;
  const agentWorking = team.filter(a => a.status !== 'done' && a.status !== 'paused').length;
  const isDone = brief.status === 'done';

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* ── Stats header ──────────────────────────────────────────────────── */}
      <div className="shrink-0 border-b border-border bg-muted/10 px-4 py-2.5">
        <div className="flex items-center gap-4 flex-wrap">
          {/* Status badge */}
          <div className={cn('flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
            isDone ? 'bg-emerald-500/15 text-emerald-500' : 'bg-primary/10 text-primary'
          )}>
            {isDone
              ? <CheckCheck className="h-3.5 w-3.5" />
              : <Activity className="h-3.5 w-3.5 animate-pulse" />
            }
            {isDone ? 'Complete' : `${agentWorking} agent${agentWorking !== 1 ? 's' : ''} working`}
          </div>

          <div className="h-4 w-px bg-border" />

          {/* Task completion */}
          <div className="flex items-center gap-2 min-w-[160px]">
            <CheckCheck className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
            <div className="flex-1">
              <div className="flex items-center justify-between mb-0.5">
                <span className="text-xs text-muted-foreground">Tasks</span>
                <span className="text-xs font-semibold tabular-nums">
                  {taskDone}/{taskTotal} <span className="text-muted-foreground font-normal">({taskPct}%)</span>
                </span>
              </div>
              <div className="h-1.5 rounded-full bg-muted overflow-hidden">
                <div
                  className={cn('h-full rounded-full transition-all duration-700',
                    taskPct === 100 ? 'bg-emerald-500' : 'bg-primary'
                  )}
                  style={{ width: `${taskPct}%` }}
                />
              </div>
            </div>
          </div>

          {/* Budget usage */}
          {budgetTotal > 0 && (
            <div className="flex items-center gap-2 min-w-[160px]">
              <DollarSign className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
              <div className="flex-1">
                <div className="flex items-center justify-between mb-0.5">
                  <span className="text-xs text-muted-foreground">Budget</span>
                  <span className="text-xs font-semibold tabular-nums">
                    ${(budgetUsed / 100).toFixed(2)}
                    <span className="text-muted-foreground font-normal"> / ${(budgetTotal / 100).toFixed(2)}</span>
                    <span className="text-muted-foreground font-normal"> ({budgetPct}%)</span>
                  </span>
                </div>
                <div className="h-1.5 rounded-full bg-muted overflow-hidden">
                  <div
                    className={cn('h-full rounded-full transition-all duration-700',
                      budgetPct >= 90 ? 'bg-red-500' : budgetPct >= 70 ? 'bg-amber-500' : 'bg-emerald-500'
                    )}
                    style={{ width: `${budgetPct}%` }}
                  />
                </div>
              </div>
            </div>
          )}

          <button
            onClick={() => { setTeamLoading(true); refresh(); }}
            className="qr-btn qr-btn-ghost qr-btn-icon ml-auto shrink-0"
            title="Refresh"
          >
            <RefreshCw className={cn('h-3.5 w-3.5', teamLoading && 'animate-spin')} />
          </button>
        </div>
      </div>

      {/* ── Two-column body ───────────────────────────────────────────────── */}
      <div className="flex flex-1 overflow-hidden min-h-0">

        {/* Left: live task feed */}
        <div className="flex flex-col flex-1 min-w-0 border-r border-border overflow-hidden">
          {/* Task filter bar */}
          <div className="shrink-0 flex items-center gap-0.5 px-3 py-1.5 border-b border-border bg-muted/5">
            {(['all', 'active', 'done', 'failed'] as const).map(f => (
              <button
                key={f}
                onClick={() => setTaskFilter(f)}
                className={cn(
                  'qr-btn qr-btn-xs capitalize',
                  taskFilter === f ? 'qr-btn-primary' : 'qr-btn-ghost'
                )}
              >
                {f}
                {f === 'active' && taskActive > 0 && (
                  <span className="ml-1 rounded-full bg-amber-500/20 text-amber-500 px-1 text-[10px]">{taskActive}</span>
                )}
                {f === 'done' && taskDone > 0 && (
                  <span className="ml-1 rounded-full bg-emerald-500/20 text-emerald-500 px-1 text-[10px]">{taskDone}</span>
                )}
              </button>
            ))}
            <span className="ml-auto text-xs text-muted-foreground">{filteredTasks.length} task{filteredTasks.length !== 1 ? 's' : ''}</span>
          </div>

          {/* Task list */}
          <div className="flex-1 overflow-y-auto px-4 py-3 space-y-2">
            {filteredTasks.length === 0 && (
              liveTasks.length === 0 ? (
                <div className="flex flex-col items-center gap-2 py-10 text-muted-foreground">
                  <Clock className="h-6 w-6 opacity-30" />
                  <p className="text-sm">Waiting for tasks to start…</p>
                  <p className="text-xs opacity-60">Tasks appear as agents begin work</p>
                </div>
              ) : (
                <div className="flex flex-col items-center gap-2 py-10 text-muted-foreground">
                  <CheckCheck className="h-6 w-6 opacity-30" />
                  <p className="text-sm">No tasks match this filter</p>
                </div>
              )
            )}
            {filteredTasks.map(task => (
              <LiveTaskRow key={task.id} task={task} agentName={daemonAgents[task.owner]?.name} />
            ))}
          </div>
        </div>

        {/* Right: agent cards */}
        <div className="w-64 shrink-0 flex flex-col overflow-hidden">
          <div className="shrink-0 px-4 py-2 border-b border-border bg-muted/5">
            <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              Agents — {agentDone}/{team.length} done
            </span>
          </div>
          <div className="flex-1 overflow-y-auto px-3 py-3 space-y-2">
            {team.map(a => <AgentStatusCard key={a.id} agent={a} />)}
            {teamLoading && team.length === 0 && (
              <div className="flex items-center gap-2 py-6 justify-center text-muted-foreground text-xs">
                <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading…
              </div>
            )}
          </div>
        </div>

      </div>
    </div>
  );
}

// ── LiveTaskRow ───────────────────────────────────────────────────────────────

const TASK_STATUS_ICON = {
  queued:      <Clock className="h-3.5 w-3.5 text-muted-foreground shrink-0" />,
  in_progress: <Loader2 className="h-3.5 w-3.5 text-amber-400 animate-spin shrink-0" />,
  done:        <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400 shrink-0" />,
  failed:      <AlertCircle className="h-3.5 w-3.5 text-red-400 shrink-0" />,
  cancelled:   <Clock className="h-3.5 w-3.5 text-muted-foreground/40 shrink-0" />,
};

function LiveTaskRow({ task, agentName }: { task: DaemonTask; agentName?: string }) {
  const [open, setOpen] = useState(false);
  const icon = TASK_STATUS_ICON[task.status as keyof typeof TASK_STATUS_ICON] ?? TASK_STATUS_ICON.queued;
  const files = task.files_changed ?? [];

  return (
    <div className={cn(
      'qr-card transition-colors',
      task.status === 'in_progress' && 'border-amber-500/30',
      task.status === 'done'        && 'border-emerald-500/20',
      task.status === 'failed'      && 'border-red-500/30',
    )}>
      <button
        className="flex w-full items-start gap-2.5 px-3 py-2.5 text-left hover:bg-muted/20 transition-colors"
        onClick={() => setOpen(v => !v)}
      >
        <span className="mt-0.5">{icon}</span>
        <div className="flex-1 min-w-0">
          <p className="text-xs font-medium truncate">{task.title}</p>
          {agentName && <p className="text-[10px] text-muted-foreground mt-0.5">{agentName}</p>}
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          {files.length > 0 && (
            <span className="flex items-center gap-0.5 text-[10px] text-muted-foreground">
              <File className="h-3 w-3" />{files.length}
            </span>
          )}
          {open ? <ChevronDown className="h-3 w-3 text-muted-foreground" /> : <ChevronRight className="h-3 w-3 text-muted-foreground" />}
        </div>
      </button>

      {/* Progress bar for in-progress tasks */}
      {task.status === 'in_progress' && typeof task.percent === 'number' && task.percent > 0 && (
        <div className="px-3 pb-2">
          <div className="flex items-center justify-between mb-1">
            <span className="text-[10px] text-muted-foreground">Progress</span>
            <span className="text-[10px] font-medium tabular-nums">{task.percent}%</span>
          </div>
          <div className="h-1 rounded-full bg-muted overflow-hidden">
            <div className="h-full rounded-full bg-amber-400 transition-all" style={{ width: `${task.percent}%` }} />
          </div>
        </div>
      )}

      {/* Expanded detail */}
      {open && (
        <div className="border-t border-border px-3 py-2 space-y-1.5 bg-muted/10">
          <div className="flex items-center gap-1.5">
            <span className={cn(
              'text-[10px] font-medium rounded-full px-1.5 py-0.5',
              task.status === 'done'        ? 'bg-emerald-500/10 text-emerald-400' :
              task.status === 'in_progress' ? 'bg-amber-500/10 text-amber-400' :
              task.status === 'failed'      ? 'bg-red-500/10 text-red-400' :
                                              'bg-muted text-muted-foreground'
            )}>
              {task.status}
            </span>
            {task.priority && (
              <span className="text-[10px] text-muted-foreground capitalize">{task.priority} priority</span>
            )}
          </div>
          {task.summary && (
            <p className="text-[10px] text-emerald-400 leading-relaxed">{task.summary}</p>
          )}
          {task.error && (
            <p className="text-[10px] text-red-400 leading-relaxed">{task.error}</p>
          )}
          {!task.summary && !task.error && task.status !== 'done' && (
            <p className="text-[10px] text-muted-foreground italic">
              {task.status === 'in_progress' ? 'Agent is working on this…' : 'Waiting to be picked up'}
            </p>
          )}
          {files.length > 0 && (
            <div className="space-y-0.5">
              <p className="text-[10px] font-medium text-muted-foreground">Files changed</p>
              {files.slice(0, 8).map(f => (
                <p key={f} className="text-[10px] text-muted-foreground font-mono truncate">{f}</p>
              ))}
              {files.length > 8 && (
                <p className="text-[10px] text-muted-foreground">+{files.length - 8} more files</p>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ── AgentStatusCard ───────────────────────────────────────────────────────────

function AgentStatusCard({ agent }: { agent: BriefAgent }) {
  const budgetPct = agent.credit_budget_cents
    ? Math.min(100, Math.round((agent.credit_used_cents / agent.credit_budget_cents) * 100))
    : null;

  return (
    <div className={cn(
      'qr-card p-3 space-y-2 transition-colors',
      agent.status === 'done'   && 'border-emerald-500/20',
      agent.status === 'paused' && 'border-amber-500/20',
      agent.status !== 'done' && agent.status !== 'paused' && 'border-primary/20',
    )}>
      <div className="flex items-center gap-2">
        <div className={cn('h-2 w-2 rounded-full shrink-0',
          agent.status === 'done'   ? 'bg-emerald-500' :
          agent.status === 'paused' ? 'bg-amber-500' :
          'bg-primary animate-pulse'
        )} />
        <div className="flex-1 min-w-0">
          <p className="text-xs font-medium truncate">{agent.display_name}</p>
          <p className="text-[10px] text-muted-foreground capitalize">{agent.role}</p>
        </div>
        <span className={cn(
          'text-[10px] font-medium rounded-full px-1.5 py-0.5',
          agent.status === 'done'   ? 'bg-emerald-500/10 text-emerald-500' :
          agent.status === 'paused' ? 'bg-amber-500/10 text-amber-500' :
          'bg-primary/10 text-primary'
        )}>
          {agent.status}
        </span>
      </div>

      {/* Budget bar */}
      {budgetPct !== null && (
        <div>
          <div className="flex items-center justify-between mb-1">
            <span className="text-[10px] text-muted-foreground">Budget used</span>
            <span className="text-[10px] font-medium tabular-nums">
              ${(agent.credit_used_cents / 100).toFixed(2)} / ${((agent.credit_budget_cents ?? 0) / 100).toFixed(2)}
            </span>
          </div>
          <div className="h-1 rounded-full bg-muted overflow-hidden">
            <div
              className={cn('h-full rounded-full transition-all',
                budgetPct >= 90 ? 'bg-red-500' : budgetPct >= 70 ? 'bg-amber-500' : 'bg-emerald-500'
              )}
              style={{ width: `${budgetPct}%` }}
            />
          </div>
        </div>
      )}
      {budgetPct === null && agent.credit_used_cents > 0 && (
        <p className="text-[10px] text-muted-foreground">${(agent.credit_used_cents / 100).toFixed(2)} used</p>
      )}
    </div>
  );
}

// ── ApprovalSection ───────────────────────────────────────────────────────────

interface SectionProps {
  step: number;
  title: string;
  icon: React.ElementType;
  approved: boolean;
  onApprove: () => void;
  onRevise: () => void;
  children: React.ReactNode;
}

function ApprovalSection({ step, title, icon: Icon, approved, onApprove, onRevise, children }: SectionProps) {
  return (
    <div className={cn(
      'qr-card p-4 space-y-3 transition-colors',
      approved && 'border-emerald-500/30',
    )}>
      <div className="flex items-center gap-2">
        <span className={cn(
          'flex h-5 w-5 items-center justify-center rounded-full text-xs font-bold',
          approved ? 'text-emerald-500' : 'bg-muted text-muted-foreground',
        )}>
          {approved ? <CheckCircle2 className="h-4 w-4" /> : step}
        </span>
        <Icon className="h-4 w-4 text-muted-foreground" />
        <span className="text-sm font-semibold">{title}</span>
        {approved && (
          <button onClick={onRevise} className="qr-btn qr-btn-ghost qr-btn-xs ml-auto">
            Revise
          </button>
        )}
      </div>

      <div>{children}</div>

      {!approved && (
        <button onClick={onApprove} className="qr-btn qr-btn-outline qr-btn-sm text-emerald-500 border-emerald-500/40">
          <CheckCircle2 className="h-4 w-4" /> Approve
        </button>
      )}
    </div>
  );
}

// ── StepperHeader ─────────────────────────────────────────────────────────────

function StepperHeader({ steps }: { steps: { label: string; done: boolean }[] }) {
  return (
    <div className="flex items-center gap-1 qr-card px-3 py-2.5">
      {steps.map((s, i) => (
        <div key={s.label} className="flex items-center gap-1 flex-1 justify-center">
          <span className={cn('text-xs font-medium', s.done ? 'text-emerald-500' : 'text-muted-foreground')}>
            {s.done ? '✓' : `${i + 1}.`} {s.label}
          </span>
          {i < steps.length - 1 && <span className="text-muted-foreground/30 text-xs ml-1">→</span>}
        </div>
      ))}
    </div>
  );
}

// ── Chip ──────────────────────────────────────────────────────────────────────

function Chip({ label, value, wide }: { label: string; value: string; wide?: boolean }) {
  return (
    <div className={cn('rounded-lg border border-border bg-muted/20 p-2', wide && 'col-span-3')}>
      <p className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">{label}</p>
      <p className="text-xs font-semibold mt-0.5 capitalize">{value}</p>
    </div>
  );
}

// ── ProposedAgentRow ──────────────────────────────────────────────────────────

function ProposedAgentRow({ agent }: { agent: ProposedAgent }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="qr-card overflow-hidden">
      <button
        onClick={() => setOpen(v => !v)}
        className="flex w-full items-center gap-3 px-3 py-2.5 text-left hover:bg-accent/50 transition-colors"
      >
        <Bot className="h-4 w-4 text-muted-foreground shrink-0" />
        <div className="flex-1 min-w-0">
          <span className="text-sm font-medium">{agent.display_name}</span>
          <span className="ml-2 rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground capitalize">{agent.role}</span>
        </div>
        <span className="text-xs text-muted-foreground shrink-0">
          ${(agent.est_min_cents / 100).toFixed(0)}–${(agent.est_max_cents / 100).toFixed(0)}
        </span>
        {open
          ? <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
          : <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
        }
      </button>
      {open && (
        <div className="border-t border-border px-3 py-2 bg-muted/10">
          <p className="text-xs text-muted-foreground mb-1">{agent.model_label}</p>
          <ul className="space-y-1">
            {agent.tasks.map(t => (
              <li key={t} className="flex items-center gap-2 text-xs text-muted-foreground">
                <span className="h-1 w-1 rounded-full bg-muted-foreground/40 shrink-0" />
                {t}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

// ── ProposedTaskRow ───────────────────────────────────────────────────────────

const PRIORITY_DOT: Record<string, string> = {
  P0: 'bg-destructive',
  P1: 'bg-amber-500',
  P2: 'bg-muted-foreground/40',
};

function ProposedTaskRow({ task }: { task: ProposedTask }) {
  return (
    <div className="flex items-start gap-2.5 py-1.5 border-b border-border/50 last:border-0">
      <span className={cn('mt-1.5 h-2 w-2 rounded-full shrink-0', PRIORITY_DOT[task.priority] ?? 'bg-muted-foreground/40')} />
      <div className="flex-1 min-w-0">
        <p className="text-xs">{task.title}</p>
        <p className="text-[10px] text-muted-foreground capitalize">{task.role}</p>
      </div>
      <span className="text-[10px] text-muted-foreground shrink-0">
        ${(task.est_min_cents / 100).toFixed(0)}–${(task.est_max_cents / 100).toFixed(0)}
      </span>
    </div>
  );
}
