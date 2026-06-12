'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { cn } from '@/lib/utils';
import { soulGradient } from '@/components/soul-card';
import type { TimelineItem } from '@/lib/api-workspace';
import { Repeat, Clock } from 'lucide-react';

// bg-soul-idle = emerald (ok/done), bg-soul-error = red (error),
// bg-primary = brand blue (running/scheduled), bg-muted-foreground = grey (paused/cancelled).
// bg-success and bg-warning are not defined tokens in this project.
const STATUS_DOT: Record<string, string> = {
  scheduled:  'bg-muted-foreground',
  running:    'bg-primary animate-pulse',
  ok:         'bg-soul-idle',
  done:       'bg-soul-idle',
  error:      'bg-destructive',
  paused:     'bg-muted-foreground',
  cancelled:  'bg-muted-foreground',
};

export function TimelineItemBlock({
  item,
  onClick,
  compact,
}: {
  item: TimelineItem;
  onClick?: () => void;
  compact?: boolean;
}) {
  const name = item.agent_name || 'Unassigned';
  return (
    <button
      onClick={onClick}
      className={cn(
        'flex w-full items-center gap-2 rounded-md border border-border bg-card px-2 py-1 text-left transition-colors hover:bg-accent',
        compact ? 'text-2xs' : 'text-xs'
      )}
    >
      <span
        className={cn(
          'h-1.5 w-1.5 shrink-0 rounded-full',
          STATUS_DOT[item.status] ?? 'bg-muted-foreground'
        )}
      />
      <span
        className={cn(
          'flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-gradient-to-br text-2xs font-semibold text-white',
          soulGradient(name)
        )}
      >
        {name.charAt(0).toUpperCase()}
      </span>
      <span className="flex-1 truncate">{item.title}</span>
      {item.recurring ? (
        <Repeat className="h-3 w-3 shrink-0 text-muted-foreground" />
      ) : (
        <Clock className="h-3 w-3 shrink-0 text-muted-foreground" />
      )}
    </button>
  );
}
