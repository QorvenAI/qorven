'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * /sessions — session list + search (T3.5).
 *
 * Two modes:
 *  • Browse — full session list, most-recent first.
 *  • Search — /v1/sessions/search?q=… returns sessions whose content
 *    matches; we surface match count.
 *
 * Unified timeline lives on /qors/[id] (per-agent), not here — this
 * page is intentionally scoped to "find a session" not "read agent
 * history".
 */

import { useCallback, useEffect, useState } from 'react';
import Link from 'next/link';
import { AlertCircle, Clock, Search as SearchIcon, X, Loader2 } from 'lucide-react';
import { sessions, agents } from '@/lib/api';
import { ErrorBoundary } from '@/components/error-boundary';
import { TableSkeleton } from '@/components/page-skeleton';
import { EmptyState, emptyStates } from '@/components/empty-state';
import type { Session, Soul } from '@/types';

export default function SessionsPage() {
  const [list, setList] = useState<Session[]>([]);
  const [agentMap, setAgentMap] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [query, setQuery] = useState('');
  const [searching, setSearching] = useState(false);
  const [searchHits, setSearchHits] = useState<Session[] | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    Promise.all([sessions.list(), agents.list()])
      .then(([s, a]) => {
        setList(s);
        const map: Record<string, string> = {};
        (a as Soul[]).forEach((agent) => {
          map[agent.id] = agent.display_name || agent.agent_key || agent.id;
        });
        setAgentMap(map);
        setLoading(false);
      })
      .catch((e) => { setError(e.message); setLoading(false); });
  }, []);

  useEffect(() => { load(); }, [load]);

  const runSearch = async (e?: React.FormEvent) => {
    e?.preventDefault();
    const q = query.trim();
    if (!q) {
      setSearchHits(null);
      return;
    }
    setError(null);
    setSearching(true);
    try {
      const res = await sessions.search(q);
      setSearchHits(res.sessions ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Search failed');
    } finally {
      setSearching(false);
    }
  };

  const clearSearch = () => {
    setQuery('');
    setSearchHits(null);
  };

  const rows = searchHits ?? list;
  const mode = searchHits ? 'search' : 'browse';

  return (
    <ErrorBoundary fallbackTitle="Failed to load sessions">
      <div className="space-y-5">
        <header>
          <h1 className="text-lg font-semibold">Sessions</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {mode === 'search'
              ? `${rows.length} match${rows.length !== 1 ? 'es' : ''} for "${query.trim()}"`
              : loading
              ? 'Loading sessions…'
              : `${rows.length} session${rows.length !== 1 ? 's' : ''}`}
          </p>
        </header>

        <form onSubmit={runSearch} className="flex items-center gap-2">
          <div className="relative flex-1 max-w-xl">
            <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search across session content…"
              className="qr-input pl-8 pr-8"
            />
            {query && (
              <button
                type="button"
                onClick={clearSearch}
                className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                title="Clear"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
          <button
            type="submit"
            disabled={searching || !query.trim()}
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-2 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            {searching ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <SearchIcon className="h-3.5 w-3.5" />}
            Search
          </button>
        </form>

        {error && (
          <div className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
            <AlertCircle className="mt-0.5 h-4 w-4" />
            <div>
              <p className="font-medium">{error}</p>
              <button onClick={load} className="mt-1 text-2xs text-primary hover:underline">
                Retry
              </button>
            </div>
          </div>
        )}

        {loading ? (
          <TableSkeleton rows={6} />
        ) : rows.length === 0 ? (
          mode === 'search' ? (
            <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-border/60 bg-card/40 px-6 py-10 text-center">
              <SearchIcon className="h-6 w-6 text-muted-foreground/60" />
              <p className="text-sm">No sessions match that query.</p>
              <button onClick={clearSearch} className="text-2xs text-primary hover:underline">
                Back to all sessions
              </button>
            </div>
          ) : (
            <EmptyState {...emptyStates.sessions} />
          )
        ) : (
          <div className="overflow-hidden rounded-xl border border-border bg-card">
            <div className="grid grid-cols-[1fr_auto_auto_auto_auto] gap-4 border-b border-border bg-muted/30 px-4 py-2 text-2xs font-medium uppercase tracking-wider text-muted-foreground">
              <span>Session</span>
              <span>Agent</span>
              <span>Channel</span>
              <span>Tokens</span>
              <span>Created</span>
            </div>
            {rows.map((s) => (
              <Link
                key={s.id}
                href={mode === 'search' ? `/sessions/${s.id}` : s.agent_id ? `/qors/${s.agent_id}` : `/sessions/${s.id}`}
                className="grid grid-cols-[1fr_auto_auto_auto_auto] items-center gap-4 border-b border-border px-4 py-3 transition-colors last:border-0 hover:bg-accent/50"
              >
                <span className="truncate font-mono text-sm text-foreground">
                  {s.label || s.id.slice(0, 8)}
                </span>
                <span className="text-sm text-muted-foreground">
                  {s.agent_id ? (agentMap[s.agent_id] || s.agent_id.slice(0, 8)) : '—'}
                </span>
                <span className="rounded-full border border-border px-2 py-0.5 text-2xs text-muted-foreground">
                  {s.channel || 'webchat'}
                </span>
                <span className="text-sm text-muted-foreground">
                  {((s as any).input_tokens ?? 0) + ((s as any).output_tokens ?? 0)}
                </span>
                <span className="flex items-center gap-1 text-sm text-muted-foreground">
                  <Clock className="h-3 w-3" />
                  {new Date(s.created_at).toLocaleDateString()}
                </span>
              </Link>
            ))}
          </div>
        )}
      </div>
    </ErrorBoundary>
  );
}
