'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect } from 'react';
import {
  CheckCircle2, Loader2, Users, ListChecks, DollarSign,
  Zap, AlertCircle, ChevronDown, ChevronRight, Bot,
} from 'lucide-react';
import { projectBriefs as api } from '@/lib/api';
import { BudgetBar } from '@/components/code/budget-bar';
import type { ProjectBrief, ProposedAgent, ProposedTask, BriefAgent } from '@/types';
import { cn } from '@/lib/utils';

interface Props {
  brief: ProjectBrief;
  approveResult: { agents: Record<string, string>; tickets: Record<string, string> } | null;
  onApprove: (result: { brief: ProjectBrief; agents: Record<string, string>; tickets: Record<string, string> }) => void;
}

const QUALITY_COLOR: Record<string, string> = {
  mvp:        'text-emerald-500 bg-emerald-500/10',
  production: 'text-blue-500 bg-blue-500/10',
  enterprise: 'text-violet-500 bg-violet-500/10',
};

const STATUS_COLOR: Record<string, string> = {
  intake:    'text-muted-foreground',
  proposed:  'text-amber-500',
  approved:  'text-blue-500',
  active:    'text-emerald-500',
  done:      'text-emerald-600',
  cancelled: 'text-destructive',
};

function AgentRow({ agent }: { agent: ProposedAgent }) {
  const [expanded, setExpanded] = useState(false);
  return (
    <div className="rounded-xl border border-border bg-muted/20 overflow-hidden">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-3 px-3.5 py-2.5 text-left hover:bg-muted/40 transition-colors"
      >
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-semibold">{agent.display_name}</span>
            <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground capitalize">{agent.role}</span>
          </div>
          <p className="text-xs text-muted-foreground mt-0.5 truncate">{agent.model_label}</p>
        </div>
        <div className="text-right shrink-0">
          <p className="text-xs font-medium">${(agent.est_min_cents / 100).toFixed(0)}–${(agent.est_max_cents / 100).toFixed(0)}</p>
          <p className="text-xs text-muted-foreground">est.</p>
        </div>
        {expanded
          ? <ChevronDown className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
          : <ChevronRight className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
        }
      </button>
      {expanded && (
        <div className="border-t border-border px-3.5 py-2 bg-muted/10">
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

function TaskRow({ task }: { task: ProposedTask }) {
  return (
    <div className="flex items-start gap-2.5 py-1.5 border-b border-border/50 last:border-0">
      <div className={cn(
        'mt-0.5 h-2 w-2 rounded-full shrink-0',
        task.priority === 'high' ? 'bg-amber-500' : task.priority === 'low' ? 'bg-muted-foreground/30' : 'bg-primary/50'
      )} />
      <div className="flex-1 min-w-0">
        <p className="text-xs font-medium">{task.title}</p>
        {task.blocked_by.length > 0 && (
          <p className="text-xs text-muted-foreground mt-0.5">
            after: {task.blocked_by.join(', ')}
          </p>
        )}
      </div>
      <span className="text-xs text-muted-foreground capitalize shrink-0">{task.role}</span>
    </div>
  );
}

function LiveTeamPanel({ brief }: { brief: ProjectBrief }) {
  const [agents, setAgents] = useState<BriefAgent[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    api.team(brief.id).then(r => { if (active) { setAgents(r?.agents ?? []); setLoading(false); } }).catch(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [brief.id]);

  // Re-fetch on budget_warning or project_updated events
  useEffect(() => {
    const refresh = () => {
      api.team(brief.id).then(r => setAgents(r?.agents ?? [])).catch(() => {});
    };
    window.addEventListener('qorven:project_updated', refresh);
    window.addEventListener('qorven:budget_warning', refresh);
    return () => {
      window.removeEventListener('qorven:project_updated', refresh);
      window.removeEventListener('qorven:budget_warning', refresh);
    };
  }, [brief.id]);

  const agentIds = agents.map(a => a.id);
  const agentLabels = Object.fromEntries(agents.map(a => [a.id, a.display_name]));

  return (
    <div className="rounded-2xl border border-border bg-gradient-to-br from-emerald-500/5 to-transparent p-5 space-y-4">
      <div className="flex items-center gap-3">
        {brief.status === 'done'
          ? <CheckCircle2 className="h-5 w-5 text-emerald-500" />
          : <Loader2 className="h-5 w-5 text-primary animate-spin" />
        }
        <div>
          <h3 className="text-sm font-semibold">
            {brief.status === 'done' ? 'Project Complete' : 'Work in Progress'}
          </h3>
          <p className="text-xs text-muted-foreground mt-0.5">{brief.title}</p>
        </div>
      </div>

      {loading ? (
        <div className="flex justify-center py-4">
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <>
          {/* Agent list */}
          {agents.length > 0 ? (
            <div className="space-y-2">
              <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Team</p>
              {agents.map(a => (
                <div key={a.id} className="rounded-xl border border-border bg-muted/20 flex items-center gap-3 px-3.5 py-2.5">
                  <Bot className="h-4 w-4 text-muted-foreground shrink-0" />
                  <div className="flex-1 min-w-0">
                    <p className="text-xs font-semibold truncate">{a.display_name}</p>
                    <p className="text-xs text-muted-foreground capitalize">{a.role} · {a.model}</p>
                  </div>
                  <span className={cn(
                    'rounded-full px-2 py-0.5 text-xs font-medium capitalize',
                    a.status === 'active' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-muted text-muted-foreground'
                  )}>
                    {a.status}
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground text-center py-2">No agents yet</p>
          )}

          {/* Budget bars */}
          {agentIds.length > 0 && (
            <div className="space-y-2">
              <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Budget</p>
              <BudgetBar agentIds={agentIds} agentLabels={agentLabels} />
            </div>
          )}
        </>
      )}
    </div>
  );
}

export function TeamProposalCard({ brief, approveResult, onApprove }: Props) {
  const [approving, setApproving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const approve = async () => {
    if (approving) return;
    setApproving(true);
    setError(null);
    try {
      const result = await api.approve(brief.id);
      onApprove(result);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Approval failed');
    } finally {
      setApproving(false);
    }
  };

  // Active / done: show live team fetched from backend
  if (brief.status === 'active' || brief.status === 'done') {
    return <LiveTeamPanel brief={brief} />;
  }

  // No proposal yet
  if (!brief.proposal) {
    return (
      <div className="flex flex-col items-center justify-center h-64 text-center space-y-3">
        <div className="rounded-full bg-muted/40 p-4">
          <Zap className="h-6 w-6 text-muted-foreground/50" />
        </div>
        <div>
          <p className="text-sm font-medium">No proposal yet</p>
          <p className="text-xs text-muted-foreground mt-1">
            {brief.status === 'intake'
              ? 'Complete the intake chat to generate a proposal'
              : 'Proposal will appear here'}
          </p>
        </div>
      </div>
    );
  }

  const { proposal } = brief;

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-start gap-3">
        <div className="flex-1">
          <h2 className="text-base font-semibold">{brief.title}</h2>
          <div className="flex items-center gap-2 mt-1.5">
            <span className={cn('rounded-full px-2 py-0.5 text-xs font-semibold capitalize', QUALITY_COLOR[brief.quality] || 'text-muted-foreground bg-muted')}>
              {brief.quality}
            </span>
            <span className={cn('text-xs capitalize', STATUS_COLOR[brief.status] || 'text-muted-foreground')}>
              {brief.status}
            </span>
          </div>
        </div>
        <div className="text-right shrink-0">
          <p className="text-lg font-bold">
            ${(proposal.est_min_cents / 100).toFixed(0)}–${(proposal.est_max_cents / 100).toFixed(0)}
          </p>
          <p className="text-xs text-muted-foreground">
            estimated · budget {brief.budget_cents > 0 ? `$${brief.budget_cents / 100}` : 'no limit'}
          </p>
        </div>
      </div>

      {/* Reasoning */}
      <div className="rounded-xl border border-border bg-muted/20 px-4 py-3">
        <p className="text-xs text-muted-foreground leading-relaxed">{proposal.reasoning}</p>
      </div>

      {/* Agents */}
      <div>
        <div className="flex items-center gap-2 mb-2.5">
          <Users className="h-4 w-4 text-muted-foreground" />
          <h3 className="text-sm font-semibold">Team ({proposal.agents.length})</h3>
        </div>
        <div className="space-y-2">
          {proposal.agents.map(agent => (
            <AgentRow key={agent.role} agent={agent} />
          ))}
        </div>
      </div>

      {/* Task graph */}
      <div>
        <div className="flex items-center gap-2 mb-2.5">
          <ListChecks className="h-4 w-4 text-muted-foreground" />
          <h3 className="text-sm font-semibold">Task Graph ({proposal.tasks.length})</h3>
        </div>
        <div className="rounded-xl border border-border bg-muted/10 px-3.5 py-1">
          {proposal.tasks.map(task => (
            <TaskRow key={task.title} task={task} />
          ))}
        </div>
      </div>

      {/* Error */}
      {error && (
        <div className="flex items-center gap-2 rounded-xl border border-destructive/30 bg-destructive/5 px-3.5 py-2.5">
          <AlertCircle className="h-4 w-4 text-destructive shrink-0" />
          <p className="text-xs text-destructive">{error}</p>
        </div>
      )}

      {/* Approve */}
      {brief.status === 'proposed' && (
        <button
          onClick={approve}
          disabled={approving}
          className="flex w-full items-center justify-center gap-2 rounded-xl bg-primary px-4 py-3 text-sm font-bold text-primary-foreground hover:bg-primary/90 disabled:opacity-50 transition-colors"
        >
          {approving ? <Loader2 className="h-4 w-4 animate-spin" /> : <DollarSign className="h-4 w-4" />}
          {approving ? 'Creating team & tickets…' : 'Approve & Start Work'}
        </button>
      )}
    </div>
  );
}
