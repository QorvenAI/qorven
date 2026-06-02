'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import { Loader2, RefreshCw, ChevronDown, ChevronRight } from 'lucide-react';
import { cn } from '@/lib/utils';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { providers as providersApi } from '@/lib/api';
import { Button } from '@/components/qor/button';
import { ProviderIcon } from '@/components/provider-icon';

// ─── Types ─────────────────────────────────────────────────────────────────────

interface KeySpend {
  key_id: string;
  label: string;
  key_hash: string;
  spent_usd_month: number;
  spent_tokens_month: number;
  budget_type: string;
  limit_usd?: number;
}

interface ProviderSpend {
  provider_id: string;
  spent_usd_month: number;
  spent_tokens_month: number;
  budget_usd?: number;
  keys: KeySpend[];
}

interface SpendSummary {
  month: string;
  providers: ProviderSpend[];
}

// ─── Progress bar ───────────────────────────────────────────────────────────────

function SpendBar({ spent, limit }: { spent: number; limit: number }) {
  const pct = limit > 0 ? Math.min((spent / limit) * 100, 100) : 0;
  return (
    <div className="h-1.5 w-full rounded-full bg-muted overflow-hidden">
      <div
        className={cn('h-full rounded-full transition-all', pct >= 95 ? 'bg-destructive' : pct >= 80 ? 'bg-amber-400' : 'bg-primary')}
        style={{ width: `${pct}%` }}
      />
    </div>
  );
}

// ─── Key spend status badge ─────────────────────────────────────────────────────

function KeyStatusBadge({ spent, limit, budgetType }: { spent: number; limit?: number; budgetType: string }) {
  if (!limit || budgetType === 'free') return <span className="text-xs text-muted-foreground">—</span>;
  const pct = (spent / limit) * 100;
  if (pct >= 100)  return <span className="text-xs text-destructive font-medium">Over limit</span>;
  if (pct >= 95)   return <span className="text-xs text-destructive">Near limit</span>;
  if (pct >= 80)   return <span className="text-xs text-amber-400">Near limit</span>;
  return <span className="text-xs text-emerald-400">On track</span>;
}

// ─── Format helpers ─────────────────────────────────────────────────────────────

function fmtUSD(v: number) {
  if (v === 0) return '$0.00';
  if (v < 0.01) return '<$0.01';
  return `$${v.toFixed(2)}`;
}

function fmtTokens(v: number) {
  if (v >= 1_000_000) return `${(v / 1_000_000).toFixed(1)}M`;
  if (v >= 1_000)     return `${(v / 1_000).toFixed(1)}k`;
  return v.toLocaleString();
}

// ─── Provider spend card ────────────────────────────────────────────────────────

function ProviderSpendCard({ p }: { p: ProviderSpend }) {
  const [open, setOpen] = useState(false);

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <div className="flex items-center gap-3 px-5 py-4">
        <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-muted border border-border/50 shrink-0">
          <ProviderIcon provider={p.provider_id} size={20} />
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-semibold truncate capitalize">{p.provider_id.replace(/_/g, ' ')}</p>
          {p.budget_usd && (
            <p className="text-xs text-muted-foreground">{fmtUSD(p.spent_usd_month)} / {fmtUSD(p.budget_usd)} cap</p>
          )}
        </div>
        <div className="text-right shrink-0">
          <p className="text-sm font-semibold tabular-nums">{fmtUSD(p.spent_usd_month)}</p>
          <p className="text-xs text-muted-foreground tabular-nums">{fmtTokens(p.spent_tokens_month)} tokens</p>
        </div>
        <button
          onClick={() => setOpen(o => !o)}
          className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent shrink-0 transition-colors"
        >
          {open ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        </button>
      </div>

      {p.budget_usd && (
        <div className="px-5 pb-3">
          <SpendBar spent={p.spent_usd_month} limit={p.budget_usd} />
        </div>
      )}

      {open && (
        <div className="border-t border-border/60">
          {p.keys.length === 0 ? (
            <p className="px-5 py-3 text-xs text-muted-foreground">No key spend data this month.</p>
          ) : (
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-border/40 bg-muted/20">
                  <th className="px-5 py-2 text-left font-medium text-muted-foreground">Key</th>
                  <th className="px-3 py-2 text-right font-medium text-muted-foreground">Spent</th>
                  <th className="px-3 py-2 text-right font-medium text-muted-foreground">Tokens</th>
                  <th className="px-3 py-2 text-left font-medium text-muted-foreground">Budget</th>
                  <th className="px-5 py-2 text-right font-medium text-muted-foreground">Status</th>
                </tr>
              </thead>
              <tbody>
                {p.keys.map(k => (
                  <tr key={k.key_id} className="border-b border-border/30 last:border-0 hover:bg-accent/20">
                    <td className="px-5 py-2.5">
                      <p className="font-medium truncate max-w-[140px]">{k.label || `Key …${k.key_hash}`}</p>
                      <p className="text-muted-foreground font-mono">{k.key_hash}</p>
                    </td>
                    <td className="px-3 py-2.5 text-right tabular-nums font-medium">{fmtUSD(k.spent_usd_month)}</td>
                    <td className="px-3 py-2.5 text-right tabular-nums text-muted-foreground">{fmtTokens(k.spent_tokens_month)}</td>
                    <td className="px-3 py-2.5">
                      <span className="capitalize text-muted-foreground">{k.budget_type}</span>
                      {k.limit_usd && <span className="ml-1 text-muted-foreground">· {fmtUSD(k.limit_usd)}</span>}
                    </td>
                    <td className="px-5 py-2.5 text-right">
                      <KeyStatusBadge spent={k.spent_usd_month} limit={k.limit_usd} budgetType={k.budget_type} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  );
}

// ─── Page ────────────────────────────────────────────────────────────────────────

export default function SpendPage() {
  const [data, setData]         = useState<SpendSummary | null>(null);
  const [loading, setLoading]   = useState(true);
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const d = await providersApi.getSpendSummary();
      setData(d as SpendSummary);
      setLastRefresh(new Date());
    } catch { /* silently fail — no data yet */ }
    finally { setLoading(false); }
  }, []);

  useEffect(() => {
    load();
    const id = setInterval(load, 60_000);
    return () => clearInterval(id);
  }, [load]);

  const total = data?.providers.reduce((s, p) => s + p.spent_usd_month, 0) ?? 0;

  return (
    <div className="space-y-4">
      <CanvasHeader
        title="Provider Spend"
        description={data ? `${data.month} · refreshes every 60s` : 'Monthly spend summary across all providers'}
        actions={
          <Button variant="ghost" mode="icon" size="sm" onClick={load} title="Refresh">
            <RefreshCw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
          </Button>
        }
      />

      {loading && !data ? (
        <div className="flex items-center justify-center gap-2 py-20 text-sm text-muted-foreground">
          <Loader2 className="h-5 w-5 animate-spin" /> Loading spend data…
        </div>
      ) : !data || data.providers.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border px-6 py-12 text-center text-sm text-muted-foreground">
          <p className="font-medium">No spend data yet</p>
          <p className="mt-1 text-xs">Spend is tracked automatically once providers are configured and keys are verified.</p>
        </div>
      ) : (
        <>
          {/* Total header */}
          <div className="rounded-xl border border-border bg-card px-6 py-4 flex items-center justify-between">
            <div>
              <p className="text-xs text-muted-foreground uppercase tracking-wide font-medium">This month total</p>
              <p className="text-2xl font-bold tabular-nums mt-0.5">{fmtUSD(total)}</p>
            </div>
            {lastRefresh && (
              <p className="text-xs text-muted-foreground">Updated {lastRefresh.toLocaleTimeString()}</p>
            )}
          </div>

          {/* Per-provider cards */}
          <div className="space-y-3">
            {data.providers.map(p => (
              <ProviderSpendCard key={p.provider_id} p={p} />
            ))}
          </div>
        </>
      )}
    </div>
  );
}
