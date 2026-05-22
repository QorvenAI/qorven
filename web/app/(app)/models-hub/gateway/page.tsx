'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import {
  RefreshCw, Trash2, Check, Loader2, AlertCircle, Plus, X,
  Activity, Cpu, Database, DollarSign, Zap, Server,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { Button } from '@/components/qor/button';
import { Badge } from '@/components/qor/badge';
import { gatewayAdmin } from '@/lib/api-providers';
import { toast } from 'sonner';

// ─── Shared card ─────────────────────────────────────────────────────────────

function Card({ title, icon: Icon, children, headerRight }: {
  title: string; icon: React.ElementType; children: React.ReactNode; headerRight?: React.ReactNode;
}) {
  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <div className="flex items-center justify-between px-5 py-3.5 border-b border-border/70 bg-muted/20">
        <div className="flex items-center gap-2">
          <Icon className="h-4 w-4 text-muted-foreground" />
          <h3 className="text-sm font-semibold">{title}</h3>
        </div>
        {headerRight && <div className="flex items-center gap-2">{headerRight}</div>}
      </div>
      <div className="px-5 py-4 space-y-3">{children}</div>
    </div>
  );
}

// ─── Pipeline status ──────────────────────────────────────────────────────────

function PipelineStatus({ active, uptime }: { active: boolean; uptime: number }) {
  const h = Math.floor(uptime / 3600);
  const m = Math.floor((uptime % 3600) / 60);
  const uptimeStr = h > 0 ? `${h}h ${m}m` : `${m}m`;
  return (
    <div className="flex items-center gap-3 rounded-lg border border-border px-4 py-3">
      <span className={cn('h-2.5 w-2.5 rounded-full shrink-0', active ? 'bg-emerald-400' : 'bg-muted-foreground/40')} />
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium">{active ? 'Pipeline active' : 'Pipeline inactive'}</p>
        <p className="text-xs text-muted-foreground">Uptime: {uptimeStr}</p>
      </div>
    </div>
  );
}

// ─── Priority queue depths ────────────────────────────────────────────────────

function QueueSection({ data }: { data: { interactive: number; background: number; batch: number; capacities: { interactive: number; background: number; batch: number } } | null }) {
  const tiers = [
    { key: 'interactive' as const, label: 'Interactive', desc: 'User-facing requests' },
    { key: 'background'  as const, label: 'Background',  desc: 'Agent tasks' },
    { key: 'batch'       as const, label: 'Batch',       desc: 'Bulk processing' },
  ];
  return (
    <div className="grid grid-cols-3 gap-2">
      {tiers.map(t => {
        const inFlight = data ? data[t.key] : 0;
        const cap = data?.capacities?.[t.key] ?? 0;
        const pct = cap > 0 ? Math.round((inFlight / cap) * 100) : 0;
        return (
          <div key={t.key} className="rounded-lg border border-border px-3.5 py-3">
            <p className="text-xs font-medium text-muted-foreground">{t.label}</p>
            <p className="text-2xl font-semibold mt-0.5">{inFlight}</p>
            <p className="text-xs text-muted-foreground mt-0.5">{pct}% of {cap}</p>
          </div>
        );
      })}
    </div>
  );
}

// ─── Circuit breakers ─────────────────────────────────────────────────────────

type Breaker = { key_id: string; state: string; requests: number; failures: number };

function CircuitBreakersSection({ breakers }: { breakers: Breaker[] }) {
  if (breakers.length === 0) {
    return <p className="text-xs text-muted-foreground py-2">No circuit breakers tracked yet.</p>;
  }
  return (
    <div className="rounded-lg border border-border overflow-hidden">
      <div className="grid grid-cols-4 gap-3 px-4 py-2 border-b border-border/50 bg-muted/20 text-xs font-medium text-muted-foreground">
        <span>Key ID</span><span>State</span><span className="text-right">Requests</span><span className="text-right">Failures</span>
      </div>
      {breakers.map(b => (
        <div key={b.key_id} className="grid grid-cols-4 gap-3 px-4 py-2.5 border-b border-border/30 last:border-0 items-center">
          <span className="text-xs font-mono truncate">{b.key_id.slice(-8)}</span>
          <Badge
            variant={b.state === 'closed' ? 'success' : b.state === 'open' ? 'destructive' : 'warning'}
            appearance="light" size="sm"
          >
            {b.state}
          </Badge>
          <span className="text-xs text-right">{b.requests.toLocaleString()}</span>
          <span className={cn('text-xs text-right', b.failures > 0 ? 'text-destructive' : 'text-muted-foreground')}>
            {b.failures}
          </span>
        </div>
      ))}
    </div>
  );
}

// ─── Model aliases ────────────────────────────────────────────────────────────

type AliasRow = { alias: string; model_id: string; priority: number };

const DEFAULT_ALIASES = ['fast', 'smart', 'cheap', 'vision', 'code', 'reason'];

function AliasesSection({ aliases, onSave, onDelete }: {
  aliases: AliasRow[];
  onSave: (alias: string, modelId: string) => Promise<void>;
  onDelete: (alias: string) => Promise<void>;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const [modelInput, setModelInput] = useState('');
  const [saving, setSaving] = useState<string | null>(null);

  const allAliases = DEFAULT_ALIASES.map(a => ({
    alias: a,
    model_id: aliases.find(r => r.alias === a)?.model_id ?? '',
  }));

  const save = async (alias: string) => {
    if (!modelInput.trim()) return;
    setSaving(alias);
    await onSave(alias, modelInput.trim());
    setEditing(null);
    setModelInput('');
    setSaving(null);
  };

  return (
    <div className="rounded-lg border border-border overflow-hidden">
      <div className="grid grid-cols-3 gap-3 px-4 py-2 border-b border-border/50 bg-muted/20 text-xs font-medium text-muted-foreground">
        <span>Alias</span><span>Resolved Model</span><span />
      </div>
      {allAliases.map(({ alias, model_id }) => (
        <div key={alias} className="grid grid-cols-3 gap-3 px-4 py-2.5 border-b border-border/30 last:border-0 items-center">
          <span className="text-xs font-mono text-primary">{alias}</span>
          {editing === alias ? (
            <input
              value={modelInput}
              onChange={e => setModelInput(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') save(alias); if (e.key === 'Escape') setEditing(null); }}
              placeholder="claude-sonnet-4-6"
              autoFocus
              className="qr-input text-xs py-1"
            />
          ) : (
            <span className="text-xs font-mono text-muted-foreground truncate">
              {model_id || <span className="italic opacity-50">auto</span>}
            </span>
          )}
          <div className="flex items-center gap-1 justify-end">
            {editing === alias ? (
              <>
                <Button variant="primary" size="sm" onClick={() => save(alias)} disabled={!!saving}>
                  {saving === alias ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />}
                </Button>
                <Button variant="ghost" mode="icon" size="sm" onClick={() => setEditing(null)}><X className="h-3 w-3" /></Button>
              </>
            ) : (
              <>
                <Button variant="ghost" size="sm" onClick={() => { setEditing(alias); setModelInput(model_id); }}>
                  Edit
                </Button>
                {model_id && (
                  <Button variant="ghost" mode="icon" size="sm" onClick={() => onDelete(alias)} className="hover:text-destructive hover:bg-destructive/10">
                    <X className="h-3 w-3" />
                  </Button>
                )}
              </>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}

// ─── Budgets ──────────────────────────────────────────────────────────────────

type BudgetRow = { id: string; agent_id?: string; monthly_usd?: number; daily_usd?: number; spent_month_usd: number; spent_today_usd: number };

function BudgetsSection({ budgets }: { budgets: BudgetRow[] }) {
  if (budgets.length === 0) {
    return <p className="text-xs text-muted-foreground py-2">No budgets configured. Use the API to set per-agent limits.</p>;
  }
  return (
    <div className="space-y-2">
      {budgets.map(b => {
        const pct = b.monthly_usd ? Math.min(100, Math.round((b.spent_month_usd / b.monthly_usd) * 100)) : null;
        return (
          <div key={b.id} className="rounded-lg border border-border px-4 py-3">
            <div className="flex items-center justify-between mb-2">
              <p className="text-xs font-mono text-muted-foreground truncate">{b.agent_id ?? 'team'}</p>
              <div className="flex items-center gap-3 text-xs text-muted-foreground shrink-0">
                <span>${b.spent_month_usd.toFixed(4)} / {b.monthly_usd != null ? `$${b.monthly_usd}` : '∞'} mo</span>
                <span>${b.spent_today_usd.toFixed(4)} today</span>
              </div>
            </div>
            {pct !== null && (
              <div className="h-1.5 rounded-full bg-muted overflow-hidden">
                <div
                  className={cn('h-full rounded-full transition-all', pct >= 90 ? 'bg-destructive' : pct >= 70 ? 'bg-amber-400' : 'bg-emerald-400')}
                  style={{ width: `${pct}%` }}
                />
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

// ─── Cache stats ──────────────────────────────────────────────────────────────

function CacheStatsSection({ stats, onFlush }: {
  stats: { size: number; capacity: number } | null;
  onFlush: () => void;
}) {
  const pct = stats && stats.capacity > 0 ? Math.round((stats.size / stats.capacity) * 100) : 0;
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-2">
        <div className="rounded-lg border border-border px-4 py-3">
          <p className="text-xs text-muted-foreground">Entries</p>
          <p className="text-2xl font-semibold mt-0.5">{stats?.size ?? 0}</p>
        </div>
        <div className="rounded-lg border border-border px-4 py-3">
          <p className="text-xs text-muted-foreground">Capacity</p>
          <p className="text-2xl font-semibold mt-0.5">{stats?.capacity ?? 0}</p>
        </div>
      </div>
      {stats && stats.capacity > 0 && (
        <div className="space-y-1">
          <div className="flex justify-between text-xs text-muted-foreground">
            <span>Usage</span><span>{pct}%</span>
          </div>
          <div className="h-1.5 rounded-full bg-muted overflow-hidden">
            <div className="h-full rounded-full bg-primary/60 transition-all" style={{ width: `${pct}%` }} />
          </div>
        </div>
      )}
      <Button variant="outline" size="sm" onClick={onFlush} className="w-full">
        <Trash2 className="h-3.5 w-3.5" /> Flush cache
      </Button>
    </div>
  );
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function GatewayPage() {
  const [loading,   setLoading]   = useState(true);
  const [stats,     setStats]     = useState<{ uptime_seconds: number; pipeline: boolean } | null>(null);
  const [circuit,   setCircuit]   = useState<Breaker[]>([]);
  const [queue,     setQueue]     = useState<{ interactive: number; background: number; batch: number; capacities: { interactive: number; background: number; batch: number } } | null>(null);
  const [aliases,   setAliases]   = useState<AliasRow[]>([]);
  const [budgets,   setBudgets]   = useState<BudgetRow[]>([]);
  const [cacheStats, setCacheStats] = useState<{ size: number; capacity: number } | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    const [s, c, q, a, b, cs] = await Promise.allSettled([
      gatewayAdmin.stats(),
      gatewayAdmin.circuit(),
      gatewayAdmin.queue(),
      gatewayAdmin.aliases(),
      gatewayAdmin.budgets(),
      gatewayAdmin.cacheStats(),
    ]);
    if (s.status === 'fulfilled' && s.value) setStats(s.value);
    if (c.status === 'fulfilled' && c.value) setCircuit(c.value.breakers ?? []);
    if (q.status === 'fulfilled' && q.value) setQueue(q.value);
    if (a.status === 'fulfilled' && Array.isArray(a.value)) setAliases(a.value);
    if (b.status === 'fulfilled' && Array.isArray(b.value)) setBudgets(b.value);
    if (cs.status === 'fulfilled' && cs.value) setCacheStats(cs.value);
    setLoading(false);
  }, []);

  useEffect(() => { load(); }, [load]);

  const saveAlias = async (alias: string, modelId: string) => {
    try {
      await gatewayAdmin.upsertAlias(alias, modelId);
      toast.success(`Alias "${alias}" → ${modelId}`);
      load();
    } catch { toast.error('Failed to save alias'); }
  };

  const deleteAlias = async (alias: string) => {
    try {
      await gatewayAdmin.deleteAlias(alias);
      toast.success(`Alias "${alias}" removed`);
      load();
    } catch { toast.error('Failed to remove alias'); }
  };

  const flushCache = async () => {
    try {
      await gatewayAdmin.cacheFlush();
      toast.success('Cache flushed');
      load();
    } catch { toast.error('Failed to flush cache'); }
  };

  return (
    <div className="space-y-4">
      <CanvasHeader
        title="AI Gateway"
        description="Pipeline status, circuit breakers, priority queues, model aliases, budgets and cache"
        actions={
          <Button variant="ghost" mode="icon" size="sm" onClick={load} title="Refresh">
            <RefreshCw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
          </Button>
        }
      />

      {/* Row 1: Pipeline + Queue */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Card title="Pipeline" icon={Activity}>
          {stats ? (
            <PipelineStatus active={stats.pipeline} uptime={stats.uptime_seconds} />
          ) : (
            <div className="h-14 rounded-lg bg-muted animate-pulse" />
          )}
        </Card>

        <Card title="Priority Queue" icon={Cpu}>
          <QueueSection data={queue} />
        </Card>
      </div>

      {/* Row 2: Circuit Breakers */}
      <Card title="Circuit Breakers" icon={Zap}>
        {loading ? (
          <div className="space-y-1.5">{[0,1].map(i => <div key={i} className="h-9 rounded-lg bg-muted animate-pulse" />)}</div>
        ) : (
          <CircuitBreakersSection breakers={circuit} />
        )}
      </Card>

      {/* Row 3: Aliases + Cache */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Card title="Model Aliases" icon={Server}>
          {loading ? (
            <div className="space-y-1.5">{[0,1,2].map(i => <div key={i} className="h-9 rounded-lg bg-muted animate-pulse" />)}</div>
          ) : (
            <AliasesSection aliases={aliases} onSave={saveAlias} onDelete={deleteAlias} />
          )}
        </Card>

        <Card title="LLM Cache" icon={Database}>
          {loading ? (
            <div className="space-y-2">{[0,1].map(i => <div key={i} className="h-16 rounded-lg bg-muted animate-pulse" />)}</div>
          ) : (
            <CacheStatsSection stats={cacheStats} onFlush={flushCache} />
          )}
        </Card>
      </div>

      {/* Row 4: Budgets */}
      <Card title="Agent Budgets" icon={DollarSign}>
        {loading ? (
          <div className="space-y-2">{[0,1].map(i => <div key={i} className="h-16 rounded-lg bg-muted animate-pulse" />)}</div>
        ) : (
          <BudgetsSection budgets={budgets} />
        )}
      </Card>
    </div>
  );
}
