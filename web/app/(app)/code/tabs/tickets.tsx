'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState, useEffect, useCallback } from 'react';
import { Plus, Loader2, ChevronDown, Filter, Trash2, UserPlus } from 'lucide-react';
import { cn } from '@/lib/utils';
import { tickets as ticketsApi } from '@/lib/api';
import { useStore } from '@/store';
import { TicketDrawer } from '@/components/code/ticket-drawer';
import type { Ticket, TicketPriority, TicketStatus } from '@/types';

const STATUS_OPTS: { value: string; label: string }[] = [
  { value: '', label: 'All statuses' },
  { value: 'todo', label: 'Todo' },
  { value: 'in_progress', label: 'In Progress' },
  { value: 'blocked', label: 'Blocked' },
  { value: 'done', label: 'Done' },
];

const PRIORITY_OPTS: { value: string; label: string }[] = [
  { value: '', label: 'All priorities' },
  { value: 'critical', label: 'Critical' },
  { value: 'high', label: 'High' },
  { value: 'normal', label: 'Normal' },
  { value: 'low', label: 'Low' },
];

const STATUS_COLOR: Record<string, string> = {
  todo: 'bg-muted text-muted-foreground',
  in_progress: 'bg-blue-500/10 text-blue-500',
  blocked: 'bg-destructive/10 text-destructive',
  done: 'bg-emerald-500/10 text-emerald-600',
};

const PRIORITY_COLOR: Record<string, string> = {
  critical: 'text-destructive font-semibold',
  high: 'text-orange-500',
  normal: 'text-muted-foreground',
  low: 'text-muted-foreground/50',
};

export function TicketsTab() {
  const souls = useStore(s => s.souls);
  const [items, setItems] = useState<Ticket[]>([]);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState('');
  const [priorityFilter, setPriorityFilter] = useState('');
  const [selected, setSelected] = useState<Ticket | null>(null);
  const [showCreate, setShowCreate] = useState(false);

  // Create form
  const [createTitle, setCreateTitle] = useState('');
  const [createDesc, setCreateDesc] = useState('');
  const [createPriority, setCreatePriority] = useState<TicketPriority>('normal');
  const [creating, setCreating] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, string> = {};
      if (statusFilter) params.status = statusFilter;
      if (priorityFilter) params.priority = priorityFilter;
      const data = await ticketsApi.list(params);
      setItems(data);
    } finally {
      setLoading(false);
    }
  }, [statusFilter, priorityFilter]);

  useEffect(() => { load(); }, [load]);

  useEffect(() => {
    const handler = () => load();
    window.addEventListener('qorven:ticket_updated', handler);
    return () => window.removeEventListener('qorven:ticket_updated', handler);
  }, [load]);

  const createTicket = async () => {
    if (!createTitle.trim()) return;
    setCreating(true);
    try {
      await ticketsApi.create({ title: createTitle.trim(), description: createDesc.trim() || undefined, priority: createPriority });
      setCreateTitle('');
      setCreateDesc('');
      setCreatePriority('normal');
      setShowCreate(false);
      await load();
    } finally {
      setCreating(false);
    }
  };

  const assign = async (ticket: Ticket, agentId: string) => {
    try {
      await ticketsApi.assign(ticket.id, agentId);
      await load();
    } catch { /* UI keeps current state; user can retry */ }
  };

  const deleteTicket = async (id: string) => {
    try {
      await ticketsApi.delete(id);
      setItems(prev => prev.filter(t => t.id !== id));
    } catch { /* server rejected — keep item visible */ }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="flex shrink-0 items-center gap-2 border-b border-border px-4 py-2.5">
        <Filter className="h-4 w-4 text-muted-foreground shrink-0" />
        <select
          value={statusFilter}
          onChange={e => setStatusFilter(e.target.value)}
          className="bg-transparent text-sm text-muted-foreground outline-none cursor-pointer"
        >
          {STATUS_OPTS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
        <select
          value={priorityFilter}
          onChange={e => setPriorityFilter(e.target.value)}
          className="bg-transparent text-sm text-muted-foreground outline-none cursor-pointer"
        >
          {PRIORITY_OPTS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
        <div className="ml-auto">
          <button
            onClick={() => setShowCreate(!showCreate)}
            className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-semibold text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            <Plus className="h-3.5 w-3.5" />
            New ticket
          </button>
        </div>
      </div>

      {/* Create form */}
      {showCreate && (
        <div className="shrink-0 border-b border-border bg-muted/20 px-4 py-3 space-y-2">
          <input
            autoFocus
            value={createTitle}
            onChange={e => setCreateTitle(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) createTicket(); if (e.key === 'Escape') setShowCreate(false); }}
            placeholder="Ticket title…"
            className="qr-input" />
          <textarea
            value={createDesc}
            onChange={e => setCreateDesc(e.target.value)}
            placeholder="Description (optional)…"
            rows={2}
            className="qr-textarea resize-none" />
          <div className="flex items-center gap-2">
            <select
              value={createPriority}
              onChange={e => setCreatePriority(e.target.value as TicketPriority)}
              className="flex-1 rounded-lg border border-border bg-background px-2 py-1.5 text-xs outline-none"
            >
              {PRIORITY_OPTS.filter(p => p.value).map(p => <option key={p.value} value={p.value}>{p.label}</option>)}
            </select>
            <button onClick={createTicket} disabled={!createTitle.trim() || creating}
              className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-semibold text-primary-foreground hover:bg-primary/90 disabled:opacity-50 transition-colors">
              {creating ? <Loader2 className="h-3 w-3 animate-spin" /> : <Plus className="h-3 w-3" />}
              Create
            </button>
            <button onClick={() => setShowCreate(false)} className="text-xs text-muted-foreground hover:text-foreground">
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Table */}
      <div className="flex-1 overflow-auto">
        {loading ? (
          <div className="flex items-center justify-center py-20">
            <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
          </div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 gap-2 text-center">
            <p className="text-sm text-muted-foreground">No tickets</p>
            <p className="text-xs text-muted-foreground/60">Create your first ticket to get started</p>
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <th className="px-4 py-2.5 text-left w-20">ID</th>
                <th className="px-4 py-2.5 text-left">Title</th>
                <th className="px-4 py-2.5 text-left w-28">Status</th>
                <th className="px-4 py-2.5 text-left w-24">Priority</th>
                <th className="px-4 py-2.5 text-left w-36">Assigned</th>
                <th className="w-8" />
              </tr>
            </thead>
            <tbody>
              {items.map(t => (
                <tr
                  key={t.id}
                  className="border-b border-border/50 hover:bg-accent/30 transition-colors cursor-pointer group"
                  onClick={() => setSelected(t)}
                >
                  <td className="px-4 py-2.5 font-mono text-xs text-muted-foreground">{t.slug}</td>
                  <td className="px-4 py-2.5 max-w-xs truncate">{t.title}</td>
                  <td className="px-4 py-2.5">
                    <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium capitalize', STATUS_COLOR[t.status])}>
                      {t.status.replace('_', ' ')}
                    </span>
                  </td>
                  <td className={cn('px-4 py-2.5 text-xs capitalize', PRIORITY_COLOR[t.priority])}>{t.priority}</td>
                  <td className="px-4 py-2.5" onClick={e => e.stopPropagation()}>
                    <AssignCell ticket={t} souls={souls} onAssign={assign} />
                  </td>
                  <td className="pr-2" onClick={e => e.stopPropagation()}>
                    <button
                      onClick={() => deleteTicket(t.id)}
                      className="opacity-0 group-hover:opacity-100 p-1 rounded text-muted-foreground/50 hover:text-destructive transition-all"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Drawer */}
      {selected && (
        <>
          <div className="fixed inset-0 z-30 bg-black/20" onClick={() => setSelected(null)} />
          <TicketDrawer ticket={selected} onClose={() => setSelected(null)} />
        </>
      )}
    </div>
  );
}

function AssignCell({ ticket, souls, onAssign }: {
  ticket: Ticket;
  souls: import('@/types').Soul[];
  onAssign: (t: Ticket, agentId: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const assigned = souls.find(s => s.id === ticket.assigned_agent_id);

  return (
    <div className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1.5 text-xs rounded px-1.5 py-0.5 hover:bg-accent transition-colors"
      >
        {assigned
          ? <><span className="truncate max-w-[100px]">{assigned.display_name}</span><ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" /></>
          : <><UserPlus className="h-3.5 w-3.5 text-muted-foreground/50" /><span className="text-muted-foreground/50">Assign</span></>}
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-20" onClick={() => setOpen(false)} />
          <div className="absolute top-full left-0 z-30 mt-1 w-48 rounded-xl border border-border bg-popover shadow-xl overflow-hidden">
            {souls.map(s => (
              <button
                key={s.id}
                onClick={() => { onAssign(ticket, s.id); setOpen(false); }}
                className={cn('flex w-full items-center gap-2 px-3 py-2 text-xs hover:bg-accent transition-colors text-left',
                  s.id === ticket.assigned_agent_id && 'bg-primary/10 text-primary')}
              >
                <span className="truncate">{s.display_name}</span>
                {s.id === ticket.assigned_agent_id && <span className="ml-auto text-xs text-primary">✓</span>}
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
