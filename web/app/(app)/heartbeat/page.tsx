'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState, useEffect, useCallback } from 'react';
import { Activity, RefreshCw, Crown } from 'lucide-react';
import { EmptyState, emptyStates } from '@/components/empty-state';
import SupervisorPage from '@/app/(app)/supervisor/page';
import { cn } from '@/lib/utils';

type Health = {
  status?: string;
  uptime?: string;
  version?: string;
  database?: { status?: string };
  active_agents?: number;
  memory_usage?: string;
  cpu_usage?: string;
  [k: string]: unknown;
};

type Tab = 'health' | 'supervisor';

const TABS: { id: Tab; icon: typeof Activity; label: string }[] = [
  { id: 'health',     icon: Activity, label: 'System Health' },
  { id: 'supervisor', icon: Crown,    label: 'Supervisor' },
];

export default function HealthPage() {
  const [tab, setTab] = useState<Tab>('health');
  const [health, setHealth] = useState<Health | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [lastChecked, setLastChecked] = useState<Date | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchHealth = useCallback((manual = false) => {
    if (manual) setRefreshing(true);
    fetch('/health/detailed')
      .then((r) => r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)))
      .then((d: Health) => {
        setHealth(d);
        setError('');
        setLastChecked(new Date());
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : 'Health check failed'))
      .finally(() => { setLoading(false); setRefreshing(false); });
  }, []);

  useEffect(() => {
    fetchHealth();
    const id = setInterval(() => fetchHealth(), 30000);
    return () => clearInterval(id);
  }, [fetchHealth]);

  const Dot = ({ ok }: { ok: boolean }) => (
    <span className={cn('inline-block w-2.5 h-2.5 rounded-full', ok ? 'bg-emerald-500' : 'bg-destructive')} />
  );

  const cards = health ? [
    { label: 'Uptime',        value: health.uptime ?? '—' },
    { label: 'Version',       value: health.version ?? '—' },
    { label: 'Database',      value: health.database?.status ?? '—', showDot: true, ok: health.database?.status === 'ok' },
    { label: 'Active Agents', value: health.active_agents ?? 0 },
  ] : [];

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between gap-4">
        <h1 className="text-lg font-semibold">Health</h1>
        {tab === 'health' && (
          <button
            onClick={() => fetchHealth(true)}
            disabled={refreshing}
            className="flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-1.5 text-sm text-muted-foreground hover:border-primary hover:text-foreground transition disabled:opacity-50"
          >
            <RefreshCw className={cn('h-3.5 w-3.5', refreshing && 'animate-spin')} />
            Refresh
          </button>
        )}
      </div>

      {/* Tab bar */}
      <div className="flex items-center gap-0 border-b border-border">
        {TABS.map((t) => {
          const Icon = t.icon;
          return (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={cn(
                '-mb-px flex items-center gap-1.5 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors',
                tab === t.id
                  ? 'border-primary text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground',
              )}
            >
              <Icon className="h-4 w-4" />
              {t.label}
            </button>
          );
        })}
      </div>

      {/* System Health tab */}
      {tab === 'health' && (
        <>
          {error && <p className="text-sm text-destructive">{error}</p>}

          {loading ? (
            <p className="text-sm text-muted-foreground">Checking system health…</p>
          ) : !health ? (
            <EmptyState {...emptyStates.heartbeat} />
          ) : (
            <>
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                {cards.map((c) => (
                  <div key={c.label} className="rounded-xl border border-border bg-card p-5">
                    <p className="text-sm text-muted-foreground mb-1">{c.label}</p>
                    <div className="flex items-center gap-2">
                      {c.showDot && <Dot ok={!!c.ok} />}
                      <p className="text-xl font-semibold">{String(c.value)}</p>
                    </div>
                  </div>
                ))}
              </div>

              <div className="rounded-xl border border-border bg-card p-5">
                <h2 className="text-xs font-medium text-muted-foreground mb-3 uppercase tracking-wider">
                  Overall Status
                </h2>
                <div className="flex items-center gap-3">
                  <Dot ok={health.status === 'ok'} />
                  <span className={cn('text-lg font-semibold', health.status === 'ok' ? 'text-emerald-400' : 'text-destructive')}>
                    {health.status === 'ok' ? 'All Systems Operational' : 'Issues Detected'}
                  </span>
                </div>
                {health.memory_usage && (
                  <p className="text-sm text-muted-foreground mt-2">Memory: {health.memory_usage}</p>
                )}
                {health.cpu_usage && (
                  <p className="text-sm text-muted-foreground mt-1">CPU: {health.cpu_usage}</p>
                )}
              </div>

              <p className="text-xs text-muted-foreground text-right">
                {lastChecked && <>Last checked: {lastChecked.toLocaleTimeString()} · </>}
                Auto-refreshes every 30s
              </p>
            </>
          )}
        </>
      )}

      {/* Supervisor tab */}
      {tab === 'supervisor' && <SupervisorPage />}
    </div>
  );
}
