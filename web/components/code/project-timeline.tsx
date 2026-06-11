'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback, useRef } from 'react';
import {
  Play,
  Activity,
  GitPullRequest,
  AlertTriangle,
  CheckCircle,
  Clock,
  Loader2,
} from 'lucide-react';
import { projectBriefs } from '@/lib/api-workspace';
import type { ProjectEvent } from '@/types';
import { cn } from '@/lib/utils';

interface Props {
  briefId: string;
}

const EVENT_TYPE_ICON: Record<string, React.ComponentType<{ className?: string }>> = {
  task_started:   Play,
  task_done:      CheckCircle,
  task_blocked:   AlertTriangle,
  pr_opened:      GitPullRequest,
  pr_merged:      GitPullRequest,
  pr_ready:       GitPullRequest,
  agent_activity: Activity,
};

function iconFor(type: string): React.ComponentType<{ className?: string }> {
  return EVENT_TYPE_ICON[type] ?? Activity;
}

const EVENT_ICON_COLOR: Record<string, string> = {
  task_done:    'text-primary',
  task_blocked: 'text-destructive',
  pr_merged:    'text-primary',
  task_started: 'text-muted-foreground',
};

function iconColor(type: string): string {
  return EVENT_ICON_COLOR[type] ?? 'text-muted-foreground';
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const secs = Math.floor(diff / 1000);
  if (secs < 60) return `${secs}s ago`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  return `${Math.floor(hrs / 24)}d ago`;
}

const DEBOUNCE_MS = 1000;

export function ProjectTimeline({ briefId }: Props) {
  const [events, setEvents] = useState<ProjectEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const fetchEvents = useCallback(async () => {
    try {
      const r = await projectBriefs.events(briefId);
      setEvents(r.events ?? []);
    } catch {
      /* non-fatal */
    } finally {
      setLoading(false);
    }
  }, [briefId]);

  useEffect(() => {
    fetchEvents();
  }, [fetchEvents]);

  // Subscribe to WS CustomEvents and debounce re-fetch
  useEffect(() => {
    const handler = (e: Event) => {
      const data = (e as CustomEvent<{ project_id?: string }>).detail;
      if (data?.project_id && data.project_id !== briefId) return;
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(fetchEvents, DEBOUNCE_MS);
    };
    const evts = [
      'qorven:project_updated',
      'qorven:task_progress',
      'qorven:task_done',
      'qorven:task_blocked',
    ] as const;
    evts.forEach((name) => window.addEventListener(name, handler));
    return () => {
      evts.forEach((name) => window.removeEventListener(name, handler));
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [briefId, fetchEvents]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (events.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 gap-2">
        <Clock className="h-8 w-8 text-muted-foreground" />
        <p className="text-2sm text-muted-foreground">No events yet.</p>
      </div>
    );
  }

  return (
    <div className="overflow-y-auto h-full px-4 py-3">
      <ul className="space-y-0">
        {events.map((ev, idx) => {
          const Icon = iconFor(ev.type);
          const color = iconColor(ev.type);
          const isLast = idx === events.length - 1;
          return (
            <li key={ev.id} className="flex gap-3">
              {/* Icon + vertical line */}
              <div className="flex flex-col items-center">
                <div className={cn('mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-muted', color)}>
                  <Icon className="h-3.5 w-3.5" />
                </div>
                {!isLast && <div className="w-px flex-1 bg-border mt-1" />}
              </div>
              {/* Content */}
              <div className={cn('flex-1 pb-4', isLast && 'pb-2')}>
                <p className="text-2sm font-medium text-foreground leading-snug">{ev.title || ev.type}</p>
                <div className="flex items-center gap-2 mt-0.5">
                  <span className="text-xs text-muted-foreground">{relativeTime(ev.created_at)}</span>
                  {ev.agent_id && (
                    <span className="text-xs text-muted-foreground truncate max-w-[120px]">
                      · {ev.agent_id}
                    </span>
                  )}
                </div>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
