'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useCallback, useEffect, useState } from 'react';
import {
  Activity, ChevronDown, ChevronRight, Loader2, RefreshCw,
  AlertCircle, CheckCircle2, XCircle, Clock,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { traces as tracesApi, type TraceRow, type SpanRow, type TraceSummary } from '@/lib/api';
import { useStore } from '@/store';

function fmtMs(ms?: number | null) {
  if (!ms) return '—';
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

function fmtCents(c?: number) {
  if (!c) return '—';
  return `$${(c / 100).toFixed(4)}`;
}

function relTime(iso: string) {
  if (!iso) return '—';
  const diff = Date.now() - Date.parse(iso);
  if (!Number.isFinite(diff)) return iso;
  if (diff < 60_000) return `${Math.round(diff / 1_000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
  return new Date(iso).toLocaleDateString();
}

const STATUS_CLS: Record<string, string> = {
  ok:      'text-emerald-500',
  success: 'text-emerald-500',
  error:   'text-destructive',
  failed:  'text-destructive',
  running: 'text-amber-400',
};

export default function TracesPage() {
  const souls = useStore((s) => s.souls);
  const [agentFilter, setAgentFilter] = useState('');
  const [rows, setRows] = useState<TraceRow[]>([]);
  const [summary, setSummary] = useState<TraceSummary[]>([]);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    setLoading(true);
    const [r, s] = await Promise.all([
      tracesApi.list({ agent_id: agentFilter || undefined, limit: 50 }).catch(() => [] as TraceRow[]),
      tracesApi.summary().catch(() => [] as TraceSummary[]),
    ]);
    setRows(Array.isArray(r) ? r : []);
    setSummary(Array.isArray(s) ? s : []);
    setLoading(false);
  }, [agentFilter]);

  useEffect(() => { refresh(); }, [refresh]);

  const totalCost  = summary.reduce((a, s) => a + s.cost_cents, 0);
  const totalIn    = summary.reduce((a, s) => a + s.input_tokens, 0);
  const totalOut   = summary.reduce((a, s) => a + s.output_tokens, 0);

  return (
    <div className="mx-auto max-w-6xl space-y-5 p-4 lg:p-6">
      <header className="flex items-center gap-3">
        <Activity className="h-6 w-6 text-primary" />
        <div>
          <h1 className="text-lg font-semibold">Traces</h1>
          <p className="text-sm text-muted-foreground">LLM call traces with token + cost breakdown</p>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <select
            value={agentFilter}
            onChange={(e) => setAgentFilter(e.target.value)}
            className="qr-select text-xs">
            <option value="">All agents</option>
            {souls.map((s) => (
              <option key={s.id} value={s.id}>{s.display_name || s.agent_key}</option>
            ))}
          </select>
          <button
            onClick={refresh}
            disabled={loading}
            className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-accent disabled:opacity-60"
          >
            {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
            Refresh
          </button>
        </div>
      </header>

      {/* Summary strip */}
      <div className="grid grid-cols-3 gap-2 sm:grid-cols-4">
        {[
          { label: 'Total traces',  value: rows.length.toLocaleString() },
          { label: 'Input tokens',  value: totalIn.toLocaleString() },
          { label: 'Output tokens', value: totalOut.toLocaleString() },
          { label: 'Total cost',    value: fmtCents(totalCost) },
        ].map((s) => (
          <div key={s.label} className="rounded-xl border border-border bg-card p-3">
            <p className="text-2xs text-muted-foreground">{s.label}</p>
            <p className="mt-1 font-mono text-lg font-semibold">{s.value}</p>
          </div>
        ))}
      </div>

      {/* Trace list */}
      <section className="rounded-xl border border-border bg-card/40">
        <header className="flex items-center gap-2 border-b border-border/60 px-4 py-2.5">
          <Activity className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">Trace log</h2>
          <span className="ml-auto font-mono text-2xs text-muted-foreground">{rows.length}</span>
        </header>
        {loading ? (
          <div className="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" /> Loading…
          </div>
        ) : rows.length === 0 ? (
          <div className="flex flex-col items-center gap-2 py-10">
            <Activity className="h-6 w-6 text-muted-foreground/30" />
            <p className="text-xs text-muted-foreground">No traces yet — they appear when agents make LLM calls.</p>
          </div>
        ) : (
          <div className="divide-y divide-border/40">
            {rows.map((t) => <TraceRow key={t.id} trace={t} souls={souls} />)}
          </div>
        )}
      </section>
    </div>
  );
}

function TraceRow({ trace: t, souls }: { trace: TraceRow; souls: { id: string; display_name?: string; agent_key?: string }[] }) {
  const [open, setOpen] = useState(false);
  const [spans, setSpans] = useState<SpanRow[]>([]);
  const [loading, setLoading] = useState(false);

  const toggle = async () => {
    if (!open && spans.length === 0) {
      setLoading(true);
      tracesApi.spans(t.id).then(setSpans).catch(() => {}).finally(() => setLoading(false));
    }
    setOpen((v) => !v);
  };

  const soul = t.agent_id ? souls.find((s) => s.id === t.agent_id) : null;
  const agentLabel = soul?.display_name ?? soul?.agent_key ?? (t.agent_id ? t.agent_id.slice(0, 8) : '—');
  const statusCls = STATUS_CLS[t.status?.toLowerCase()] ?? 'text-muted-foreground';

  return (
    <div>
      <div
        className="flex items-center gap-3 px-4 py-2 hover:bg-accent/20 cursor-pointer"
        onClick={toggle}
      >
        {open
          ? <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          : <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
        <div className="min-w-0 flex-1 grid grid-cols-[1fr_auto_auto_auto_auto] items-center gap-3 text-xs">
          <span className="font-mono text-muted-foreground truncate">{agentLabel}</span>
          <span className={cn('font-mono text-2xs', statusCls)}>{t.status}</span>
          <span className="font-mono text-muted-foreground">{fmtMs(t.duration_ms)}</span>
          <span className="font-mono text-muted-foreground">
            {t.input_tokens.toLocaleString()} / {t.output_tokens.toLocaleString()}
          </span>
          <span className="font-mono text-emerald-400/80 shrink-0">{fmtCents(t.cost_cents)}</span>
        </div>
        <span className="shrink-0 text-2xs text-muted-foreground/60 tabular-nums">
          {relTime(t.created_at)}
        </span>
      </div>

      {open && (
        <div className="border-t border-border/30 bg-muted/20 pl-10 pr-4 py-2 space-y-1">
          {loading ? (
            <div className="flex items-center gap-2 py-2 text-xs text-muted-foreground">
              <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading spans…
            </div>
          ) : spans.length === 0 ? (
            <p className="text-2xs text-muted-foreground py-1">No spans for this trace.</p>
          ) : (
            spans.map((s) => <SpanRowItem key={s.id} span={s} />)
          )}
        </div>
      )}
    </div>
  );
}

function SpanRowItem({ span: s }: { span: SpanRow }) {
  const statusCls = STATUS_CLS[s.status?.toLowerCase()] ?? 'text-muted-foreground';
  const StatusIcon = s.status === 'error' || s.status === 'failed'
    ? XCircle
    : s.status === 'ok' || s.status === 'success'
      ? CheckCircle2
      : Clock;

  return (
    <div className="flex items-center gap-2 text-2xs rounded-md border border-border/30 bg-card/40 px-3 py-1.5">
      <StatusIcon className={cn('h-3 w-3 shrink-0', statusCls)} />
      <span className="font-mono text-muted-foreground/80 uppercase w-16 shrink-0">{s.span_type}</span>
      <span className="font-medium truncate flex-1">{s.name || s.model || '—'}</span>
      {s.provider && <span className="text-muted-foreground/60">{s.provider}</span>}
      {(s.input_tokens || s.output_tokens) ? (
        <span className="font-mono text-muted-foreground">
          {(s.input_tokens ?? 0).toLocaleString()} / {(s.output_tokens ?? 0).toLocaleString()}
        </span>
      ) : null}
      <span className="font-mono shrink-0">{fmtMs(s.duration_ms)}</span>
      {s.error && (
        <span className="text-destructive truncate max-w-[200px]" title={s.error}>
          <AlertCircle className="inline h-3 w-3 mr-0.5" />{s.error}
        </span>
      )}
    </div>
  );
}
