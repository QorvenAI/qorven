'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, Database, Loader2, WifiOff, X } from 'lucide-react';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';

const GRACE_MS = 5000;
const DISMISS_MS = 10000;

function disconnectMessage(
  reason: 'backend_down' | 'db_down' | 'degraded' | 'network' | null,
  attempt: number,
): { icon: React.ReactNode; text: string } {
  const retryNote = attempt > 1 ? ` (attempt ${attempt})` : '';
  switch (reason) {
    case 'backend_down':
      return { icon: <AlertTriangle className="h-3.5 w-3.5" />, text: `Backend unreachable — reconnecting${retryNote}…` };
    case 'db_down':
      return { icon: <Database className="h-3.5 w-3.5" />, text: `Database offline — backend degraded, retrying${retryNote}…` };
    case 'network':
      return { icon: <WifiOff className="h-3.5 w-3.5" />, text: `No network connection — will retry when online…` };
    case 'degraded':
      return { icon: <AlertTriangle className="h-3.5 w-3.5" />, text: `Backend degraded — reconnecting${retryNote}…` };
    default:
      return { icon: <AlertTriangle className="h-3.5 w-3.5" />, text: `Connection lost. Reconnecting${retryNote}…` };
  }
}

export function ReconnectBanner() {
  const wsConnected = useStore((s) => s.wsConnected);
  const attempt = useStore((s) => s.wsReconnectAttempt);
  const disconnectedAt = useStore((s) => s.wsLastDisconnectAt);
  const disconnectReason = useStore((s) => s.wsDisconnectReason);
  const serviceHealth = useStore((s) => s.serviceHealth);

  const [visible, setVisible] = useState<'disconnected' | 'recovered' | null>(null);

  // Manage disconnected/recovered state transitions
  useEffect(() => {
    if (!wsConnected && disconnectedAt) {
      const elapsed = Date.now() - disconnectedAt;
      const remaining = Math.max(0, GRACE_MS - elapsed);
      const t = setTimeout(() => setVisible('disconnected'), remaining);
      return () => clearTimeout(t);
    }
    if (wsConnected && visible === 'disconnected') {
      setVisible('recovered');
    }
  }, [wsConnected, disconnectedAt, visible]);

  // Auto-dismiss after reconnect: 10s when window is active, wait if hidden
  useEffect(() => {
    if (visible !== 'recovered') return;

    let timer: ReturnType<typeof setTimeout> | null = null;

    const startTimer = () => {
      timer = setTimeout(() => setVisible(null), DISMISS_MS);
    };

    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        document.removeEventListener('visibilitychange', onVisibilityChange);
        startTimer();
      }
    };

    if (document.visibilityState === 'visible') {
      startTimer();
    } else {
      document.addEventListener('visibilitychange', onVisibilityChange);
    }

    return () => {
      if (timer) clearTimeout(timer);
      document.removeEventListener('visibilitychange', onVisibilityChange);
    };
  }, [visible]);

  const showDegraded = wsConnected && serviceHealth.status === 'degraded';

  if (!visible && !showDegraded) return null;

  if (showDegraded && visible !== 'disconnected') {
    return (
      <div
        role="status"
        className={cn(
          'fixed top-0 left-0 right-0 z-50 flex items-center justify-center gap-2 px-4 py-1.5 text-xs font-medium shadow-md',
          'animate-in fade-in slide-in-from-top-2 duration-300',
          'bg-orange-500/95 text-orange-950',
        )}
      >
        <Database className="h-3.5 w-3.5" />
        <span>Database offline — some features may be unavailable. Recovering…</span>
      </div>
    );
  }

  const isDown = visible === 'disconnected';
  const { icon, text } = disconnectMessage(disconnectReason, attempt);

  return (
    <div
      role="status"
      onClick={!isDown ? () => setVisible(null) : undefined}
      className={cn(
        'fixed top-0 left-0 right-0 z-50 flex items-center justify-center gap-2 px-4 py-1.5 text-xs font-medium shadow-md',
        'animate-in fade-in slide-in-from-top-2 duration-300',
        !isDown && 'cursor-pointer select-none',
        isDown ? 'bg-amber-500/95 text-amber-950' : 'bg-emerald-500/95 text-emerald-950',
      )}
    >
      {isDown ? (
        <>
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          {icon}
          <span>{text}</span>
        </>
      ) : (
        <>
          <CheckCircle2 className="h-3.5 w-3.5" />
          <span>Reconnected</span>
          <X className="h-3 w-3 ml-1 opacity-60" />
        </>
      )}
    </div>
  );
}
