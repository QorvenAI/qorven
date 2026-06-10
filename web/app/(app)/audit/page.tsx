'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import {
  ChevronLeft, ChevronRight, User, Bot, Settings, Zap,
  AlertTriangle, Plus, Trash2, Edit3, RefreshCw, Search, Filter,
} from 'lucide-react';
import { PageShell } from '@/components/layouts/page-shell';
import { cn } from '@/lib/utils';
import { request } from '@/lib/api-core';
import { relativeTime } from '@/lib/relative-time';

type Entry = {
  id: number;
  actor_type: string;
  actor_id: string;
  actor_name: string;
  action: string;
  resource: string;
  resource_id: string;
  details: any;
  ip_address: string;
  created_at: string;
};

// Action icon + color per action type
const ACTION_META: Record<string, { icon: React.ElementType; color: string; bg: string; label: string }> = {
  create:    { icon: Plus,          color: 'text-emerald-400', bg: 'bg-emerald-500/10', label: 'Created' },
  update:    { icon: Edit3,         color: 'text-blue-400',    bg: 'bg-blue-500/10',    label: 'Updated' },
  delete:    { icon: Trash2,        color: 'text-red-400',     bg: 'bg-red-500/10',     label: 'Deleted' },
  tool_exec: { icon: Zap,           color: 'text-amber-400',   bg: 'bg-amber-500/10',   label: 'Ran tool' },
  tool_error:{ icon: AlertTriangle, color: 'text-red-400',     bg: 'bg-red-500/10',     label: 'Tool error' },
  execute:   { icon: Zap,           color: 'text-amber-400',   bg: 'bg-amber-500/10',   label: 'Ran tool' },
};

// Human-readable resource names
const RESOURCE_LABELS: Record<string, string> = {
  agents:      'Agents',
  sessions:    'Chats',
  tasks:       'Tasks',
  connections: 'Channels',
  connectors:  'Connectors',
  workflows:   'Workflows',
  credentials: 'Credentials',
  providers:   'Model providers',
  tool:        'Tool',
  chat:        'Chat',
  admin:       'System',
};

const ACTOR_META: Record<string, { icon: React.ElementType; label: string; color: string }> = {
  user:   { icon: User,     label: 'User',   color: 'text-blue-400' },
  agent:  { icon: Bot,      label: 'Agent',  color: 'text-violet-400' },
  system: { icon: Settings, label: 'System', color: 'text-muted-foreground' },
};

function ActionIcon({ action }: { action: string }) {
  const meta = ACTION_META[action] ?? { icon: RefreshCw, color: 'text-muted-foreground', bg: 'bg-muted/30' };
  const Icon = meta.icon;
  return (
    <span className={cn('inline-flex h-6 w-6 items-center justify-center rounded-full shrink-0', meta.bg)}>
      <Icon className={cn('h-3 w-3', meta.color)} />
    </span>
  );
}

function DetailsCell({ details, action }: { details: any; action: string }) {
  if (!details) return <span className="text-muted-foreground/40">—</span>;

  // Tool exec: show tool args
  if ((action === 'tool_exec' || action === 'tool_error') && details.args) {
    return (
      <span className="font-mono text-2xs text-muted-foreground truncate max-w-xs inline-block align-middle" title={details.args}>
        {details.args.slice(0, 80)}{details.args.length > 80 ? '…' : ''}
      </span>
    );
  }

  // User action: show path
  if (details.path) {
    return <span className="text-muted-foreground/70">{details.path}</span>;
  }

  const str = typeof details === 'string' ? details : JSON.stringify(details);
  return (
    <span className="font-mono text-2xs text-muted-foreground truncate max-w-xs inline-block align-middle" title={str}>
      {str.slice(0, 80)}{str.length > 80 ? '…' : ''}
    </span>
  );
}

const PAGE_SIZE = 40;

export default function AuditPage() {
  const [entries, setEntries] = useState<Entry[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [actorType, setActorType] = useState('');
  const [resource, setResource] = useState('');
  const [action, setAction] = useState('');
  const [search, setSearch] = useState('');
  const [searchInput, setSearchInput] = useState('');

  const load = useCallback(async () => {
    try {
      const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(page * PAGE_SIZE) });
      if (actorType) params.set('actor_type', actorType);
      if (resource)  params.set('resource', resource);
      if (action)    params.set('action', action);
      if (search)    params.set('actor_id', search); // search by agent key or user id
      const r = await request<any>(`/audit?${params}`);
      setEntries(r.entries || []);
      setTotal(r.total || 0);
    } catch {
      setEntries([]); setTotal(0);
    }
  }, [page, actorType, resource, action, search]);

  useEffect(() => { load(); }, [load]);

  function applySearch() {
    setSearch(searchInput.trim());
    setPage(0);
  }

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <PageShell
      title="Work Log"
      description="Every action tracked — agent tool calls, user changes, system events"
      contentClassName="flex flex-col overflow-hidden px-0 py-0 sm:px-0"
      toolbar={
        <>
          {/* Actor type filter */}
          <select
            value={actorType}
            onChange={e => { setActorType(e.target.value); setPage(0); }}
            className="rounded-lg border border-input bg-transparent px-3 py-1.5 text-sm"
          >
            <option value="">Anyone</option>
            <option value="user">You</option>
            <option value="agent">Agent</option>
            <option value="system">System</option>
          </select>

          {/* Resource filter */}
          <select
            value={resource}
            onChange={e => { setResource(e.target.value); setPage(0); }}
            className="rounded-lg border border-input bg-transparent px-3 py-1.5 text-sm"
          >
            <option value="">All areas</option>
            {['agents','sessions','tasks','connections','connectors','workflows','credentials','providers','tool'].map(r =>
              <option key={r} value={r}>{RESOURCE_LABELS[r] ?? r}</option>
            )}
          </select>

          {/* Action filter */}
          <select
            value={action}
            onChange={e => { setAction(e.target.value); setPage(0); }}
            className="rounded-lg border border-input bg-transparent px-3 py-1.5 text-sm"
          >
            <option value="">All activity</option>
            {['create','update','delete','tool_exec','tool_error'].map(a =>
              <option key={a} value={a}>{ACTION_META[a]?.label ?? a}</option>
            )}
          </select>

          {/* Search by actor ID / agent key */}
          <div className="flex items-center gap-1 ml-auto">
            <input
              value={searchInput}
              onChange={e => setSearchInput(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && applySearch()}
              placeholder="Filter by agent or user…"
              className="rounded-lg border border-input bg-transparent px-3 py-1.5 text-sm w-48"
            />
            <button
              onClick={applySearch}
              className="rounded-lg border border-input px-2 py-1.5 hover:bg-accent transition-colors"
            >
              <Search className="h-3.5 w-3.5 text-muted-foreground" />
            </button>
          </div>

          <span className="text-xs text-muted-foreground pl-2">{total.toLocaleString()} events</span>
        </>
      }
    >
      {/* Timeline */}
      <div className="flex-1 overflow-y-auto px-6 py-4">
        {entries.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full gap-3 text-center">
            <Filter className="h-10 w-10 text-muted-foreground/20" />
            <p className="text-sm text-muted-foreground">No events match your filters</p>
          </div>
        ) : (
          <div className="space-y-0.5">
            {entries.map((e, i) => {
              const actorMeta = ACTOR_META[e.actor_type] ?? ACTOR_META['system']!;
              const ActorIcon = actorMeta.icon;
              const showDate = i === 0 || new Date(entries[i - 1]!.created_at).toDateString() !== new Date(e.created_at).toDateString();
              return (
                <div key={e.id}>
                  {showDate && (
                    <div className="py-2 pt-4 first:pt-0">
                      <span className="text-2xs font-medium text-muted-foreground/50 uppercase tracking-wider">
                        {new Date(e.created_at).toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' })}
                      </span>
                    </div>
                  )}
                  <div className="flex items-start gap-3 py-2 rounded-lg px-2 hover:bg-muted/30 transition-colors group">
                    {/* Timeline connector */}
                    <div className="flex flex-col items-center pt-0.5 shrink-0">
                      <ActionIcon action={e.action} />
                    </div>

                    {/* Content */}
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        {/* Actor */}
                        <span className={cn('inline-flex items-center gap-1 text-2xs font-medium', actorMeta.color)}>
                          <ActorIcon className="h-2.5 w-2.5" />
                          {e.actor_name || e.actor_id}
                        </span>

                        {/* Actor type badge */}
                        <span className="rounded bg-muted px-1 py-0.5 text-2xs text-muted-foreground">
                          {actorMeta.label}
                        </span>

                        {/* Action */}
                        <span className={cn('text-xs font-semibold', ACTION_META[e.action]?.color ?? 'text-muted-foreground')}>
                          {ACTION_META[e.action]?.label ?? e.action}
                        </span>

                        {/* Resource */}
                        <span className="text-xs text-foreground">{RESOURCE_LABELS[e.resource] ?? e.resource}</span>
                        {e.resource_id && (
                          <span className="text-2xs text-muted-foreground font-mono">
                            #{e.resource_id.slice(0, 8)}
                          </span>
                        )}

                        {/* Timestamp */}
                        <span className="ml-auto text-2xs text-muted-foreground/50 shrink-0">
                          {relativeTime(e.created_at)}
                        </span>
                      </div>

                      {/* Details row */}
                      <div className="mt-0.5 text-xs">
                        <DetailsCell details={e.details} action={e.action} />
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Pagination */}
      <div className="flex items-center justify-between px-6 py-3 border-t border-border text-xs shrink-0">
        <button
          onClick={() => setPage(p => Math.max(0, p - 1))}
          disabled={page === 0}
          className="flex items-center gap-1 rounded border border-input px-2 py-1 disabled:opacity-30 cursor-pointer hover:bg-accent transition-colors"
        >
          <ChevronLeft className="h-3 w-3" />Prev
        </button>
        <span className="text-muted-foreground">Page {page + 1} of {totalPages}</span>
        <button
          onClick={() => setPage(p => p + 1)}
          disabled={(page + 1) * PAGE_SIZE >= total}
          className="flex items-center gap-1 rounded border border-input px-2 py-1 disabled:opacity-30 cursor-pointer hover:bg-accent transition-colors"
        >
          Next<ChevronRight className="h-3 w-3" />
        </button>
      </div>
    </PageShell>
  );
}
