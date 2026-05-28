'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import { Bot, Clock, CheckCircle2, XCircle, Loader2, BarChart3 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { request } from '@/lib/api-core';

interface AgentJob {
  id: string;
  project_id?: string;
  project_name?: string;
  agent_id: string;
  agent_name?: string;
  title: string;
  status: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled';
  progress: number;
  started_at?: string;
  completed_at?: string;
  duration_ms?: number;
  cost_cents?: number;
  tokens_in?: number;
  tokens_out?: number;
  error?: string;
}

interface CommandCenterData {
  jobs: AgentJob[];
  stats: { queued: number; running: number; completed: number; failed: number };
}

const STATUS_CONFIG = {
  queued: { icon: Clock, color: 'text-muted-foreground', bg: 'bg-muted', label: 'Queued' },
  running: { icon: Loader2, color: 'text-blue-500', bg: 'bg-blue-500/10', label: 'Running' },
  completed: { icon: CheckCircle2, color: 'text-green-500', bg: 'bg-green-500/10', label: 'Done' },
  failed: { icon: XCircle, color: 'text-red-500', bg: 'bg-red-500/10', label: 'Failed' },
  cancelled: { icon: XCircle, color: 'text-muted-foreground', bg: 'bg-muted', label: 'Cancelled' },
} as const;

export function CommandCenter({ className }: { className?: string }) {
  const [data, setData] = useState<CommandCenterData | null>(null);

  const refresh = useCallback(async () => {
    try {
      const res = await request('/command-center') as CommandCenterData;
      setData(res);
    } catch {}
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 5000);
    return () => clearInterval(interval);
  }, [refresh]);

  if (!data) {
    return (
      <div className={cn('flex h-full items-center justify-center', className)}>
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  const { jobs, stats } = data;

  return (
    <div className={cn('flex h-full flex-col overflow-hidden', className)}>
      {/* Stats bar */}
      <div className="flex shrink-0 items-center gap-3 border-b border-border px-3 py-2">
        <BarChart3 className="h-4 w-4 text-muted-foreground" />
        <span className="text-xs font-medium">Command Center</span>
        <div className="ml-auto flex items-center gap-2">
          {stats.running > 0 && (
            <span className="flex items-center gap-1 rounded-full bg-blue-500/10 px-2 py-0.5 text-2xs text-blue-500">
              <Loader2 className="h-3 w-3 animate-spin" />
              {stats.running} running
            </span>
          )}
          {stats.queued > 0 && (
            <span className="rounded-full bg-muted px-2 py-0.5 text-2xs text-muted-foreground">
              {stats.queued} queued
            </span>
          )}
          <span className="text-2xs text-muted-foreground">
            {stats.completed} done
          </span>
        </div>
      </div>

      {/* Job list */}
      <div className="flex-1 overflow-y-auto">
        {jobs.length === 0 ? (
          <div className="flex h-full items-center justify-center">
            <div className="text-center space-y-1">
              <Bot className="mx-auto h-8 w-8 text-muted-foreground/30" />
              <p className="text-xs text-muted-foreground">No agent jobs yet</p>
            </div>
          </div>
        ) : (
          <div className="divide-y divide-border">
            {jobs.map(job => {
              const cfg = STATUS_CONFIG[job.status] || STATUS_CONFIG.queued;
              const Icon = cfg.icon;
              return (
                <div key={job.id} className="flex items-center gap-3 px-3 py-2.5 hover:bg-muted/30">
                  <div className={cn('flex h-7 w-7 items-center justify-center rounded-lg', cfg.bg)}>
                    <Icon className={cn('h-3.5 w-3.5', cfg.color, job.status === 'running' && 'animate-spin')} />
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="truncate text-xs font-medium">{job.title}</p>
                    <div className="flex items-center gap-2">
                      {job.agent_name && (
                        <span className="text-2xs text-muted-foreground">{job.agent_name}</span>
                      )}
                      {job.duration_ms != null && job.duration_ms > 0 && (
                        <span className="text-2xs text-muted-foreground">
                          {formatDuration(job.duration_ms)}
                        </span>
                      )}
                      {job.cost_cents != null && job.cost_cents > 0 && (
                        <span className="text-2xs text-muted-foreground">
                          ${(job.cost_cents / 100).toFixed(2)}
                        </span>
                      )}
                    </div>
                    {job.error && (
                      <p className="truncate text-2xs text-destructive mt-0.5">{job.error}</p>
                    )}
                  </div>
                  {job.status === 'running' && job.progress > 0 && (
                    <div className="w-12">
                      <div className="h-1.5 rounded-full bg-muted overflow-hidden">
                        <div
                          className="h-full rounded-full bg-blue-500 transition-all"
                          style={{ width: `${job.progress}%` }}
                        />
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  const rs = s % 60;
  return `${m}m ${rs}s`;
}
