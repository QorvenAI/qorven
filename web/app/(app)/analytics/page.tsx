'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { agents } from '@/lib/api';
import { traces as tracesApi, type TraceRow, type TraceSummary } from '@/lib/api-providers';
import { useStore } from '@/store';
import { ErrorBoundary } from '@/components/error-boundary';
import { cn } from '@/lib/utils';
import {
  BarChart3, Cpu, DollarSign, Activity, AlertTriangle,
  Clock, Zap, TrendingUp, RefreshCw,
} from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';

export default function AnalyticsPage() {
  const souls = useStore((s) => s.souls);
  const setSouls = useStore((s) => s.setSouls);
  const [recentTraces, setRecentTraces] = useState<TraceRow[]>([]);
  const [summary, setSummary] = useState<TraceSummary[]>([]);
  const [loading, setLoading] = useState(true);

  const load = () => {
    setLoading(true);
    Promise.all([
      souls.length === 0 ? agents.list().then(setSouls).catch(() => []) : Promise.resolve(),
      tracesApi.list({ limit: 30 }).catch(() => [] as TraceRow[]),
      tracesApi.summary().catch(() => [] as TraceSummary[]),
    ]).then(([, t, s]) => {
      setRecentTraces(t as TraceRow[]);
      setSummary(s as TraceSummary[]);
      setLoading(false);
    });
  };

  useEffect(load, []);

  // Budget overview from soul data (still the source of truth for budget caps)
  const totalBudget = souls.reduce((a, s) => a + s.credit_budget_cents, 0) / 100;
  const totalUsed = souls.reduce((a, s) => a + s.credit_used_cents, 0) / 100;
  const totalPct = totalBudget > 0 ? (totalUsed / totalBudget) * 100 : 0;

  // Aggregate from traces for this month
  const totalInputTok  = summary.reduce((a, s) => a + s.input_tokens,  0);
  const totalOutputTok = summary.reduce((a, s) => a + s.output_tokens, 0);
  const totalCostCents = summary.reduce((a, s) => a + s.cost_cents,    0);
  const totalTracesCount = summary.reduce((a, s) => a + s.traces, 0);

  // Build agent name lookup
  const soulById = Object.fromEntries(souls.map((s) => [s.id, s.display_name]));

  return (
    <ErrorBoundary fallbackTitle="Failed to load analytics">
      <div className="space-y-6">
        <CanvasHeader
          title="Analytics & Usage"
          description="Token usage, traces, and per-agent spend — current month"
          actions={
            <button onClick={load} disabled={loading}
              className="flex h-9 items-center gap-2 rounded-lg border border-border bg-input px-3 text-sm text-muted-foreground hover:bg-accent disabled:opacity-50">
              <RefreshCw className={cn('h-4 w-4', loading && 'animate-spin')} />
              Refresh
            </button>
          }
        />

        {/* ── Top stat cards ── */}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard icon={DollarSign}  label="Cost (budget)" value={`$${totalUsed.toFixed(2)}`}  sub={`of $${totalBudget.toFixed(2)} budget`} alert={totalPct > 80} loading={loading} />
          <StatCard icon={TrendingUp}  label="Cost (traces)"  value={`$${(totalCostCents / 100).toFixed(4)}`} sub="this month" loading={loading} />
          <StatCard icon={Cpu}         label="Tokens in"      value={fmtNum(totalInputTok)}  sub="input this month"  loading={loading} />
          <StatCard icon={Activity}    label="Tokens out"     value={fmtNum(totalOutputTok)} sub="output this month" loading={loading} />
        </div>

        {totalPct > 80 && (
          <div className="flex items-center gap-2 rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
            <AlertTriangle className="h-4 w-4 shrink-0" />
            Workspace is at {totalPct.toFixed(0)}% of total budget
          </div>
        )}

        {/* ── Per-agent summary (from traces) ── */}
        {summary.length > 0 && (
          <section>
            <h2 className="mb-3 text-sm font-semibold text-foreground">Per-Agent Usage (this month)</h2>
            <div className="space-y-2">
              {summary.map((s) => {
                const name = soulById[s.agent_id] || s.agent_id?.slice(0, 8) || 'Unknown';
                const pct = totalCostCents > 0 ? (s.cost_cents / totalCostCents) * 100 : 0;
                return (
                  <div key={s.agent_id} className="rounded-xl border border-border bg-card px-4 py-3">
                    <div className="flex items-center justify-between mb-1.5">
                      <span className="text-sm font-medium">{name}</span>
                      <div className="flex items-center gap-4 text-xs text-muted-foreground">
                        <span className="flex items-center gap-1"><Zap className="h-3 w-3" />{fmtNum(s.input_tokens + s.output_tokens)} tok</span>
                        <span className="flex items-center gap-1"><BarChart3 className="h-3 w-3" />{s.traces} traces</span>
                        <span className="font-medium text-foreground">${(s.cost_cents / 100).toFixed(4)}</span>
                      </div>
                    </div>
                    <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                      <div className="h-full rounded-full bg-primary transition-all" style={{ width: `${pct}%` }} />
                    </div>
                  </div>
                );
              })}
            </div>
          </section>
        )}

        {/* ── Per-soul credit spend (from agents table) ── */}
        {souls.length > 0 && (
          <section>
            <h2 className="mb-3 text-sm font-semibold text-foreground">Credit Budget per Agent</h2>
            <div className="space-y-2">
              {souls.map((soul) => {
                const budget = soul.credit_budget_cents / 100;
                const used   = soul.credit_used_cents   / 100;
                const pct    = budget > 0 ? Math.min((used / budget) * 100, 100) : 0;
                return (
                  <div key={soul.id} className="rounded-xl border border-border bg-card px-4 py-3">
                    <div className="flex items-center justify-between mb-1">
                      <span className="text-sm font-medium">{soul.display_name}</span>
                      <span className="text-xs text-muted-foreground">${used.toFixed(2)} / ${budget.toFixed(2)}</span>
                    </div>
                    <div className="h-2 overflow-hidden rounded-full bg-muted">
                      <div className={cn('h-full rounded-full', pct > 80 ? 'bg-destructive' : 'bg-primary')} style={{ width: `${pct}%` }} />
                    </div>
                  </div>
                );
              })}
            </div>
          </section>
        )}

        {/* ── Recent traces ── */}
        <section>
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-semibold text-foreground">Recent Traces</h2>
            {totalTracesCount > 0 && (
              <span className="text-xs text-muted-foreground">{totalTracesCount.toLocaleString()} this month</span>
            )}
          </div>

          {loading ? (
            <div className="space-y-2">
              {Array.from({ length: 5 }).map((_, i) => (
                <div key={i} className="h-12 rounded-xl animate-pulse bg-muted" />
              ))}
            </div>
          ) : recentTraces.length === 0 ? (
            <div className="rounded-xl border border-dashed border-border bg-card/40 px-4 py-10 flex flex-col items-center text-center">
              <Clock className="h-6 w-6 text-muted-foreground/60 mb-2" />
              <p className="text-sm text-muted-foreground">No traces yet — traces are recorded as agents process requests.</p>
            </div>
          ) : (
            <div className="rounded-xl border border-border overflow-hidden">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-border bg-muted/40">
                    <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">Agent</th>
                    <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">Status</th>
                    <th className="px-4 py-2.5 text-right font-medium text-muted-foreground">Tokens in</th>
                    <th className="px-4 py-2.5 text-right font-medium text-muted-foreground">Tokens out</th>
                    <th className="px-4 py-2.5 text-right font-medium text-muted-foreground">Duration</th>
                    <th className="px-4 py-2.5 text-right font-medium text-muted-foreground">Time</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {recentTraces.map((t) => (
                    <tr key={t.id} className="hover:bg-accent/40 transition-colors">
                      <td className="px-4 py-2.5 font-medium text-foreground">
                        {t.agent_id ? (soulById[t.agent_id] || t.agent_id.slice(0, 8)) : '—'}
                      </td>
                      <td className="px-4 py-2.5">
                        <span className={cn('inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-2xs font-medium',
                          t.status === 'completed' ? 'bg-emerald-500/10 text-emerald-600' :
                          t.status === 'running'   ? 'bg-blue-500/10 text-blue-600' :
                          t.status === 'error'     ? 'bg-destructive/10 text-destructive' :
                          'bg-muted text-muted-foreground')}>
                          {t.status}
                        </span>
                      </td>
                      <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{t.input_tokens.toLocaleString()}</td>
                      <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">{t.output_tokens.toLocaleString()}</td>
                      <td className="px-4 py-2.5 text-right tabular-nums text-muted-foreground">
                        {t.duration_ms != null ? `${t.duration_ms.toLocaleString()}ms` : '—'}
                      </td>
                      <td className="px-4 py-2.5 text-right text-muted-foreground">
                        {new Date(t.created_at).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </div>
    </ErrorBoundary>
  );
}

function fmtNum(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
  if (n >= 1_000)     return (n / 1_000).toFixed(1) + 'K';
  return String(n);
}

function StatCard({ icon: Icon, label, value, sub, alert, loading }: {
  icon: typeof DollarSign; label: string; value: string; sub?: string; alert?: boolean; loading?: boolean;
}) {
  return (
    <div className={cn('rounded-xl border bg-card p-5', alert ? 'border-destructive/30' : 'border-border')}>
      <div className="flex items-center gap-2">
        <Icon className={cn('h-4 w-4', alert ? 'text-destructive' : 'text-muted-foreground')} />
        <p className="text-xs text-muted-foreground">{label}</p>
      </div>
      {loading
        ? <div className="mt-2 h-8 w-20 animate-pulse rounded bg-muted" />
        : <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
      }
      {sub && <p className="text-2xs text-muted-foreground mt-0.5">{sub}</p>}
    </div>
  );
}
