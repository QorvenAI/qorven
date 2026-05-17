'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState, useEffect } from 'react';
import { RefreshCw, Send, CheckCircle, AlertCircle } from 'lucide-react';
import { mail, type MailMessage } from '@/lib/api-content';
import { cn } from '@/lib/utils';

const STATUS_STYLES: Record<string, { icon: React.ElementType; color: string; label: string }> = {
  delivered: { icon: CheckCircle,  color: 'text-emerald-500', label: 'Delivered' },
  sent:      { icon: CheckCircle,  color: 'text-emerald-500', label: 'Sent' },
  pending:   { icon: Send,         color: 'text-amber-500',   label: 'Pending' },
  failed:    { icon: AlertCircle,  color: 'text-destructive', label: 'Failed' },
};

function relativeTime(iso: string) {
  const diff = Date.now() - new Date(iso).getTime();
  if (diff < 60_000) return 'just now';
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`;
  return new Date(iso).toLocaleDateString();
}

export function MailSent({ agentId }: { agentId: string }) {
  const [messages, setMessages] = useState<MailMessage[]>([]);
  const [loading, setLoading] = useState(true);

  const load = () => {
    setLoading(true);
    mail.sent(agentId)
      .then(setMessages)
      .catch(() => setMessages([]))
      .finally(() => setLoading(false));
  };

  useEffect(load, [agentId]);

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <p className="text-sm font-medium text-muted-foreground">
          {loading ? 'Loading…' : `${messages.length} sent message${messages.length !== 1 ? 's' : ''}`}
        </p>
        <button onClick={load} className="rounded p-1 hover:bg-accent" disabled={loading}>
          <RefreshCw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
        </button>
      </div>

      {messages.length === 0 && !loading && (
        <div className="flex flex-1 flex-col items-center justify-center text-sm text-muted-foreground">
          <Send className="mb-2 h-8 w-8 opacity-30" />
          No sent messages yet.
        </div>
      )}

      <div className="divide-y divide-border overflow-y-auto">
        {messages.map((m) => {
          const fallback = { icon: CheckCircle as React.ElementType, color: 'text-emerald-500', label: 'Sent' };
          const s = STATUS_STYLES[m.status] ?? fallback;
          const Icon = s.icon;
          return (
            <div key={m.id} className="flex items-start gap-3 px-4 py-3">
              <Icon className={cn('mt-0.5 h-4 w-4 shrink-0', s.color)} />
              <div className="flex-1 min-w-0">
                <div className="flex items-center justify-between gap-2">
                  <span className="truncate text-sm font-medium">
                    To: {m.to_addresses?.join(', ') ?? 'Unknown'}
                  </span>
                  <span className="shrink-0 text-[11px] text-muted-foreground">{relativeTime(m.created_at)}</span>
                </div>
                <p className="truncate text-xs text-muted-foreground">{m.subject || '(no subject)'}</p>
                <span className={cn('text-[10px] font-medium', s.color)}>{s.label}</span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
