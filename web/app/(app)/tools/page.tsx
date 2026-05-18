'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * /tools — static tool catalog + live metrics (T3.11).
 *
 * The backend registers 80+ tools, too many for a flat list. The
 * catalog here is a curated hand-written summary grouped by category;
 * the Metrics tab pulls real data from /v1/tools/metrics so users can
 * see which tools are actually hot, which error, and latency.
 */

import { useCallback, useEffect, useState } from 'react';
import { Wrench, Search, BarChart3, Loader2, AlertCircle, RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import { toolMetrics, type ToolMetric, type ToolMetricsSummary } from '@/lib/api';

const catalog: Record<string, Record<string, string>> = {
  coding: {
    glob: 'Find files matching glob patterns',
    grep: 'Search file contents with regex',
    diagnostics: 'Get code diagnostics and lint errors',
    apply_patch: 'Apply a code patch to files',
    undo: 'Undo the last code change',
  },
  files: {
    read_file: 'Read contents of a file',
    write_file: 'Write content to a file',
    edit: 'Edit a file with targeted changes',
    list_files: 'List files in a directory',
  },
  exec: {
    exec: 'Execute a shell command',
  },
  web: {
    web_search: 'Search the web for information',
    web_fetch: 'Fetch content from a URL',
    crawl: 'Crawl a website recursively',
    research: 'Deep research on a topic',
    browse_and_act: 'Browse a page and perform actions',
  },
  memory: {
    memory_search: 'Search stored memories',
    memory_save: 'Save information to memory',
    knowledge_graph_search: 'Query the knowledge graph',
  },
  communication: {
    email_send: 'Send an email message',
    send_telegram: 'Send a Telegram message',
    send_dm: 'Send a direct message',
  },
  agents: {
    delegate: 'Delegate a task to another agent',
    list_agents: 'List all available agents',
    spawn: 'Spawn a new agent instance',
    manage_agents: 'Start, stop, or configure agents',
  },
  self: {
    self_knowledge: 'Query self-knowledge base',
    self_patch: 'Patch own behavior or config',
    self_test: 'Run self-diagnostic tests',
    self_improve: 'Trigger self-improvement routines',
  },
};

type Tab = 'catalog' | 'metrics';

export default function ToolsPage() {
  const [tab, setTab] = useState<Tab>('catalog');
  const [search, setSearch] = useState('');
  const q = search.toLowerCase();

  return (
    <div className="space-y-5">
      <header className="flex items-center gap-3">
        <Wrench className="h-6 w-6 text-primary" />
        <h1 className="text-lg font-semibold">Tools</h1>
      </header>

      {/* Tab bar */}
      <div className="flex items-center gap-0 border-b border-border">
        {(['catalog', 'metrics'] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={cn(
              'flex items-center gap-1.5 border-b-2 px-3 py-2 text-xs font-medium transition-colors -mb-px',
              tab === t
                ? 'border-primary text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground',
            )}
          >
            {t === 'catalog' ? <Wrench className="h-3.5 w-3.5" /> : <BarChart3 className="h-3.5 w-3.5" />}
            {t === 'catalog' ? 'Catalog' : 'Metrics'}
          </button>
        ))}
      </div>

      {tab === 'catalog' ? (
        <CatalogTab search={search} setSearch={setSearch} q={q} />
      ) : (
        <MetricsTab />
      )}
    </div>
  );
}

function CatalogTab({ search, setSearch, q }: { search: string; setSearch: (s: string) => void; q: string }) {
  return (
    <>
      <div className="relative max-w-md">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Filter tools…"
          className="w-full rounded-lg border border-border bg-card py-2 pl-10 pr-4 text-sm text-foreground outline-none placeholder:text-muted-foreground focus:border-primary"
        />
      </div>
      {Object.entries(catalog).map(([category, items]) => {
        const filtered = Object.entries(items).filter(
          ([name, desc]) =>
            name.includes(q) || desc.toLowerCase().includes(q) || category.includes(q),
        );
        if (!filtered.length) return null;
        return (
          <div key={category} className="mb-8">
            <h2 className="mb-3 text-xs font-semibold uppercase tracking-wider text-primary">
              {category}
            </h2>
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3">
              {filtered.map(([name, desc]) => (
                <div key={name} className="rounded-lg border border-border p-4">
                  <p className="font-mono text-sm font-semibold text-foreground">{name}</p>
                  <p className="mt-1 text-xs text-muted-foreground">{desc}</p>
                </div>
              ))}
            </div>
          </div>
        );
      })}
    </>
  );
}

function MetricsTab() {
  const [data, setData] = useState<ToolMetricsSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [sort, setSort] = useState<'calls' | 'errors' | 'latency'>('calls');

  const refresh = useCallback(() => {
    setLoading(true);
    setErr(null);
    toolMetrics
      .all()
      .then(setData)
      .catch((e) => setErr(e instanceof Error ? e.message : 'Failed to load metrics'))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  const rows = (data?.tools ?? []).slice().sort((a, b) => {
    if (sort === 'calls') return b.call_count - a.call_count;
    if (sort === 'errors') return (b.error_count ?? 0) - (a.error_count ?? 0);
    return (b.avg_latency_ms ?? 0) - (a.avg_latency_ms ?? 0);
  });

  return (
    <>
      {/* Summary strip */}
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <StatCard label="Tools" value={data?.tool_count ?? 0} />
        <StatCard label="Total calls" value={data?.total_calls ?? 0} />
        <StatCard
          label="Errors"
          value={data?.total_errors ?? 0}
          tone={(data?.total_errors ?? 0) > 0 ? 'destructive' : undefined}
        />
        <StatCard
          label="Error rate"
          value={data?.total_calls ? ((data.total_errors / data.total_calls) * 100).toFixed(1) + '%' : '—'}
          tone={data && data.total_calls > 0 && data.total_errors / data.total_calls > 0.05 ? 'amber' : undefined}
        />
      </div>

      {/* Controls */}
      <div className="flex items-center gap-2">
        <span className="text-2xs text-muted-foreground">Sort by</span>
        {(['calls', 'errors', 'latency'] as const).map((s) => (
          <button
            key={s}
            onClick={() => setSort(s)}
            className={cn(
              'rounded-md border px-2 py-1 text-2xs font-mono',
              sort === s ? 'border-primary bg-primary/10 text-primary' : 'border-border text-muted-foreground hover:bg-accent',
            )}
          >
            {s}
          </button>
        ))}
        <button
          onClick={refresh}
          disabled={loading}
          className="ml-auto inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-2xs text-muted-foreground hover:bg-accent"
        >
          {loading ? <Loader2 className="h-3 w-3 animate-spin" /> : <RefreshCw className="h-3 w-3" />}
          Refresh
        </button>
      </div>

      {err && (
        <div className="flex items-center gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
          <AlertCircle className="h-4 w-4" />
          <span>{err}</span>
        </div>
      )}

      {loading ? (
        <div className="py-10 text-center text-xs text-muted-foreground">
          <Loader2 className="mx-auto h-4 w-4 animate-spin" />
        </div>
      ) : rows.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border/60 bg-card/40 px-6 py-10 text-center text-sm text-muted-foreground">
          No tool calls recorded yet. Metrics populate as agents run.
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-border bg-card">
          <table className="w-full text-xs">
            <thead>
              <tr className="border-b border-border/60 bg-muted/40 text-2xs uppercase tracking-wider text-muted-foreground">
                <th className="px-4 py-2 text-left font-medium">Tool</th>
                <th className="px-4 py-2 text-right font-medium">Calls</th>
                <th className="px-4 py-2 text-right font-medium">Errors</th>
                <th className="px-4 py-2 text-right font-medium">Success</th>
                <th className="px-4 py-2 text-right font-medium">Avg latency</th>
                <th className="px-4 py-2 text-right font-medium">Max</th>
                <th className="px-4 py-2 text-left font-medium">Last call</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((t) => <MetricsRow key={t.name} m={t} />)}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

function MetricsRow({ m }: { m: ToolMetric }) {
  const errorRate = m.call_count > 0 ? (m.error_count / m.call_count) : 0;
  const highError = errorRate > 0.05;
  return (
    <tr className={cn('border-b border-border/30 last:border-0 hover:bg-accent/30', highError && 'bg-destructive/5')}>
      <td className="px-4 py-2 font-mono font-medium">{m.name}</td>
      <td className="px-4 py-2 text-right font-mono">{m.call_count.toLocaleString()}</td>
      <td className={cn('px-4 py-2 text-right font-mono', m.error_count > 0 && 'text-destructive')}>
        {m.error_count.toLocaleString()}
      </td>
      <td className="px-4 py-2 text-right font-mono">
        {(m.success_rate * 100).toFixed(1)}%
      </td>
      <td className="px-4 py-2 text-right font-mono text-muted-foreground">
        {m.avg_latency_ms.toFixed(1)} ms
      </td>
      <td className="px-4 py-2 text-right font-mono text-muted-foreground">
        {m.max_latency_ms.toFixed(0)} ms
      </td>
      <td className="px-4 py-2 text-muted-foreground">
        {m.last_call_at ? new Date(m.last_call_at).toLocaleString() : '—'}
      </td>
    </tr>
  );
}

function StatCard({
  label,
  value,
  tone,
}: {
  label: string;
  value: number | string;
  tone?: 'destructive' | 'amber';
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-3">
      <p className="text-2xs text-muted-foreground">{label}</p>
      <p
        className={cn(
          'mt-1 font-mono text-xl font-semibold',
          tone === 'destructive' && 'text-destructive',
          tone === 'amber' && 'text-amber-400',
        )}
      >
        {typeof value === 'number' ? value.toLocaleString() : value}
      </p>
    </div>
  );
}
