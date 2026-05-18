'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect } from 'react';
import { RefreshCw, ChevronRight, Clock } from 'lucide-react';
import { mail, type MailMessage } from '@/lib/api-content';
import { cn } from '@/lib/utils';

const DECISION_STYLES: Record<string, { bg: string; text: string; label: string }> = {
  auto_replied:     { bg: 'bg-emerald-500/10', text: 'text-emerald-600', label: 'Auto-replied' },
  drafted:          { bg: 'bg-blue-500/10',    text: 'text-blue-600',    label: 'Drafted' },
  pending_approval: { bg: 'bg-amber-500/10',   text: 'text-amber-600',   label: 'Awaiting approval' },
  logged:           { bg: 'bg-muted/50',        text: 'text-muted-foreground', label: 'Logged' },
  dropped:          { bg: 'bg-destructive/10',  text: 'text-destructive', label: 'Dropped' },
};

function decisionBadge(msg: MailMessage) {
  const d = msg.agent_decision ?? msg.status ?? '';
  const s = DECISION_STYLES[d] ?? { bg: 'bg-muted/50', text: 'text-muted-foreground', label: d || '—' };
  return (
    <span className={cn('rounded-full px-2 py-0.5 text-[10px] font-medium', s.bg, s.text)}>
      {s.label}
    </span>
  );
}

function relativeTime(iso: string) {
  const diff = Date.now() - new Date(iso).getTime();
  if (diff < 60_000) return 'just now';
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`;
  return new Date(iso).toLocaleDateString();
}

function ThreadView({ msg, onBack }: { msg: MailMessage; onBack: () => void }) {
  const [thread, setThread] = useState<MailMessage[]>([]);

  useEffect(() => {
    if (msg.thread_id) {
      mail.thread(msg.thread_id).then(setThread).catch(() => setThread([msg]));
    } else {
      setThread([msg]);
    }
  }, [msg]);

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b border-border px-4 py-3">
        <button onClick={onBack} className="rounded p-1 hover:bg-accent">
          <ChevronRight className="h-4 w-4 rotate-180" />
        </button>
        <div>
          <p className="text-sm font-medium">{msg.subject || '(no subject)'}</p>
          <p className="text-xs text-muted-foreground">{msg.from_address}</p>
        </div>
      </div>
      <div className="flex-1 space-y-3 overflow-y-auto p-4">
        {thread.map((m) => (
          <div
            key={m.id}
            className={cn(
              'max-w-[80%] rounded-xl px-4 py-3 text-sm',
              m.direction === 'inbound'
                ? 'mr-auto bg-muted/40'
                : 'ml-auto bg-primary/10 text-primary-foreground/90'
            )}
          >
            <p className="mb-1 text-[11px] font-medium text-muted-foreground">
              {m.direction === 'inbound' ? m.from_address : 'You (agent)'} · {relativeTime(m.created_at)}
            </p>
            <p className="whitespace-pre-wrap leading-relaxed">{m.body_text ?? '(no content)'}</p>
            {m.agent_decision === 'pending_approval' && (
              <div className="mt-3 rounded-lg border border-amber-300/50 bg-amber-50/50 p-3 text-xs dark:bg-amber-900/20">
                <p className="mb-2 font-semibold text-amber-700 dark:text-amber-400">Draft pending approval</p>
                <div className="flex gap-2">
                  <button className="rounded bg-primary px-3 py-1 text-[11px] text-primary-foreground hover:bg-primary/90">
                    Approve &amp; Send
                  </button>
                  <button className="rounded border border-border px-3 py-1 text-[11px] hover:bg-accent">
                    Edit
                  </button>
                  <button className="rounded border border-destructive/50 px-3 py-1 text-[11px] text-destructive hover:bg-destructive/10">
                    Discard
                  </button>
                </div>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

export function MailInbox({ agentId }: { agentId: string }) {
  const [messages, setMessages] = useState<MailMessage[]>([]);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<MailMessage | null>(null);

  const load = () => {
    setLoading(true);
    mail.inbox(agentId)
      .then(setMessages)
      .catch(() => setMessages([]))
      .finally(() => setLoading(false));
  };

  useEffect(load, [agentId]);

  if (selected) {
    return <ThreadView msg={selected} onBack={() => setSelected(null)} />;
  }

  const pending = messages.filter((m) => m.agent_decision === 'pending_approval');
  const rest = messages.filter((m) => m.agent_decision !== 'pending_approval');

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <p className="text-sm font-medium text-muted-foreground">
          {loading ? 'Loading…' : `${messages.length} message${messages.length !== 1 ? 's' : ''}`}
        </p>
        <button onClick={load} className="rounded p-1 hover:bg-accent" disabled={loading}>
          <RefreshCw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
        </button>
      </div>

      {messages.length === 0 && !loading && (
        <div className="flex flex-1 flex-col items-center justify-center text-sm text-muted-foreground">
          <Clock className="mb-2 h-8 w-8 opacity-30" />
          No messages yet. Configure a mailbox in Setup.
        </div>
      )}

      <div className="divide-y divide-border">
        {pending.length > 0 && (
          <>
            <p className="bg-amber-50/50 px-4 py-1.5 text-[11px] font-semibold uppercase tracking-wide text-amber-600 dark:bg-amber-900/20">
              Awaiting Approval
            </p>
            {pending.map((m) => (
              <MessageRow key={m.id} msg={m} onClick={() => setSelected(m)} highlight />
            ))}
          </>
        )}
        {rest.map((m) => (
          <MessageRow key={m.id} msg={m} onClick={() => setSelected(m)} />
        ))}
      </div>
    </div>
  );
}

function MessageRow({
  msg, onClick, highlight,
}: { msg: MailMessage; onClick: () => void; highlight?: boolean }) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-accent/50',
        highlight && 'border-l-2 border-amber-400',
        !msg.is_read && 'font-medium'
      )}
    >
      <div className="flex-1 min-w-0">
        <div className="flex items-center justify-between gap-2">
          <span className="truncate text-sm">{msg.from_address}</span>
          <span className="shrink-0 text-[11px] text-muted-foreground">{relativeTime(msg.created_at)}</span>
        </div>
        <p className="truncate text-xs text-muted-foreground">{msg.subject || '(no subject)'}</p>
      </div>
      <div className="shrink-0">{decisionBadge(msg)}</div>
    </button>
  );
}
