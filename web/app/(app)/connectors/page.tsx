'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Connector catalog — a user-facing directory of every SaaS the agent
// can plug into. Mirrors the shape of Claude's Connectors page:
//
//   ┌────────────────────────────────────────────────────────────┐
//   │  Search: [____________________]                             │
//   │                                                              │
//   │  Category  ▼    Auth  ▼    Status ▼                          │
//   │                                                              │
//   │  [card] [card] [card] [card] [card] [card]                   │
//   │  [card] [card] [card] [card] [card] [card]                   │
//   └────────────────────────────────────────────────────────────┘
//
// Each card represents one platform the agent knows how to talk to.
// Clicking "Connect" opens a credential sheet (api key / OAuth2 /
// basic auth) scoped to the platform's auth_schema. Once stored
// the card flips to a "Connected" state with Disconnect + Configure
// actions.

import { useEffect, useMemo, useState } from 'react';
import {
  Plug, Search, Check, ExternalLink, Shield, AlertCircle,
  Loader2, X, RefreshCw, Package, Trash2, Bot,
} from 'lucide-react';
import { toast } from 'sonner';
import {
  connectors,
  type ConnectorManifest,
  type ConnectorPlatform,
} from '@/lib/api';
import { listApps, patchApp, uninstallApp, type QorvenApp } from '@/lib/api-apps';
import { ErrorBoundary } from '@/components/error-boundary';
import { OAuthAppsSettings } from '@/components/settings/oauth-apps-settings';
import { cn } from '@/lib/utils';
import { BASE, request } from '@/lib/api-core';

// --- Types & constants -------------------------------------------

// Category metadata — order + emoji glyph + "look here for X" copy.
// Keeps the catalog skimmable. If the backend ships a category we
// don't know about, it falls through to "Other".
const CATEGORY_META: Record<string, { label: string; glyph: string; blurb: string }> = {
  messaging:    { label: 'Messaging',    glyph: '💬', blurb: 'Chat platforms, SMS, email' },
  productivity: { label: 'Productivity', glyph: '📝', blurb: 'Notes, docs, sheets, tasks' },
  storage:      { label: 'Storage',      glyph: '📦', blurb: 'File hosts and sync services' },
  crm:          { label: 'CRM',          glyph: '👥', blurb: 'Contacts, deals, pipelines' },
  development:  { label: 'Development',  glyph: '🧑‍💻', blurb: 'Code repos, CI, issue trackers' },
  payments:     { label: 'Payments',     glyph: '💳', blurb: 'Invoicing, subscriptions, payouts' },
  social:       { label: 'Social',       glyph: '📢', blurb: 'Social networks and monitoring' },
  ecommerce:    { label: 'E-commerce',   glyph: '🛒', blurb: 'Storefronts and merchant APIs' },
  content:      { label: 'Content',      glyph: '📰', blurb: 'Blogs, CMS, publishing' },
  email:        { label: 'Email',        glyph: '📧', blurb: 'Inbox, send, search' },
  data:         { label: 'Data',         glyph: '📊', blurb: 'Databases, warehouses, analytics' },
  workplace:    { label: 'Workplace',    glyph: '🏢', blurb: 'HR, finance, knowledge' },
  infra:        { label: 'Infrastructure', glyph: '⚙️', blurb: 'Cloud, DNS, monitoring' },
  commerce:     { label: 'Commerce',     glyph: '🛍️', blurb: 'Orders, inventory' },
  other:        { label: 'Other',        glyph: '🧩', blurb: 'Everything else' },
};

// Normalise a platform row into the minimum shape the catalog needs.
// Using a local shape keeps the UI insulated from whichever backend
// endpoint we read from — both /connectors and /connectors/platforms
// map here.
interface CatalogEntry {
  id: string;
  name: string;
  description: string;
  category: string;
  icon?: string;
  authType: string;       // "oauth2" | "api_key" | "bearer" | "basic" | "none"
  docsURL?: string;
  baseURL?: string;
  featured: boolean;      // has a real Go manifest + tested auth
}

function toCatalogEntry(p: ConnectorPlatform | ConnectorManifest, featured = false): CatalogEntry {
  const cat = ((p as any).category ?? 'other').toLowerCase();
  const icon = (p as any).icon ?? '';
  return {
    id: p.id,
    name: p.name ?? p.id,
    description: (p as any).description ?? '',
    category: cat,
    icon,
    authType: ((p as ConnectorPlatform).auth_type) ??
      ((p as ConnectorManifest).auth_schema?.type) ?? 'none',
    docsURL: (p as ConnectorPlatform).docs_url,
    baseURL: (p as ConnectorPlatform).base_url,
    featured,
  };
}

// --- Page component ----------------------------------------------

export default function ConnectorsPage() {
  const [entries, setEntries] = useState<CatalogEntry[]>([]);
  const [connected, setConnected] = useState<Set<string>>(new Set());
  // oauthConfigured tracks which OAuth providers have valid client
  // creds (hosted default or user-set). Cards surface a "Configure"
  // indicator when oauth2 auth is required but unconfigured so the
  // user doesn't end up stuck on a provider error during the
  // authorize redirect.
  const [oauthConfigured, setOauthConfigured] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // view toggles the page between the catalog (Connect buttons) and
  // the OAuth-apps management panel (BYO client_id/secret). Self-
  // hosted users live in `oauth_apps` until they've configured
  // credentials; everyone else mostly stays in `catalog`.
  const [view, setView] = useState<'catalog' | 'oauth_apps' | 'installed'>('catalog');
  const [search, setSearch] = useState('');
  const [activeCategory, setActiveCategory] = useState<string | null>(null);
  const [activeAuth, setActiveAuth] = useState<string | null>(null);
  const [connectTarget, setConnectTarget] = useState<CatalogEntry | null>(null);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      // Pull manifests + platforms + oauth-app status concurrently.
      // Manifests are the curated "featured" set, platforms is the
      // long tail, and oauth-apps tells us which OAuth providers
      // are ready to accept a Connect click.
      const [mans, plats, oauthRes] = await Promise.all([
        connectors.list().catch(() => [] as ConnectorManifest[]),
        connectors.platforms().catch(() => [] as ConnectorPlatform[]),
        request<any>('/oauth/apps').catch(() => ({ apps: [] })),
      ]);
      const byID = new Map<string, CatalogEntry>();
      for (const p of plats) byID.set(p.id, toCatalogEntry(p, false));
      for (const m of mans) byID.set(m.id, toCatalogEntry(m, true));
      setEntries([...byID.values()].sort((a, b) => a.name.localeCompare(b.name)));

      // Build the set of oauth providers that have valid creds so
      // the card can show a "Configure first" hint where needed.
      const configured = new Set<string>();
      for (const app of (oauthRes.apps ?? []) as Array<{ id: string; has_client_id: boolean }>) {
        if (app.has_client_id) configured.add(app.id);
      }
      setOauthConfigured(configured);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load catalog');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  // Keep connected-state in localStorage so the UI reflects the
  // user's prior connections without a round-trip. Real wiring
  // (credential store, OAuth tokens) happens via the backend.
  useEffect(() => {
    try {
      const raw = localStorage.getItem('qorven_connected') ?? '[]';
      const ids: string[] = JSON.parse(raw);
      setConnected(new Set(ids));
    } catch {}
  }, []);
  const markConnected = (id: string) => {
    const next = new Set(connected);
    next.add(id);
    setConnected(next);
    localStorage.setItem('qorven_connected', JSON.stringify([...next]));
  };
  const markDisconnected = (id: string) => {
    const next = new Set(connected);
    next.delete(id);
    setConnected(next);
    localStorage.setItem('qorven_connected', JSON.stringify([...next]));
  };

  // Filter logic. Case-insensitive substring match on name/description.
  const visible = useMemo(() => {
    const q = search.trim().toLowerCase();
    return entries.filter((e) => {
      if (activeCategory && e.category !== activeCategory) return false;
      if (activeAuth && e.authType !== activeAuth) return false;
      if (!q) return true;
      return (
        e.name.toLowerCase().includes(q) ||
        e.description.toLowerCase().includes(q) ||
        e.id.toLowerCase().includes(q)
      );
    });
  }, [entries, search, activeCategory, activeAuth]);

  // Count connectors per category for the filter chips.
  const categoryCounts = useMemo(() => {
    const m = new Map<string, number>();
    for (const e of entries) {
      m.set(e.category, (m.get(e.category) ?? 0) + 1);
    }
    return m;
  }, [entries]);

  return (
    <ErrorBoundary fallbackTitle="Failed to load connectors">
      <div className="space-y-5">
        <header className="flex items-start justify-between gap-4">
          <div>
            <h1 className="text-lg font-semibold">Connectors</h1>
            <p className="text-sm text-muted-foreground mt-1">
              {view === 'catalog'
                ? `Plug your agents into the services you already use. ${entries.length} platforms available.`
                : view === 'installed'
                  ? 'Connectors your agents built and installed at runtime. Each one is a compiled Go binary the agent generated from API docs.'
                  : 'Register OAuth apps on each provider\u2019s developer console and paste credentials here. The same credentials serve every user on this Qorven install.'}
            </p>
          </div>
          {(view === 'catalog' || view === 'installed') && (
            <button
              onClick={load}
              disabled={loading}
              className="inline-flex items-center gap-2 rounded-md border border-border px-3 py-1.5 text-xs text-muted-foreground hover:bg-accent disabled:opacity-50"
            >
              <RefreshCw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
              Refresh
            </button>
          )}
        </header>

        {/* Tab toggle */}
        <div className="inline-flex rounded-lg border border-border bg-card p-0.5 text-xs">
          <button
            onClick={() => setView('catalog')}
            className={cn(
              'rounded-md px-3 py-1 font-medium transition-colors',
              view === 'catalog' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground',
            )}
          >
            Catalog
          </button>
          <button
            onClick={() => setView('oauth_apps')}
            className={cn(
              'rounded-md px-3 py-1 font-medium transition-colors',
              view === 'oauth_apps' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground',
            )}
          >
            OAuth apps
          </button>
          <button
            onClick={() => setView('installed')}
            className={cn(
              'rounded-md px-3 py-1 font-medium transition-colors',
              view === 'installed' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground',
            )}
          >
            Installed
          </button>
        </div>

        {view === 'oauth_apps' ? (
          <OAuthAppsSettings />
        ) : view === 'installed' ? (
          <InstalledConnectors />
        ) : (<>

        {/* Search */}
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search connectors — try &quot;email&quot;, &quot;github&quot;, &quot;calendar&quot;…"
            className="qr-input pl-9"
          />
          {search && (
            <button
              onClick={() => setSearch('')}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
              title="Clear"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>

        {/* Filter chips */}
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-2xs font-medium uppercase tracking-wider text-muted-foreground mr-1">
            Category
          </span>
          <FilterChip
            label="All"
            active={!activeCategory}
            onClick={() => setActiveCategory(null)}
            count={entries.length}
          />
          {[...categoryCounts.entries()]
            .sort((a, b) => b[1] - a[1])
            .map(([cat, count]) => {
              const meta = CATEGORY_META[cat] ?? CATEGORY_META.other!;
              return (
                <FilterChip
                  key={cat}
                  label={`${meta.glyph} ${meta.label}`}
                  active={activeCategory === cat}
                  onClick={() => setActiveCategory(activeCategory === cat ? null : cat)}
                  count={count}
                />
              );
            })}
          <div className="mx-2 h-4 w-px bg-border" />
          <span className="text-2xs font-medium uppercase tracking-wider text-muted-foreground mr-1">
            Auth
          </span>
          {(['oauth2', 'api_key', 'bearer', 'basic', 'none'] as const).map((t) => (
            <FilterChip
              key={t}
              label={authLabel(t)}
              active={activeAuth === t}
              onClick={() => setActiveAuth(activeAuth === t ? null : t)}
            />
          ))}
        </div>

        {/* Body */}
        {loading ? (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="h-36 rounded-xl border border-border bg-muted/20 animate-pulse" />
            ))}
          </div>
        ) : error ? (
          <div className="flex flex-col items-center py-16 text-center">
            <AlertCircle className="h-8 w-8 text-destructive" />
            <p className="mt-2 text-sm text-destructive">{error}</p>
            <button
              onClick={load}
              className="mt-3 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
            >
              Retry
            </button>
          </div>
        ) : visible.length === 0 ? (
          <div className="flex flex-col items-center py-16 text-center text-muted-foreground">
            <Plug className="h-8 w-8 mb-2" />
            <p className="text-sm">No connectors match your filters.</p>
            <button
              onClick={() => { setSearch(''); setActiveCategory(null); setActiveAuth(null); }}
              className="mt-3 text-xs text-primary hover:underline"
            >
              Clear filters
            </button>
          </div>
        ) : (
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {visible.map((e) => (
              <ConnectorCard
                key={e.id}
                entry={e}
                connected={connected.has(e.id)}
                needsOAuthApp={e.authType === 'oauth2' && !oauthConfigured.has(e.id)}
                onConnect={() => setConnectTarget(e)}
                onDisconnect={() => {
                  markDisconnected(e.id);
                  toast.success(`Disconnected from ${e.name}`);
                }}
                onConfigureOAuth={() => setView('oauth_apps')}
              />
            ))}
          </div>
        )}

        {/* Connect sheet — lives under the catalog view */}
        {connectTarget && (
          <ConnectSheet
            entry={connectTarget}
            onClose={() => setConnectTarget(null)}
            onConnected={() => {
              markConnected(connectTarget.id);
              toast.success(`Connected to ${connectTarget.name}`);
              setConnectTarget(null);
            }}
          />
        )}
        </>)}
      </div>
    </ErrorBoundary>
  );
}

// --- Small helpers -----------------------------------------------

function authLabel(t: string): string {
  switch (t) {
    case 'oauth2': return 'OAuth 2.0';
    case 'api_key': return 'API key';
    case 'bearer': return 'Bearer token';
    case 'basic': return 'Basic auth';
    case 'none': return 'No auth';
    default: return t;
  }
}

function FilterChip({
  label, active, onClick, count,
}: {
  label: string; active: boolean; onClick: () => void; count?: number;
}) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-xs transition-colors',
        active
          ? 'bg-primary text-primary-foreground'
          : 'border border-border bg-card hover:bg-accent',
      )}
    >
      {label}
      {count != null && (
        <span className={cn(
          'rounded px-1 text-xs font-mono',
          active ? 'bg-primary-foreground/15' : 'bg-muted-foreground/15',
        )}>
          {count}
        </span>
      )}
    </button>
  );
}

// --- Card --------------------------------------------------------

function ConnectorCard({
  entry, connected, needsOAuthApp, onConnect, onDisconnect, onConfigureOAuth,
}: {
  entry: CatalogEntry;
  connected: boolean;
  needsOAuthApp: boolean;
  onConnect: () => void;
  onDisconnect: () => void;
  onConfigureOAuth: () => void;
}) {
  const meta = CATEGORY_META[entry.category] ?? CATEGORY_META.other!;
  return (
    <div className={cn(
      'rounded-xl border p-4 transition-colors',
      connected ? 'border-emerald-500/30 bg-emerald-500/5' : 'border-border bg-card hover:bg-accent/20',
    )}>
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted text-lg">
          {meta.glyph}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5">
            <h3 className="text-sm font-semibold truncate">{entry.name}</h3>
            {entry.featured && (
              <span className="inline-flex items-center rounded bg-primary/15 px-1.5 py-0.5 text-xs font-medium text-primary">
                Featured
              </span>
            )}
            {connected && (
              <span className="inline-flex items-center gap-0.5 rounded bg-emerald-500/15 px-1.5 py-0.5 text-xs font-medium text-emerald-400">
                <Check className="h-3 w-3" /> Connected
              </span>
            )}
            {needsOAuthApp && !connected && (
              <span
                className="inline-flex items-center rounded bg-amber-500/15 px-1.5 py-0.5 text-xs font-medium text-amber-400"
                title="This provider needs an OAuth app registered before users can connect. Click Configure below."
              >
                Needs setup
              </span>
            )}
          </div>
          <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">{entry.description}</p>
          <div className="mt-2 flex items-center gap-2 text-2xs text-muted-foreground">
            <span className="inline-flex items-center gap-1">
              <Shield className="h-3 w-3" />
              {authLabel(entry.authType)}
            </span>
            {entry.docsURL && (
              <a
                href={entry.docsURL}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-0.5 hover:text-foreground"
                onClick={(e) => e.stopPropagation()}
              >
                Docs <ExternalLink className="h-2.5 w-2.5" />
              </a>
            )}
          </div>
        </div>
      </div>
      <div className="mt-3 flex justify-end">
        {connected ? (
          <button
            onClick={onDisconnect}
            className="rounded-md border border-border bg-card px-3 py-1 text-xs hover:bg-accent"
          >
            Disconnect
          </button>
        ) : needsOAuthApp ? (
          // Self-hosted users see this when the OAuth app isn't
          // configured yet — jumps them to the OAuth-apps view so
          // the Connect flow doesn't fail with a cryptic provider
          // error mid-popup.
          <button
            onClick={onConfigureOAuth}
            className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-1 text-xs font-medium text-amber-400 hover:bg-amber-500/20"
          >
            Configure
          </button>
        ) : (
          <button
            onClick={onConnect}
            className="rounded-md bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90"
          >
            Connect
          </button>
        )}
      </div>
    </div>
  );
}

// --- Installed connectors ----------------------------------------

const SCOPE_BADGE: Record<string, { label: string; cls: string }> = {
  workspace: { label: 'Workspace', cls: 'bg-blue-500/15 text-blue-400' },
  agent:     { label: 'Agent',     cls: 'bg-violet-500/15 text-violet-400' },
  team:      { label: 'Team',      cls: 'bg-amber-500/15 text-amber-400' },
};

function InstalledConnectors() {
  const [apps, setApps] = useState<QorvenApp[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await listApps();
      setApps(res.apps);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load installed connectors');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const handleToggle = async (app: QorvenApp) => {
    try {
      const updated = await patchApp(app.id, { enabled: !app.enabled });
      setApps((prev) => prev.map((a) => a.id === updated.id ? updated : a));
      toast.success(`${updated.display_name} ${updated.enabled ? 'enabled' : 'disabled'}`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to update connector');
    }
  };

  const handleUninstall = async (app: QorvenApp) => {
    if (!confirm(`Uninstall "${app.display_name}"? This cannot be undone.`)) return;
    try {
      await uninstallApp(app.id);
      setApps((prev) => prev.filter((a) => a.id !== app.id));
      toast.success(`${app.display_name} uninstalled`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to uninstall connector');
    }
  };

  if (loading) {
    return (
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="h-40 rounded-xl border border-border bg-muted/20 animate-pulse" />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center py-16 text-center">
        <AlertCircle className="h-8 w-8 text-destructive" />
        <p className="mt-2 text-sm text-destructive">{error}</p>
        <button
          onClick={load}
          className="mt-3 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
        >
          Retry
        </button>
      </div>
    );
  }

  if (apps.length === 0) {
    return (
      <div className="flex flex-col items-center py-20 text-center text-muted-foreground">
        <Bot className="h-10 w-10 mb-3 opacity-40" />
        <p className="text-sm font-medium">No agent-built connectors yet</p>
        <p className="mt-1 text-xs max-w-sm">
          Ask your agent to build one — e.g. &ldquo;Build a connector for Binance prices and pin it to the dashboard.&rdquo;
          The agent will generate Go source, compile it, and install it here automatically.
        </p>
        <div className="mt-4 rounded-lg border border-border bg-muted/30 px-4 py-3 text-xs text-left max-w-sm font-mono">
          <span className="text-muted-foreground">Tools the agent uses:</span><br />
          <span className="text-primary">get_connector_template</span><br />
          <span className="text-primary">build_connector</span><br />
          <span className="text-primary">store_credential</span>
        </div>
      </div>
    );
  }

  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {apps.map((app) => (
        <InstalledCard
          key={app.id}
          app={app}
          onToggle={() => handleToggle(app)}
          onUninstall={() => handleUninstall(app)}
        />
      ))}
    </div>
  );
}

function InstalledCard({
  app, onToggle, onUninstall,
}: {
  app: QorvenApp;
  onToggle: () => void;
  onUninstall: () => void;
}) {
  const scopeBadge = SCOPE_BADGE[app.scope] ?? SCOPE_BADGE.workspace!;
  const tools: string[] = Array.isArray((app.config as any)?.tools)
    ? (app.config as any).tools
    : [];

  return (
    <div className={cn(
      'rounded-xl border p-4 transition-colors',
      app.enabled ? 'border-border bg-card' : 'border-border/50 bg-card/50 opacity-70',
    )}>
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted">
          <Package className="h-5 w-5 text-muted-foreground" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex flex-wrap items-center gap-1.5">
            <h3 className="text-sm font-semibold truncate">{app.display_name}</h3>
            <span className={cn('inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium', scopeBadge.cls)}>
              {scopeBadge.label}
            </span>
            {!app.enabled && (
              <span className="inline-flex items-center rounded bg-muted px-1.5 py-0.5 text-xs font-medium text-muted-foreground">
                Disabled
              </span>
            )}
          </div>
          <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">{app.description}</p>
          <div className="mt-1.5 text-2xs text-muted-foreground">
            v{app.version} · {app.slug}
          </div>
        </div>
      </div>

      {tools.length > 0 && (
        <div className="mt-3 flex flex-wrap gap-1">
          {tools.map((t) => (
            <span key={t} className="inline-flex items-center rounded-md bg-muted px-2 py-0.5 text-xs font-mono text-muted-foreground">
              {t}
            </span>
          ))}
        </div>
      )}

      <div className="mt-3 flex items-center justify-between">
        <button
          onClick={onToggle}
          className={cn(
            'inline-flex items-center gap-1.5 rounded-md px-3 py-1 text-xs font-medium transition-colors',
            app.enabled
              ? 'border border-border bg-card text-muted-foreground hover:bg-accent'
              : 'bg-primary text-primary-foreground hover:bg-primary/90',
          )}
        >
          {app.enabled ? 'Disable' : 'Enable'}
        </button>
        <button
          onClick={onUninstall}
          className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs text-destructive hover:bg-destructive/10"
          title="Uninstall connector"
        >
          <Trash2 className="h-3.5 w-3.5" />
          Uninstall
        </button>
      </div>
    </div>
  );
}

// --- Connect sheet -----------------------------------------------

function ConnectSheet({
  entry, onClose, onConnected,
}: {
  entry: CatalogEntry;
  onClose: () => void;
  onConnected: () => void;
}) {
  const [values, setValues] = useState<Record<string, string>>({});
  const [testing, setTesting] = useState(false);

  // Minimal field list inferred from auth_type. For featured
  // manifests we could read auth_schema.fields; for now this
  // generic fallback covers every entry.
  const fields: Array<{ name: string; label: string; type: string; placeholder?: string }> = (() => {
    switch (entry.authType) {
      case 'oauth2': return []; // handled via redirect flow below
      case 'api_key': return [{ name: 'api_key', label: 'API key', type: 'password' }];
      case 'bearer': return [{ name: 'token', label: 'Bearer token', type: 'password' }];
      case 'basic': return [
        { name: 'username', label: 'Username', type: 'text' },
        { name: 'password', label: 'Password', type: 'password' },
      ];
      case 'none': return [];
      default: return [{ name: 'token', label: 'Credential', type: 'password' }];
    }
  })();

  const handleConnect = async () => {
    setTesting(true);
    try {
      if (entry.featured && entry.authType !== 'oauth2' && fields.length > 0) {
        // Exercise the credentials against the backend's test endpoint
        // so the user gets an immediate pass/fail rather than
        // discovering the bad credential hours later during a run.
        await connectors.test(entry.id, values);
      }
      onConnected();
    } catch (e) {
      toast.error(e instanceof Error ? `Connection failed: ${e.message}` : 'Connection failed');
    } finally {
      setTesting(false);
    }
  };

  const startOAuth = () => {
    // OAuth flow is backend-driven — the server generates the
    // authorize redirect (with state), handles the callback, and
    // stores the token in the vault. We open in a popup so the
    // catalog page stays mounted.
    //
    // Backend route: GET /v1/oauth/{provider}/authorize  (routes_v1.go)
    window.open(
      `${BASE}/oauth/${entry.id}/authorize`,
      'qorven-oauth',
      'width=600,height=750',
    );
    // We optimistically mark connected on popup open. A real
    // callback eventually stores the token server-side; if the
    // user cancels the flow they can click Disconnect.
    onConnected();
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-end sm:items-center justify-center bg-black/50"
      onClick={onClose}
    >
      <div
        className="w-full sm:max-w-md rounded-t-2xl sm:rounded-2xl border border-border bg-card p-5"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start gap-3 mb-3">
          <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-muted text-xl">
            {(CATEGORY_META[entry.category] ?? CATEGORY_META.other!).glyph}
          </div>
          <div className="flex-1">
            <h2 className="text-base font-semibold">Connect to {entry.name}</h2>
            <p className="text-xs text-muted-foreground">{entry.description}</p>
          </div>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground">
            <X className="h-4 w-4" />
          </button>
        </div>

        {entry.authType === 'oauth2' ? (
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">
              Sign in through {entry.name} — you&apos;ll see the full permission list on their consent screen.
            </p>
            <button
              onClick={startOAuth}
              className="w-full rounded-md bg-primary py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
            >
              Sign in with {entry.name}
            </button>
          </div>
        ) : fields.length === 0 ? (
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">
              This connector needs no credentials. You&apos;re ready to go.
            </p>
            <button
              onClick={onConnected}
              className="w-full rounded-md bg-primary py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
            >
              Mark connected
            </button>
          </div>
        ) : (
          <div className="space-y-3">
            {fields.map((f) => (
              <label key={f.name} className="block">
                <span className="text-xs font-medium text-foreground/90">{f.label}</span>
                <input
                  type={f.type}
                  value={values[f.name] ?? ''}
                  onChange={(e) => setValues({ ...values, [f.name]: e.target.value })}
                  placeholder={f.placeholder}
                  className="mt-1 qr-input"
                  autoComplete="new-password"
                />
              </label>
            ))}
            <div className="flex items-start gap-2 rounded-md bg-muted/30 p-2 text-2xs text-muted-foreground">
              <Shield className="h-3 w-3 mt-0.5 shrink-0" />
              <span>
                Credentials are encrypted with your gateway&apos;s key and never leave your instance.
              </span>
            </div>
            <button
              onClick={handleConnect}
              disabled={testing || fields.some((f) => !values[f.name])}
              className="inline-flex w-full items-center justify-center gap-2 rounded-md bg-primary py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {testing ? (
                <>
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  Testing connection…
                </>
              ) : (
                <>
                  <Check className="h-3.5 w-3.5" />
                  Test &amp; connect
                </>
              )}
            </button>
          </div>
        )}

        {entry.docsURL && (
          <p className="mt-3 text-2xs text-muted-foreground text-center">
            <a href={entry.docsURL} target="_blank" rel="noopener noreferrer" className="hover:text-foreground">
              {entry.name} API docs ↗
            </a>
          </p>
        )}
      </div>
    </div>
  );
}
