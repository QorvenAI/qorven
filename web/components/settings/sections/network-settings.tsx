'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { Loader2, Network, Zap, Lock, X } from 'lucide-react';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';
import { networkApi, type NetworkStatus } from '@/lib/api';
import { Card } from './primitives';

export function NetworkSettings() {
  const [net, setNet] = useState<NetworkStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [authKey, setAuthKey] = useState('');
  const [busy, setBusy] = useState<'install' | 'bind' | 'unbind' | null>(null);

  const reload = () => {
    setLoading(true);
    networkApi.status().then(setNet).catch(() => setNet(null)).finally(() => setLoading(false));
  };
  useEffect(reload, []);

  const act = async (action: 'install' | 'bind' | 'unbind') => {
    setBusy(action);
    try {
      const res = await networkApi.tailscale(action, action === 'install' ? authKey : undefined);
      const status = ('status' in res && (res as any).status) ? (res as any).status as NetworkStatus : res as NetworkStatus;
      setNet(status);
      const msg = ('message' in res) ? String((res as any).message) : '';
      toast.success(msg || (action === 'install' ? 'Tailscale installed' : action === 'bind' ? `Bound to ${status.tailscale_ip}` : 'Unbound') + (action !== 'install' ? ' — restart service to apply' : ''));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'action failed');
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="space-y-4">
      <Card id="network_tailscale" title="Tailscale VPN binding"
        description="Restrict access to devices on your Tailscale network. No firewall rules required."
        headerRight={
          <button onClick={reload} className="text-muted-foreground hover:text-foreground transition-colors cursor-pointer">
            <Network className="h-4 w-4" />
          </button>
        }>
        {loading ? (
          <div className="flex items-center gap-2 py-2">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            <span className="text-sm text-muted-foreground">Checking Tailscale status…</span>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="flex items-center justify-between rounded-lg border border-border px-4 py-3">
              <div>
                <p className="text-sm font-medium">Tailscale</p>
                <p className="text-xs text-muted-foreground">
                  {net?.tailscale_installed
                    ? (net.tailscale_ip ? `Connected — ${net.tailscale_hostname || net.tailscale_ip}` : 'Installed, not connected')
                    : 'Not installed'}
                </p>
              </div>
              <span className={cn(
                'flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium',
                net?.tailscale_ip ? 'bg-emerald-500/10 text-emerald-500' : 'bg-muted text-muted-foreground'
              )}>
                {net?.tailscale_ip && <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-pulse inline-block" />}
                {net?.tailscale_ip ? net.tailscale_ip : 'offline'}
              </span>
            </div>

            <div className="flex items-center justify-between rounded-lg border border-border px-4 py-3">
              <div>
                <p className="text-sm font-medium">Bind mode</p>
                <p className="text-xs text-muted-foreground">{net?.web_listen || '—'}</p>
              </div>
              <span className={cn(
                'rounded-full px-3 py-1 text-xs font-medium',
                net?.bind_mode === 'tailscale' ? 'bg-violet-500/15 text-violet-400' :
                net?.bind_mode === 'localhost' ? 'bg-amber-500/10 text-amber-400' :
                'bg-muted text-muted-foreground'
              )}>
                {net?.bind_mode ?? '—'}
              </span>
            </div>

            {!net?.tailscale_installed && (
              <div className="space-y-3 pt-1">
                <div>
                  <label className="block text-xs font-medium text-muted-foreground mb-1.5">
                    Auth key <span className="text-muted-foreground/60">(optional — leave empty to authenticate in browser)</span>
                  </label>
                  <input
                    value={authKey} onChange={e => setAuthKey(e.target.value)}
                    placeholder="tskey-auth-..."
                    className="qr-input font-mono" />
                </div>
                <button onClick={() => act('install')} disabled={busy !== null}
                  className="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40 cursor-pointer">
                  {busy === 'install' ? <Loader2 className="h-4 w-4 animate-spin" /> : <Zap className="h-4 w-4" />}
                  Install Tailscale
                </button>
              </div>
            )}

            {net?.tailscale_installed && !net.tailscale_ip && (
              <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-300">
                Tailscale is installed but not connected to a network.
                Run <code className="font-mono text-xs">tailscale up</code> on the server then click refresh.
              </div>
            )}

            {net?.tailscale_installed && net.tailscale_ip && (
              <div className="flex gap-2 pt-1">
                {net.bind_mode !== 'tailscale' ? (
                  <button onClick={() => act('bind')} disabled={busy !== null}
                    className="inline-flex items-center gap-2 rounded-lg bg-violet-600 px-4 py-2 text-sm font-medium text-white hover:bg-violet-500 disabled:opacity-40 cursor-pointer">
                    {busy === 'bind' ? <Loader2 className="h-4 w-4 animate-spin" /> : <Lock className="h-4 w-4" />}
                    Bind to {net.tailscale_ip} (Tailscale only)
                  </button>
                ) : (
                  <button onClick={() => act('unbind')} disabled={busy !== null}
                    className="inline-flex items-center gap-2 rounded-lg border border-border px-4 py-2 text-sm text-muted-foreground hover:bg-accent disabled:opacity-40 cursor-pointer">
                    {busy === 'unbind' ? <Loader2 className="h-4 w-4 animate-spin" /> : <X className="h-4 w-4" />}
                    Remove Tailscale binding (make public)
                  </button>
                )}
              </div>
            )}

            {(net?.bind_mode === 'tailscale' || net?.bind_mode === 'public') && net.tailscale_ip && (
              <p className="text-xs text-muted-foreground">
                {net.bind_mode === 'tailscale'
                  ? 'Restart the Qorven service for the new binding to take effect.'
                  : 'After binding, restart the service: sudo systemctl restart qorven'}
              </p>
            )}
          </div>
        )}
      </Card>
    </div>
  );
}
