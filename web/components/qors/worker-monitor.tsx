'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import {
  Activity, ListTodo, UserCircle, Plug, MessageSquare, Loader2, ChevronLeft,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { tasks as tasksApi, agents as agentsApi } from '@/lib/api';
import { ConnectorsPanel } from '@/components/connectors/connectors-panel';
import { ProfileSkillsTab } from '@/components/qor-profile-tabs';
import { TaskComments } from '@/components/qors/task-comments';
import type { Soul } from '@/types';

type WorkerTask = {
  id: string; title: string; description?: string; state: string;
  priority?: string; assigned_agent_id?: string | null; created_at: string;
};

type LiveTaskLite = { id: string; title: string; status: string; iteration: number; scratchpad: string };

const MONITOR_TABS = [
  { id: 'activity', label: 'Activity', icon: Activity },
  { id: 'tasks',    label: 'Tasks',    icon: ListTodo },
  { id: 'profile',  label: 'Profile',  icon: UserCircle },
  { id: 'services', label: 'Services', icon: Plug },
] as const;
type MonitorTab = (typeof MONITOR_TABS)[number]['id'];

export function WorkerMonitor({
  soul,
  liveTasks,
}: {
  soul: Soul;
  liveTasks: LiveTaskLite[];
}) {
  const [tab, setTab] = useState<MonitorTab>('activity');
  const [manager, setManager] = useState<Soul | null>(null);

  useEffect(() => {
    let cancelled = false;
    const mid = soul.manager_id;
    if (!mid) {
      Promise.resolve().then(() => { if (!cancelled) setManager(null); });
      return () => { cancelled = true; };
    }
    agentsApi.get(mid)
      .then((m) => { if (!cancelled) setManager(m); })
      .catch(() => { if (!cancelled) setManager(null); });
    return () => { cancelled = true; };
  }, [soul]);

  const department = soul.org_role ? soul.org_role.toUpperCase() : 'Worker';

  return (
    <div className="full-bleed flex h-full flex-col" style={{ height: 'calc(100vh - var(--header-height) - var(--toolbar-height) - var(--status-bar-height, 24px))' }}>
      <div className="shrink-0 border-b border-border px-5 py-3 flex items-center gap-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-full bg-primary/10 text-primary text-sm font-bold">
          {(soul.display_name || 'W')[0]?.toUpperCase()}
        </div>
        <div className="min-w-0">
          <h1 className="text-sm font-semibold leading-tight truncate">{soul.display_name}</h1>
          <p className="text-xs text-muted-foreground">
            {department} · Worker (L3)
            {manager && <> · reports to {manager.display_name}</>}
          </p>
        </div>
        <div className="flex-1" />
        {manager ? (
          <Link
            href={`/qors/${manager.id}?tab=chat`}
            className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            <MessageSquare className="h-3.5 w-3.5" />
            Message {manager.display_name}
          </Link>
        ) : (
          <span className="text-xs text-muted-foreground/60">No manager assigned</span>
        )}
      </div>

      <nav className="shrink-0 flex items-stretch border-b border-border px-3">
        {MONITOR_TABS.map(({ id, label, icon: Icon }) => (
          <button key={id} onClick={() => setTab(id)}
            className={cn('flex items-center gap-1.5 px-3 py-2 text-xs font-medium border-b-2 transition-colors',
              tab === id ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground')}>
            <Icon className="h-3.5 w-3.5" />
            {label}
          </button>
        ))}
      </nav>

      <div className="flex-1 overflow-hidden">
        {tab === 'activity' && <ActivityView soul={soul} liveTasks={liveTasks} />}
        {tab === 'tasks' && <TasksView agentId={soul.id} />}
        {tab === 'profile' && (
          <div className="h-full overflow-y-auto p-5 space-y-5">
            <ProfileBlock soul={soul} />
            <ProfileSkillsTab agentId={soul.id} />
          </div>
        )}
        {tab === 'services' && (
          <div className="h-full overflow-y-auto p-5">
            <p className="text-xs text-muted-foreground mb-3">
              External services this worker can use for its jobs (GitHub, Zoho, scrapers, etc.).
              Comms channels (Telegram/WhatsApp/email) are reserved for C-officers.
            </p>
            <ConnectorsPanel agentId={soul.id} />
          </div>
        )}
      </div>
    </div>
  );
}

function ActivityView({ soul, liveTasks }: { soul: Soul; liveTasks: LiveTaskLite[] }) {
  if (liveTasks.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
        <Activity className="h-8 w-8 text-muted-foreground/20" />
        <p className="text-sm text-muted-foreground/70">{soul.display_name} is idle right now.</p>
        <p className="text-xs text-muted-foreground/50">Live activity appears here when it picks up a task.</p>
      </div>
    );
  }
  return (
    <div className="h-full overflow-y-auto p-4 space-y-2">
      {liveTasks.map((t) => {
        const lastLine = t.scratchpad?.split('\n').filter(Boolean).pop() ?? '';
        const cls = t.status === 'done' ? 'border-emerald-500/30 bg-emerald-500/5'
          : t.status === 'blocked' ? 'border-amber-500/30 bg-amber-500/5'
          : 'border-blue-500/30 bg-blue-500/5';
        const icon = t.status === 'done' ? '✓' : t.status === 'blocked' ? '⚠' : '⚡';
        return (
          <div key={t.id} className={cn('rounded-xl border px-4 py-3 text-sm', cls)}>
            <div className="flex items-center justify-between gap-2">
              <span className="font-medium">{icon} {t.title}</span>
              {t.status === 'in_progress' && (
                <span className="text-xs text-muted-foreground tabular-nums">Iteration {t.iteration}</span>
              )}
            </div>
            {lastLine && <p className="mt-1 text-xs text-muted-foreground/80 truncate">▸ {lastLine}</p>}
          </div>
        );
      })}
    </div>
  );
}

function TasksView({ agentId }: { agentId: string }) {
  const [items, setItems] = useState<WorkerTask[] | null>(null);
  const [openId, setOpenId] = useState<string | null>(null);

  useEffect(() => {
    // tasks.list(agentId?) hits /tasks?agent_id=<id> and returns an array (listRequest).
    tasksApi.list(agentId)
      .then((t: WorkerTask[]) => setItems(Array.isArray(t) ? t : []))
      .catch(() => setItems([]));
  }, [agentId]);

  const open = useMemo(() => items?.find((t) => t.id === openId) ?? null, [items, openId]);

  if (items === null) {
    return <div className="flex h-full items-center justify-center"><Loader2 className="h-5 w-5 animate-spin text-muted-foreground" /></div>;
  }

  if (open) {
    return (
      <div className="flex h-full flex-col">
        <div className="shrink-0 flex items-center gap-2 border-b border-border px-4 py-2.5">
          <button onClick={() => setOpenId(null)} className="rounded p-1 hover:bg-accent transition-colors">
            <ChevronLeft className="h-4 w-4" />
          </button>
          <h2 className="flex-1 text-sm font-semibold truncate">{open.title}</h2>
          <span className="rounded-full border border-border px-2 py-0.5 text-xs capitalize">{open.state}</span>
        </div>
        {open.description && (
          <div className="shrink-0 border-b border-border px-4 py-3">
            <p className="text-xs text-muted-foreground whitespace-pre-wrap leading-relaxed">{open.description}</p>
          </div>
        )}
        <div className="flex-1 min-h-0">
          <TaskComments taskId={open.id} />
        </div>
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
        <ListTodo className="h-8 w-8 text-muted-foreground/20" />
        <p className="text-sm text-muted-foreground/70">No tasks assigned yet.</p>
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto p-3 space-y-1.5">
      {items.map((t) => (
        <button key={t.id} onClick={() => setOpenId(t.id)}
          className="w-full text-left rounded-lg border border-border px-3 py-2.5 hover:bg-accent/50 transition-colors flex items-center gap-2">
          <span className="flex-1 truncate text-sm">{t.title}</span>
          <span className="shrink-0 rounded-full border border-border px-2 py-0.5 text-xs capitalize text-muted-foreground">{t.state}</span>
        </button>
      ))}
    </div>
  );
}

function ProfileBlock({ soul }: { soul: Soul }) {
  return (
    <div className="rounded-xl border border-border p-4 space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Model</span>
        <span className="text-xs font-mono">{soul.model || 'default'}</span>
      </div>
      <div>
        <span className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">System prompt</span>
        <pre className="mt-1.5 text-xs text-muted-foreground/80 leading-relaxed whitespace-pre-wrap max-h-64 overflow-y-auto rounded-lg bg-muted/30 border border-border p-2">
          {soul.system_prompt || '(none)'}
        </pre>
      </div>
    </div>
  );
}
