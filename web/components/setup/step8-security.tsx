'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { CheckCircle2, Globe, Lock, ShieldCheck, Zap } from 'lucide-react';
import { cn } from '@/lib/utils';
import { networkApi, type NetworkStatus } from '@/lib/api';
import { QorvenSpinner, SectionTitle, LabeledInput } from './setup-atoms';

type AccessOption = 'direct' | 'domain' | 'tailscale';

export function Step8Security(p: {
  mode: 'auto'|'reverse-proxy'|'custom'|'disabled'; setMode: (v: 'auto'|'reverse-proxy'|'custom'|'disabled') => void;
  domain: string; setDomain: (v: string) => void;
  webPort: string; setWebPort: (v: string) => void;
}) {
  const [accessOption, setAccessOption] = useState<AccessOption>(() => {
    if (p.mode === 'reverse-proxy') return 'tailscale';
    if (p.domain) return 'domain';
    return 'direct';
  });
  const [tsAuthKey, setTsAuthKey] = useState('');
  const [tsBusy, setTsBusy] = useState(false);
  const [tsMsg, setTsMsg] = useState('');
  const [netStatus, setNetStatus] = useState<NetworkStatus | null>(null);

  useEffect(() => {
    networkApi.status().then(setNetStatus).catch(() => {});
  }, []);

  function selectOption(o: AccessOption) {
    setAccessOption(o);
    if (o === 'direct')    { p.setMode('disabled'); p.setDomain(''); }
    if (o === 'domain')    { p.setMode('auto'); }
    if (o === 'tailscale') { p.setMode('reverse-proxy'); p.setDomain(''); }
  }

  const doTailscale = async (action: 'install' | 'bind') => {
    setTsBusy(true); setTsMsg('');
    try {
      const res = await networkApi.tailscale(action, action === 'install' ? tsAuthKey : undefined);
      const status = ('status' in res && (res as any).status) ? (res as any).status as NetworkStatus : res as NetworkStatus;
      setNetStatus(status);
      const msg = ('message' in res) ? String((res as any).message) : '';
      setTsMsg(msg || (action === 'install' ? 'Tailscale installed — now click Bind' : `Bound to ${status.tailscale_ip}`));
    } catch (e) {
      setTsMsg(e instanceof Error ? e.message : 'failed');
    } finally {
      setTsBusy(false);
    }
  };

  const options: Array<{ id: AccessOption; label: string; hint: string; icon: React.ElementType }> = [
    { id: 'direct',    label: 'Direct IP (unsafe)',        hint: "HTTP only. Exposed on your server's public IP. Dev / LAN only.",    icon: Globe },
    { id: 'domain',    label: 'Domain + HTTPS',            hint: "Point a domain at this server. Qorven auto-obtains a Let's Encrypt cert.", icon: ShieldCheck },
    { id: 'tailscale', label: 'Tailscale / Reverse proxy', hint: 'VPN-only or behind Nginx/Caddy. No inbound firewall holes.',        icon: Lock },
  ];

  return (
    <div className="space-y-4">
      <SectionTitle icon={ShieldCheck} title="Security & access"
        subtitle="How will Qorven be accessible? You can change this later in Settings." />

      <div className="grid grid-cols-3 gap-2">
        {options.map(o => {
          const on = accessOption === o.id;
          return (
            <button key={o.id} onClick={() => selectOption(o.id)}
              className={cn('rounded-lg border px-3 py-3 text-left transition-colors',
                on ? 'border-primary bg-primary/10' : 'border-border hover:border-border/70')}>
              <div className="flex items-center gap-1.5 mb-1">
                <o.icon className={cn('h-3.5 w-3.5 shrink-0', on ? 'text-primary' : 'text-muted-foreground')} />
                <div className={cn('text-xs font-medium leading-tight', on && 'text-primary')}>{o.label}</div>
              </div>
              <div className="text-xs text-muted-foreground leading-snug">{o.hint}</div>
            </button>
          );
        })}
      </div>

      {accessOption === 'direct' && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">
          Plain HTTP — never expose this publicly. Only use on a private LAN or behind a VPN.
        </div>
      )}

      {accessOption === 'domain' && (
        <div className="space-y-2">
          <LabeledInput label="Your domain" value={p.domain} onChange={p.setDomain} placeholder="qorven.example.com" />
          <p className="text-xs text-muted-foreground">
            DNS must point to this server. Qorven will auto-obtain a Let&apos;s Encrypt cert on first start (port 80 must be reachable).
          </p>
        </div>
      )}

      {accessOption === 'tailscale' && (
        <div className="space-y-3">
          <div className="rounded-lg border border-border bg-muted/20 px-3 py-3 space-y-2.5">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium">Tailscale status</span>
              <span className={cn('text-xs', netStatus?.tailscale_installed ? 'text-emerald-400' : 'text-muted-foreground')}>
                {netStatus === null ? 'checking…' : netStatus.tailscale_installed
                  ? (netStatus.tailscale_ip ?? 'installed, not connected')
                  : 'not installed'}
              </span>
            </div>
            {!netStatus?.tailscale_installed && (
              <div className="space-y-2">
                <LabeledInput label="Auth key (optional)" value={tsAuthKey} onChange={setTsAuthKey} placeholder="tskey-auth-..." />
                <button onClick={() => doTailscale('install')} disabled={tsBusy}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40 cursor-pointer">
                  {tsBusy ? <QorvenSpinner className="h-3 w-3" /> : <Zap className="h-3 w-3" />}
                  Install Tailscale
                </button>
              </div>
            )}
            {netStatus?.tailscale_installed && netStatus.tailscale_ip && netStatus.bind_mode !== 'tailscale' && (
              <button onClick={() => doTailscale('bind')} disabled={tsBusy}
                className="inline-flex items-center gap-1.5 rounded-lg bg-violet-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-violet-500 disabled:opacity-40 cursor-pointer">
                {tsBusy ? <QorvenSpinner className="h-3 w-3" /> : <Lock className="h-3 w-3" />}
                Bind to {netStatus.tailscale_ip}
              </button>
            )}
            {netStatus?.bind_mode === 'tailscale' && (
              <div className="flex items-center gap-1.5 text-xs text-violet-400">
                <CheckCircle2 className="h-3 w-3" /> Bound to Tailscale — restart to apply
              </div>
            )}
            {tsMsg && (
              <p className={cn('text-xs', tsMsg.includes('fail') || tsMsg.includes('error') ? 'text-destructive' : 'text-emerald-400')}>{tsMsg}</p>
            )}
          </div>
          <p className="text-xs text-muted-foreground">
            If using Nginx/Caddy as a reverse proxy instead, just continue — no Tailscale setup needed.
          </p>
        </div>
      )}
    </div>
  );
}
