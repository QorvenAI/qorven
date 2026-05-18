'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * /supervisor — Prime's team-health cockpit (T2.6).
 *
 * Three surfaces stacked top-down:
 *   1. Status strip — totals from /supervisor/status, plus the
 *      supervisor daemon's own up/not-initialized state.
 *   2. Escalations — pending human decisions; approve / reject inline.
 *   3. Health roster — per-agent health rows (heartbeat, error counts,
 *      sampling rate, ack-suspension).
 *   4. Fixes — auto-fix catalog + recent history.
 *
 * Data loads in parallel on mount; each card handles its own empty
 * and error states locally so a single failing endpoint doesn't black
 * out the page.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Crown, AlertTriangle, CheckCircle2, XCircle, Activity,
  Heart, ShieldCheck, Wrench, Loader2, RefreshCw, ShieldOff,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import {
  supervisor,
  type SupervisorStatus,
  type SupervisorAgentHealth,
  type SupervisorAgentStatus,
  type SupervisorMessage,
  type SupervisorRisk,
  type SupervisorFix,
  type SupervisorFixHistory,
} from '@/lib/api';

const RISK_STYLE: Record<SupervisorRisk, string> = {
  low:    'bg-emerald-500/15 text-emerald-500 border-emerald-500/30',
  medium: 'bg-amber-400/15 text-amber-400 border-amber-400/30',
  high:   'bg-destructive/15 text-destructive border-destructive/30',
};

const AGENT_STATE_STYLE: Record<SupervisorAgentStatus, { dot: string; label: string }> = {
  healthy:      { dot: 'bg-emerald-500',  label: 'healthy' },
  degraded:     { dot: 'bg-amber-400',    label: 'degraded' },
  unresponsive: { dot: 'bg-destructive',  label: 'unresponsive' },
  suspended:    { dot: 'bg-muted',        label: 'suspended' },
};

export default function SupervisorPage() {
  const [status, setStatus] = useState<SupervisorStatus | null>(null);
  const [health, setHealth] = useState<SupervisorAgentHealth[]>([]);
  const [escalations, setEscalations] = useState<SupervisorMessage[]>([]);
  const [fixes, setFixes] = useState<SupervisorFix[]>([]);
  const [fixHistory, setFixHistory] = useState<SupervisorFixHistory[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    const safe = <T,>(p: Promise<T>, fallback: T) => p.catch(() => fallback);
    try {
      const [s, h, e, f] = await Promise.all([
        safe(supervisor.status(),      { status: 'not_initialized' as const }),
        safe(supervisor.health(),      { agents: [] }),
        safe(supervisor.escalations(), { escalations: [] }),
        safe(supervisor.fixes(),       { available: [], history: [] }),
      ]);
      setStatus(s);
      setHealth(h.agents ?? []);
      setEscalations(e.escalations ?? []);
      setFixes(f.available ?? []);
      setFixHistory(f.history ?? []);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : 'Failed to load supervisor data');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  const notInitialized = status?.status === 'not_initialized';

  return (
    <div className="mx-auto max-w-6xl space-y-5 p-4 lg:p-6">
      <header className="flex items-start gap-3">
        <Crown className="h-6 w-6 text-amber-400" />
        <div>
          <h1 className="text-lg font-semibold">Supervisor</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Prime&apos;s view of the team — health, escalations, and auto-fixes.
          </p>
        </div>
        <button
          onClick={refresh}
          disabled={loading}
          className="ml-auto inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-accent disabled:opacity-60"
        >
          {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
          Refresh
        </button>
      </header>

      {loadError && (
        <div className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <div>
            <p className="font-medium">Something failed to load</p>
            <p className="mt-0.5 text-destructive/80">{loadError}</p>
          </div>
        </div>
      )}

      {notInitialized && (
        <div className="flex items-start gap-2 rounded-lg border border-amber-400/40 bg-amber-400/5 p-3 text-xs text-amber-400">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <div>
            <p className="font-medium">Supervisor daemon not initialized</p>
            <p className="mt-0.5 text-amber-400/80">
              The per-agent health monitor is off — status, escalations, and
              auto-fix history will stay empty until it&apos;s started.
            </p>
          </div>
        </div>
      )}

      <StatusStrip status={status} escalationsCount={escalations.length} />

      <EscalationsSection
        items={escalations}
        onResolved={refresh}
      />

      <HealthTable items={health} onUnsuspend={refresh} />

      <FixesSection available={fixes} history={fixHistory} />
    </div>
  );
}

// ───────────────────────────────────────────────────────────────────

function StatusStrip({
  status,
  escalationsCount,
}: {
  status: SupervisorStatus | null;
  escalationsCount: number;
}) {
  const items = useMemo(
    () => [
      { icon: Activity,       label: 'Total exchanges',  value: status?.total_exchanges ?? 0 },
      { icon: Heart,          label: 'Open',             value: status?.open_exchanges ?? 0 },
      { icon: CheckCircle2,   label: 'Acknowledged',     value: status?.acked_exchanges ?? 0, tone: 'emerald' },
      { icon: AlertTriangle,  label: 'Escalated',        value: status?.escalated_exchanges ?? 0, tone: 'amber' },
      { icon: XCircle,        label: 'Timed out',        value: status?.timeout_exchanges ?? 0, tone: 'destructive' },
      { icon: ShieldCheck,    label: 'Awaiting human',   value: status?.pending_escalations ?? escalationsCount, tone: status?.pending_escalations || escalationsCount ? 'amber' : undefined },
    ] as const,
    [status, escalationsCount],
  );
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6">
      {items.map((it) => (
        <StatCard key={it.label} {...it} />
      ))}
    </div>
  );
}

function StatCard({
  icon: Icon,
  label,
  value,
  tone,
}: {
  icon: typeof Activity;
  label: string;
  value: number;
  tone?: 'emerald' | 'amber' | 'destructive';
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-3">
      <div className="flex items-center gap-1.5 text-2xs text-muted-foreground">
        <Icon
          className={cn(
            'h-3.5 w-3.5',
            tone === 'emerald' && 'text-emerald-500',
            tone === 'amber' && 'text-amber-400',
            tone === 'destructive' && 'text-destructive',
          )}
        />
        {label}
      </div>
      <div className="mt-1 font-mono text-xl font-semibold">{value.toLocaleString()}</div>
    </div>
  );
}

function EscalationsSection({
  items,
  onResolved,
}: {
  items: SupervisorMessage[];
  onResolved: () => void;
}) {
  return (
    <section className="rounded-xl border border-border bg-card/40">
      <header className="flex items-center gap-2 border-b border-border/60 px-4 py-2.5">
        <AlertTriangle className="h-4 w-4 text-amber-400" />
        <h2 className="text-sm font-semibold">Escalations</h2>
        <span className="text-2xs text-muted-foreground">awaiting human decision</span>
        <span className="ml-auto font-mono text-2xs text-muted-foreground">{items.length}</span>
      </header>
      {items.length === 0 ? (
        <p className="px-4 py-6 text-center text-2xs text-muted-foreground">
          No open escalations — the team is running unattended.
        </p>
      ) : (
        <ul className="divide-y divide-border/60">
          {items.map((m) => (
            <EscalationRow key={m.id} message={m} onResolved={onResolved} />
          ))}
        </ul>
      )}
    </section>
  );
}

function EscalationRow({
  message,
  onResolved,
}: {
  message: SupervisorMessage;
  onResolved: () => void;
}) {
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState<'approve' | 'reject' | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const act = async (kind: 'approve' | 'reject') => {
    setBusy(kind);
    setErr(null);
    try {
      if (kind === 'approve') await supervisor.approve(message.id, reason.trim() || undefined);
      else await supervisor.reject(message.id, reason.trim() || undefined);
      onResolved();
    } catch (e) {
      setErr(e instanceof Error ? e.message : `${kind} failed`);
      setBusy(null);
    }
  };

  const risk: SupervisorRisk = (message.risk as SupervisorRisk) ?? 'medium';

  return (
    <li className="p-4">
      <div className="flex items-start gap-2">
        <span
          className={cn(
            'shrink-0 rounded-md border px-1.5 py-0.5 text-xs font-mono uppercase',
            RISK_STYLE[risk],
          )}
        >
          {risk}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2 text-2xs text-muted-foreground">
            <span className="font-mono">{message.from} → {message.to}</span>
            <span>·</span>
            <span>{new Date(message.timestamp).toLocaleString()}</span>
            {message.exchange_id && (
              <>
                <span>·</span>
                <span className="font-mono">exch {message.exchange_id.slice(0, 8)}</span>
              </>
            )}
          </div>
          <p className="mt-1 text-sm leading-relaxed">{message.content}</p>

          {message.context && Object.keys(message.context).length > 0 && (
            <details className="mt-2 rounded-md border border-border/60 bg-background/50 text-2xs">
              <summary className="cursor-pointer px-2 py-1 text-muted-foreground hover:text-foreground">
                context
              </summary>
              <pre className="overflow-x-auto whitespace-pre-wrap break-all px-2 pb-2 font-mono text-muted-foreground">
                {JSON.stringify(message.context, null, 2)}
              </pre>
            </details>
          )}

          <div className="mt-3 flex items-center gap-2">
            <input
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Reason (optional)"
              className="qr-input flex-1 text-xs h-7 py-0"
            />
            <button
              onClick={() => act('approve')}
              disabled={!!busy}
              className={cn(
                'inline-flex items-center gap-1.5 rounded-md border px-3 py-1 text-xs font-medium',
                'border-emerald-500/40 bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20',
                'disabled:opacity-50 disabled:cursor-not-allowed',
              )}
            >
              {busy === 'approve' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <CheckCircle2 className="h-3.5 w-3.5" />}
              Approve
            </button>
            <button
              onClick={() => act('reject')}
              disabled={!!busy}
              className={cn(
                'inline-flex items-center gap-1.5 rounded-md border px-3 py-1 text-xs font-medium',
                'border-destructive/40 bg-destructive/10 text-destructive hover:bg-destructive/20',
                'disabled:opacity-50 disabled:cursor-not-allowed',
              )}
            >
              {busy === 'reject' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <XCircle className="h-3.5 w-3.5" />}
              Reject
            </button>
          </div>
          {err && <p className="mt-1 text-2xs text-destructive">{err}</p>}
        </div>
      </div>
    </li>
  );
}

function HealthTable({ items, onUnsuspend }: { items: SupervisorAgentHealth[]; onUnsuspend: () => void }) {
  return (
    <section className="rounded-xl border border-border bg-card/40">
      <header className="flex items-center gap-2 border-b border-border/60 px-4 py-2.5">
        <Heart className="h-4 w-4 text-emerald-500" />
        <h2 className="text-sm font-semibold">Agent health</h2>
        <span className="ml-auto font-mono text-2xs text-muted-foreground">{items.length}</span>
      </header>
      {items.length === 0 ? (
        <p className="px-4 py-6 text-center text-2xs text-muted-foreground">
          No agents reporting. The health probe runs every 30s; give it a moment after cold-start.
        </p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[640px] text-xs">
            <thead>
              <tr className="border-b border-border/60 text-2xs uppercase tracking-wider text-muted-foreground">
                <th className="px-4 py-2 text-left font-medium">Agent</th>
                <th className="px-4 py-2 text-left font-medium">Status</th>
                <th className="px-4 py-2 text-left font-medium">Last heartbeat</th>
                <th className="px-4 py-2 text-right font-medium">Errors (7d)</th>
                <th className="px-4 py-2 text-right font-medium">Streak</th>
                <th className="px-4 py-2 text-right font-medium">Sample rate</th>
                <th className="px-4 py-2 text-right font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {items.map((a) => (
                <HealthRow key={a.agent_id} agent={a} onUnsuspend={onUnsuspend} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function HealthRow({ agent: a, onUnsuspend }: { agent: SupervisorAgentHealth; onUnsuspend: () => void }) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const s = AGENT_STATE_STYLE[a.status] ?? AGENT_STATE_STYLE.degraded;

  const handleUnsuspend = async () => {
    setBusy(true);
    setErr(null);
    try {
      await supervisor.unsuspend(a.agent_id);
      onUnsuspend();
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'unsuspend failed');
      setBusy(false);
    }
  };

  return (
    <tr className="border-b border-border/30 last:border-0 hover:bg-accent/30">
      <td className="px-4 py-2">
        <div className="font-medium">{a.agent_name}</div>
        <div className="font-mono text-2xs text-muted-foreground">{a.agent_id.slice(0, 8)}</div>
      </td>
      <td className="px-4 py-2">
        <span className="inline-flex items-center gap-1.5 flex-wrap">
          <span className={cn('h-2 w-2 rounded-full', s.dot)} />
          <span>{s.label}</span>
          {a.suspended_from_ack && (
            <span className="rounded-sm border border-amber-400/40 bg-amber-400/10 px-1 text-2xs font-mono uppercase text-amber-400">
              ack-suspended
            </span>
          )}
          {err && <span className="text-2xs text-destructive">{err}</span>}
        </span>
      </td>
      <td className="px-4 py-2 font-mono text-muted-foreground">
        {relTime(a.last_heartbeat)}
      </td>
      <td className="px-4 py-2 text-right font-mono">
        {a.total_errors_7d}
      </td>
      <td className="px-4 py-2 text-right font-mono">
        <span className={a.consecutive_errors > 0 ? 'text-destructive' : ''}>
          {a.consecutive_errors}
        </span>
      </td>
      <td className="px-4 py-2 text-right font-mono">
        {(a.sampling_rate * 100).toFixed(0)}%
      </td>
      <td className="px-4 py-2 text-right">
        {a.suspended_from_ack && (
          <button
            onClick={handleUnsuspend}
            disabled={busy}
            className={cn(
              'inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-2xs font-medium',
              'border-emerald-500/40 bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20',
              'disabled:opacity-50 disabled:cursor-not-allowed',
            )}
          >
            {busy
              ? <Loader2 className="h-3 w-3 animate-spin" />
              : <ShieldOff className="h-3 w-3" />}
            Unsuspend
          </button>
        )}
      </td>
    </tr>
  );
}

function FixesSection({
  available,
  history,
}: {
  available: SupervisorFix[];
  history: SupervisorFixHistory[];
}) {
  return (
    <section className="grid grid-cols-1 gap-3 lg:grid-cols-2">
      {/* Catalog */}
      <div className="rounded-xl border border-border bg-card/40">
        <header className="flex items-center gap-2 border-b border-border/60 px-4 py-2.5">
          <Wrench className="h-4 w-4 text-primary" />
          <h2 className="text-sm font-semibold">Fix catalog</h2>
          <span className="ml-auto font-mono text-2xs text-muted-foreground">{available.length}</span>
        </header>
        {available.length === 0 ? (
          <p className="px-4 py-5 text-center text-2xs text-muted-foreground">
            No fixes registered.
          </p>
        ) : (
          <ul className="divide-y divide-border/60">
            {available.map((f) => (
              <li key={f.type} className="flex items-start gap-2 px-4 py-2.5 text-xs">
                <span
                  className={cn(
                    'mt-0.5 shrink-0 rounded-md border px-1.5 py-0.5 text-xs font-mono uppercase',
                    RISK_STYLE[f.risk] ?? RISK_STYLE.medium,
                  )}
                >
                  {f.risk}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="font-mono text-2xs font-medium text-foreground">{f.type}</div>
                  <p className="mt-0.5 leading-relaxed text-muted-foreground">{f.description}</p>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* History */}
      <div className="rounded-xl border border-border bg-card/40">
        <header className="flex items-center gap-2 border-b border-border/60 px-4 py-2.5">
          <Activity className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">Recent auto-fixes</h2>
          <span className="ml-auto font-mono text-2xs text-muted-foreground">{history.length}</span>
        </header>
        {history.length === 0 ? (
          <p className="px-4 py-5 text-center text-2xs text-muted-foreground">
            No fixes applied yet.
          </p>
        ) : (
          <ul className="divide-y divide-border/60">
            {history.slice(0, 12).map((h, i) => (
              <li key={`${h.fix_type}-${h.timestamp}-${i}`} className="flex items-start gap-2 px-4 py-2 text-2xs">
                {h.success ? (
                  <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-emerald-500" />
                ) : (
                  <XCircle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-destructive" />
                )}
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-mono font-medium">{h.fix_type}</span>
                    <span className="font-mono text-muted-foreground">{fmtDuration(h.duration)}</span>
                  </div>
                  <div className="text-muted-foreground">{new Date(h.timestamp).toLocaleString()}</div>
                  {!h.success && h.error && (
                    <p className="mt-0.5 text-destructive/80">{h.error}</p>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}

// ───────────────────────────────────────────────────────────────────

function relTime(iso: string): string {
  if (!iso) return '—';
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return iso;
  const diff = Date.now() - t;
  if (diff < 60_000) return `${Math.round(diff / 1000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
  return new Date(iso).toLocaleDateString();
}

function fmtDuration(ns: number): string {
  // Backend sends fix duration in nanoseconds (Go's time.Duration.String).
  if (!Number.isFinite(ns) || ns <= 0) return '';
  const ms = ns / 1_000_000;
  if (ms < 1) return `${ns} ns`;
  if (ms < 1000) return `${ms.toFixed(1)} ms`;
  return `${(ms / 1000).toFixed(2)} s`;
}
