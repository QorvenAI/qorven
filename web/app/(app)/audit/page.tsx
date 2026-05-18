'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import { Shield, ChevronLeft, ChevronRight } from 'lucide-react';
import { cn } from '@/lib/utils';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { request } from '@/lib/api-core';

type Entry = { id: number; actor_type: string; actor_id: string; actor_name: string; action: string; resource: string; resource_id: string; details: any; ip_address: string; created_at: string };

const ACTION_COLORS: Record<string, string> = { create: 'text-emerald-400', update: 'text-blue-400', delete: 'text-red-400', execute: 'text-yellow-400' };

export default function AuditPage() {
  const [entries, setEntries] = useState<Entry[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [resource, setResource] = useState('');
  const [action, setAction] = useState('');

  const load = useCallback(async () => {
    try {
      const params = new URLSearchParams({ limit: '30', offset: String(page * 30) });
      if (resource) params.set('resource', resource);
      if (action) params.set('action', action);
      const r = await request<any>(`/audit?${params}`);
      setEntries(r.entries || []); setTotal(r.total || 0);
    } catch {
      setEntries([]); setTotal(0);
    }
  }, [page, resource, action]);

  useEffect(() => { load(); }, [load]);

  // Empty state for fresh install
  // if (events.length === 0) return <EmptyState {...emptyStates.audit} />;
  return (
    <div className="p-6 max-w-6xl mx-auto space-y-4">
      <div>
        <h1 className="text-lg font-semibold flex items-center gap-2"><Shield className="h-5 w-5" />Audit Log</h1>
        <p className="text-sm text-muted-foreground">Every action tracked — who did what, when</p>
      </div>
      <div className="flex gap-2 items-center">
        <select value={resource} onChange={e => { setResource(e.target.value); setPage(0); }}
          className="rounded-lg border border-input bg-transparent px-3 py-1.5 text-sm">
          <option value="">All Resources</option>
          {['agents','sessions','tasks','connections','connectors','workflows','credentials','providers'].map(r => <option key={r} value={r}>{r}</option>)}
        </select>
        <select value={action} onChange={e => { setAction(e.target.value); setPage(0); }}
          className="rounded-lg border border-input bg-transparent px-3 py-1.5 text-sm">
          <option value="">All Actions</option>
          {['create','update','delete','execute'].map(a => <option key={a} value={a}>{a}</option>)}
        </select>
        <span className="text-xs text-muted-foreground ml-auto">{total} events</span>
      </div>
      <div className="rounded-lg border border-border overflow-hidden">
        <table className="w-full text-sm">
          <thead><tr className="border-b border-border bg-muted/30">
            <th className="text-left px-3 py-2 text-xs text-muted-foreground font-medium">Time</th>
            <th className="text-left px-3 py-2 text-xs text-muted-foreground font-medium">Actor</th>
            <th className="text-left px-3 py-2 text-xs text-muted-foreground font-medium">Action</th>
            <th className="text-left px-3 py-2 text-xs text-muted-foreground font-medium">Resource</th>
            <th className="text-left px-3 py-2 text-xs text-muted-foreground font-medium">Details</th>
          </tr></thead>
          <tbody>
            {entries.map(e => (
              <tr key={e.id} className="border-b border-border/50 hover:bg-muted/20">
                <td className="px-3 py-2 text-xs text-muted-foreground whitespace-nowrap">{new Date(e.created_at).toLocaleString()}</td>
                <td className="px-3 py-2 text-xs"><span className="rounded bg-muted px-1.5 py-0.5 text-2xs">{e.actor_type}</span> {e.actor_name || e.actor_id}</td>
                <td className={cn('px-3 py-2 text-xs font-medium', ACTION_COLORS[e.action] || '')}>{e.action}</td>
                <td className="px-3 py-2 text-xs">{e.resource} <span className="text-muted-foreground">{e.resource_id ? `#${e.resource_id.slice(0,8)}` : ''}</span></td>
                <td className="px-3 py-2 text-2xs text-muted-foreground font-mono max-w-xs truncate">{JSON.stringify(e.details)}</td>
              </tr>
            ))}
            {entries.length === 0 && <tr><td colSpan={5} className="px-3 py-8 text-center text-sm text-muted-foreground">No audit events yet</td></tr>}
          </tbody>
        </table>
      </div>
      <div className="flex items-center justify-between text-xs">
        <button onClick={() => setPage(p => Math.max(0, p-1))} disabled={page === 0} className="flex items-center gap-1 rounded border border-input px-2 py-1 disabled:opacity-30 cursor-pointer"><ChevronLeft className="h-3 w-3" />Prev</button>
        <span className="text-muted-foreground">Page {page + 1} of {Math.max(1, Math.ceil(total / 30))}</span>
        <button onClick={() => setPage(p => p+1)} disabled={(page+1)*30 >= total} className="flex items-center gap-1 rounded border border-input px-2 py-1 disabled:opacity-30 cursor-pointer">Next<ChevronRight className="h-3 w-3" /></button>
      </div>
    </div>
  );
}
