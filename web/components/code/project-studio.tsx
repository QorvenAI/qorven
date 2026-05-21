'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useCallback, useEffect } from 'react';
import {
  CheckCircle2, Loader2, ChevronDown, ChevronRight,
  Rocket, Users, ListChecks, FileText, Zap, Bot, RefreshCw,
} from 'lucide-react';
import { projectBriefs as api } from '@/lib/api';
import { cn } from '@/lib/utils';
import type { ProjectBrief, ProposedAgent, ProposedTask, BriefAgent } from '@/types';

interface Props {
  brief: ProjectBrief;
  onBriefUpdate: (b: ProjectBrief) => void;
}

export function ProjectStudio({ brief, onBriefUpdate }: Props) {
  if (brief.status === 'intake') return <IntakeCanvas brief={brief} onBriefUpdate={onBriefUpdate} />;
  if (brief.status === 'proposed') return <ApprovalCanvas brief={brief} onBriefUpdate={onBriefUpdate} />;
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
          className="w-full flex items-center justify-center gap-2 rounded-xl bg-primary py-2.5 text-sm font-semibold text-primary-foreground hover:bg-primary/90 disabled:opacity-50 transition-colors"
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
        <div className="rounded-xl border border-primary/30 bg-primary/5 p-4 space-y-3">
          <p className="text-sm font-medium">All sections approved — ready to launch.</p>
          <p className="text-xs text-muted-foreground">
            Prime will spawn the team, assign tasks, and begin working autonomously.
            You can monitor progress and intervene at any time.
          </p>
          {err && <p className="text-xs text-destructive">{err}</p>}
          <button
            onClick={launch}
            disabled={launching}
            className="w-full flex items-center justify-center gap-2 rounded-xl bg-primary py-2.5 text-sm font-semibold text-primary-foreground hover:bg-primary/90 disabled:opacity-50 transition-colors"
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
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const res = await api.team(brief.id);
      setTeam(res.agents ?? []);
    } catch {
      // non-fatal
    } finally {
      setLoading(false);
    }
  }, [brief.id]);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, 10_000);
    return () => clearInterval(id);
  }, [refresh]);

  const done = team.filter(a => a.status === 'done').length;
  const working = team.filter(a => a.status !== 'done' && a.status !== 'paused').length;

  return (
    <div className="max-w-2xl mx-auto py-8 px-4 space-y-5">
      <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/5 p-4">
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-emerald-500/10">
            <Rocket className="h-5 w-5 text-emerald-500" />
          </div>
          <div>
            <p className="text-sm font-semibold text-emerald-500">
              {brief.status === 'done' ? 'Project complete' : 'Team working'}
            </p>
            <p className="text-xs text-muted-foreground">
              {working > 0 ? `${working} agent${working > 1 ? 's' : ''} active` : `${done} agents done`}
              {team.length > 0 && ` — ${done}/${team.length} finished`}
            </p>
          </div>
          <button
            onClick={() => { setLoading(true); refresh(); }}
            className="ml-auto text-muted-foreground hover:text-foreground"
          >
            <RefreshCw className={cn('h-4 w-4', loading && 'animate-spin')} />
          </button>
        </div>
      </div>

      <div>
        <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-2">
          Team — {team.length} agent{team.length !== 1 ? 's' : ''}
        </h3>
        <div className="space-y-2">
          {team.map(a => (
            <div key={a.id} className="flex items-center gap-3 rounded-xl border border-border bg-card p-3">
              <div className={cn('h-2 w-2 rounded-full shrink-0',
                a.status === 'done'   ? 'bg-emerald-500' :
                a.status === 'paused' ? 'bg-amber-500' :
                'bg-primary animate-pulse'
              )} />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium truncate">{a.display_name}</p>
                <p className="text-xs text-muted-foreground capitalize">{a.role} · {a.model}</p>
              </div>
              <div className="text-right shrink-0">
                <p className="text-xs font-medium">${(a.credit_used_cents / 100).toFixed(2)}</p>
                {a.credit_budget_cents != null && (
                  <p className="text-xs text-muted-foreground">of ${(a.credit_budget_cents / 100).toFixed(2)}</p>
                )}
              </div>
            </div>
          ))}
          {loading && team.length === 0 && (
            <div className="flex items-center gap-2 py-6 text-muted-foreground text-sm justify-center">
              <Loader2 className="h-4 w-4 animate-spin" /> Loading team…
            </div>
          )}
        </div>
      </div>
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
      'rounded-xl border p-4 space-y-3 transition-colors',
      approved ? 'border-emerald-500/30 bg-emerald-500/5' : 'border-border bg-card',
    )}>
      <div className="flex items-center gap-2">
        <span className="flex h-5 w-5 items-center justify-center rounded-full bg-muted text-xs font-bold text-muted-foreground">
          {approved ? <CheckCircle2 className="h-4 w-4 text-emerald-500" /> : step}
        </span>
        <Icon className="h-4 w-4 text-muted-foreground" />
        <span className="text-sm font-semibold">{title}</span>
        {approved && (
          <button onClick={onRevise} className="ml-auto text-xs text-muted-foreground hover:text-foreground transition-colors">
            Request changes
          </button>
        )}
      </div>

      <div>{children}</div>

      {!approved && (
        <button
          onClick={onApprove}
          className="flex items-center gap-1.5 rounded-lg border border-emerald-500/40 bg-emerald-500/10 px-4 py-1.5 text-sm font-medium text-emerald-500 hover:bg-emerald-500/20 transition-colors"
        >
          <CheckCircle2 className="h-4 w-4" /> Approve
        </button>
      )}
    </div>
  );
}

// ── StepperHeader ─────────────────────────────────────────────────────────────

function StepperHeader({ steps }: { steps: { label: string; done: boolean }[] }) {
  return (
    <div className="flex items-center gap-1 rounded-xl border border-border bg-muted/20 p-3">
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
    <div className="rounded-xl border border-border bg-muted/20 overflow-hidden">
      <button
        onClick={() => setOpen(v => !v)}
        className="flex w-full items-center gap-3 px-3 py-2.5 text-left hover:bg-muted/40 transition-colors"
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
