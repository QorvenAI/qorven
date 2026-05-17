'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useMemo, useState } from 'react';
import { useStore } from '@/store';
import { TaskCard } from './TaskCard';
import type { DaemonTask } from '@/hooks/use-agents-stream';
import { Filter } from 'lucide-react';
import { cn } from '@/lib/utils';

type StatusFilter = 'all' | DaemonTask['status'];

const FILTERS: { label: string; value: StatusFilter }[] = [
  { label: 'All',         value: 'all' },
  { label: 'Active',      value: 'in_progress' },
  { label: 'Queued',      value: 'queued' },
  { label: 'Done',        value: 'done' },
  { label: 'Failed',      value: 'failed' },
];

interface TaskFeedProps {
  /** If set, only show tasks for this agent id */
  agentId?: string;
  compact?: boolean;
}

export function TaskFeed({ agentId, compact = false }: TaskFeedProps) {
  const allTasks  = useStore(s => s.daemonTasks);
  const allAgents = useStore(s => s.daemonAgents);
  const [filter, setFilter] = useState<StatusFilter>('all');

  const tasks = useMemo(() => {
    let list = Object.values(allTasks);
    if (agentId) list = list.filter(t => t.owner === agentId);
    if (filter !== 'all') list = list.filter(t => t.status === filter);
    // Sort: in_progress first, then queued, then by status, newest first for done/failed
    return list.sort((a, b) => {
      const rank = { in_progress: 0, queued: 1, done: 2, failed: 3, cancelled: 4 };
      return (rank[a.status] ?? 5) - (rank[b.status] ?? 5);
    });
  }, [allTasks, agentId, filter]);

  const counts = useMemo(() => {
    const src = agentId
      ? Object.values(allTasks).filter(t => t.owner === agentId)
      : Object.values(allTasks);
    return {
      all:         src.length,
      in_progress: src.filter(t => t.status === 'in_progress').length,
      queued:      src.filter(t => t.status === 'queued').length,
      done:        src.filter(t => t.status === 'done').length,
      failed:      src.filter(t => t.status === 'failed').length,
    };
  }, [allTasks, agentId]);

  return (
    <div className="space-y-3">
      {/* Filter bar */}
      <div className="flex items-center gap-1.5">
        <Filter className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
        <div className="flex items-center gap-1 flex-wrap">
          {FILTERS.map(f => {
            const count = counts[f.value as keyof typeof counts] ?? counts.all;
            return (
              <button key={f.value} onClick={() => setFilter(f.value)}
                className={cn(
                  'flex items-center gap-1 rounded-full px-2.5 py-1 text-2xs font-medium transition-colors',
                  filter === f.value
                    ? 'bg-primary/10 text-primary'
                    : 'text-muted-foreground hover:text-foreground hover:bg-accent',
                )}>
                {f.label}
                <span className="text-2xs opacity-60">{count}</span>
              </button>
            );
          })}
        </div>
      </div>

      {/* Task list */}
      {tasks.length === 0 ? (
        <p className="text-sm text-muted-foreground text-center py-6">No tasks{filter !== 'all' ? ` with status "${filter}"` : ''}</p>
      ) : (
        <div className="space-y-2">
          {tasks.map(task => (
            <TaskCard
              key={task.id}
              task={task}
              agentName={task.owner ? allAgents[task.owner]?.name : undefined}
              compact={compact}
            />
          ))}
        </div>
      )}
    </div>
  );
}
