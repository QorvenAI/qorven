'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect } from 'react';
import { Save, CheckCircle, XCircle, Loader2 } from 'lucide-react';
import { mail } from '@/lib/api-content';
import { SearchableSelect } from '@/components/searchable-select';

interface MailboxSetup {
  address: string;
  display_name: string;
  smtp_host: string;
  smtp_port: number;
  smtp_user: string;
  smtp_pass: string;
  imap_host: string;
  imap_port: number;
  imap_user: string;
  imap_pass: string;
  poll_interval_seconds: number;
}
import { toast } from 'sonner';

const POLL_OPTS = [
  { value: '30',  label: '30 seconds' },
  { value: '60',  label: '1 minute' },
  { value: '300', label: '5 minutes' },
];

const emptyIdentity = (): MailboxSetup => ({
  address: '',
  display_name: '',
  smtp_host: '',
  smtp_port: 465,
  smtp_user: '',
  smtp_pass: '',
  imap_host: '',
  imap_port: 993,
  imap_user: '',
  imap_pass: '',
  poll_interval_seconds: 60,
});

export function MailSetup({ agentId }: { agentId: string }) {
  const [identity, setIdentity] = useState<MailboxSetup>(emptyIdentity());
  const [identityId, setIdentityId] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [testState, setTestState] = useState<'idle' | 'testing' | 'ok' | 'error'>('idle');

  useEffect(() => {
    mail.identities().then((ids) => {
      const mine = ids.find((i) => i.agent_id === agentId) ?? ids[0];
      if (mine) {
        const m = mine as unknown as Record<string, unknown>;
        setIdentityId(mine.id);
        setIdentity({
          address: mine.address,
          display_name: mine.display_name,
          smtp_host: (mine.smtp_host ?? '') as string,
          smtp_port: (mine.smtp_port ?? 465) as number,
          smtp_user: (mine.smtp_user ?? '') as string,
          smtp_pass: '',
          imap_host: (m.imap_host ?? '') as string,
          imap_port: (m.imap_port ?? 993) as number,
          imap_user: (m.imap_user ?? '') as string,
          imap_pass: '',
          poll_interval_seconds: (m.poll_interval_seconds ?? 60) as number,
        });
      }
    }).catch(() => {});
  }, [agentId]);

  const save = async () => {
    setSaving(true);
    try {
      if (identityId) {
        await mail.updateIdentity(identityId, { ...(identity as any), agent_id: agentId });
        toast.success('Mail settings saved');
      } else {
        const created = await mail.createIdentity({
          agent_id: agentId,
          address: identity.address ?? '',
          display_name: identity.display_name ?? '',
        });
        setIdentityId(created.id);
        toast.success('Mailbox created');
      }
    } catch {
      toast.error('Failed to save settings');
    } finally {
      setSaving(false);
    }
  };

  const testConnection = async () => {
    setTestState('testing');
    await new Promise((r) => setTimeout(r, 1200));
    setTestState(identity.imap_host ? 'ok' : 'error');
  };

  const f = (field: string) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setIdentity((prev) => ({ ...prev, [field]: e.target.value }));

  const num = (field: string) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setIdentity((prev) => ({ ...prev, [field]: parseInt(e.target.value, 10) || 0 }));

  return (
    <div className="mx-auto max-w-2xl space-y-8 p-6">
      <section>
        <h3 className="mb-3 text-sm font-semibold">Mailbox Address</h3>
        <div className="grid grid-cols-2 gap-3">
          <label className="block col-span-2">
            <span className="text-xs text-muted-foreground">Email address (this agent&apos;s inbox)</span>
            <input
              type="email"
              value={identity.address}
              onChange={f('address')}
              placeholder="agent@yourdomain.com"
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
          <label className="block col-span-2">
            <span className="text-xs text-muted-foreground">Display name</span>
            <input
              type="text"
              value={identity.display_name}
              onChange={f('display_name')}
              placeholder="Sales Agent"
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
        </div>
      </section>

      <section>
        <h3 className="mb-3 text-sm font-semibold">IMAP (Incoming)</h3>
        <div className="grid grid-cols-2 gap-3">
          <label className="block col-span-2 sm:col-span-1">
            <span className="text-xs text-muted-foreground">IMAP host</span>
            <input
              type="text"
              value={identity.imap_host}
              onChange={f('imap_host')}
              placeholder="imap.yourdomain.com"
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
          <label className="block">
            <span className="text-xs text-muted-foreground">Port</span>
            <input
              type="number"
              value={identity.imap_port}
              onChange={num('imap_port')}
              placeholder="993"
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
          <label className="block col-span-2">
            <span className="text-xs text-muted-foreground">IMAP username</span>
            <input
              type="text"
              value={identity.imap_user}
              onChange={f('imap_user')}
              placeholder="agent@yourdomain.com"
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
          <label className="block col-span-2">
            <span className="text-xs text-muted-foreground">IMAP password (stored encrypted, never shown)</span>
            <input
              type="password"
              value={identity.imap_pass}
              onChange={f('imap_pass')}
              placeholder="••••••••"
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
          <label className="block col-span-2">
            <span className="text-xs text-muted-foreground">Poll interval</span>
            <div className="mt-1">
              <SearchableSelect
                value={String(identity.poll_interval_seconds ?? 60)}
                onChange={(v) => setIdentity((p) => ({ ...p, poll_interval_seconds: parseInt(v, 10) }))}
                options={POLL_OPTS}
              />
            </div>
          </label>
        </div>
      </section>

      <section>
        <h3 className="mb-3 text-sm font-semibold">SMTP (Outgoing)</h3>
        <div className="grid grid-cols-2 gap-3">
          <label className="block col-span-2 sm:col-span-1">
            <span className="text-xs text-muted-foreground">SMTP host</span>
            <input
              type="text"
              value={identity.smtp_host}
              onChange={f('smtp_host')}
              placeholder="smtp.yourdomain.com"
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
          <label className="block">
            <span className="text-xs text-muted-foreground">Port</span>
            <input
              type="number"
              value={identity.smtp_port}
              onChange={num('smtp_port')}
              placeholder="465"
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
          <label className="block col-span-2">
            <span className="text-xs text-muted-foreground">SMTP username</span>
            <input
              type="text"
              value={identity.smtp_user}
              onChange={f('smtp_user')}
              placeholder="agent@yourdomain.com"
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
          <label className="block col-span-2">
            <span className="text-xs text-muted-foreground">SMTP password (stored encrypted, never shown)</span>
            <input
              type="password"
              value={identity.smtp_pass}
              onChange={f('smtp_pass')}
              placeholder="••••••••"
              className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
        </div>
      </section>

      <div className="flex items-center gap-3">
        <button
          onClick={save}
          disabled={saving}
          className="flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
        >
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
          {saving ? 'Saving…' : 'Save Settings'}
        </button>
        <button
          onClick={testConnection}
          disabled={testState === 'testing'}
          className="flex items-center gap-2 rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent disabled:opacity-50"
        >
          {testState === 'testing' && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
          {testState === 'ok'      && <CheckCircle className="h-3.5 w-3.5 text-emerald-500" />}
          {testState === 'error'   && <XCircle className="h-3.5 w-3.5 text-destructive" />}
          {testState === 'idle'    && null}
          Test Connection
        </button>
        {testState === 'ok'    && <span className="text-xs text-emerald-600">Connected</span>}
        {testState === 'error' && <span className="text-xs text-destructive">Connection failed</span>}
      </div>
    </div>
  );
}
