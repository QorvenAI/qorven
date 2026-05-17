'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import { ChevronRight, ExternalLink } from 'lucide-react';
import { cn } from '@/lib/utils';
import { fileColor } from './code-utils';

export function FileChangeChip({ path, linesAdded = 0, linesRemoved = 0, totalLines, onClick }: {
  path: string;
  linesAdded?: number;
  linesRemoved?: number;
  totalLines?: number;
  onClick?: (path: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const name = path.split('/').pop() || path;
  const dir = path.includes('/') ? path.split('/').slice(0, -1).join('/') : '';
  const hasStats = linesAdded > 0 || linesRemoved > 0;

  return (
    <div className="inline-flex flex-col my-0.5">
      <div className="inline-flex items-center gap-1.5 rounded-md border border-border bg-card px-2 py-1 text-xs max-w-full group hover:border-primary/40 transition-colors">
        <button
          onClick={() => setExpanded(v => !v)}
          className="flex items-center gap-0.5 text-muted-foreground hover:text-foreground transition-colors shrink-0"
        >
          <ChevronRight className={cn('h-3 w-3 transition-transform', expanded && 'rotate-90')} />
        </button>

        <button
          onClick={() => onClick?.(path)}
          className="flex items-center gap-1.5 min-w-0 flex-1"
        >
          <span className={cn('shrink-0 text-xs', fileColor(name))}>◆</span>
          <span className="font-mono font-medium text-foreground truncate">{name}</span>
          {dir && <span className="text-muted-foreground/50 truncate text-2xs hidden sm:block">{dir}</span>}
        </button>

        {hasStats && (
          <div className="flex items-center gap-1 shrink-0 font-mono">
            {linesAdded > 0 && <span className="text-emerald-500">+{linesAdded}</span>}
            {linesRemoved > 0 && <span className="text-destructive">-{linesRemoved}</span>}
            {totalLines !== undefined && <span className="text-muted-foreground/50">{totalLines}</span>}
          </div>
        )}

        <button
          onClick={() => onClick?.(path)}
          className="opacity-0 group-hover:opacity-100 shrink-0 text-muted-foreground hover:text-foreground transition-all"
          title="Open file"
        >
          <ExternalLink className="h-3 w-3" />
        </button>
      </div>

      {expanded && (
        <div className="mt-0.5 ml-4 rounded-b-md border border-t-0 border-border bg-muted/20 px-2 py-1 text-2xs text-muted-foreground font-mono">
          {path}
        </div>
      )}
    </div>
  );
}
