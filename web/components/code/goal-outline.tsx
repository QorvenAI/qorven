'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import { ChevronRight, ChevronDown, Target, CheckCircle2, Circle } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { WorkGoalTreeNode } from '@/types';

function GoalRow({
  node, depth, onToggle,
}: {
  node: WorkGoalTreeNode;
  depth: number;
  onToggle: (id: string, status: 'open' | 'done') => void;
}) {
  const [open, setOpen] = useState(depth < 1);
  const hasChildren = node.children.length > 0;
  const pct = node.ticket_count > 0 ? Math.round((node.done_count / node.ticket_count) * 100) : null;

  return (
    <>
      <div
        className="flex items-center gap-2 py-1.5 pr-2 rounded-lg hover:bg-accent/50 transition-colors group"
        style={{ paddingLeft: `${depth * 20 + 8}px` }}
      >
        {hasChildren ? (
          <button onClick={() => setOpen(!open)} className="p-0.5 text-muted-foreground/50 hover:text-foreground shrink-0">
            {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
          </button>
        ) : <span className="w-5 shrink-0" />}

        <button
          onClick={() => onToggle(node.id, node.status === 'done' ? 'open' : 'done')}
          className="shrink-0 text-muted-foreground/50 hover:text-primary transition-colors"
        >
          {node.status === 'done'
            ? <CheckCircle2 className="h-4 w-4 text-emerald-500" />
            : <Circle className="h-4 w-4" />}
        </button>

        <span className={cn('flex-1 text-sm truncate', node.status === 'done' && 'line-through text-muted-foreground')}>
          {node.title}
        </span>

        {pct !== null && (
          <div className="flex items-center gap-1.5 shrink-0">
            <div className="w-16 h-1.5 rounded-full bg-muted overflow-hidden">
              <div className="h-full rounded-full bg-primary transition-all" style={{ width: `${pct}%` }} />
            </div>
            <span className="text-xs text-muted-foreground tabular-nums w-7 text-right">{pct}%</span>
          </div>
        )}
      </div>

      {open && node.children.map(c => (
        <GoalRow key={c.id} node={c} depth={depth + 1} onToggle={onToggle} />
      ))}
    </>
  );
}

export function GoalOutline({
  goals,
  onToggle,
}: {
  goals: WorkGoalTreeNode[];
  onToggle: (id: string, status: 'open' | 'done') => void;
}) {
  if (goals.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 gap-2 text-center">
        <Target className="h-10 w-10 text-muted-foreground/20" />
        <p className="text-sm text-muted-foreground">No goals yet</p>
        <p className="text-xs text-muted-foreground/60">Add a mission goal to start planning</p>
      </div>
    );
  }

  return (
    <div className="space-y-0.5">
      {goals.map(g => <GoalRow key={g.id} node={g} depth={0} onToggle={onToggle} />)}
    </div>
  );
}
