'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { ExternalLink, Check, AlertTriangle, Trash2, Plus, Key, Search, Puzzle, Loader2, ChevronDown } from 'lucide-react';
import { toast } from 'sonner';
import { Card, Btn, Input } from './primitives';
import { integrationsApi, RelayKeyRecord, CatalogEntry } from '@/lib/api-integrations';
import { cn } from '@/lib/utils';

const RELAY_PROVIDERS = [
  { id: 'outstand', name: 'Outstand', category: 'social', description: 'Unified social API — handles OAuth, token refresh, rate limits', pricing: '$5/mo (1000 posts)', keyPrefix: 'sk_', keyHint: 'Get key from Outstand dashboard → Settings → API Keys', docsUrl: 'https://www.outstand.so/docs/getting-started' },
  { id: 'postforme', name: 'PostForMe', category: 'social', description: 'Social posting API — white-label OAuth flows', pricing: '$10/mo (1000 posts)', keyPrefix: '', keyHint: 'Get key from app.postforme.dev → API Keys', docsUrl: 'https://api.postforme.dev/docs' },
  { id: 'buffer', name: 'Buffer', category: 'social', description: 'Schedule and publish via Buffer — connect accounts in Buffer dashboard', pricing: 'Free (3 channels) or $5/channel/mo', keyPrefix: '', keyHint: 'Get token from publish.buffer.com → Settings → API', docsUrl: 'https://developers.buffer.com' },
  { id: 'pipedream', name: 'Pipedream', category: 'work', description: 'Work tools — Gmail, Calendar, Slack, Notion, CRM', pricing: 'Free (100 actions/mo)', keyPrefix: 'pd_', keyHint: 'Get key from pipedream.com → Settings → API Keys', docsUrl: 'https://pipedream.com/docs' },
];

function StatusBadge({ status }: { status: string }) {
  const isActive = status === 'active';
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${
      isActive ? 'bg-green-500/10 text-green-600' : 'bg-red-500/10 text-red-600'
    }`}>
      {isActive ? <Check className="h-3 w-3" /> : <AlertTriangle className="h-3 w-3" />}
      {isActive ? 'Active' : 'Error'}
    </span>
  );
}

function ProviderCard({
  provider,
  keys,
  onAdd,
  onDelete,
}: {
  provider: typeof RELAY_PROVIDERS[number];
  keys: RelayKeyRecord[];
  onAdd: (label: string, apiKey: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
}) {
  const [showForm, setShowForm] = useState(false);
  const [label, setLabel] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    if (!label.trim() || !apiKey.trim()) return;
    setSaving(true);
    try {
      await onAdd(label.trim(), apiKey.trim());
      setLabel('');
      setApiKey('');
      setShowForm(false);
    } catch (e: any) {
      toast.error(e?.message || 'Failed to save key');
    }
    setSaving(false);
  };

  const handleDelete = async (id: string, keyLabel: string) => {
    if (!window.confirm(`Delete key "${keyLabel}"? This cannot be undone.`)) return;
    await onDelete(id);
  };

  return (
    <Card
      id={`relay-${provider.id}`}
      title={provider.name}
      description={`${provider.description} — ${provider.pricing}`}
      headerRight={
        <div className="flex items-center gap-2">
          <a href={provider.docsUrl} target="_blank" rel="noopener noreferrer"
            className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-primary transition-colors">
            Docs <ExternalLink className="h-3 w-3" />
          </a>
          {!showForm && (
            <Btn variant="ghost" onClick={() => setShowForm(true)}>
              <Plus className="h-3.5 w-3.5" /> Add Key
            </Btn>
          )}
        </div>
      }
    >
      {keys.length > 0 && (
        <div className="space-y-2">
          {keys.map(k => (
            <div key={k.id} className="flex items-center justify-between rounded-lg border border-border px-3 py-2">
              <div className="flex items-center gap-3">
                <Key className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="text-sm font-medium">{k.label}</span>
                <StatusBadge status={k.status} />
                {k.accounts_count > 0 && (
                  <span className="text-xs text-muted-foreground">
                    {k.accounts_count} account{k.accounts_count !== 1 ? 's' : ''}
                  </span>
                )}
              </div>
              <button
                onClick={() => handleDelete(k.id, k.label)}
                className="text-muted-foreground hover:text-destructive transition-colors"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>
      )}

      {keys.length === 0 && !showForm && (
        <p className="text-sm text-muted-foreground">No keys configured. Add one to get started.</p>
      )}

      {showForm && (
        <div className="space-y-3 rounded-lg border border-border p-4 bg-muted/30">
          <p className="text-xs text-muted-foreground">{provider.keyHint}</p>
          <div className="flex flex-col sm:flex-row gap-2">
            <Input
              placeholder="Label (e.g. Production)"
              value={label}
              onChange={setLabel}
            />
            <Input
              type="password"
              placeholder={provider.keyPrefix ? `${provider.keyPrefix}...` : 'API key'}
              value={apiKey}
              onChange={setApiKey}
            />
          </div>
          <div className="flex gap-2">
            <Btn variant="primary" loading={saving} disabled={!label.trim() || !apiKey.trim()} onClick={handleSave}>
              Save
            </Btn>
            <Btn variant="ghost" onClick={() => { setShowForm(false); setLabel(''); setApiKey(''); }}>
              Cancel
            </Btn>
          </div>
        </div>
      )}
    </Card>
  );
}

function CatalogBrowser() {
  const [expanded, setExpanded] = useState(false);
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<CatalogEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [available, setAvailable] = useState<boolean | null>(null);
  const [activatingSlug, setActivatingSlug] = useState<string | null>(null);
  const [connectedPlatforms, setConnectedPlatforms] = useState<Set<string>>(new Set());
  const [activatedSlugs, setActivatedSlugs] = useState<Set<string>>(new Set());
  const [connecting, setConnecting] = useState<string | null>(null);
  const [discovering, setDiscovering] = useState<string | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const loadConnected = useCallback(() => {
    integrationsApi.listConnectedAccounts()
      .then(accounts => setConnectedPlatforms(new Set(accounts.map(a => a.platform_id))))
      .catch(() => {});
  }, []);

  useEffect(() => { loadConnected(); }, [loadConnected]);

  const fetchCatalog = useCallback(async (q: string) => {
    setLoading(true);
    try {
      const data = await integrationsApi.searchCatalog(q, 50);
      setResults(data.results || []);
      setTotal(data.total || 0);
      setAvailable(true);
    } catch (e: any) {
      if (e?.status === 503 || e?.message?.includes('503')) {
        setAvailable(false);
      } else {
        setAvailable(true);
        setResults([]);
        setTotal(0);
      }
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    if (expanded && available === null) {
      fetchCatalog('');
    }
  }, [expanded, available, fetchCatalog]);

  const handleSearch = (value: string) => {
    setQuery(value);
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      fetchCatalog(value);
    }, 300);
  };

  const handleActivate = async (entry: CatalogEntry) => {
    setActivatingSlug(entry.slug);
    try {
      await integrationsApi.activateCatalog(entry.slug, entry.name, entry.categories);
      toast.success(`${entry.name} activated`);
      // Mark as installed locally
      setResults(prev => prev.map(r => r.slug === entry.slug ? { ...r, installed: true } : r));
      setActivatedSlugs(prev => new Set(prev).add(entry.slug));
    } catch (e: any) {
      toast.error(e?.message || `Failed to activate ${entry.name}`);
    }
    setActivatingSlug(null);
  };

  const connectPlatform = async (slug: string) => {
    setConnecting(slug);
    try {
      const res = await integrationsApi.connectPlatformOAuth(slug);
      const popup = window.open(res.connect_link_url, 'pipedream_connect', 'width=600,height=700');
      const poll = setInterval(() => {
        if (popup?.closed) {
          clearInterval(poll);
          loadConnected();
          setConnecting(null);
          toast.success(`${slug} account connected`);
        }
      }, 1000);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to start connect');
      setConnecting(null);
    }
  };

  const discoverActions = async (slug: string) => {
    setDiscovering(slug);
    try {
      const res = await integrationsApi.discoverActions(slug);
      toast.success(`Discovered ${res.actions_stored} actions for ${slug}`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Discovery failed');
    } finally {
      setDiscovering(null);
    }
  };

  return (
    <div className="rounded-lg border border-border bg-card overflow-hidden">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between px-4 py-3 hover:bg-muted/50 transition-colors"
      >
        <div className="flex items-center gap-2">
          <Puzzle className="h-4 w-4 text-muted-foreground" />
          <span className="text-sm font-semibold text-foreground">Pipedream Integration Catalog</span>
        </div>
        <ChevronDown className={cn('h-4 w-4 text-muted-foreground transition-transform', expanded && 'rotate-180')} />
      </button>

      {expanded && (
        <div className="border-t border-border px-4 py-4 space-y-4">
          {available === false ? (
            <p className="text-sm text-muted-foreground">
              Add a Pipedream API key above to browse 2,400+ integrations.
            </p>
          ) : (
            <>
              <div className="relative">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                <input
                  type="text"
                  placeholder="Search integrations..."
                  value={query}
                  onChange={(e) => handleSearch(e.target.value)}
                  className="w-full pl-9 pr-3 py-2 text-sm rounded-md border border-border bg-background text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
                />
              </div>

              {loading ? (
                <div className="flex items-center justify-center py-8">
                  <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                </div>
              ) : (
                <>
                  <p className="text-xs text-muted-foreground">
                    Showing {results.length} of {total.toLocaleString()} available integrations
                  </p>

                  <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                    {results.map(entry => {
                      const isInstalled = entry.installed || activatedSlugs.has(entry.slug);
                      const isConnected = connectedPlatforms.has(entry.slug);

                      return (
                        <div
                          key={entry.id}
                          className="flex flex-col items-center gap-2 rounded-lg border border-border p-3 bg-background"
                        >
                          {entry.img_src ? (
                            <img
                              src={entry.img_src}
                              alt={entry.name}
                              className="h-8 w-8 rounded object-contain"
                            />
                          ) : (
                            <Puzzle className="h-8 w-8 text-muted-foreground" />
                          )}
                          <span className="text-sm font-medium text-foreground text-center leading-tight">
                            {entry.name}
                          </span>
                          {entry.categories.length > 0 && (
                            <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
                              {entry.categories[0]}
                            </span>
                          )}
                          {isConnected ? (
                            <div className="flex items-center gap-1.5">
                              <span className="text-xs text-emerald-600 flex items-center gap-1">
                                <Check className="h-3 w-3" /> Connected
                              </span>
                              <button
                                onClick={() => discoverActions(entry.slug)}
                                disabled={discovering === entry.slug}
                                className="text-xs px-2 py-0.5 rounded border border-border hover:bg-accent transition-colors cursor-pointer disabled:opacity-50"
                              >
                                {discovering === entry.slug ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Discover Actions'}
                              </button>
                            </div>
                          ) : isInstalled ? (
                            <button
                              onClick={() => connectPlatform(entry.slug)}
                              disabled={connecting === entry.slug}
                              className="text-xs px-2.5 py-1 rounded bg-emerald-600 text-white hover:bg-emerald-700 disabled:opacity-50 cursor-pointer flex items-center gap-1"
                            >
                              {connecting === entry.slug ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Connect'}
                            </button>
                          ) : (
                            <button
                              onClick={() => handleActivate(entry)}
                              disabled={activatingSlug === entry.slug}
                              className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium rounded-md bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50 transition-colors"
                            >
                              {activatingSlug === entry.slug ? (
                                <Loader2 className="h-3 w-3 animate-spin" />
                              ) : (
                                'Activate'
                              )}
                            </button>
                          )}
                        </div>
                      );
                    })}
                  </div>

                  {results.length === 0 && !loading && (
                    <p className="text-sm text-muted-foreground text-center py-4">
                      No integrations found{query ? ` for "${query}"` : ''}.
                    </p>
                  )}
                </>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}

export function IntegrationsSettings() {
  const [keys, setKeys] = useState<RelayKeyRecord[]>([]);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      const result = await integrationsApi.listRelayKeys();
      setKeys(result || []);
    } catch {
      /* ignore — endpoint may not exist yet */
    }
    setLoading(false);
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  const handleAdd = async (providerId: string, label: string, apiKey: string) => {
    await integrationsApi.addRelayKey(providerId, label, apiKey);
    toast.success('Key added successfully');
    await refresh();
  };

  const handleDelete = async (id: string) => {
    try {
      await integrationsApi.deleteRelayKeyById(id);
      toast.success('Key removed');
      await refresh();
    } catch {
      toast.error('Failed to delete key');
    }
  };

  if (loading) return <div className="p-6 text-muted-foreground text-sm">Loading...</div>;

  const socialProviders = RELAY_PROVIDERS.filter(p => p.category === 'social');
  const workProviders = RELAY_PROVIDERS.filter(p => p.category === 'work');

  return (
    <div className="space-y-4">
      <div className="space-y-1 mb-2">
        <h2 className="text-base font-semibold text-foreground">Social Relay Providers</h2>
        <p className="text-xs text-muted-foreground">Connect social publishing APIs to post across platforms. Each provider handles OAuth and rate limits for you.</p>
      </div>

      {socialProviders.map(provider => (
        <ProviderCard
          key={provider.id}
          provider={provider}
          keys={keys.filter(k => k.provider === provider.id)}
          onAdd={(label, apiKey) => handleAdd(provider.id, label, apiKey)}
          onDelete={handleDelete}
        />
      ))}

      <div className="space-y-1 mt-6 mb-2">
        <h2 className="text-base font-semibold text-foreground">Work Tools (Pipedream)</h2>
        <p className="text-xs text-muted-foreground">Connect work integrations — email, calendar, CRM, and collaboration tools.</p>
      </div>

      {workProviders.map(provider => (
        <ProviderCard
          key={provider.id}
          provider={provider}
          keys={keys.filter(k => k.provider === provider.id)}
          onAdd={(label, apiKey) => handleAdd(provider.id, label, apiKey)}
          onDelete={handleDelete}
        />
      ))}

      <div className="mt-6">
        <CatalogBrowser />
      </div>
    </div>
  );
}
