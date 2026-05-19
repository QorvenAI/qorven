'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { cron as cronApi, agents } from '@/lib/api';
import { useStore } from '@/store';
import { cronToHuman, timeUntil } from '@/components/cron/cron-utils';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { ErrorBoundary } from '@/components/error-boundary';
import { TableRowSkeleton } from '@/components/skeletons';
import { cn } from '@/lib/utils';
import { Clock, Play, Pause, Trash2, Plus, ChevronDown, ChevronRight } from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import type { CronJob } from '@/types';

export default function CronPage() {
  const [jobs, setJobs] = useState<CronJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const souls = useStore((s) => s.souls);
  const setSouls = useStore((s) => s.setSouls);

  useEffect(() => {
    Promise.all([cronApi.list(), souls.length === 0 ? agents.list() : Promise.resolve(null)])
      .then(([cronJobs, agentData]) => {
        setJobs(cronJobs);
        if (agentData) setSouls(agentData);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, [souls.length, setSouls]);

  const soulMap = Object.fromEntries(souls.map((s) => [s.id, s]));

  const statusColor: Record<string, string> = {
    active: 'bg-soul-idle',
    paused: 'bg-soul-offline',
    running: 'bg-soul-running animate-pulse',
    completed: 'bg-soul-idle',
    failed: 'bg-soul-error',
  };

  return (
    <ErrorBoundary fallbackTitle="Failed to load cron jobs">
      <div className="space-y-6">
        <CanvasHeader
          title="Cron Jobs"
          description={`${jobs.length} scheduled job${jobs.length !== 1 ? 's' : ''}`}
          actions={
            <button className="qr-btn-primary">
              <Plus className="h-4 w-4" />
              New Schedule
            </button>
          }
        />

        {loading ? (
          <div className="space-y-1">{Array.from({ length: 4 }).map((_, i) => <TableRowSkeleton key={i} cols={5} />)}</div>
        ) : jobs.length === 0 ? (
          <EmptyState {...emptyStates.cron} />
        ) : (
          <div className="space-y-2">
            {jobs.map((job) => {
              const soul = soulMap[job.agent_id];
              const expanded = expandedId === job.id;

              return (
                <div key={job.id} className="rounded-xl border border-border bg-card">
                  {/* Job header */}
                  <button
                    onClick={() => setExpandedId(expanded ? null : job.id)}
                    className="flex w-full items-center gap-3 px-4 py-3 text-left"
                  >
                    {expanded ? <ChevronDown className="h-4 w-4 text-muted-foreground" /> : <ChevronRight className="h-4 w-4 text-muted-foreground" />}
                    <span className={cn('h-2 w-2 shrink-0 rounded-full', statusColor[job.status] ?? 'bg-soul-offline')} />
                    <Clock className="h-4 w-4 text-muted-foreground" />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{job.task}</p>
                      <p className="text-2xs text-muted-foreground">
                        {cronToHuman(job.expression)} · {soul?.display_name ?? 'Unknown'}
                        {job.next_run && <span className="ml-2">· Next: {timeUntil(job.next_run)}</span>}
                      </p>
                    </div>
                    <span className={cn('text-2xs font-medium', job.status === 'active' ? 'text-soul-idle' : 'text-muted-foreground')}>
                      {job.status}
                    </span>
                  </button>

                  {/* Expanded detail */}
                  {expanded && (
                    <div className="border-t border-border px-4 py-3 space-y-3">
                      <div className="grid gap-3 sm:grid-cols-3 text-sm">
                        <div>
                          <p className="text-2xs text-muted-foreground">Expression</p>
                          <code className="text-2sm font-[family-name:var(--font-mono)]">{job.expression}</code>
                        </div>
                        <div>
                          <p className="text-2xs text-muted-foreground">Last Run</p>
                          <p className="text-2sm">{job.last_run ? new Date(job.last_run).toLocaleString() : 'Never'}</p>
                        </div>
                        <div>
                          <p className="text-2xs text-muted-foreground">Next Run</p>
                          <p className="text-2sm">{job.next_run ? new Date(job.next_run).toLocaleString() : '—'}</p>
                        </div>
                      </div>

                      <div className="flex gap-2">
                        <button className="inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-2sm hover:bg-accent">
                          {job.status === 'active' ? <><Pause className="h-3.5 w-3.5" /> Pause</> : <><Play className="h-3.5 w-3.5" /> Resume</>}
                        </button>
                        <button className="inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-2sm text-destructive hover:bg-destructive/10">
                          <Trash2 className="h-3.5 w-3.5" /> Delete
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </ErrorBoundary>
  );
}
