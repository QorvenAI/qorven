'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import {
  Package, Trash2, RefreshCw,
  Loader2, ToggleLeft, ToggleRight,
  Sparkles, Layers,
} from 'lucide-react';
import { listApps, installApp, patchApp, uninstallApp, reloadApp } from '@/lib/api-apps';
import type { QorvenApp } from '@/lib/api-apps';
import { ErrorBoundary } from '@/components/error-boundary';
import { cn } from '@/lib/utils';
import SkillsPage from '../skills/page';
import MarketplacePage from '../marketplace/page';

// ─── Apps content ─────────────────────────────────────────────────────────────

function AppsContent() {
  const [apps, setApps] = useState<QorvenApp[]>([]);
  const [loading, setLoading] = useState(true);

  // Install modal state
  const [installPath, setInstallPath] = useState('');
  const [installing, setInstalling] = useState(false);
  const [showInstall, setShowInstall] = useState(false);

  // Per-row operation tracking
  const [togglingId, setTogglingId] = useState<string | null>(null);
  const [reloadingId, setReloadingId] = useState<string | null>(null);
  const [uninstallingId, setUninstallingId] = useState<string | null>(null);
  const [confirmUninstall, setConfirmUninstall] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await listApps();
      setApps(data.apps ?? []);
    } catch {
      toast.error('Could not load apps. Please refresh the page.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function handleToggle(app: QorvenApp) {
    setTogglingId(app.id);
    try {
      await patchApp(app.id, { enabled: !app.enabled });
      toast.success(app.enabled ? `${app.display_name} disabled` : `${app.display_name} enabled`);
      await load();
    } catch {
      toast.error('Failed to update app');
    } finally {
      setTogglingId(null);
    }
  }

  async function handleReload(app: QorvenApp) {
    setReloadingId(app.id);
    try {
      await reloadApp(app.id);
      toast.success(`${app.display_name} reloaded`);
      await load();
    } catch {
      toast.error('Failed to reload app');
    } finally {
      setReloadingId(null);
    }
  }

  async function handleUninstall(app: QorvenApp) {
    setUninstallingId(app.id);
    try {
      await uninstallApp(app.id, false);
      toast.success(`${app.display_name} uninstalled`);
      setConfirmUninstall(null);
      await load();
    } catch {
      toast.error('Failed to uninstall app');
    } finally {
      setUninstallingId(null);
    }
  }

  async function handleInstall() {
    if (!installPath.trim()) return;
    setInstalling(true);
    try {
      const app = await installApp(installPath.trim());
      toast.success(`${app.display_name} installed`);
      setInstallPath('');
      setShowInstall(false);
      await load();
    } catch (e: any) {
      toast.error(e?.message ?? 'Failed to install app');
    } finally {
      setInstalling(false);
    }
  }

  return (
    <>
      <div className="max-w-4xl">
        <div className="flex items-center justify-between mb-6">
          <div>
            <h1 className="text-xl font-semibold">Apps</h1>
            <p className="text-sm text-muted-foreground mt-0.5">
              Extend Qorven with sideloaded apps
            </p>
          </div>
          <button
            onClick={() => setShowInstall(true)}
            className="flex items-center gap-2 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
          >
            <Package className="h-4 w-4" />
            Install App
          </button>
        </div>

        {loading ? (
          <div className="flex items-center gap-2 text-muted-foreground py-8 justify-center">
            <Loader2 className="h-4 w-4 animate-spin" />
            <span className="text-sm">Loading apps…</span>
          </div>
        ) : apps.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-muted mb-4">
              <Package className="h-7 w-7 text-muted-foreground" />
            </div>
            <p className="font-medium">No apps installed</p>
            <p className="text-sm text-muted-foreground mt-1">
              Install an app from a local directory path.
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {apps.map((app) => (
              <div
                key={app.id}
                className={cn(
                  'flex items-center gap-4 rounded-lg border border-input bg-card px-4 py-3',
                  !app.enabled && 'opacity-60'
                )}
              >
                {/* Icon / initials */}
                <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                  {app.icon_url ? (
                    <img src={app.icon_url} alt="" className="h-8 w-8 rounded" />
                  ) : (
                    <Package className="h-5 w-5" />
                  )}
                </div>

                {/* Info */}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-sm">{app.display_name}</span>
                    <span className="text-xs text-muted-foreground">v{app.version}</span>
                    {app.author && (
                      <span className="text-xs text-muted-foreground">by {app.author}</span>
                    )}
                  </div>
                  {app.description && (
                    <p className="text-xs text-muted-foreground mt-0.5 truncate">{app.description}</p>
                  )}
                  <p className="text-xs text-muted-foreground/60 mt-0.5 truncate font-mono">
                    {app.slug}
                  </p>
                </div>

                {/* Actions */}
                <div className="flex items-center gap-1 shrink-0">
                  {/* Enable/disable toggle */}
                  <button
                    title={app.enabled ? 'Disable' : 'Enable'}
                    onClick={() => handleToggle(app)}
                    disabled={togglingId === app.id}
                    className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent transition-colors disabled:opacity-50"
                  >
                    {togglingId === app.id ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : app.enabled ? (
                      <ToggleRight className="h-4 w-4 text-primary" />
                    ) : (
                      <ToggleLeft className="h-4 w-4" />
                    )}
                  </button>

                  {/* Reload */}
                  <button
                    title="Reload"
                    onClick={() => handleReload(app)}
                    disabled={reloadingId === app.id}
                    className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent transition-colors disabled:opacity-50"
                  >
                    {reloadingId === app.id ? (
                      <Loader2 className="h-4 w-4 animate-spin" />
                    ) : (
                      <RefreshCw className="h-4 w-4" />
                    )}
                  </button>

                  {/* Uninstall */}
                  <button
                    title="Uninstall"
                    onClick={() => setConfirmUninstall(app.id)}
                    className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Install modal */}
      {showInstall && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
          onClick={(e) => e.target === e.currentTarget && setShowInstall(false)}
          onKeyDown={(e) => e.key === 'Escape' && setShowInstall(false)}
          role="dialog"
          aria-modal="true"
        >
          <div className="bg-card border border-input rounded-xl shadow-xl w-full max-w-md mx-4 p-6">
            <h2 className="text-base font-semibold mb-1">Install App from Path</h2>
            <p className="text-sm text-muted-foreground mb-4">
              Enter the absolute path to a directory containing an <code className="text-xs">app.yaml</code>.
            </p>
            <input
              type="text"
              value={installPath}
              onChange={(e) => setInstallPath(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleInstall()}
              placeholder="~/.qorven/apps/my-app"
              className="qr-input"
              autoFocus
            />
            <div className="flex justify-end gap-2 mt-4">
              <button
                onClick={() => setShowInstall(false)}
                className="px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={handleInstall}
                disabled={installing || !installPath.trim()}
                className="flex items-center gap-2 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors disabled:opacity-50"
              >
                {installing && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                Install
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Uninstall confirm modal */}
      {confirmUninstall && (() => {
        const app = apps.find((a) => a.id === confirmUninstall);
        if (!app) return null;
        return (
          <div
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
            onClick={(e) => e.target === e.currentTarget && setConfirmUninstall(null)}
            onKeyDown={(e) => e.key === 'Escape' && setConfirmUninstall(null)}
            role="alertdialog"
            aria-modal="true"
          >
            <div className="bg-card border border-input rounded-xl shadow-xl w-full max-w-sm mx-4 p-6">
              <h2 className="text-base font-semibold mb-2">Uninstall {app.display_name}?</h2>
              <p className="text-sm text-muted-foreground mb-4">
                This removes the app and its tools. App database tables are preserved.
              </p>
              <div className="flex justify-end gap-2">
                <button
                  onClick={() => setConfirmUninstall(null)}
                  className="px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
                >
                  Cancel
                </button>
                <button
                  onClick={() => handleUninstall(app)}
                  disabled={uninstallingId === app.id}
                  className="flex items-center gap-2 px-3 py-1.5 rounded-md bg-destructive text-destructive-foreground text-sm font-medium hover:bg-destructive/90 transition-colors disabled:opacity-50"
                >
                  {uninstallingId === app.id && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                  Uninstall
                </button>
              </div>
            </div>
          </div>
        );
      })()}
    </>
  );
}

// ─── Sidebar section definitions ─────────────────────────────────────────────

type Section = 'apps' | 'skills' | 'blueprints';

const SECTIONS: { id: Section; icon: React.ElementType; label: string }[] = [
  { id: 'apps',       icon: Package,  label: 'Apps' },
  { id: 'skills',     icon: Sparkles, label: 'Skills' },
  { id: 'blueprints', icon: Layers,   label: 'Blueprints' },
];

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function AppsPage() {
  const [section, setSection] = useState<Section>('apps');

  return (
    <ErrorBoundary>
      <div className="flex h-full min-h-0">
        {/* Left sidebar */}
        <div className="w-40 shrink-0 border-r border-border">
          <nav className="flex flex-col gap-0.5 p-2">
            {SECTIONS.map(({ id, icon: Icon, label }) => (
              <button
                key={id}
                onClick={() => setSection(id)}
                className={cn(
                  'flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors w-full text-left',
                  section === id
                    ? 'bg-primary/10 text-primary'
                    : 'text-muted-foreground hover:text-foreground hover:bg-accent',
                )}
              >
                <Icon className="h-4 w-4 shrink-0" />
                {label}
              </button>
            ))}
          </nav>
        </div>

        {/* Content panel */}
        <div className="flex-1 min-w-0 overflow-y-auto p-6">
          {section === 'apps'       && <AppsContent />}
          {section === 'skills'     && <SkillsPage />}
          {section === 'blueprints' && <MarketplacePage />}
        </div>
      </div>
    </ErrorBoundary>
  );
}
