'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

/**
 * /pairing — device/channel pairing (T2.11b).
 *
 * Pending requests come from the channels policy checker (an agent
 * wants to start listening on a chat; user must ok it). Approval
 * takes a code string. Paired devices are shown below so users know
 * what they've already authorized.
 */

import { useCallback, useEffect, useState } from 'react';
import {
  Smartphone, CheckCircle2, Loader2, AlertCircle, Monitor, ShieldCheck, RefreshCw,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { pairing, type PairingDevice, type PairingRequest } from '@/lib/api';

export default function PairingPage() {
  const [pending, setPending] = useState<PairingRequest[]>([]);
  const [devices, setDevices] = useState<PairingDevice[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setErr(null);
    const safe = <T,>(p: Promise<T>, fallback: T) => p.catch(() => fallback);
    const [p, d] = await Promise.all([
      safe(pairing.pending(), [] as PairingRequest[]),
      safe(pairing.devices(), [] as PairingDevice[]),
    ]);
    setPending(Array.isArray(p) ? p : []);
    setDevices(Array.isArray(d) ? d : []);
    setLoading(false);
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  return (
    <div className="mx-auto max-w-3xl space-y-5 p-4 lg:p-6">
      <header className="flex items-center gap-3">
        <ShieldCheck className="h-6 w-6 text-primary" />
        <h1 className="text-lg font-semibold">Pairing</h1>
        <p className="text-xs text-muted-foreground">
          Authorize new chat endpoints before agents can reach them.
        </p>
        <button
          onClick={refresh}
          disabled={loading}
          className="ml-auto inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-accent"
        >
          {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
          Refresh
        </button>
      </header>

      {err && (
        <div className="flex items-center gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
          <AlertCircle className="h-4 w-4" />
          <span>{err}</span>
        </div>
      )}

      <ManualApprove onApproved={refresh} onError={setErr} />

      <section>
        <h2 className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          <Smartphone className="h-3.5 w-3.5 text-amber-400" />
          Pending requests ({pending.length})
        </h2>
        {pending.length === 0 ? (
          <p className="rounded-xl border border-dashed border-border/60 bg-card/40 px-4 py-6 text-center text-2xs text-muted-foreground">
            No requests waiting. When an agent receives a message from an unknown
            chat, it lands here.
          </p>
        ) : (
          <ul className="space-y-2">
            {pending.map((p, i) => (
              <PendingRow key={`${p.code ?? ''}-${i}`} req={p} onResolved={refresh} onError={setErr} />
            ))}
          </ul>
        )}
      </section>

      <section>
        <h2 className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          <Monitor className="h-3.5 w-3.5 text-emerald-500" />
          Paired devices ({devices.length})
        </h2>
        {devices.length === 0 ? (
          <p className="rounded-xl border border-dashed border-border/60 bg-card/40 px-4 py-6 text-center text-2xs text-muted-foreground">
            None paired yet.
          </p>
        ) : (
          <ul className="divide-y divide-border/60 overflow-hidden rounded-xl border border-border bg-card">
            {devices.map((d) => (
              <li key={d.id} className="flex items-center gap-3 px-4 py-2 text-xs">
                <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-emerald-500" />
                <span className="rounded-sm bg-muted px-1.5 py-0.5 font-mono text-xs uppercase text-muted-foreground">
                  {d.channel_type}
                </span>
                <span className="min-w-0 flex-1 truncate font-mono text-muted-foreground">
                  {d.sender_id ?? d.chat_id ?? d.id}
                </span>
                <span className="shrink-0 text-2xs text-muted-foreground">
                  {d.paired_at ? new Date(d.paired_at).toLocaleDateString() : ''}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

function PendingRow({
  req,
  onResolved,
  onError,
}: {
  req: PairingRequest;
  onResolved: () => void;
  onError: (m: string | null) => void;
}) {
  const [busy, setBusy] = useState(false);
  const approve = async () => {
    const code = String(req.code ?? '');
    if (!code) return;
    setBusy(true);
    onError(null);
    try {
      await pairing.approve(code);
      onResolved();
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Approve failed');
      setBusy(false);
    }
  };
  return (
    <li className="rounded-xl border border-border bg-card p-3 text-xs">
      <div className="flex items-start gap-3">
        <Smartphone className="mt-0.5 h-4 w-4 shrink-0 text-amber-400" />
        <div className="min-w-0 flex-1 space-y-0.5">
          <div className="flex items-center gap-2 text-2xs">
            {req.channel_type && (
              <span className="rounded-sm bg-muted px-1.5 py-0.5 font-mono uppercase text-muted-foreground">
                {req.channel_type}
              </span>
            )}
            {req.agent_id && <span className="font-mono text-muted-foreground">agent {String(req.agent_id).slice(0, 8)}</span>}
            {req.requested_at && <span className="ml-auto text-muted-foreground">{new Date(req.requested_at).toLocaleString()}</span>}
          </div>
          {req.sender_id && <div className="font-mono text-muted-foreground">from {req.sender_id}</div>}
          {req.code && <div className="font-mono">code <span className="rounded-sm bg-primary/10 px-1.5 py-0.5 text-primary">{req.code}</span></div>}
        </div>
        <button
          onClick={approve}
          disabled={busy}
          className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
        >
          {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <CheckCircle2 className="h-3.5 w-3.5" />}
          Approve
        </button>
      </div>
    </li>
  );
}

function ManualApprove({
  onApproved,
  onError,
}: {
  onApproved: () => void;
  onError: (m: string | null) => void;
}) {
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!code.trim()) return;
    setBusy(true);
    onError(null);
    try {
      await pairing.approve(code.trim());
      setCode('');
      onApproved();
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Approve failed');
    } finally {
      setBusy(false);
    }
  };
  return (
    <form onSubmit={submit} className="flex gap-2 rounded-xl border border-border bg-card p-3">
      <input
        value={code}
        onChange={(e) => setCode(e.target.value)}
        placeholder="Enter pairing code"
        className="qr-input flex-1 text-xs h-7 py-0"
      />
      <button
        type="submit"
        disabled={busy || !code.trim()}
        className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
      >
        {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <ShieldCheck className="h-3.5 w-3.5" />}
        Approve code
      </button>
    </form>
  );
}
