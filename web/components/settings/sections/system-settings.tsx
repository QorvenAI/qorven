'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import { Loader2, Monitor, ExternalLink, RefreshCw, ArrowUpCircle, CheckCircle2, AlertCircle } from 'lucide-react';
import { cn } from '@/lib/utils';
import { systemInfo, admin } from '@/lib/api';
import { Card, Btn, Input } from './primitives';
import { toast } from 'sonner';
import { clearToken } from '@/lib/api-core';

export function SystemSettings() {
  const [info, setInfo] = useState<any>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    systemInfo.get()
      .then(setInfo)
      .catch(() => setInfo({}))
      .finally(() => setLoading(false));
  }, []);

  const rows = [
    { label: 'Platform Version', value: info?.version,     mono: true },
    { label: 'Go Runtime',       value: info?.go_version,  mono: true },
    { label: 'Architecture',     value: info?.arch,        mono: true },
    { label: 'OS',               value: info?.os },
    { label: 'Environment',      value: info?.environment },
    { label: 'Uptime',           value: info?.uptime },
    { label: 'Local Models',     value: info?.local_ok === true ? 'Supported' : info?.local_ok === false ? 'Not supported' : undefined },
  ].filter(r => r.value !== undefined && r.value !== '');

  return (
    <div className="space-y-4">
      <Card id="system_info" title="System Information" description="Runtime details and platform diagnostics.">
        {loading ? (
          <div className="flex items-center gap-2 py-4">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            <span className="text-sm text-muted-foreground">Loading…</span>
          </div>
        ) : rows.length === 0 ? (
          <p className="text-sm text-muted-foreground py-2">System information not available — check backend connectivity.</p>
        ) : (
          <div className="space-y-2">
            {rows.map(({ label, value, mono }) => (
              <div key={label} className="flex items-center justify-between rounded-lg border border-border px-4 py-2.5">
                <p className="text-sm text-muted-foreground">{label}</p>
                <p className={cn('text-sm font-medium', mono && 'font-mono text-xs')}>{value}</p>
              </div>
            ))}
          </div>
        )}

        <a href="/system" target="_blank" rel="noreferrer"
          className="flex items-center gap-3 rounded-xl border border-border px-4 py-3 text-sm hover:bg-accent transition-colors group mt-2">
          <Monitor className="h-4 w-4 text-muted-foreground shrink-0" />
          <span className="flex-1 font-medium">System Dashboard</span>
          <ExternalLink className="h-3.5 w-3.5 text-muted-foreground group-hover:text-foreground transition-colors" />
        </a>
      </Card>

      <UpdateCard />
      <DangerZone />
    </div>
  );
}

// ─── Software Update ────────────────────────────────────────────────────────

type UpdateInfo = { current: string; latest: string; up_to_date: boolean; release_url: string; changelog_url: string };

function UpdateCard() {
  const [info, setInfo] = useState<UpdateInfo | null>(null);
  const [checking, setChecking] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [error, setError] = useState('');
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const check = async () => {
    setChecking(true);
    setError('');
    try {
      setInfo(await admin.checkUpdate());
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not reach update server');
    } finally {
      setChecking(false);
    }
  };

  useEffect(() => { check(); }, []);

  // Poll /health until server is back up, then reload the page.
  const waitForRestart = () => {
    setReconnecting(true);
    let attempts = 0;
    pollRef.current = setInterval(async () => {
      attempts++;
      try {
        const r = await fetch('/health', { cache: 'no-store' });
        if (r.ok) {
          clearInterval(pollRef.current!);
          window.location.reload();
        }
      } catch {
        // server still down — keep polling
      }
      if (attempts >= 40) { // 40 × 2s = 80s timeout
        clearInterval(pollRef.current!);
        setReconnecting(false);
        setError('Server did not restart within 80 s — check service logs.');
      }
    }, 2000);
  };

  useEffect(() => () => { if (pollRef.current) clearInterval(pollRef.current); }, []);

  const install = async () => {
    setInstalling(true);
    setError('');
    try {
      const res = await admin.installUpdate();
      if (res.restart) {
        toast.success(`Installing ${res.to} — restarting…`);
        // Give the server ~1 s to begin shutting down, then start polling.
        setTimeout(waitForRestart, 1000);
      } else {
        toast.success(res.message ?? 'Already up to date');
        await check();
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Install failed');
      setInstalling(false);
    }
    // Don't clear installing — the page will reload on successful restart.
  };

  const upToDate = info?.up_to_date ?? false;

  if (reconnecting) {
    return (
      <Card id="software_update" title="Software Update" description="Restarting with the new version…">
        <div className="flex flex-col items-center gap-3 py-6">
          <Loader2 className="h-8 w-8 animate-spin text-primary" />
          <p className="text-sm text-muted-foreground">Server is restarting — reconnecting…</p>
          <p className="text-xs text-muted-foreground">The page will reload automatically.</p>
        </div>
      </Card>
    );
  }

  return (
    <Card
      id="software_update"
      title="Software Update"
      description="Check for new Qorven releases and install updates."
      headerRight={
        <button
          onClick={check}
          disabled={checking}
          className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors disabled:opacity-40"
        >
          <RefreshCw className={cn('h-3.5 w-3.5', checking && 'animate-spin')} />
          {checking ? 'Checking…' : 'Check now'}
        </button>
      }
    >
      {checking && !info ? (
        <div className="flex items-center gap-2 py-3">
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          <span className="text-sm text-muted-foreground">Checking for updates…</span>
        </div>
      ) : error ? (
        <div className="flex items-center gap-2 py-3 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 shrink-0" />
          {error}
        </div>
      ) : info ? (
        <div className="space-y-3">
          {/* Version row */}
          <div className="flex items-center justify-between rounded-lg border border-border px-4 py-3">
            <div>
              <p className="text-sm font-medium">
                {upToDate ? 'Up to date' : `Update available: ${info.latest}`}
              </p>
              <p className="text-xs text-muted-foreground mt-0.5">
                Current: <span className="font-mono">{info.current}</span>
                {!upToDate && <> &nbsp;→&nbsp; Latest: <span className="font-mono">{info.latest}</span></>}
              </p>
            </div>
            {upToDate ? (
              <CheckCircle2 className="h-5 w-5 text-emerald-500 shrink-0" />
            ) : (
              <span className="flex h-2 w-2 rounded-full bg-amber-400 shrink-0" />
            )}
          </div>

          {/* Actions */}
          <div className="flex items-center gap-2">
            {!upToDate && (
              <Btn variant="primary" loading={installing} onClick={install}>
                <ArrowUpCircle className="h-3.5 w-3.5" />
                Install {info.latest}
              </Btn>
            )}
            <a
              href={info.changelog_url}
              target="_blank"
              rel="noreferrer"
              className="flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs font-medium hover:bg-accent transition-colors"
            >
              <ExternalLink className="h-3.5 w-3.5" />
              {upToDate ? 'Release notes' : 'Changelog'}
            </a>
          </div>
        </div>
      ) : null}
    </Card>
  );
}

// ─── Selective Reset ────────────────────────────────────────────────────────

const RESET_ACTIONS = [
  { target: 'sessions',      label: 'Sessions & Messages',  desc: 'all chat sessions and agent messages' },
  { target: 'tasks',         label: 'Tasks & Events',       desc: 'all tasks and their event logs' },
  { target: 'memories',      label: 'Memories',             desc: 'all memory embeddings and hierarchy' },
  { target: 'audit_log',     label: 'Audit Log',            desc: 'the complete audit trail' },
  { target: 'provider_keys', label: 'Provider Keys',        desc: 'all AI provider API keys and credentials' },
  { target: 'agents',        label: 'Custom Agents',        desc: 'all agents except Chief and Prime' },
] as const;

function DangerZone() {
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);
  const [resetting, setResetting] = useState<string | null>(null);
  const [showFactory, setShowFactory] = useState(false);

  const action = RESET_ACTIONS.find(a => a.target === confirmTarget);

  async function doReset(target: string) {
    setResetting(target);
    setConfirmTarget(null);
    try {
      const res = await admin.reset(target);
      toast.success(`Deleted ${res.deleted_rows} row(s) from ${target}`);
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : 'Reset failed');
    } finally {
      setResetting(null);
    }
  }

  return (
    <>
      <Card
        id="danger_zone"
        title="Danger Zone"
        description="Permanently delete data. These actions cannot be undone."
        headerRight={<span className="text-xs text-destructive font-medium">Admin only</span>}
      >
        <div className="rounded-lg border border-destructive/30 overflow-hidden divide-y divide-border">
          {RESET_ACTIONS.map(({ target, label, desc }) => (
            <div key={target} className="flex items-center justify-between gap-4 px-4 py-3">
              <div>
                <p className="text-sm font-medium">{label}</p>
                <p className="text-xs text-muted-foreground">Delete {desc}</p>
              </div>
              <Btn
                variant="danger"
                loading={resetting === target}
                onClick={() => setConfirmTarget(target)}
              >
                Reset
              </Btn>
            </div>
          ))}
        </div>
      </Card>

      <Card
        id="factory_reset"
        title="Factory Reset"
        description="Wipe all data and return to initial setup. This cannot be undone."
      >
        <div className="rounded-lg border border-destructive/40 bg-destructive/5 px-4 py-4 flex items-start justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-destructive">Nuclear option</p>
            <p className="text-xs text-muted-foreground mt-0.5">
              Drops the entire database, re-runs migrations, and wipes workspace files.
              The server remains running — next request will show the setup wizard.
            </p>
          </div>
          <Btn variant="danger" onClick={() => setShowFactory(true)}>
            Reset to Factory Defaults
          </Btn>
        </div>
      </Card>

      {confirmTarget && action && (
        <ConfirmDialog
          title={`Delete ${action.label}?`}
          body={`This permanently deletes ${action.desc}. This cannot be undone.`}
          onCancel={() => setConfirmTarget(null)}
          onConfirm={() => doReset(action.target)}
        />
      )}

      {showFactory && (
        <FactoryResetModal onClose={() => setShowFactory(false)} />
      )}
    </>
  );
}

// ─── ConfirmDialog ──────────────────────────────────────────────────────────

function ConfirmDialog({ title, body, onCancel, onConfirm }: {
  title: string; body: string; onCancel: () => void; onConfirm: () => void;
}) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onCancel();
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [onCancel, onConfirm]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onCancel}>
      <div role="alertdialog" className="w-full max-w-sm rounded-xl border border-border bg-card p-6 shadow-xl space-y-4" onClick={e => e.stopPropagation()}>
        <h2 className="text-base font-semibold">{title}</h2>
        <p className="text-sm text-muted-foreground">{body}</p>
        <div className="flex justify-end gap-2 pt-2">
          <Btn variant="ghost" onClick={onCancel}>Cancel</Btn>
          <Btn variant="danger" onClick={onConfirm}>Delete</Btn>
        </div>
      </div>
    </div>
  );
}

// ─── FactoryResetModal ──────────────────────────────────────────────────────

function FactoryResetModal({ onClose }: { onClose: () => void }) {
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !loading) onClose();
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [loading, onClose]);

  async function handleSubmit() {
    setError('');
    setLoading(true);
    try {
      await admin.factoryReset(password, confirm);
      clearToken();
      window.location.href = '/setup';
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Factory reset failed');
    } finally {
      setLoading(false);
    }
  }

  const canSubmit = password.length > 0 && confirm === 'RESET';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => { if (!loading) onClose(); }}>
      <div role="alertdialog" className="w-full max-w-sm rounded-xl border border-border bg-card p-6 shadow-xl space-y-4" onClick={e => e.stopPropagation()}>
        <div>
          <h2 className="text-base font-semibold">Factory Reset</h2>
          <p className="text-xs text-muted-foreground mt-1">
            This will wipe all data. The server will show the setup wizard on next load.
          </p>
        </div>

        <div className="space-y-3">
          <div>
            <label className="block text-xs font-medium mb-1">Admin Password</label>
            <Input
              type="password"
              value={password}
              onChange={setPassword}
              placeholder="Enter your password"
            />
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Type "RESET" to confirm</label>
            <Input
              type="text"
              value={confirm}
              onChange={setConfirm}
              placeholder="RESET"
            />
          </div>
        </div>

        {error && (
          <p className="text-sm text-destructive">{error}</p>
        )}

        <div className="flex justify-end gap-2 pt-2">
          <Btn variant="ghost" onClick={onClose} disabled={loading}>Cancel</Btn>
          <Btn variant="danger" onClick={handleSubmit} loading={loading} disabled={!canSubmit || loading}>
            Confirm Reset
          </Btn>
        </div>
      </div>
    </div>
  );
}
