'use client';

import { useEffect, useState } from 'react';
import { channels as channelsApi } from '@/lib/api';
import type { PendingSender } from '@/lib/api-agents';
import { CheckCircle, XCircle, Loader2, Clock } from 'lucide-react';

interface WhatsAppSendersPanelProps {
  channelId: string;
}

export function WhatsAppSendersPanel({ channelId }: WhatsAppSendersPanelProps) {
  const [senders, setSenders] = useState<PendingSender[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<string | null>(null);

  const load = async () => {
    try {
      const data = await channelsApi.whatsapp.listPending(channelId);
      setSenders(data ?? []);
    } catch {}
    setLoading(false);
  };

  useEffect(() => {
    load();
    const t = setInterval(load, 10000);
    return () => clearInterval(t);
  }, [channelId]);

  const approve = async (id: string) => {
    setBusy(id);
    try {
      await channelsApi.whatsapp.approve(channelId, id);
      setSenders((s) => s.filter((x) => x.id !== id));
    } catch {}
    setBusy(null);
  };

  const deny = async (id: string) => {
    setBusy(id);
    try {
      await channelsApi.whatsapp.deny(channelId, id);
      setSenders((s) => s.filter((x) => x.id !== id));
    } catch {}
    setBusy(null);
  };

  if (loading) {
    return (
      <div className="flex items-center gap-2 p-4 text-xs text-muted-foreground">
        <Loader2 className="h-3 w-3 animate-spin" /> Loading…
      </div>
    );
  }

  if (senders.length === 0) {
    return <p className="p-4 text-xs text-muted-foreground">No pending sender approvals.</p>;
  }

  return (
    <div className="space-y-2 p-4">
      <p className="text-xs font-medium text-muted-foreground">Pending WhatsApp Senders</p>
      <p className="text-xs text-muted-foreground">
        Share the OTP code with the user out-of-band. They must enter it in WhatsApp.
      </p>
      {senders.map((s) => {
        const isLocked = s.locked_until ? new Date(s.locked_until) > new Date() : false;
        return (
          <div key={s.id} className="rounded-lg border border-border bg-background/60 p-3 space-y-2">
            <div className="flex items-start justify-between gap-2">
              <div>
                <p className="text-xs font-medium">{s.display_name || s.sender_jid}</p>
                <p className="text-xs text-muted-foreground">{s.sender_jid}</p>
              </div>
              <div className="flex items-center gap-1.5">
                <button
                  onClick={() => approve(s.id)}
                  disabled={busy === s.id}
                  className="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20 disabled:opacity-50"
                >
                  {busy === s.id ? (
                    <Loader2 className="h-3 w-3 animate-spin" />
                  ) : (
                    <CheckCircle className="h-3 w-3" />
                  )}
                  Approve
                </button>
                <button
                  onClick={() => deny(s.id)}
                  disabled={busy === s.id}
                  className="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium bg-destructive/10 text-destructive hover:bg-destructive/20 disabled:opacity-50"
                >
                  <XCircle className="h-3 w-3" />
                  Deny
                </button>
              </div>
            </div>
            <div className={`flex items-center gap-2 rounded bg-muted/50 px-2 py-1.5 ${isLocked ? 'opacity-50' : ''}`}>
              <span className="text-xs text-muted-foreground">OTP:</span>
              <code className="text-sm font-mono font-bold tracking-widest text-foreground">
                {s.otp_code}
              </code>
              {isLocked && (
                <span className="ml-auto flex items-center gap-1 text-xs text-amber-400">
                  <Clock className="h-3 w-3" /> Locked
                </span>
              )}
              {!isLocked && s.attempts > 0 && (
                <span className="ml-auto text-xs text-muted-foreground">
                  {s.attempts}/3 attempts
                </span>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}
