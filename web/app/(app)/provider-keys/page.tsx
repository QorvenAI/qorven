'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState, useCallback } from 'react';
import { Key, Plus, Trash2, CheckCircle, Eye, EyeOff, Loader2, GitBranch, ExternalLink } from 'lucide-react';
import { cn } from '@/lib/utils';
import { providers as providersApi, connections } from '@/lib/api';
import { toast } from 'sonner';
import { ErrorBoundary } from '@/components/error-boundary';

type Provider = { id: string; name: string; display_name?: string; provider_type: string; enabled?: boolean };
type ProviderKey = { id: string; provider_id: string; status: string; created_at: string; last_used_at?: string; usage_count: number };
type Connection = { id: string; platform_id: string; label: string; auth_type: string; created_at: string };

// ─── GitHub section ───────────────────────────────────────────────────────────

function GitHubTokenSection() {
  const [connected, setConnected] = useState<Connection | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [token, setToken] = useState('');
  const [showInput, setShowInput] = useState(false);
  const [showToken, setShowToken] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await connections.list();
      const gh = (data?.connections || []).find((c: Connection) => c.platform_id === 'github');
      setConnected(gh || null);
    } catch {
      setConnected(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const save = async () => {
    if (!token.trim()) return;
    setSaving(true);
    try {
      await connections.save('github', token.trim());
      toast.success('GitHub token saved — agents can now use gh_* tools');
      setToken('');
      setShowInput(false);
      await load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to save token');
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    if (!confirm('Remove GitHub token? Agents will lose access to GitHub tools.')) return;
    setDeleting(true);
    try {
      await connections.delete('github');
      toast.success('GitHub token removed');
      setConnected(null);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to remove');
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-3 border-b border-border bg-muted/30">
        <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-foreground/5">
          <GitBranch className="h-4 w-4" />
        </div>
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">GitHub</span>
            <span className="text-xs text-muted-foreground">Personal Access Token</span>
          </div>
          <p className="text-xs text-muted-foreground mt-0.5">
            Enables <code className="bg-muted px-1 rounded">gh_*</code> tools — agents can read issues, create branches, push code, open PRs
          </p>
        </div>
        <a
          href="https://github.com/settings/tokens/new?scopes=repo,read:user&description=Qorven+Agent"
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          Create token <ExternalLink className="h-3 w-3" />
        </a>
      </div>

      {/* Status / form */}
      <div className="px-4 py-3">
        {loading ? (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Checking…
          </div>
        ) : connected ? (
          <div className="flex items-center gap-3">
            <span className="h-2 w-2 rounded-full bg-emerald-400 shrink-0" />
            <div className="flex-1 min-w-0">
              <span className="text-sm font-medium text-emerald-600">Connected</span>
              <span className="text-xs text-muted-foreground ml-2">
                since {new Date(connected.created_at).toLocaleDateString()}
              </span>
            </div>
            <button
              onClick={() => setShowInput(!showInput)}
              className="text-xs text-muted-foreground hover:text-foreground"
            >
              Replace
            </button>
            <button
              onClick={remove}
              disabled={deleting}
              className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-red-500 hover:bg-red-500/10 disabled:opacity-50"
            >
              {deleting ? <Loader2 className="h-3 w-3 animate-spin" /> : <Trash2 className="h-3 w-3" />}
              Remove
            </button>
          </div>
        ) : (
          <div className="flex items-center gap-2">
            <span className="h-2 w-2 rounded-full bg-muted-foreground/30 shrink-0" />
            <span className="text-sm text-muted-foreground flex-1">No token configured</span>
            <button
              onClick={() => setShowInput(true)}
              className="flex items-center gap-1 rounded-md bg-primary/10 px-2.5 py-1 text-xs text-primary hover:bg-primary/20"
            >
              <Plus className="h-3 w-3" /> Add Token
            </button>
          </div>
        )}

        {showInput && (
          <div className="mt-3 flex items-center gap-2">
            <div className="relative flex-1">
              <input
                value={token}
                onChange={e => setToken(e.target.value)}
                type={showToken ? 'text' : 'password'}
                placeholder="ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
                autoFocus
                onKeyDown={e => e.key === 'Enter' && save()}
                className="qr-input text-xs font-mono pr-9"
              />
              <button
                type="button"
                onClick={() => setShowToken(!showToken)}
                className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
              >
                {showToken ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
              </button>
            </div>
            <button
              onClick={save}
              disabled={!token.trim() || saving}
              className="rounded-md bg-primary px-3 py-1.5 text-xs text-primary-foreground hover:bg-primary/90 disabled:opacity-40 flex items-center gap-1"
            >
              {saving ? <Loader2 className="h-3 w-3 animate-spin" /> : <CheckCircle className="h-3 w-3" />}
              Save
            </button>
            <button
              onClick={() => { setShowInput(false); setToken(''); }}
              className="rounded-md border border-border px-2 py-1.5 text-xs hover:bg-accent"
            >
              Cancel
            </button>
          </div>
        )}

        {/* Required scopes hint */}
        {showInput && (
          <p className="mt-2 text-xs text-muted-foreground">
            Required scopes: <code className="bg-muted px-1 rounded">repo</code> (full repository access) and <code className="bg-muted px-1 rounded">read:user</code>
          </p>
        )}
      </div>
    </div>
  );
}

// ─── Provider API keys section ────────────────────────────────────────────────

function ProviderKeysSection() {
  const [providers, setProviders] = useState<Provider[]>([]);
  const [keys, setKeys] = useState<Record<string, ProviderKey[]>>({});
  const [addingTo, setAddingTo] = useState<string | null>(null);
  const [newKey, setNewKey] = useState('');
  const [showKey, setShowKey] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const loadKeys = useCallback(async (provs: Provider[]) => {
    const keyMap: Record<string, ProviderKey[]> = {};
    await Promise.all(provs.map(async p => {
      try {
        const k = await providersApi.listKeys(p.id);
        keyMap[p.id] = Array.isArray(k) ? k : [];
      } catch {
        keyMap[p.id] = [];
      }
    }));
    setKeys(keyMap);
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const provs = await providersApi.list();
      setProviders(provs);
      await loadKeys(provs);
    } finally {
      setLoading(false);
    }
  }, [loadKeys]);

  useEffect(() => { load(); }, [load]);

  const addKey = async (providerId: string) => {
    if (!newKey.trim()) return;
    setSaving(true);
    try {
      await providersApi.addKey(providerId, { label: '', key: newKey });
      toast.success('API key added');
      setNewKey('');
      setAddingTo(null);
      await loadKeys(providers);
    } catch (e) {
      toast.error('Failed to add key');
    } finally {
      setSaving(false);
    }
  };

  const deleteKey = async (keyId: string) => {
    try {
      await providersApi.retireKey(keyId);
      toast.success('Key removed');
      await loadKeys(providers);
    } catch {
      toast.error('Failed to remove key');
    }
  };

  const verifyKey = async (keyId: string) => {
    try {
      await providersApi.verifyKey(keyId);
      toast.success('Verification started');
      await loadKeys(providers);
    } catch {
      toast.error('Verification failed');
    }
  };

  if (loading) {
    return (
      <div className="space-y-3">
        {[1, 2, 3].map(i => (
          <div key={i} className="h-20 animate-pulse rounded-xl bg-muted" />
        ))}
      </div>
    );
  }

  if (providers.length === 0) {
    return (
      <div className="rounded-xl border border-border bg-card py-12 text-center">
        <Key className="h-8 w-8 mx-auto text-muted-foreground/30 mb-2" />
        <p className="text-sm text-muted-foreground">No LLM providers configured yet</p>
        <p className="text-xs text-muted-foreground/60 mt-1">
          Go to <a href="/models-hub" className="underline">Models Hub</a> to add a provider first
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {providers.map(p => (
        <div key={p.id} className="rounded-xl border border-border bg-card overflow-hidden">
          <div className="flex items-center gap-3 px-4 py-3 border-b border-border bg-muted/30">
            <Key className="h-4 w-4 text-muted-foreground" />
            <div className="flex-1">
              <span className="text-sm font-medium">{p.display_name || p.name}</span>
              <span className="text-xs text-muted-foreground ml-2">{p.provider_type}</span>
            </div>
            <button
              onClick={() => { setAddingTo(addingTo === p.id ? null : p.id); setNewKey(''); setShowKey(false); }}
              className="flex items-center gap-1 rounded-md bg-primary/10 px-2 py-1 text-xs text-primary hover:bg-primary/20"
            >
              <Plus className="h-3 w-3" /> Add Key
            </button>
          </div>

          {addingTo === p.id && (
            <div className="flex items-center gap-2 px-4 py-2 bg-primary/5 border-b border-border">
              <div className="relative flex-1">
                <input
                  value={newKey}
                  onChange={e => setNewKey(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && addKey(p.id)}
                  placeholder="Paste API key…"
                  type={showKey ? 'text' : 'password'}
                  autoFocus
                  className="qr-input text-xs font-mono pr-9"
                />
                <button
                  type="button"
                  onClick={() => setShowKey(!showKey)}
                  className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  {showKey ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                </button>
              </div>
              <button
                onClick={() => addKey(p.id)}
                disabled={!newKey.trim() || saving}
                className="rounded-md bg-primary px-3 py-1.5 text-xs text-primary-foreground hover:bg-primary/90 disabled:opacity-40"
              >
                {saving ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Save'}
              </button>
              <button
                onClick={() => setAddingTo(null)}
                className="rounded-md border border-border px-2 py-1.5 text-xs hover:bg-accent"
              >
                Cancel
              </button>
            </div>
          )}

          <div className="divide-y divide-border/50">
            {(keys[p.id] || []).length === 0 ? (
              <div className="px-4 py-3 text-xs text-muted-foreground text-center">No keys configured</div>
            ) : (keys[p.id] || []).map(k => (
              <div key={k.id} className="flex items-center gap-3 px-4 py-2.5">
                <span className={cn(
                  'h-2 w-2 rounded-full shrink-0',
                  k.status === 'verified' ? 'bg-emerald-400' :
                  k.status === 'failed' ? 'bg-red-400' : 'bg-amber-400'
                )} />
                <span className="text-xs font-mono text-muted-foreground">…{k.id.slice(-8)}</span>
                <span className={cn(
                  'text-xs px-1.5 py-0.5 rounded',
                  k.status === 'verified' ? 'bg-emerald-500/10 text-emerald-500' :
                  k.status === 'failed' ? 'bg-red-500/10 text-red-500' : 'bg-amber-500/10 text-amber-500'
                )}>
                  {k.status}
                </span>
                <span className="text-xs text-muted-foreground ml-auto">{k.usage_count} uses</span>
                <button
                  onClick={() => verifyKey(k.id)}
                  title="Verify key"
                  className="rounded p-1 hover:bg-accent"
                >
                  <CheckCircle className="h-3.5 w-3.5 text-muted-foreground" />
                </button>
                <button
                  onClick={() => deleteKey(k.id)}
                  title="Delete key"
                  className="rounded p-1 hover:bg-red-500/10"
                >
                  <Trash2 className="h-3.5 w-3.5 text-red-400" />
                </button>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function ProviderKeysPage() {
  return (
    <ErrorBoundary fallbackTitle="Failed to load API keys">
      <div className="space-y-8">
        <div>
          <h1 className="text-lg font-semibold">API Keys & Integrations</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            Connect external services so agents can take autonomous actions
          </p>
        </div>

        {/* GitHub — most important for autonomous dev loop */}
        <section className="space-y-3">
          <div>
            <h2 className="text-sm font-semibold">GitHub Integration</h2>
            <p className="text-xs text-muted-foreground">
              Required for the autonomous development loop — reading issues, creating branches, pushing code, opening PRs
            </p>
          </div>
          <GitHubTokenSection />
        </section>

        {/* LLM provider keys */}
        <section className="space-y-3">
          <div>
            <h2 className="text-sm font-semibold">LLM Provider Keys</h2>
            <p className="text-xs text-muted-foreground">
              API keys for your configured language model providers
            </p>
          </div>
          <ProviderKeysSection />
        </section>
      </div>
    </ErrorBoundary>
  );
}
