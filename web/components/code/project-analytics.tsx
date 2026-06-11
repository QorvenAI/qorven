'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback, useRef } from 'react';
import { Loader2, GitBranch, Link2Off } from 'lucide-react';
import { projectBriefs } from '@/lib/api-workspace';
import { BudgetBar } from './budget-bar';
import type { ProjectAnalytics, ProjectAgentRow } from '@/types';
import { cn } from '@/lib/utils';

interface Props {
  briefId: string;
}

const DEBOUNCE_MS = 1000;

const TASK_STATUS_LABELS: Record<string, string> = {
  backlog:     'Backlog',
  assigned:    'Assigned',
  in_progress: 'In Progress',
  review:      'Review',
  done:        'Done',
  blocked:     'Blocked',
};

const TASK_STATUS_COLOR: Record<string, string> = {
  backlog:     'bg-muted text-muted-foreground',
  assigned:    'bg-muted text-foreground',
  in_progress: 'bg-primary/10 text-primary',
  review:      'bg-muted text-foreground',
  done:        'bg-primary/20 text-primary',
  blocked:     'bg-destructive/10 text-destructive',
};

function agentHealthBadge(row: ProjectAgentRow): { label: string; cls: string } {
  const h = row.health;
  if (row.status === 'suspended' || h?.suspended_from_ack) {
    return { label: 'Suspended', cls: 'bg-destructive/10 text-destructive border border-destructive/20' };
  }
  if (h && typeof h.consecutive_errors === 'number' && h.consecutive_errors > 0) {
    return { label: `${h.consecutive_errors} err${h.consecutive_errors !== 1 ? 's' : ''}`, cls: 'bg-amber-500/10 text-amber-600 border border-amber-500/20' };
  }
  return { label: 'OK', cls: 'bg-primary/10 text-primary border border-primary/20' };
}

export function ProjectAnalytics({ briefId }: Props) {
  const [data, setData] = useState<ProjectAnalytics | null>(null);
  const [loading, setLoading] = useState(true);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const fetchData = useCallback(async () => {
    try {
      const r = await projectBriefs.analytics(briefId);
      setData(r);
    } catch {
      /* non-fatal */
    } finally {
      setLoading(false);
    }
  }, [briefId]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  // Subscribe to WS CustomEvents and debounce re-fetch
  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent<{ project_id?: string }>).detail;
      if (detail?.project_id && detail.project_id !== briefId) return;
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(fetchData, DEBOUNCE_MS);
    };
    const evts = [
      'qorven:dashboard_refresh',
      'qorven:project_updated',
    ] as const;
    evts.forEach((name) => window.addEventListener(name, handler));
    return () => {
      evts.forEach((name) => window.removeEventListener(name, handler));
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [briefId, fetchData]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  // Compute max uusd for bar scaling
  const maxUusd = data ? Math.max(...(data.burn_trend.map(p => p.uusd)), 1) : 1;

  // All known statuses in order
  const STATUS_ORDER = ['backlog', 'assigned', 'in_progress', 'review', 'done', 'blocked'];

  return (
    <div className="overflow-y-auto h-full px-4 py-4 space-y-4">

      {/* Panel 1: Cost Burn */}
      <section className="rounded-lg border border-border bg-card p-4 space-y-3">
        <h3 className="text-sm font-semibold text-foreground">Cost Burn</h3>
        {/* Live gauge */}
        <BudgetBar projectId={briefId} />
        {/* Trend bars */}
        {data && data.burn_trend.length > 0 ? (
          <div className="space-y-1.5 pt-1">
            {data.burn_trend.map((pt) => {
              const pct = maxUusd > 0 ? Math.round((pt.uusd / maxUusd) * 100) : 0;
              return (
                <div key={pt.day} className="flex items-center gap-2">
                  <span className="w-20 shrink-0 text-xs text-muted-foreground truncate">{pt.day}</span>
                  <div className="flex-1 h-2 rounded-full bg-muted overflow-hidden">
                    <div
                      className="h-full rounded-full bg-primary/60 transition-all"
                      style={{ width: `${pct}%` }}
                    />
                  </div>
                  <span className="w-16 text-right text-xs text-muted-foreground">
                    ${(pt.uusd / 1_000_000).toFixed(2)}
                  </span>
                </div>
              );
            })}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">No burn trend data yet.</p>
        )}
      </section>

      {/* Panel 2: Agent Workload */}
      <section className="rounded-lg border border-border bg-card p-4 space-y-3">
        <h3 className="text-sm font-semibold text-foreground">Agent Workload</h3>
        {data && data.agents.length > 0 ? (
          <div className="space-y-1">
            {data.agents.map((row) => {
              const badge = agentHealthBadge(row);
              return (
                <div key={row.agent_id} className="flex items-center gap-3 py-1.5 border-b border-border last:border-0">
                  <div className="flex-1 min-w-0">
                    <p className="text-2sm font-medium text-foreground truncate">{row.name || row.agent_id}</p>
                    <p className="text-xs text-muted-foreground">{row.role}</p>
                  </div>
                  <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium', badge.cls)}>
                    {badge.label}
                  </span>
                </div>
              );
            })}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">No agents assigned yet.</p>
        )}
      </section>

      {/* Panel 3: Task Flow */}
      <section className="rounded-lg border border-border bg-card p-4 space-y-3">
        <h3 className="text-sm font-semibold text-foreground">Task Flow</h3>
        {data ? (
          <div className="flex flex-wrap gap-2">
            {STATUS_ORDER.map((status) => {
              const count = data.task_counts[status] ?? 0;
              const colorCls = TASK_STATUS_COLOR[status] ?? 'bg-muted text-muted-foreground';
              return (
                <div key={status} className={cn('rounded-md px-2.5 py-1.5 text-xs font-medium', colorCls)}>
                  <span className="font-semibold">{count}</span>
                  <span className="ml-1">{TASK_STATUS_LABELS[status] ?? status}</span>
                </div>
              );
            })}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">No task data.</p>
        )}
      </section>

      {/* Panel 4: PR / CI */}
      <section className="rounded-lg border border-border bg-card p-4 space-y-2">
        <h3 className="text-sm font-semibold text-foreground">PR / CI</h3>
        {data?.pr.connected ? (
          <div className="flex items-center gap-2">
            <GitBranch className="h-4 w-4 text-muted-foreground shrink-0" />
            <span className="text-2sm text-foreground font-medium">
              {data.pr.owner}/{data.pr.repo}
            </span>
            <span className="ml-auto text-xs text-muted-foreground">Live PR / CI in the GitHub tab</span>
          </div>
        ) : (
          <div className="flex items-center gap-2 text-muted-foreground">
            <Link2Off className="h-4 w-4 shrink-0" />
            <span className="text-2sm">Connect a repo to enable PR and CI tracking.</span>
          </div>
        )}
      </section>

    </div>
  );
}
