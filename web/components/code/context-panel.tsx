'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { FileText, X } from 'lucide-react';
import { cn } from '@/lib/utils';

interface ContextPanelProps {
  files: string[];
  onRemove: (path: string) => void;
  className?: string;
}

export function ContextPanel({ files, onRemove, className }: ContextPanelProps) {
  if (files.length === 0) return null;

  return (
    <div className={cn('flex flex-wrap gap-1 border-b border-border px-3 py-1.5', className)}>
      {files.map(path => {
        const name = path.split('/').pop() ?? path;
        return (
          <span
            key={path}
            title={path}
            className="flex items-center gap-1 rounded-md border border-border bg-muted/40 pl-1.5 pr-1 py-0.5 text-2xs text-muted-foreground"
          >
            <FileText className="h-2.5 w-2.5 shrink-0" />
            <span className="max-w-[120px] truncate font-mono">{name}</span>
            <button
              type="button"
              onClick={() => onRemove(path)}
              className="ml-0.5 flex h-3.5 w-3.5 items-center justify-center rounded-sm hover:bg-destructive/20 hover:text-destructive transition-colors"
              aria-label={`Remove ${name} from context`}
            >
              <X className="h-2.5 w-2.5" />
            </button>
          </span>
        );
      })}
    </div>
  );
}
