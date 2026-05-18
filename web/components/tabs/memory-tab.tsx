'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { cn } from '@/lib/utils';
import { Search, Plus, Pencil, Trash2 } from 'lucide-react';

interface Props { agentId: string }

const categories = ['All', 'Facts', 'Preferences', 'Context', 'System'];

export function MemoryTab({ agentId }: Props) {
  const [memories, setMemories] = useState<any[]>([]);
  const [search, setSearch] = useState('');
  const [activeFilter, setActiveFilter] = useState('All');
  const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

  useEffect(() => {
    fetch(`/api/v1/agents/${agentId}/messages`, { headers: { Authorization: `Bearer ${getToken()}` } })
      .then((r) => r.json())
      .then((d) => { const arr = Array.isArray(d) ? d : Object.values(d).find(Array.isArray) ?? []; setMemories(arr); })
      .catch(() => {});
  }, [agentId, getToken()]);

  const filtered = memories.filter((m) => {
    const content = m.content || JSON.stringify(m);
    if (search && !content.toLowerCase().includes(search.toLowerCase())) return false;
    if (activeFilter !== 'All' && m.type && !m.type.toLowerCase().includes(activeFilter.toLowerCase())) return false;
    return true;
  });

  return (
    <div className="max-w-3xl mx-auto">
      {/* Search + Add */}
      <div className="flex items-center gap-2 mb-3">
        <div className="flex-1 flex items-center gap-2 rounded-lg border border-border bg-muted/60 px-3 py-1.5">
          <Search className="h-3.5 w-3.5 text-muted-foreground" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search memories…"
            className="flex-1 bg-transparent text-xs outline-none placeholder:text-muted-foreground" />
        </div>
        <button className="flex items-center gap-1 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90">
          <Plus className="h-3.5 w-3.5" /> Add
        </button>
      </div>

      {/* Filter chips */}
      <div className="flex gap-1.5 mb-4">
        {categories.map((cat) => (
          <button key={cat} onClick={() => setActiveFilter(cat)}
            className={cn('rounded-full px-3 py-1 text-2xs font-medium transition-colors',
              activeFilter === cat ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:text-foreground')}>
            {cat}
          </button>
        ))}
      </div>

      {/* Memory cards grid */}
      {filtered.length === 0 ? (
        <p className="py-12 text-center text-sm text-muted-foreground">{search ? 'No matches' : 'No memories stored yet'}</p>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {filtered.slice(0, 30).map((m, i) => (
            <div key={i} className="rounded-xl border border-border bg-card p-3 group">
              <div className="flex items-start justify-between gap-2">
                <span className="rounded-md bg-pink-400/10 text-pink-400 px-1.5 py-0.5 text-2xs font-medium uppercase">
                  {m.type || 'memory'}
                </span>
                <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                  <button className="h-5 w-5 flex items-center justify-center rounded text-muted-foreground hover:text-foreground"><Pencil className="h-3 w-3" /></button>
                  <button className="h-5 w-5 flex items-center justify-center rounded text-muted-foreground hover:text-destructive"><Trash2 className="h-3 w-3" /></button>
                </div>
              </div>
              <p className="mt-2 text-xs leading-relaxed">{m.content?.slice(0, 200) || JSON.stringify(m).slice(0, 200)}</p>
              <p className="mt-2 text-2xs text-muted-foreground">{m.created_at ? new Date(m.created_at).toLocaleDateString() : ''}</p>
            </div>
          ))}
        </div>
      )}

      {/* Footer */}
      <div className="mt-4 text-2xs text-muted-foreground text-center">
        {filtered.length} memories • Sharing: private
      </div>
    </div>
  );
}
