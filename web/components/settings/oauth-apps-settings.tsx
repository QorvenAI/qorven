'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Settings → Connectors → OAuth Apps.
//
// To use OAuth-based connectors (Google Drive, Slack, GitHub, etc.)
// you register an OAuth app on each provider's developer console
// and paste its client_id + client_secret here. The credentials you
// register serve every user on this Qorven install.
//
// Flow:
//   1. We show the user the redirect URL to paste into the provider's
//      developer console ("Authorized redirect URI").
//   2. User clicks through to the provider's registration console.
//   3. User pastes client_id + client_secret back here.
//   4. The backend saves them (encrypted) and re-registers the
//      provider so subsequent Connect flows use the new creds.
//
// Once configured, the connector catalog shows "Connect" as usual.
// Without configuration, the catalog shows "Configure" pointing here.

import { useCallback, useEffect, useState } from 'react';
import {
  Copy, Check, ExternalLink, KeyRound, Loader2, Pencil, Trash2,
  AlertTriangle, BookOpen,
} from 'lucide-react';
import { toast } from 'sonner';
import { cn } from '@/lib/utils';
import { apiBase } from '@/lib/api-url';

interface OAuthApp {
  id: string;
  name: string;
  scopes: string[];
  redirect_url: string;
  has_client_id: boolean;
  is_user_set: boolean;
  docs_url?: string;
  setup_guide?: string;
}

async function fetchApps(): Promise<OAuthApp[]> {
  const token = typeof window !== 'undefined' ? localStorage.getItem('qorven_token') ?? '' : '';
  const r = await fetch(`${apiBase()}/oauth/apps`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!r.ok) throw new Error(`Failed to load OAuth apps (${r.status})`);
  const d = await r.json();
  return d.apps ?? [];
}

async function saveApp(id: string, clientID: string, clientSecret: string) {
  const token = typeof window !== 'undefined' ? localStorage.getItem('qorven_token') ?? '' : '';
  const r = await fetch(`${apiBase()}/oauth/apps/${id}`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ client_id: clientID, client_secret: clientSecret }),
  });
  if (!r.ok) {
    const body = await r.text().catch(() => '');
    throw new Error(body || `Save failed (${r.status})`);
  }
}

async function deleteApp(id: string) {
  const token = typeof window !== 'undefined' ? localStorage.getItem('qorven_token') ?? '' : '';
  const r = await fetch(`${apiBase()}/oauth/apps/${id}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!r.ok) throw new Error(`Delete failed (${r.status})`);
}

export function OAuthAppsSettings() {
  const [apps, setApps] = useState<OAuthApp[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editTarget, setEditTarget] = useState<OAuthApp | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const list = await fetchApps();
      setApps(list);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground p-6">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading OAuth apps…
      </div>
    );
  }
  if (error) {
    return (
      <div className="flex items-start gap-2 rounded-xl border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
        <AlertTriangle className="h-4 w-4 mt-0.5" />
        <div>{error}</div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="rounded-xl border border-border bg-card overflow-hidden">
        <header className="flex items-start gap-3 px-4 py-3 border-b border-border/70 bg-muted/20">
          <KeyRound className="h-4 w-4 mt-0.5 text-primary shrink-0" />
          <div>
            <h3 className="text-sm font-semibold">OAuth apps</h3>
            <p className="text-xs text-muted-foreground mt-0.5">
              Register an OAuth app on each provider&apos;s developer console and paste the client_id + client_secret here. The same credentials are used for every user on this Qorven install.
            </p>
          </div>
        </header>
        <ul className="divide-y divide-border/50">
          {apps.map((app) => (
            <OAuthAppRow
              key={app.id}
              app={app}
              onEdit={() => setEditTarget(app)}
              onRemoved={load}
            />
          ))}
        </ul>
      </div>

      {editTarget && (
        <OAuthAppDialog
          app={editTarget}
          onClose={() => setEditTarget(null)}
          onSaved={async () => {
            setEditTarget(null);
            await load();
            toast.success(`Saved ${editTarget.name} credentials`);
          }}
        />
      )}
    </div>
  );
}

function OAuthAppRow({
  app, onEdit, onRemoved,
}: {
  app: OAuthApp;
  onEdit: () => void;
  onRemoved: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const copyRedirect = async () => {
    try {
      await navigator.clipboard.writeText(app.redirect_url);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error('Copy failed');
    }
  };
  const handleRemove = async () => {
    if (!confirm(`Remove ${app.name} OAuth credentials? Agents that rely on it will stop working until you re-connect.`)) return;
    try {
      await deleteApp(app.id);
      toast.success(`Removed ${app.name}`);
      onRemoved();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Delete failed');
    }
  };

  // Status pill reflects backend flags:
  //   - is_user_set: credentials registered via this UI
  //   - has_client_id without is_user_set: a gateway-level env
  //     default is in play (set by the operator via config / env var)
  //   - neither: nothing configured yet, Configure button shows
  const statusPill = app.is_user_set
    ? { text: 'Configured', tone: 'bg-emerald-500/15 text-emerald-400' }
    : app.has_client_id
      ? { text: 'Using gateway default', tone: 'bg-sky-500/15 text-sky-400' }
      : { text: 'Needs setup', tone: 'bg-amber-500/15 text-amber-400' };

  return (
    <li className="px-4 py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h4 className="text-sm font-semibold">{app.name}</h4>
            <span className={cn('rounded-full px-1.5 py-0.5 text-2xs font-semibold', statusPill.tone)}>
              {statusPill.text}
            </span>
          </div>

          <div className="mt-2 flex items-center gap-2">
            <code className="flex-1 truncate rounded bg-muted px-2 py-1 text-2xs font-mono text-muted-foreground">
              {app.redirect_url}
            </code>
            <button
              onClick={copyRedirect}
              className="inline-flex items-center gap-1 rounded-md border border-border px-2 py-1 text-2xs hover:bg-accent"
              title="Copy redirect URL"
            >
              {copied ? <Check className="h-3 w-3 text-emerald-400" /> : <Copy className="h-3 w-3" />}
              {copied ? 'Copied' : 'Copy'}
            </button>
          </div>
          <p className="mt-1 text-2xs text-muted-foreground">
            Paste the URL above into the &ldquo;Authorized redirect URI&rdquo; field of your {app.name} OAuth app.
          </p>

          {app.setup_guide && (
            <details className="mt-2 group">
              <summary className="inline-flex items-center gap-1 cursor-pointer text-2xs text-primary hover:underline">
                <BookOpen className="h-3 w-3" />
                Setup guide
              </summary>
              <p className="mt-1 text-2xs text-muted-foreground leading-relaxed pl-4">
                {app.setup_guide}
              </p>
              {app.docs_url && (
                <a
                  href={app.docs_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="mt-1 pl-4 inline-flex items-center gap-0.5 text-2xs text-primary hover:underline"
                >
                  Open {app.name} developer console <ExternalLink className="h-3 w-3" />
                </a>
              )}
            </details>
          )}

          {app.scopes.length > 0 && (
            <div className="mt-2 flex flex-wrap gap-1">
              {app.scopes.slice(0, 5).map((s) => (
                <span key={s} className="rounded bg-muted px-1.5 py-0.5 text-2xs font-mono text-muted-foreground">
                  {s}
                </span>
              ))}
              {app.scopes.length > 5 && (
                <span className="text-2xs text-muted-foreground">
                  +{app.scopes.length - 5} more
                </span>
              )}
            </div>
          )}
        </div>

        <div className="flex flex-col gap-1 shrink-0">
          <button
            onClick={onEdit}
            className="inline-flex items-center gap-1 rounded-md bg-primary px-2 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90"
          >
            <Pencil className="h-3 w-3" />
            {app.is_user_set ? 'Edit' : 'Configure'}
          </button>
          {app.is_user_set && (
            <button
              onClick={handleRemove}
              className="inline-flex items-center gap-1 rounded-md border border-border px-2 py-1 text-xs text-muted-foreground hover:text-destructive hover:border-destructive/40"
            >
              <Trash2 className="h-3 w-3" />
              Remove
            </button>
          )}
        </div>
      </div>
    </li>
  );
}

function OAuthAppDialog({
  app, onClose, onSaved,
}: {
  app: OAuthApp;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [clientID, setClientID] = useState('');
  const [clientSecret, setClientSecret] = useState('');
  const [busy, setBusy] = useState(false);

  const onSubmit = async () => {
    if (!clientID.trim() || !clientSecret.trim()) {
      toast.error('Both client_id and client_secret are required');
      return;
    }
    setBusy(true);
    try {
      await saveApp(app.id, clientID.trim(), clientSecret.trim());
      await onSaved();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Save failed');
    } finally {
      setBusy(false);
    }
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
        <div className="mb-3">
          <h2 className="text-base font-semibold">Configure {app.name} OAuth app</h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            Paste the client_id and client_secret from your {app.name} app. Credentials are encrypted with your gateway&apos;s key.
          </p>
        </div>

        <div className="space-y-2">
          <label className="block">
            <span className="text-xs font-medium">Client ID</span>
            <input
              type="text"
              value={clientID}
              onChange={(e) => setClientID(e.target.value)}
              placeholder="123456789012-xxxxx.apps.googleusercontent.com"
              className="mt-1 qr-input font-mono"
              autoComplete="off"
            />
          </label>
          <label className="block">
            <span className="text-xs font-medium">Client secret</span>
            <input
              type="password"
              value={clientSecret}
              onChange={(e) => setClientSecret(e.target.value)}
              className="mt-1 qr-input font-mono"
              autoComplete="new-password"
            />
          </label>
          {app.docs_url && (
            <a
              href={app.docs_url}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-2xs text-primary hover:underline"
            >
              Open {app.name} developer console <ExternalLink className="h-3 w-3" />
            </a>
          )}
        </div>

        <div className="mt-4 flex justify-end gap-2">
          <button
            onClick={onClose}
            className="rounded-md border border-border bg-card px-3 py-1.5 text-xs hover:bg-accent"
          >
            Cancel
          </button>
          <button
            onClick={onSubmit}
            disabled={busy || !clientID.trim() || !clientSecret.trim()}
            className="inline-flex items-center gap-1 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            {busy && <Loader2 className="h-3 w-3 animate-spin" />}
            Save
          </button>
        </div>
      </div>
    </div>
  );
}
