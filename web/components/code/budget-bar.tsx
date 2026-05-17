'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState, useEffect } from 'react';
import { AlertTriangle } from 'lucide-react';
import { cn } from '@/lib/utils';

interface BudgetWarning {
  agent_id: string;
  used: number;
  budget: number;
  pct: number;
}

interface Props {
  agentIds: string[];
  agentLabels: Record<string, string>;
}

export function BudgetBar({ agentIds, agentLabels }: Props) {
  const [warnings, setWarnings] = useState<Record<string, BudgetWarning>>({});

  useEffect(() => {
    const handler = (e: Event) => {
      const data = (e as CustomEvent<BudgetWarning>).detail;
      if (data?.agent_id && agentIds.includes(data.agent_id)) {
        setWarnings(prev => ({ ...prev, [data.agent_id]: data }));
      }
    };
    window.addEventListener('qorven:budget_warning', handler);
    return () => window.removeEventListener('qorven:budget_warning', handler);
  }, [agentIds.join(',')]);

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
