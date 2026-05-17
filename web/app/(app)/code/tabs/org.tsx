'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState, useEffect } from 'react';
import { LayoutGrid, Network } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useStore } from '@/store';
import { OrgTree } from '@/components/code/org-tree';
import { OrgGrid } from '@/components/code/org-grid';

type ViewMode = 'tree' | 'grid';

export function OrgTab() {
  const souls = useStore(s => s.souls);
  const [view, setView] = useState<ViewMode>(() => {
    if (typeof window !== 'undefined') {
      return (localStorage.getItem('qorven:org_view') as ViewMode) || 'tree';
    }
    return 'tree';
  });

  useEffect(() => {
    localStorage.setItem('qorven:org_view', view);
  }, [view]);

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="flex shrink-0 items-center gap-2 border-b border-border px-4 py-2.5">
        <span className="text-sm font-semibold flex-1">Organisation</span>
        <div className="flex rounded-lg border border-border overflow-hidden">
          <button
            onClick={() => setView('tree')}
            className={cn('flex items-center gap-1.5 px-3 py-1.5 text-xs transition-colors',
              view === 'tree' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground hover:bg-accent')}
          >
            <Network className="h-3.5 w-3.5" />
            Tree
          </button>
          <button
            onClick={() => setView('grid')}
            className={cn('flex items-center gap-1.5 px-3 py-1.5 text-xs transition-colors',
              view === 'grid' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground hover:bg-accent')}
          >
            <LayoutGrid className="h-3.5 w-3.5" />
            Grid
          </button>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto px-4 py-4">
        {view === 'tree' ? <OrgTree souls={souls} /> : <OrgGrid souls={souls} />}
      </div>
    </div>
  );
}
