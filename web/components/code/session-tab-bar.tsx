'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { Plus, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { CodeProject } from './code-types';

export interface SessionEntry {
  id: string;
  project: CodeProject;
  hasActivity: boolean;
}

interface SessionTabBarProps {
  sessions: SessionEntry[];
  activeSessionId: string | null;
  onSwitch: (id: string) => void;
  onClose: (id: string) => void;
  onNew: () => void;
}

export function SessionTabBar({
  sessions,
  activeSessionId,
  onSwitch,
  onClose,
  onNew,
}: SessionTabBarProps) {
  return (
    <div className="flex h-8 shrink-0 items-stretch overflow-x-auto border-b border-border bg-muted/30 scrollbar-none">
      {sessions.map(s => {
        const active = s.id === activeSessionId;
        const label = s.project.display_name || s.project.name;
        return (
          <div
            key={s.id}
            role="tab"
            aria-selected={active}
            onClick={() => onSwitch(s.id)}
            className={cn(
              'group relative flex min-w-0 max-w-[180px] shrink-0 cursor-pointer select-none items-center',
              'border-r border-border px-3 text-xs transition-colors',
              active
                ? 'bg-background text-foreground'
                : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground',
            )}
          >
            {s.hasActivity && (
              <span className="absolute left-1 top-1/2 h-1.5 w-1.5 -translate-y-1/2 rounded-full bg-primary animate-pulse" />
            )}
            <span className={cn('flex-1 truncate', s.hasActivity && 'pl-2.5')}>{label}</span>
            <button
              type="button"
              aria-label={`Close ${label}`}
              onClick={e => { e.stopPropagation(); onClose(s.id); }}
              className={cn(
                'ml-1.5 flex h-4 w-4 shrink-0 items-center justify-center rounded',
                'opacity-0 group-hover:opacity-100 transition-opacity',
                'hover:bg-destructive/20 hover:text-destructive',
              )}
            >
              <X className="h-2.5 w-2.5" />
            </button>
          </div>
        );
      })}

      <button
        type="button"
        title="Open project in new tab"
        onClick={onNew}
        className="flex h-full shrink-0 items-center px-2.5 text-muted-foreground hover:bg-muted/60 hover:text-foreground transition-colors"
      >
        <Plus className="h-3.5 w-3.5" />
      </button>
    </div>
  );
}
