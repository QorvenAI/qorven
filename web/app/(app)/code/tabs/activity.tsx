'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useMemo } from 'react';
import { useStore } from '@/store';
import {
  Zap, MessageSquare, CheckCircle2, AlertCircle, GitCommit,
  ShieldAlert, Bot, RotateCcw, Activity,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import type { LiveEvent } from '@/types';

function relTime(ts: number): string {
  const diff = Date.now() - ts;
  if (diff < 60_000) return `${Math.round(diff / 1_000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  return new Date(ts).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
}

const KIND_META: Record<string, { icon: typeof Zap; cls: string; label: string }> = {
  'message':              { icon: MessageSquare, cls: 'text-primary',       label: 'Message' },
  'agent_thought':        { icon: Bot,           cls: 'text-sky-400',       label: 'Thinking' },
  'tool_call':            { icon: Zap,           cls: 'text-amber-400',     label: 'Tool call' },
  'tool_result':          { icon: CheckCircle2,  cls: 'text-emerald-400',   label: 'Tool result' },
  'task_started':         { icon: Zap,           cls: 'text-blue-400',      label: 'Task started' },
  'task_completed':       { icon: CheckCircle2,  cls: 'text-emerald-500',   label: 'Task done' },
  'task_failed':          { icon: AlertCircle,   cls: 'text-destructive',   label: 'Task failed' },
  'permission.requested': { icon: ShieldAlert,   cls: 'text-amber-500',     label: 'Needs approval' },
  'permission.replied':   { icon: CheckCircle2,  cls: 'text-emerald-400',   label: 'Approved' },
  'github.commit_pending':{ icon: GitCommit,     cls: 'text-primary',       label: 'Commit pending' },
  'github.pr_ready':      { icon: GitCommit,     cls: 'text-emerald-400',   label: 'PR ready' },
  'session_started':      { icon: Bot,           cls: 'text-primary/70',    label: 'Session started' },
  'heartbeat':            { icon: Activity,      cls: 'text-muted-foreground/40', label: 'Heartbeat' },
};

function kindOf(type: string) {
  return KIND_META[type] ?? { icon: RotateCcw, cls: 'text-muted-foreground/50', label: type };
}

function EventRow({ event }: { event: LiveEvent }) {
  const { icon: Icon, cls, label } = kindOf(event.type);
  const souls = useStore((s) => s.souls);
  const soul = event.agent_id ? souls.find((s) => s.id === event.agent_id) : null;
  const name = soul?.display_name ?? event.soul_key ?? event.agent_id?.slice(0, 8);

  if ((event.type as string) === 'heartbeat') return null;

  return (
    <div className="flex items-start gap-2 px-3 py-1.5 hover:bg-accent/30 transition-colors">
      <Icon className={cn('h-3.5 w-3.5 shrink-0 mt-0.5', cls)} />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-1.5 flex-wrap">
          <span className={cn('text-2xs font-medium', cls)}>{label}</span>
          {name && <span className="text-2xs text-muted-foreground truncate max-w-[120px]">{name}</span>}
          {event.detail && (
            <span className="text-2xs text-foreground/70 truncate flex-1">{event.detail}</span>
          )}
        </div>
      </div>
      <span className="text-2xs text-muted-foreground/50 shrink-0 tabular-nums">{relTime(event.timestamp)}</span>
    </div>
  );
}

export function ActivityFeed() {
  const liveEvents = useStore((s) => s.liveEvents);

  const visible = useMemo(
    () => liveEvents.filter((e) => (e.type as string) !== 'heartbeat').slice(0, 100),
    [liveEvents],
  );

  if (visible.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 py-10 text-center">
        <Activity className="h-6 w-6 text-muted-foreground/30" />
        <p className="text-xs text-muted-foreground">No activity yet — events appear here in real time.</p>
      </div>
    );
  }

  return (
    <div className="divide-y divide-border/40">
      {visible.map((e) => <EventRow key={e.id} event={e} />)}
    </div>
  );
}
