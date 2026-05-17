'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { File, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import { fileColor } from './code-utils';
import type { FileTab } from './code-types';

export function EditorTabBar({ tabs, active, onSelect, onClose }: {
  tabs: FileTab[]; active: string; onSelect: (p: string) => void; onClose: (p: string) => void;
}) {
  if (tabs.length === 0) return null;
  return (
    <div className="flex h-[35px] shrink-0 items-end overflow-x-auto bg-muted/30 scrollbar-none border-b border-border">
      {tabs.map(t => (
        <button key={t.path} onClick={() => onSelect(t.path)}
          className={cn(
            'group relative flex h-[35px] shrink-0 items-center gap-1.5 border-r border-border px-3 text-[13px] transition-colors',
            t.path === active ? 'bg-background text-foreground border-t-2 border-t-primary' : 'bg-muted/20 text-muted-foreground hover:text-foreground'
          )}>
          <File className={cn('h-3 w-3 shrink-0', fileColor(t.name))} />
          <span className="max-w-[120px] truncate">{t.name}</span>
          {t.dirty && <span className="h-[6px] w-[6px] rounded-full bg-amber-400 shrink-0" />}
          <span onClick={e => { e.stopPropagation(); onClose(t.path); }}
            className={cn('flex h-4 w-4 items-center justify-center rounded text-muted-foreground hover:bg-accent shrink-0', t.dirty ? 'opacity-100' : 'opacity-0 group-hover:opacity-100')}>
            <X className="h-3 w-3" />
          </span>
        </button>
      ))}
    </div>
  );
}
