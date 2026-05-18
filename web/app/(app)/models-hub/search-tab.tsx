'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { Loader2, Globe, CheckCircle2, Check, Key, AlertTriangle, ArrowRight } from 'lucide-react';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';
import { useServiceEnabled } from '@/hooks/use-service-enabled';
import { useRouter } from 'next/navigation';

const SEARCH_PROVIDERS_CATALOG = [
  { id: 'brave',      name: 'Brave Search',   desc: 'Independent privacy-first web index',   docsUrl: 'https://api.search.brave.com' },
  { id: 'tavily',     name: 'Tavily',         desc: 'AI-native search with structured output', docsUrl: 'https://tavily.com' },
  { id: 'exa',        name: 'Exa',            desc: 'Neural search with full content retrieval', docsUrl: 'https://exa.ai' },
  { id: 'serper',     name: 'Serper (Google)', desc: 'Google Search results via Serper API', docsUrl: 'https://serper.dev' },
  { id: 'searxng',    name: 'SearXNG',        desc: 'Self-hosted meta-search engine',         docsUrl: '' },
  { id: 'perplexity', name: 'Perplexity',     desc: 'AI-powered search with citations',       docsUrl: 'https://perplexity.ai' },
  { id: 'kagi',       name: 'Kagi',           desc: 'Premium ad-free search',                  docsUrl: 'https://kagi.com' },
  { id: 'jina',       name: 'Jina Reader',    desc: 'Web reader & neural search',             docsUrl: 'https://jina.ai' },
];

const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

export function SearchProvidersTab() {
  const { enabled: searchEnabled, loading: searchLoading } = useServiceEnabled('services.web_search');
  const router = useRouter();
  const [keys, setKeys] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const [form, setForm] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetch('/api/v1/system/search-providers', {
      headers: { Authorization: `Bearer ${getToken()}` },
    }).then(r => r.json()).then(d => {
      if (d && typeof d === 'object') setKeys(d);
    }).catch(() => {}).finally(() => setLoading(false));
  }, []);

  const saveKey = async (id: string) => {
    if (!form[id]?.trim()) return;
    setSaving(id);
    try {
      await fetch('/api/v1/system/search-providers', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${getToken()}`,
        },
        body: JSON.stringify({ provider: id, api_key: form[id] }),
      });
      setKeys(prev => ({ ...prev, [id]: '••••••' }));
      setForm(prev => ({ ...prev, [id]: '' }));
      setSaved(id);
      toast.success(`${SEARCH_PROVIDERS_CATALOG.find(p => p.id === id)?.name} API key saved`);
      setTimeout(() => setSaved(null), 2000);
    } catch { toast.error('Could not save changes. Please try again.'); }
    finally { setSaving(null); }
  };

  return (
    <div className="space-y-5">
      {/* Disabled banner */}
      {!searchLoading && !searchEnabled && (
        <div className="flex items-start gap-3 rounded-xl border border-amber-400/30 bg-amber-400/5 px-4 py-3.5">
          <AlertTriangle className="h-4 w-4 text-amber-400 shrink-0 mt-0.5" />
          <div className="flex-1 min-w-0">
            <p className="text-sm font-medium text-amber-400">Web Search is disabled</p>
            <p className="text-xs text-muted-foreground mt-0.5">Providers are shown but inactive. Enable Web Search in Settings → Services to use them.</p>
          </div>
          <button onClick={() => router.push('/settings')}
            className="flex items-center gap-1 text-xs text-amber-400 hover:underline shrink-0">
            Settings <ArrowRight className="h-3 w-3" />
          </button>
        </div>
      )}

      <div className={cn('rounded-xl border border-border bg-card overflow-hidden', !searchEnabled && !searchLoading && 'opacity-60')}>
        <div className="px-5 py-4 border-b border-border/70 bg-muted/20">
          <h3 className="text-sm font-semibold">Search Providers</h3>
          <p className="text-xs text-muted-foreground mt-0.5">Configure API keys for web search grounding. Qors will auto-select the best available provider.</p>
        </div>
        <div className="px-5 py-4">
          {loading ? (
            <div className="space-y-3">{Array.from({ length: 4 }).map((_, i) => <div key={i} className="h-16 rounded-lg bg-muted animate-pulse" />)}</div>
          ) : (
            <div className="space-y-3">
              {SEARCH_PROVIDERS_CATALOG.map(provider => {
                const hasKey = !!keys[provider.id];
                return (
                  <div key={provider.id} className="rounded-xl border border-border px-4 py-3">
                    <div className="flex items-center justify-between mb-2.5">
                      <div className="flex items-center gap-3">
                        <div className={cn('flex h-8 w-8 items-center justify-center rounded-md shrink-0',
                          hasKey ? 'bg-emerald-500/10 text-emerald-400' : 'bg-muted text-muted-foreground')}>
                          <Globe className="h-4 w-4" />
                        </div>
                        <div>
                          <p className="text-sm font-medium">{provider.name}</p>
                          <p className="text-2xs text-muted-foreground">{provider.desc}</p>
                        </div>
                      </div>
                      {hasKey && (
                        <span className="flex items-center gap-1 text-2xs text-emerald-400">
                          <CheckCircle2 className="h-3 w-3" /> Configured
                        </span>
                      )}
                    </div>
                    <div className="flex gap-2">
                      <input
                        type="password"
                        value={form[provider.id] ?? ''}
                        onChange={e => setForm(prev => ({ ...prev, [provider.id]: e.target.value }))}
                        placeholder={hasKey ? 'Enter new key to replace…' : 'Paste API key…'}
                        className="flex-1 qr-input text-xs"
                        onKeyDown={e => e.key === 'Enter' && saveKey(provider.id)}
                      />
                      <button
                        onClick={() => saveKey(provider.id)}
                        disabled={!form[provider.id]?.trim() || saving === provider.id}
                        className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40"
                      >
                        {saving === provider.id
                          ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          : saved === provider.id
                          ? <Check className="h-3.5 w-3.5" />
                          : <Key className="h-3.5 w-3.5" />}
                        Save
                      </button>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
