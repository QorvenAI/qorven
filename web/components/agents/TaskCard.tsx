'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { cn } from '@/lib/utils';
import type { DaemonTask } from '@/hooks/use-agents-stream';
import { CheckCircle2, AlertCircle, Loader2, Clock, File } from 'lucide-react';

const PRIORITY_STYLE: Record<string, string> = {
  high:   'bg-red-500/10 text-red-400',
  normal: 'bg-muted text-muted-foreground',
  low:    'bg-muted/50 text-muted-foreground/70',
};

const STATUS_ICON = {
  queued:      <Clock className="h-3.5 w-3.5 text-muted-foreground" />,
  in_progress: <Loader2 className="h-3.5 w-3.5 text-amber-400 animate-spin" />,
  done:        <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400" />,
  failed:      <AlertCircle className="h-3.5 w-3.5 text-red-400" />,
  cancelled:   <Clock className="h-3.5 w-3.5 text-muted-foreground/50" />,
};

interface TaskCardProps {
  task: DaemonTask;
  agentName?: string;
  compact?: boolean;
}

export function TaskCard({ task, agentName, compact = false }: TaskCardProps) {
  const icon = STATUS_ICON[task.status] ?? STATUS_ICON.queued;
  const files = task.files_changed ?? [];

  return (
    <div className={cn(
      'rounded-xl border border-border bg-card transition-colors',
      task.status === 'in_progress' && 'border-amber-500/30',
      task.status === 'done'        && 'border-emerald-500/20',
      task.status === 'failed'      && 'border-red-500/30',
      compact ? 'px-3 py-2.5' : 'px-4 py-3',
    )}>
      {/* Header */}
      <div className="flex items-start gap-2">
        <span className="mt-0.5 shrink-0">{icon}</span>
        <div className="flex-1 min-w-0">
          <p className={cn('font-medium truncate', compact ? 'text-xs' : 'text-sm')}>{task.title}</p>
          {!compact && task.description && task.description !== task.title && (
            <p className="text-xs text-muted-foreground mt-0.5 line-clamp-2">{task.description}</p>
          )}
        </div>
        <span className={cn('shrink-0 text-2xs font-medium px-1.5 py-0.5 rounded-full', PRIORITY_STYLE[task.priority])}>
          {task.priority}
        </span>
      </div>

      {/* Progress bar */}
      {task.status === 'in_progress' && typeof task.percent === 'number' && task.percent > 0 && (
        <div className="mt-2.5 h-1 rounded-full bg-muted overflow-hidden">
          <div className="h-full rounded-full bg-amber-400 transition-all" style={{ width: `${task.percent}%` }} />
        </div>
      )}

      {/* Footer */}
      {!compact && (
        <div className="mt-2.5 flex items-center gap-3 text-2xs text-muted-foreground">
          {agentName && <span>{agentName}</span>}
          {task.summary && task.status === 'done' && (
            <span className="text-emerald-400 truncate">{task.summary}</span>
          )}
          {task.error && task.status === 'failed' && (
            <span className="text-red-400 truncate">{task.error}</span>
          )}
          {files.length > 0 && (
            <span className="flex items-center gap-0.5 ml-auto shrink-0">
              <File className="h-3 w-3" />{files.length} file{files.length !== 1 ? 's' : ''}
            </span>
          )}
        </div>
      )}
    </div>
  );
}
