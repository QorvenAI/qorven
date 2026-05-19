'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import { Plus, Search, Building2, User, Loader2, Filter, Brain } from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { cn } from '@/lib/utils';
import { EmptyState } from '@/components/empty-state';
import { useStore } from '@/store';
import { memoryApi } from '@/lib/api';
import { request } from '@/lib/api-core';
import { toast } from 'sonner';

type Memory = {
  id: string;
  content: string;
  memory_type?: string;
  scope?: string;
  importance?: number;
  decay_exempt?: boolean;
  created_at?: string;
  relevance?: number;
};

export default function MemoriesPage() {
  const souls = useStore((s) => s.souls);
  const [tab, setTab] = useState<'company' | 'agent'>('company');

  // scopes
  const [scopes, setScopes] = useState<string[]>([]);
  useEffect(() => {
    request<{ scopes: string[] }>('/memory/scopes')
      .then((d) => setScopes(d.scopes ?? []))
      .catch(() => {});
  }, []);

  // company tab
  const [companyMems, setCompanyMems] = useState<Memory[]>([]);
  const [newContent, setNewContent] = useState('');
  const [companyLoading, setCompanyLoading] = useState(true);
  const [adding, setAdding] = useState(false);

  const loadCompany = useCallback(() => {
    setCompanyLoading(true);
    request<{ memories: Memory[] }>('/memory/company')
      .then((d) => setCompanyMems(d.memories ?? []))
      .catch(() => {})
      .finally(() => setCompanyLoading(false));
  }, []);

  useEffect(() => { loadCompany(); }, [loadCompany]);

  const addCompany = async () => {
    if (!newContent.trim()) return;
    setAdding(true);
    try {
      await request('/memory/company', {
        method: 'POST',
        body: JSON.stringify({ content: newContent, source: 'manual' }),
      });
      setNewContent('');
      toast.success('Memory added');
      loadCompany();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to add memory');
    } finally {
      setAdding(false);
    }
  };

  // agent tab
  const [agentMems, setAgentMems] = useState<Memory[]>([]);
  const [search, setSearch] = useState('');
  const [scopeFilter, setScopeFilter] = useState('');
  const [selectedAgent, setSelectedAgent] = useState('');
  const [agentLoading, setAgentLoading] = useState(false);

  useEffect(() => {
    if (!selectedAgent && souls.length > 0) setSelectedAgent(souls[0]!.id);
  }, [souls, selectedAgent]);

  const loadAgentMems = useCallback(async () => {
    if (!selectedAgent) return;
    setAgentLoading(true);
    try {
      const q = search.trim();
      if (q || scopeFilter) {
        const result = await request<{ memories: Memory[] } | Memory[]>('/memory/search', {
          method: 'POST',
          body: JSON.stringify({
            scope: scopeFilter || 'agent',
            agent_id: selectedAgent,
            query: q,
            limit: 50,
          }),
        });
        setAgentMems(Array.isArray(result) ? result : (result?.memories ?? []));
      } else {
        const result = await memoryApi.list(selectedAgent);
        setAgentMems(result);
      }
    } catch {
      setAgentMems([]);
    } finally {
      setAgentLoading(false);
    }
  }, [selectedAgent, search, scopeFilter]);

  useEffect(() => {
    if (tab === 'agent' && selectedAgent) loadAgentMems();
  }, [tab, selectedAgent, loadAgentMems]);

  return (
    <div className="space-y-6">
      <CanvasHeader title="Memory" description="Company-wide knowledge + per-agent vector memories" />

      <div className="flex gap-1 border-b border-border">
        {[
          { key: 'company' as const, label: 'Company', icon: Building2 },
          { key: 'agent'   as const, label: 'Agent',   icon: User },
        ].map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={cn(
              'flex items-center gap-1.5 px-4 py-2.5 text-xs font-medium border-b-2 -mb-px transition-colors',
              tab === t.key
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:text-foreground',
            )}
          >
            <t.icon className="h-3.5 w-3.5" />
            {t.label}
          </button>
        ))}
      </div>

      {/* ── Company tab ── */}
      {tab === 'company' && (
        <div className="space-y-4">
          <div className="flex gap-2">
            <input
              value={newContent}
              onChange={(e) => setNewContent(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && addCompany()}
              placeholder="Add company knowledge that all agents will know…"
              className="qr-input flex-1"
            />
            <button
              onClick={addCompany}
              disabled={!newContent.trim() || adding}
              className="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {adding ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
              Add
            </button>
          </div>

          {companyLoading ? (
            <div className="space-y-2">
              {[1, 2, 3].map((i) => <div key={i} className="h-16 animate-pulse rounded-xl bg-muted" />)}
            </div>
          ) : companyMems.length === 0 ? (
            <EmptyState
              icon={Brain}
              title="No company memories yet"
              description="Add knowledge that all agents should know — product context, rules, preferences."
            />
          ) : (
            <div className="space-y-2">
              {companyMems.map((m) => <MemoryCard key={m.id} memory={m} />)}
            </div>
          )}
        </div>
      )}

      {/* ── Agent tab ── */}
      {tab === 'agent' && (
        <div className="space-y-4">
          <div className="flex flex-wrap gap-2">
            <select
              value={selectedAgent}
              onChange={(e) => setSelectedAgent(e.target.value)}
              className="qr-select max-w-[200px]"
            >
              {souls.length === 0
                ? <option>No agents</option>
                : souls.map((s) => (
                    <option key={s.id} value={s.id}>{s.display_name || s.agent_key}</option>
                  ))}
            </select>

            {scopes.length > 0 && (
              <div className="relative">
                <Filter className="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
                <select
                  value={scopeFilter}
                  onChange={(e) => setScopeFilter(e.target.value)}
                  className="qr-select pl-8"
                >
                  <option value="">All scopes</option>
                  {scopes.map((s) => <option key={s} value={s}>{s}</option>)}
                </select>
              </div>
            )}

            <div className="relative flex-1 min-w-[160px]">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && loadAgentMems()}
                placeholder="Search memories…"
                className="qr-input pl-9"
              />
            </div>

            <button
              onClick={loadAgentMems}
              disabled={agentLoading}
              className="inline-flex items-center gap-2 rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent disabled:opacity-50"
            >
              {agentLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Search className="h-4 w-4" />}
              Search
            </button>
          </div>

          {agentLoading ? (
            <div className="space-y-2">
              {[1, 2, 3].map((i) => <div key={i} className="h-16 animate-pulse rounded-xl bg-muted" />)}
            </div>
          ) : agentMems.length === 0 ? (
            <EmptyState
              icon={Brain}
              title="No memories found"
              description="This agent hasn't stored any memories yet, or nothing matches your search."
            />
          ) : (
            <div className="space-y-2">
              {agentMems.map((m) => <MemoryCard key={m.id} memory={m} />)}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function MemoryCard({ memory: m }: { memory: Memory }) {
  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="flex items-center justify-between mb-2 gap-2 flex-wrap">
        <div className="flex items-center gap-2 flex-wrap">
          {m.scope && (
            <span className={cn(
              'text-xs rounded-full px-2 py-0.5',
              m.scope === 'company' ? 'bg-primary/10 text-primary'
              : m.scope === 'team' ? 'bg-emerald-500/10 text-emerald-500'
              : 'bg-muted text-muted-foreground',
            )}>
              {m.scope}
            </span>
          )}
          {m.memory_type && m.memory_type !== m.scope && (
            <span className="text-xs rounded bg-primary/10 px-1.5 py-0.5 text-primary/80">{m.memory_type}</span>
          )}
          {m.decay_exempt && (
            <span className="text-xs rounded bg-emerald-500/10 text-emerald-600 px-1.5 py-0.5">pinned</span>
          )}
          {m.importance != null && (
            <span className="text-xs text-muted-foreground">importance: {(m.importance * 100).toFixed(0)}%</span>
          )}
          {m.relevance != null && (
            <span className="text-xs text-muted-foreground">{Math.round(m.relevance * 100)}% match</span>
          )}
        </div>
        {m.created_at && (
          <span className="text-xs text-muted-foreground shrink-0">{new Date(m.created_at).toLocaleDateString()}</span>
        )}
      </div>
      <p className="text-sm whitespace-pre-wrap">{m.content}</p>
    </div>
  );
}
