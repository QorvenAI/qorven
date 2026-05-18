'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import { channels as channelsApi } from '@/lib/api';
import { Loader2, CheckCircle } from 'lucide-react';

interface WhatsAppQRPanelProps {
  channelId: string;
}

export function WhatsAppQRPanel({ channelId }: WhatsAppQRPanelProps) {
  const [qr, setQr] = useState<string | null>(null);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    const url = channelsApi.whatsapp.qrStreamUrl(channelId);
    const es = new EventSource(url);
    esRef.current = es;

    es.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data);
        if (data.type === 'qr') {
          setQr(data.qr);
          setError(null);
        } else if (data.type === 'connected') {
          setConnected(true);
          setQr(null);
        }
      } catch {}
    };

    es.onerror = () => {
      setError('Connection to bridge lost. Retrying…');
    };

    return () => {
      es.close();
    };
  }, [channelId]);

  if (connected) {
    return (
      <div className="flex items-center gap-2 rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-4 text-sm text-emerald-400">
        <CheckCircle className="h-4 w-4 shrink-0" />
        WhatsApp connected successfully.
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">
        Open WhatsApp &rarr; Linked Devices &rarr; Link a Device &rarr; scan the QR code below.
        The code refreshes automatically every 20 seconds.
      </p>
      {error && <p className="text-xs text-destructive">{error}</p>}
      <div className="flex items-center justify-center rounded-lg border border-border bg-white p-4">
        {qr ? (
          <img src={qr} alt="WhatsApp QR code" className="h-56 w-56" />
        ) : (
          <div className="flex h-56 w-56 flex-col items-center justify-center gap-2 text-muted-foreground">
            <Loader2 className="h-6 w-6 animate-spin" />
            <span className="text-xs">Waiting for bridge&hellip;</span>
          </div>
        )}
      </div>
      <p className="text-center text-xs text-muted-foreground">
        Prefer a code?{' '}
        <code className="text-foreground">
          qorven channels whatsapp qr {channelId} --pairing-code
        </code>
      </p>
    </div>
  );
}
