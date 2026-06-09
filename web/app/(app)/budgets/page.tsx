'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import Link from 'next/link';
import { Loader2, RefreshCw, AlertTriangle, Check, X } from 'lucide-react';
import { toast } from 'sonner';
import { cn } from '@/lib/utils';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { budgets as budgetsApi } from '@/lib/api';
import { Button } from '@/components/qor/button';
import { Input } from '@/components/qor/input';

// ─── Types ─────────────────────────────────────────────────────────────────────

interface FinanceSettings {
  cfo_authority: string;
  cfo_threshold_usd: number;
}

interface ProposalLine {
  id: string;
  scope: string;
  scope_id: string;
  proposed_monthly_usd: number;
  allocation_mode: string;
  status: string;
}

interface Proposal {
  id: string;
  reason: string;
  status: string;
  lines: ProposalLine[];
}

interface Effective {
  declared_remaining_uusd: number;
  provider_remaining_uusd: number;
  effective_uusd: number;
  binding: 'declared' | 'providers';
  warnings: string[];
}

// ─── Format helpers ──────────────────────────────────────────────────────────────

function fmtUSD(v: number) {
  if (v === 0) return '$0.00';
  if (v > 0 && v < 0.01) return '<$0.01';
  return `$${v.toFixed(2)}`;
}

const uusdToUsd = (v: number) => v / 1_000_000;

const AUTHORITY_OPTIONS: Array<{ value: string; label: string }> = [
  { value: 'ask', label: 'Ask every time' },
  { value: 'threshold', label: 'Within threshold' },
  { value: 'full', label: 'Full power' },
];

// ─── CFO Authority ─────────────────────────────────────────────────────────────

function CfoAuthorityCard() {
  const [authority, setAuthority] = useState('ask');
  const [threshold, setThreshold] = useState('');
  const [loading, setLoading]     = useState(true);
  const [saving, setSaving]       = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const s = (await budgetsApi.getFinanceSettings()) as FinanceSettings;
      setAuthority(s.cfo_authority || 'ask');
      setThreshold(s.cfo_threshold_usd != null ? String(s.cfo_threshold_usd) : '');
    } catch { /* settings may not be set yet */ }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const save = async () => {
    const n = parseFloat(threshold);
    const thresholdUsd = Number.isFinite(n) && n >= 0 ? n : 0;
    if (authority === 'threshold' && !(thresholdUsd > 0)) {
      toast.error('Enter a positive threshold amount');
      return;
    }
    setSaving(true);
    try {
      await budgetsApi.setFinanceSettings({ cfo_authority: authority, cfo_threshold_usd: thresholdUsd });
      toast.success('CFO authority saved');
    } catch {
      toast.error('Failed to save CFO authority');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="rounded-xl border border-border bg-card px-6 py-4">
      <p className="text-xs text-muted-foreground uppercase tracking-wide font-medium">CFO authority</p>
      <p className="mt-1 text-xs text-muted-foreground">
        Controls how much the CFO agent can commit to before requiring your approval.
      </p>

      {loading ? (
        <div className="mt-4 flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" /> Loading…
        </div>
      ) : (
        <div className="mt-4 flex flex-wrap items-end gap-3">
          <div>
            <label className="block text-xs text-muted-foreground mb-1">Authority</label>
            <select
              value={authority}
              onChange={e => setAuthority(e.target.value)}
              className="h-9 rounded-md border border-border bg-background px-3 text-sm focus:outline-none focus:ring-1 focus:ring-ring"
            >
              {AUTHORITY_OPTIONS.map(o => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>
          </div>

          {authority === 'threshold' && (
            <div>
              <label className="block text-xs text-muted-foreground mb-1">Threshold ($)</label>
              <Input
                type="number"
                min={0}
                step="0.01"
                value={threshold}
                onChange={e => setThreshold(e.target.value)}
                placeholder="Auto-approve up to ($)"
                className="w-44"
              />
            </div>
          )}

          <Button size="sm" onClick={save} disabled={saving}>
            {saving && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            Save
          </Button>
        </div>
      )}
    </div>
  );
}

// ─── Pending proposals ───────────────────────────────────────────────────────────

function ProposalCard({ proposal, onDecided }: { proposal: Proposal; onDecided: () => void }) {
  const [decisions, setDecisions] = useState<Record<string, boolean>>(
    () => Object.fromEntries(proposal.lines.map(l => [l.id, true])),
  );
  const [applying, setApplying] = useState(false);

  const apply = async () => {
    setApplying(true);
    try {
      await budgetsApi.decideProposal(
        proposal.id,
        proposal.lines.map(l => ({ line_id: l.id, approve: decisions[l.id] ?? true })),
      );
      toast.success('Decisions applied');
      onDecided();
    } catch {
      toast.error('Failed to apply decisions');
    } finally {
      setApplying(false);
    }
  };

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <div className="flex items-start justify-between gap-4 px-5 py-4">
        <div className="min-w-0">
          <p className="text-sm font-medium">{proposal.reason || 'Budget proposal'}</p>
          <p className="mt-0.5 text-xs text-muted-foreground capitalize">Status: {proposal.status}</p>
        </div>
        <Button size="sm" onClick={apply} disabled={applying} className="shrink-0">
          {applying && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
          Apply decisions
        </Button>
      </div>

      <div className="border-t border-border/60 divide-y divide-border/40">
        {proposal.lines.map(l => {
          const approve = decisions[l.id] ?? true;
          return (
            <div key={l.id} className="flex items-center justify-between gap-4 px-5 py-3">
              <div className="min-w-0 text-sm">
                <p className="truncate">
                  <span className="capitalize">{l.scope}</span>{' '}
                  <span className="text-muted-foreground">{l.scope_id}</span>{' '}
                  — <span className="tabular-nums font-medium">{fmtUSD(l.proposed_monthly_usd)}</span>/mo{' '}
                  <span className="text-muted-foreground">({l.allocation_mode})</span>
                </p>
              </div>
              <div className="inline-flex rounded-md border border-border p-0.5 shrink-0">
                <button
                  type="button"
                  onClick={() => setDecisions(d => ({ ...d, [l.id]: true }))}
                  className={cn(
                    'flex items-center gap-1 rounded px-2.5 py-1 text-xs font-medium transition-colors',
                    approve ? 'bg-emerald-500 text-white' : 'text-muted-foreground hover:text-foreground',
                  )}
                >
                  <Check className="h-3 w-3" /> Approve
                </button>
                <button
                  type="button"
                  onClick={() => setDecisions(d => ({ ...d, [l.id]: false }))}
                  className={cn(
                    'flex items-center gap-1 rounded px-2.5 py-1 text-xs font-medium transition-colors',
                    !approve ? 'bg-destructive text-white' : 'text-muted-foreground hover:text-foreground',
                  )}
                >
                  <X className="h-3 w-3" /> Reject
                </button>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ─── Reports summary ───────────────────────────────────────────────────────────

function ReportsCard({ eff }: { eff: Effective | null }) {
  if (!eff) {
    return (
      <div className="rounded-xl border border-border bg-card px-6 py-4">
        <p className="text-xs text-muted-foreground uppercase tracking-wide font-medium">Reports</p>
        <p className="mt-2 text-sm text-muted-foreground">No budget data available yet.</p>
        <Link href="/models-hub/spend" className="mt-2 inline-block text-xs text-primary hover:underline">
          View full spend report →
        </Link>
      </div>
    );
  }

  const declared  = uusdToUsd(eff.declared_remaining_uusd);
  const provider  = uusdToUsd(eff.provider_remaining_uusd);
  const effective = uusdToUsd(eff.effective_uusd);
  const noBudget  = eff.effective_uusd === 0 && eff.declared_remaining_uusd === 0;

  return (
    <div className="rounded-xl border border-border bg-card px-6 py-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <p className="text-xs text-muted-foreground uppercase tracking-wide font-medium">Effective available</p>
          {noBudget ? (
            <p className="text-2xl font-bold tabular-nums mt-0.5 text-muted-foreground">No overall budget set</p>
          ) : (
            <p className="text-2xl font-bold tabular-nums mt-0.5">{fmtUSD(effective)}</p>
          )}
        </div>
        {!noBudget && (
          <span className="rounded-md border border-border bg-muted/40 px-2 py-1 text-xs text-muted-foreground shrink-0">
            Limited by: {eff.binding === 'declared' ? 'declared' : 'provider keys'}
          </span>
        )}
      </div>

      {!noBudget && (
        <div className="mt-3 flex flex-wrap gap-x-6 gap-y-1 text-xs text-muted-foreground">
          <span>Declared remaining: <span className="tabular-nums text-foreground">{fmtUSD(declared)}</span></span>
          <span>Provider ceiling: <span className="tabular-nums text-foreground">{fmtUSD(provider)}</span></span>
        </div>
      )}

      {eff.warnings.length > 0 && (
        <div className="mt-3 space-y-1">
          {eff.warnings.map((w, i) => (
            <div key={i} className="flex items-start gap-2 rounded-md bg-amber-400/10 px-3 py-2 text-xs text-amber-400">
              <AlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
              <span>{w}</span>
            </div>
          ))}
        </div>
      )}

      <Link href="/models-hub/spend" className="mt-3 inline-block text-xs text-primary hover:underline">
        View full spend report →
      </Link>
    </div>
  );
}

// ─── Page ────────────────────────────────────────────────────────────────────────

export default function BudgetsPage() {
  const [proposals, setProposals] = useState<Proposal[]>([]);
  const [eff, setEff]             = useState<Effective | null>(null);
  const [loading, setLoading]     = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [p, e] = await Promise.allSettled([
        budgetsApi.listProposals(),
        budgetsApi.effective(),
      ]);
      if (p.status === 'fulfilled') {
        setProposals(((p.value as { proposals?: Proposal[] }).proposals ?? []));
      }
      if (e.status === 'fulfilled') {
        setEff(e.value as Effective);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  return (
    <div className="space-y-4">
      <CanvasHeader
        title="Budgets"
        description="Set CFO authority, review pending budget proposals, and check effective spend."
        actions={
          <Button variant="ghost" mode="icon" size="sm" onClick={load} title="Refresh">
            <RefreshCw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
          </Button>
        }
      />

      {/* 1. CFO authority */}
      <CfoAuthorityCard />

      {/* 2. Pending proposals */}
      <div className="space-y-3">
        <p className="text-xs text-muted-foreground uppercase tracking-wide font-medium px-1">Pending proposals</p>
        {loading && proposals.length === 0 ? (
          <div className="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
            <Loader2 className="h-5 w-5 animate-spin" /> Loading proposals…
          </div>
        ) : proposals.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border px-6 py-10 text-center text-sm text-muted-foreground">
            No pending proposals.
          </div>
        ) : (
          proposals.map(p => <ProposalCard key={p.id} proposal={p} onDecided={load} />)
        )}
      </div>

      {/* 3. Reports */}
      <ReportsCard eff={eff} />
    </div>
  );
}
