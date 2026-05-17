'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { cn } from '@/lib/utils';
import { useStore } from '@/store';
import Link from 'next/link';
import type { DaemonAgent } from '@/hooks/use-agents-stream';
import type { Soul, SoulActivity } from '@/types';
import { modelDisplayName } from '@/lib/model-names';
import { Cpu, Zap, AlertCircle, WifiOff, Bot } from 'lucide-react';

// ─── Shared helpers ────────────────────────────────────────────────────────────

const PROVIDER_LABELS: Record<string, string> = {
  kiro_cli:      'Kiro',
  claude_code:   'Claude',
  qorven_native: 'Qorven',
  custom:        'Custom',
};

const CAP_COLORS: Record<string, string> = {
  code:     'bg-violet-500/10 text-violet-400',
  frontend: 'bg-sky-500/10 text-sky-400',
  backend:  'bg-emerald-500/10 text-emerald-400',
  review:   'bg-amber-500/10 text-amber-400',
  plan:     'bg-orange-500/10 text-orange-400',
  test:     'bg-pink-500/10 text-pink-400',
  research: 'bg-indigo-500/10 text-indigo-400',
};

const SOUL_ACTIVITY_DOT: Record<SoulActivity, string> = {
  idle:     'bg-emerald-400',
  thinking: 'bg-sky-400 animate-pulse',
  running:  'bg-amber-400 animate-pulse',
  offline:  'bg-muted-foreground/40',
  error:    'bg-red-400',
};

const SOUL_ACTIVITY_BADGE: Record<SoulActivity, string> = {
  idle:     'bg-emerald-500/10 text-emerald-400',
  thinking: 'bg-sky-500/10 text-sky-400',
  running:  'bg-amber-500/10 text-amber-400',
  offline:  'bg-muted text-muted-foreground',
  error:    'bg-red-500/10 text-red-400',
};

// ─── Soul card ─────────────────────────────────────────────────────────────────

function SoulCard({ soul }: { soul: Soul }) {
  const state = useStore((s) => s.soulStates[soul.id]);
  const activity: SoulActivity = state?.activity ?? (soul.status === 'active' ? 'idle' : 'offline');
  const lastEvent = state?.lastEvent;

  return (
    <Link href={`/qors/${soul.id}`}
      className="group rounded-xl border border-border bg-card p-4 space-y-2.5 hover:border-primary/30 transition-colors block">
      <div className="flex items-start justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <span className={cn('mt-0.5 h-2 w-2 rounded-full shrink-0', SOUL_ACTIVITY_DOT[activity])} />
          <div className="min-w-0">
            <p className="text-sm font-medium truncate group-hover:text-primary transition-colors">
              {soul.display_name}
            </p>
            <p className="text-2xs text-muted-foreground truncate">
              {soul.role || soul.title || 'Soul'} · {modelDisplayName(soul.model) || soul.model}
            </p>
          </div>
        </div>
        <span className={cn('shrink-0 text-2xs font-medium px-1.5 py-0.5 rounded-full', SOUL_ACTIVITY_BADGE[activity])}>
          {activity}
        </span>
      </div>

      {lastEvent && activity !== 'idle' && activity !== 'offline' && (
        <p className="text-2xs text-muted-foreground truncate flex items-center gap-1">
          <Zap className="h-3 w-3 text-amber-400 shrink-0" />
          {lastEvent}
        </p>
      )}

      {soul.tool_profile && (
        <span className="inline-block rounded-sm bg-muted px-1.5 py-0.5 text-2xs font-mono text-muted-foreground">
          {soul.tool_profile}
        </span>
      )}
    </Link>
  );
}

// ─── Daemon agent card ─────────────────────────────────────────────────────────

function DaemonCard({ agent, task }: { agent: DaemonAgent; task?: string }) {
  const providerLabel = PROVIDER_LABELS[agent.provider] ?? agent.provider;

  return (
    <div className="rounded-xl border border-border bg-card p-4 space-y-2.5">
      <div className="flex items-start justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <span className={cn('mt-0.5 h-2 w-2 rounded-full shrink-0',
            agent.status === 'idle'    ? 'bg-emerald-400' :
            agent.status === 'working' ? 'bg-amber-400 animate-pulse' :
                                         'bg-red-400')} />
          <div className="min-w-0">
            <p className="text-sm font-medium truncate">{agent.name}</p>
            <p className="text-2xs text-muted-foreground">
              {providerLabel} · {modelDisplayName(agent.model) || 'default'}
            </p>
          </div>
        </div>
        <span className={cn('shrink-0 text-2xs font-medium px-1.5 py-0.5 rounded-full',
          agent.status === 'idle'    ? 'bg-emerald-500/10 text-emerald-400' :
          agent.status === 'working' ? 'bg-amber-500/10 text-amber-400' :
                                       'bg-red-500/10 text-red-400')}>
          {agent.status}
        </span>
      </div>

      {agent.status === 'working' && task && (
        <p className="text-2xs text-muted-foreground truncate flex items-center gap-1">
          <Zap className="h-3 w-3 text-amber-400 shrink-0" />
          {task}
        </p>
      )}

      {agent.capabilities.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {agent.capabilities.map((cap) => (
            <span key={cap} className={cn('text-2xs px-1.5 py-0.5 rounded-full font-medium',
              CAP_COLORS[cap] ?? 'bg-muted text-muted-foreground')}>
              {cap}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

// ─── Dashboard ─────────────────────────────────────────────────────────────────

export function AgentDashboard() {
  const souls         = useStore((s) => s.souls);
  const soulStates    = useStore((s) => s.soulStates);
  const daemonAgents  = useStore((s) => Object.values(s.daemonAgents));
  const daemonTasks   = useStore((s) => s.daemonTasks);
  const connected     = useStore((s) => s.daemonConnected);

  const activeSouls = souls.filter((s) => s.status === 'active');

  const daemonWorking = daemonAgents.filter((a) => a.status === 'working');
  const daemonIdle    = daemonAgents.filter((a) => a.status === 'idle');
  const daemonErrored = daemonAgents.filter((a) => a.status === 'error');

  const soulActive = activeSouls.filter((s) => {
    const act = soulStates[s.id]?.activity ?? 'idle';
    return act !== 'offline';
  });
  const activeSoulCount = soulActive.filter((s) => {
    const act = soulStates[s.id]?.activity ?? 'idle';
    return act === 'running' || act === 'thinking';
  }).length;

  const totalActive = daemonWorking.length + activeSoulCount;
  const totalError  = daemonErrored.length + soulActive.filter((s) => soulStates[s.id]?.activity === 'error').length;

  return (
    <div className="space-y-5">
      {/* Stats bar */}
      <div className="flex items-center gap-4 text-xs text-muted-foreground">
        <span className="flex items-center gap-1.5">
          {connected
            ? <span className="inline-block h-1.5 w-1.5 rounded-full bg-emerald-400" />
            : <WifiOff className="h-3 w-3 text-red-400" />}
          {connected ? 'Live' : 'Reconnecting…'}
        </span>
        <span className="flex items-center gap-1"><Bot className="h-3.5 w-3.5" />{activeSouls.length} soul{activeSouls.length !== 1 ? 's' : ''}</span>
        <span className="flex items-center gap-1"><Cpu className="h-3.5 w-3.5" />{daemonAgents.length} daemon{daemonAgents.length !== 1 ? 's' : ''}</span>
        {totalActive > 0   && <span className="text-amber-400">{totalActive} working</span>}
        {totalError > 0    && <span className="text-red-400 flex items-center gap-1"><AlertCircle className="h-3 w-3" />{totalError} error</span>}
      </div>

      {/* Soul agents */}
      {activeSouls.length > 0 && (
        <section className="space-y-2">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
            <Bot className="h-3.5 w-3.5" /> Soul Agents ({activeSouls.length})
          </h3>
          <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-3">
            {activeSouls.map((soul) => <SoulCard key={soul.id} soul={soul} />)}
          </div>
        </section>
      )}

      {/* Daemon agents */}
      {daemonAgents.length > 0 && (
        <section className="space-y-2">
          <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-1.5">
            <Cpu className="h-3.5 w-3.5" /> Daemon Agents ({daemonAgents.length})
          </h3>
          <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-3">
            {[...daemonWorking, ...daemonIdle, ...daemonErrored].map((agent) => {
              const currentTask = agent.current_task_id ? daemonTasks[agent.current_task_id]?.title : undefined;
              return <DaemonCard key={agent.id} agent={agent} task={currentTask} />;
            })}
          </div>
        </section>
      )}

      {/* Empty state */}
      {activeSouls.length === 0 && daemonAgents.length === 0 && (
        <div className="flex flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-border py-12 text-center">
          <div className="flex h-10 w-10 items-center justify-center rounded-full bg-muted">
            <Cpu className="h-5 w-5 text-muted-foreground" />
          </div>
          <div>
            <p className="text-sm font-medium">No agents</p>
            <p className="text-xs text-muted-foreground mt-0.5">
              Create soul agents in{' '}
              <Link href="/qors" className="text-primary hover:underline">/qors</Link>
              {' '}or connect daemons via{' '}
              <code className="font-mono bg-muted px-1 rounded">POST /v1/daemon/agents/register</code>
            </p>
          </div>
        </div>
      )}
    </div>
  );
}
