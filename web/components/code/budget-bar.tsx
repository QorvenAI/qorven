'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect } from 'react';
import { AlertTriangle } from 'lucide-react';
import { cn } from '@/lib/utils';
import { projectBriefs } from '@/lib/api-workspace';
import type { ProjectBurn } from '@/types';

interface BudgetWarning {
  agent_id: string;
  used: number;
  budget: number;
  pct: number;
}

interface Props {
  // Event-listener mode (existing)
  agentIds?: string[];
  agentLabels?: Record<string, string>;
  // Polled mode (new) — when provided, polls the /burn endpoint every ~12 s
  projectId?: string;
}

const POLL_INTERVAL_MS = 12_000;

export function BudgetBar({ agentIds = [], agentLabels = {}, projectId }: Props) {
  // Event-listener state (existing mode)
  const [warnings, setWarnings] = useState<Record<string, BudgetWarning>>({});

  // Polled burn state (new mode)
  const [burn, setBurn] = useState<ProjectBurn | null>(null);

  // Existing event-listener logic — unchanged
  useEffect(() => {
    if (agentIds.length === 0) return;
    const handler = (e: Event) => {
      const data = (e as CustomEvent<BudgetWarning>).detail;
      if (data?.agent_id && agentIds.includes(data.agent_id)) {
        setWarnings(prev => ({ ...prev, [data.agent_id]: data }));
      }
    };
    window.addEventListener('qorven:budget_warning', handler);
    return () => window.removeEventListener('qorven:budget_warning', handler);
  }, [agentIds.join(',')]);

  // New polled-mode logic — only active when projectId is provided
  useEffect(() => {
    if (!projectId) return;

    let cancelled = false;
    const poll = () => {
      projectBriefs.burn(projectId).then(data => {
        if (!cancelled) setBurn(data);
      }).catch(() => { /* silently ignore transient errors */ });
    };

    poll(); // immediate first fetch
    const timer = setInterval(poll, POLL_INTERVAL_MS);
    return () => { cancelled = true; clearInterval(timer); };
  }, [projectId]);

  // ── Polled-mode render ────────────────────────────────────────────────────
  if (projectId) {
    if (!burn) return null;
    const pct = Math.min(burn.pct, 100);
    const isOver = pct >= 100;
    const isWarn = !isOver && pct >= burn.warn_pct;
    const barColor = isOver ? 'bg-destructive' : isWarn ? 'bg-amber-500' : 'bg-primary';
    const textColor = isOver ? 'text-destructive' : isWarn ? 'text-amber-500' : 'text-muted-foreground';
    return (
      <div className="space-y-0.5">
        <div className="flex items-center gap-1.5">
          {isOver && <AlertTriangle className="h-3 w-3 text-destructive" />}
          <span className="text-xs text-muted-foreground">
            ${burn.used_usd.toFixed(2)} / ${burn.cap_usd.toFixed(2)}
          </span>
          <span className={cn('text-xs ml-auto font-medium', textColor)}>
            {pct.toFixed(0)}%
          </span>
        </div>
        <div className="h-1 w-full rounded-full bg-muted overflow-hidden">
          <div
            className={cn('h-full rounded-full transition-all', barColor)}
            style={{ width: `${pct}%` }}
          />
        </div>
      </div>
    );
  }

  // ── Event-listener-mode render (existing) ────────────────────────────────
  const activeWarnings = agentIds.filter(id => warnings[id]);
  if (activeWarnings.length === 0) return null;

  return (
    <div className="space-y-1.5">
      {activeWarnings.map(id => {
        const w = warnings[id]!;
        const pct = Math.min(w.pct, 100);
        const isOver = pct >= 100;
        return (
          <div key={id} className="space-y-0.5">
            <div className="flex items-center gap-1.5">
              {isOver && <AlertTriangle className="h-3 w-3 text-destructive" />}
              <span className="text-xs text-muted-foreground">
                {agentLabels[id] ?? id}: ${(w.used / 100).toFixed(2)} / ${(w.budget / 100).toFixed(2)}
              </span>
              <span className={cn('text-xs ml-auto font-medium', isOver ? 'text-destructive' : 'text-amber-500')}>
                {pct}%
              </span>
            </div>
            <div className="h-1 w-full rounded-full bg-muted overflow-hidden">
              <div
                className={cn('h-full rounded-full transition-all', isOver ? 'bg-destructive' : 'bg-amber-500')}
                style={{ width: `${pct}%` }}
              />
            </div>
          </div>
        );
      })}
    </div>
  );
}
