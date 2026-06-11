'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { OrgPipeline } from './org-pipeline';
import { ProjectHubView } from './project-hub-view';
import { ProjectAnalytics } from './project-analytics';
import { ProjectTimeline } from './project-timeline';
import { SwarmView } from './swarm-view';
import { cn } from '@/lib/utils';

type BriefTab = 'pipeline' | 'hub' | 'analytics' | 'timeline' | 'build';

const TABS: { id: BriefTab; label: string }[] = [
  { id: 'pipeline',  label: 'Pipeline'  },
  { id: 'hub',       label: 'Hub'       },
  { id: 'analytics', label: 'Analytics' },
  { id: 'timeline',  label: 'Timeline'  },
  { id: 'build',     label: 'Build'     },
];

interface Props {
  briefId: string;
}

export function ProjectBriefTabs({ briefId }: Props) {
  const [active, setActive] = useState<BriefTab>('pipeline');

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Sub-tab bar */}
      <div className="shrink-0 flex items-center gap-1 border-b border-border px-4 py-1.5">
        {TABS.map((t) => (
          <button
            key={t.id}
            onClick={() => setActive(t.id)}
            className={cn(
              'rounded-md px-2.5 py-1 text-2sm font-medium transition-colors',
              active === t.id
                ? 'bg-muted text-foreground'
                : 'text-muted-foreground hover:text-foreground hover:bg-muted/50',
            )}
          >
            {t.label}
          </button>
        ))}
      </div>

      {/* Tab content — all mounted, visibility toggled so state is preserved */}
      <div className={cn('flex-1 min-h-0 overflow-hidden', active !== 'pipeline' && 'hidden')}>
        <OrgPipeline briefId={briefId} />
      </div>
      <div className={cn('flex-1 min-h-0 overflow-hidden', active !== 'hub' && 'hidden')}>
        <ProjectHubView briefId={briefId} />
      </div>
      <div className={cn('flex-1 min-h-0 overflow-hidden', active !== 'analytics' && 'hidden')}>
        <ProjectAnalytics briefId={briefId} />
      </div>
      <div className={cn('flex-1 min-h-0 overflow-hidden', active !== 'timeline' && 'hidden')}>
        <ProjectTimeline briefId={briefId} />
      </div>
      <div className={cn('flex-1 min-h-0 overflow-hidden', active !== 'build' && 'hidden')}>
        <SwarmView briefId={briefId} />
      </div>
    </div>
  );
}
