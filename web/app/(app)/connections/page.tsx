'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState, useCallback } from 'react';
import { Plug, Unplug, ExternalLink, Check, Search, Key, Shield, ChevronDown, ChevronUp } from 'lucide-react';
import { cn } from '@/lib/utils';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { request, BASE } from '@/lib/api-core';

type Platform = { id: string; name: string; category: string; description: string; icon: string; auth_type: string; docs_url: string; connected: boolean };
type Action = { action_key: string; name: string; description: string; when_to_use: string; method: string; path: string };
type Connection = { id: string; platform_id: string; label: string; auth_type: string; scopes: string[]; expires_at: string | null };

const CATEGORIES = ['all', 'email', 'productivity', 'messaging', 'crm', 'development', 'ecommerce', 'payments', 'social', 'content', 'storage'];

export default function ConnectionsPage() {
  const [platforms, setPlatforms] = useState<Platform[]>([]);
  const [connections, setConnections] = useState<Connection[]>([]);
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('all');
  const [expanded, setExpanded] = useState<string | null>(null);
  const [actions, setActions] = useState<Record<string, Action[]>>({});
  const [connectForm, setConnectForm] = useState<{ id: string; key: string } | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [p, c] = await Promise.all([request<any>('/connectors/platforms'), request<any>('/connections')]);
      setPlatforms(p.platforms || []);
      setConnections(c.connections || []);
    } catch {
      setError('Failed to load connections');
    }
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  const loadActions = async (platformId: string) => {
    if (actions[platformId]) return;
    const r = await request<any>(`/connectors/platforms/${platformId}/actions`);
    setActions(prev => ({ ...prev, [platformId]: r.actions || [] }));
  };

  const toggleExpand = (id: string) => {
    if (expanded === id) { setExpanded(null); return; }
    setExpanded(id);
    loadActions(id);
  };

  const connect = async (platformId: string, key: string) => {
    try {
      await request<any>(`/connections/${platformId}`, { method: 'POST', body: JSON.stringify({ api_key: key, token: key }) });
      setConnectForm(null);
      refresh();
    } catch {
      setError('Failed to connect');
    }
  };

  const disconnect = async (platformId: string) => {
    try {
      await request<any>(`/connections/${platformId}`, { method: 'DELETE' });
      refresh();
    } catch {
      setError('Failed to disconnect');
    }
  };

  const startOAuth = (provider: string) => {
    window.location.href = `${BASE}/oauth/${provider}/authorize?state=default`;
  };

  if (platforms.length === 0 && connections.length === 0 && !search) return (
    <div className="p-6"><EmptyState {...emptyStates.connections} /></div>
  );

  const connectedIds = new Set(connections.map(c => c.platform_id));
  const filtered = platforms.filter(p => {
    if (search && !p.name.toLowerCase().includes(search.toLowerCase()) && !p.description.toLowerCase().includes(search.toLowerCase())) return false;
    if (category !== 'all' && p.category !== category) return false;
    return true;
  });

  const connectedPlatforms = filtered.filter(p => p.connected || connectedIds.has(p.id));
  const availablePlatforms = filtered.filter(p => !p.connected && !connectedIds.has(p.id));

  return (
    <div className="p-6 max-w-5xl mx-auto space-y-6">
      {error && (
        <div className="rounded-lg bg-destructive/10 px-4 py-2 text-sm text-destructive flex items-center justify-between">
          <span>{error}</span>
          <button onClick={() => setError(null)} className="ml-4 text-destructive/60 hover:text-destructive">✕</button>
        </div>
      )}
      <div>
        <h1 className="text-lg font-semibold">Links</h1>
        <p className="text-sm text-muted-foreground">Link external services. Your agents will use these to interact with apps on your behalf.</p>
      </div>

      <div className="flex gap-3 items-center">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <input placeholder="Search platforms..." value={search} onChange={e => setSearch(e.target.value)}
            className="w-full rounded-lg border border-input bg-transparent pl-9 pr-3 py-2 text-sm" />
        </div>
        <div className="flex gap-1 overflow-x-auto">
          {CATEGORIES.map(c => (
            <button key={c} onClick={() => setCategory(c)}
              className={cn('px-3 py-1.5 rounded-full text-xs whitespace-nowrap transition-colors cursor-pointer',
                category === c ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:text-foreground')}>
              {c === 'all' ? 'All' : c.charAt(0).toUpperCase() + c.slice(1)}
            </button>
          ))}
        </div>
      </div>

      {connectedPlatforms.length > 0 && (
        <div>
          <h2 className="text-sm font-medium mb-2 flex items-center gap-2"><Check className="h-4 w-4 text-emerald-500" />Connected ({connectedPlatforms.length})</h2>
          <div className="space-y-2">
            {connectedPlatforms.map(p => (
              <PlatformCard key={p.id} platform={p} connected actions={actions[p.id]}
                expanded={expanded === p.id} onToggle={() => toggleExpand(p.id)}
                onDisconnect={() => disconnect(p.id)} />
            ))}
          </div>
        </div>
      )}

      <div>
        <h2 className="text-sm font-medium mb-2">Available ({availablePlatforms.length})</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
          {availablePlatforms.map(p => (
            <div key={p.id} className="rounded-lg border border-border bg-card p-4 flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-muted text-xs font-semibold text-muted-foreground">
                  {p.name.charAt(0)}
                </div>
                <div>
                  <p className="text-sm font-medium">{p.name}</p>
                  <p className="text-2xs text-muted-foreground">{p.category} &bull; {p.auth_type}</p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                {p.docs_url && (
                  <a href={p.docs_url} target="_blank" rel="noopener noreferrer" className="text-muted-foreground hover:text-foreground">
                    <ExternalLink className="h-3.5 w-3.5" />
                  </a>
                )}
                {p.auth_type === 'oauth2' ? (
                  <button onClick={() => { const cfg = JSON.parse(p.auth_type === 'oauth2' ? '{}' : '{}'); startOAuth(cfg.provider || p.id); }}
                    className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 cursor-pointer">
                    <Shield className="h-3 w-3" />Connect
                  </button>
                ) : connectForm?.id === p.id ? (
                  <div className="flex gap-1">
                    <input placeholder={p.auth_type === 'api_key' ? 'API Key' : 'Token'} value={connectForm.key}
                      onChange={e => setConnectForm({ id: p.id, key: e.target.value })}
                      className="w-40 rounded border border-input bg-transparent px-2 py-1 text-xs font-mono" />
                    <button onClick={() => connect(p.id, connectForm.key)}
                      className="rounded bg-primary px-2 py-1 text-xs text-primary-foreground cursor-pointer">Save</button>
                  </div>
                ) : (
                  <button onClick={() => setConnectForm({ id: p.id, key: '' })}
                    className="inline-flex items-center gap-1.5 rounded-lg border border-input px-3 py-1.5 text-xs font-medium hover:bg-accent cursor-pointer">
                    <Key className="h-3 w-3" />Connect
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function PlatformCard({ platform, connected, actions, expanded, onToggle, onDisconnect }: {
  platform: Platform; connected: boolean; actions?: Action[]; expanded: boolean;
  onToggle: () => void; onDisconnect: () => void;
}) {
  return (
    <div className={cn('rounded-lg border bg-card overflow-hidden', connected ? 'border-emerald-500/30' : 'border-border')}>
      <div className="flex items-center justify-between p-4 cursor-pointer" onClick={onToggle}>
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-emerald-500/10 text-xs font-semibold text-emerald-500">
            {platform.name.charAt(0)}
          </div>
          <div>
            <p className="text-sm font-medium">{platform.name}</p>
            <p className="text-2xs text-muted-foreground">{platform.category} &bull; {actions?.length || '...'} actions</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-2xs text-emerald-500">
            <Check className="h-3 w-3" />Connected
          </span>
          <button onClick={e => { e.stopPropagation(); onDisconnect(); }}
            className="text-muted-foreground hover:text-red-400 cursor-pointer"><Unplug className="h-3.5 w-3.5" /></button>
          {expanded ? <ChevronUp className="h-4 w-4 text-muted-foreground" /> : <ChevronDown className="h-4 w-4 text-muted-foreground" />}
        </div>
      </div>
      {expanded && actions && (
        <div className="border-t border-border px-4 py-3 bg-muted/30">
          <p className="text-2xs text-muted-foreground mb-2 font-medium uppercase tracking-wider">Available Actions</p>
          <div className="space-y-1.5">
            {actions.map(a => (
              <div key={a.action_key} className="flex items-start gap-2">
                <span className="rounded bg-muted px-1.5 py-0.5 text-2xs font-mono text-foreground shrink-0">{a.method}</span>
                <div>
                  <p className="text-xs font-medium">{a.name} <span className="text-muted-foreground font-mono">({a.action_key})</span></p>
                  <p className="text-2xs text-muted-foreground">{a.description}</p>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
