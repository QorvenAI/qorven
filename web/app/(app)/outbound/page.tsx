'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * /outbound — human gate for agent-initiated outbound actions (T2.4).
 *
 * Two backend queues feed this page:
 *   • /outbound/pending — generic actions (telegram, social, webhook,
 *     sometimes email). Approve may execute inline (status becomes
 *     "approved_and_sent" and a `result` comes back).
 *   • /approvals/mail — dedicated mail-store queue. Shape differs
 *     enough that it's a separate section rather than merged in the
 *     same list. Approve just flips status to "approved" — the mail
 *     store picks it up async.
 *
 * We render both under one page because they're the same human
 * decision: "should this leave the building?"
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ShieldAlert, CheckCircle2, XCircle, Loader2, RefreshCw, Clock, Mail,
  MessageSquare, Globe, Webhook, Megaphone, AlertTriangle,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import {
  outbound,
  type OutboundAction,
  type OutboundActionKind,
  type MailApproval,
} from '@/lib/api';
import { useStore } from '@/store';

const KIND_META: Record<string, { icon: typeof Mail; label: string; tone: string }> = {
  email_send:    { icon: Mail,          label: 'Email',    tone: 'text-blue-400' },
  telegram_send: { icon: MessageSquare, label: 'Telegram', tone: 'text-cyan-400' },
  social_post:   { icon: Megaphone,     label: 'Social',   tone: 'text-fuchsia-400' },
  webhook:       { icon: Webhook,       label: 'Webhook',  tone: 'text-emerald-400' },
};
const kindMeta = (k: OutboundActionKind) => KIND_META[k] ?? { icon: Globe, label: k, tone: 'text-muted-foreground' };

export default function OutboundPage() {
  const [actions, setActions] = useState<OutboundAction[]>([]);
  const [mailApprovals, setMailApprovals] = useState<MailApproval[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setErr(null);
    const safe = <T,>(p: Promise<T>, fallback: T) => p.catch(() => fallback);
    const [a, m] = await Promise.all([
      safe(outbound.pending(), { pending: [] as OutboundAction[] }),
      safe(outbound.mailPending(), [] as MailApproval[]),
    ]);
    setActions(a.pending ?? []);
    setMailApprovals(Array.isArray(m) ? m : []);
    setLoading(false);
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  const total = actions.length + mailApprovals.length;

  return (
    <div className="mx-auto max-w-5xl space-y-5 p-4 lg:p-6">
      <header className="flex items-start gap-3">
        <ShieldAlert className="h-6 w-6 text-amber-400" />
        <div>
          <h1 className="text-lg font-semibold">Outbound</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Everything an agent wants to send outside the platform lands here
            first. Approve to release · reject to drop.
          </p>
        </div>
        <span className="ml-auto rounded-md border border-border bg-muted/30 px-2 py-0.5 font-mono text-xs">
          {loading ? '…' : total} pending
        </span>
        <button
          onClick={refresh}
          disabled={loading}
          className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-accent disabled:opacity-60"
        >
          {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="h-3.5 w-3.5" />}
          Refresh
        </button>
      </header>

      {err && <ErrorBanner message={err} />}

      {!loading && total === 0 && (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border/60 bg-card/40 px-6 py-12 text-center">
          <CheckCircle2 className="h-8 w-8 text-emerald-500/70" />
          <div>
            <p className="text-sm font-medium">Inbox clear</p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              No agent has anything waiting on a human decision right now.
            </p>
          </div>
        </div>
      )}

      {actions.length > 0 && (
        <section className="space-y-2">
          <h2 className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            <ShieldAlert className="h-3.5 w-3.5 text-amber-400" />
            Actions ({actions.length})
          </h2>
          <ul className="space-y-2">
            {actions.map((a) => (
              <ActionRow key={a.id} action={a} onResolved={refresh} />
            ))}
          </ul>
        </section>
      )}

      {mailApprovals.length > 0 && (
        <section className="space-y-2">
          <h2 className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            <Mail className="h-3.5 w-3.5 text-blue-400" />
            Mail approvals ({mailApprovals.length})
          </h2>
          <ul className="space-y-2">
            {mailApprovals.map((m) => (
              <MailRow key={m.id} item={m} onResolved={refresh} />
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

// ───────────────────────────────────────────────────────────────────

function ActionRow({ action, onResolved }: { action: OutboundAction; onResolved: () => void }) {
  const souls = useStore((s) => s.souls);
  const soul = souls.find((s) => s.id === action.agent_id);
  const { icon: Icon, label, tone } = kindMeta(action.action_type);
  const [notes, setNotes] = useState('');
  const [busy, setBusy] = useState<'approve' | 'reject' | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [resultMsg, setResultMsg] = useState<string | null>(null);

  const act = async (kind: 'approve' | 'reject') => {
    setBusy(kind);
    setErr(null);
    try {
      if (kind === 'approve') {
        const res = await outbound.approve(action.id, notes.trim() || undefined);
        if (res.status === 'approved_and_sent') {
          setResultMsg('Approved and sent.');
        } else {
          setResultMsg('Approved.');
        }
      } else {
        await outbound.reject(action.id, notes.trim() || undefined);
        setResultMsg('Rejected.');
      }
      setTimeout(onResolved, 600);
    } catch (e) {
      setErr(e instanceof Error ? e.message : `${kind} failed`);
      setBusy(null);
    }
  };

  return (
    <li className="rounded-xl border border-border bg-card p-4">
      <div className="flex items-start gap-3">
        <Icon className={cn('mt-0.5 h-4 w-4 shrink-0', tone)} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2 text-2xs">
            <span className="rounded-sm bg-muted px-1.5 py-0.5 font-mono uppercase text-muted-foreground">
              {label}
            </span>
            <span className="text-muted-foreground">
              {soul?.display_name ?? action.agent_id.slice(0, 8)}
            </span>
            <span className="text-muted-foreground">·</span>
            <span className="flex items-center gap-1 text-muted-foreground">
              <Clock className="h-3 w-3" />
              {relTime(action.requested_at)}
            </span>
            {action.expires_at && (
              <>
                <span className="text-muted-foreground">·</span>
                <span className="text-amber-400">expires {relTime(action.expires_at)}</span>
              </>
            )}
          </div>

          <PayloadView kind={action.action_type} payload={action.payload} />

          {resultMsg ? (
            <p className="mt-3 rounded-md border border-emerald-500/40 bg-emerald-500/5 px-2 py-1 text-2xs text-emerald-400">
              {resultMsg}
            </p>
          ) : (
            <div className="mt-3 flex items-center gap-2">
              <input
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                placeholder="Review notes (optional)"
                className="qr-input flex-1 text-xs h-7 py-0"
              />
              <button
                onClick={() => act('approve')}
                disabled={!!busy}
                className="inline-flex items-center gap-1.5 rounded-md border border-emerald-500/40 bg-emerald-500/10 px-3 py-1 text-xs font-medium text-emerald-500 hover:bg-emerald-500/20 disabled:opacity-50"
              >
                {busy === 'approve' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <CheckCircle2 className="h-3.5 w-3.5" />}
                Approve
              </button>
              <button
                onClick={() => act('reject')}
                disabled={!!busy}
                className="inline-flex items-center gap-1.5 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-1 text-xs font-medium text-destructive hover:bg-destructive/20 disabled:opacity-50"
              >
                {busy === 'reject' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <XCircle className="h-3.5 w-3.5" />}
                Reject
              </button>
            </div>
          )}

          {err && <p className="mt-1 text-2xs text-destructive">{err}</p>}
        </div>
      </div>
    </li>
  );
}

function MailRow({ item, onResolved }: { item: MailApproval; onResolved: () => void }) {
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState<'approve' | 'reject' | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const act = async (kind: 'approve' | 'reject') => {
    setBusy(kind);
    setErr(null);
    try {
      if (kind === 'approve') await outbound.mailApprove(item.id);
      else await outbound.mailReject(item.id, reason.trim() || undefined);
      setTimeout(onResolved, 400);
    } catch (e) {
      setErr(e instanceof Error ? e.message : `${kind} failed`);
      setBusy(null);
    }
  };

  const to = Array.isArray(item.to) ? item.to.join(', ') : (item.to ?? '(no recipient)');

  return (
    <li className="rounded-xl border border-border bg-card p-4">
      <div className="flex items-start gap-3">
        <Mail className="mt-0.5 h-4 w-4 shrink-0 text-blue-400" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2 text-2xs">
            <span className="rounded-sm bg-muted px-1.5 py-0.5 font-mono uppercase text-muted-foreground">
              Mail
            </span>
            <span className="font-mono text-muted-foreground">→ {to}</span>
            {item.created_at && (
              <>
                <span className="text-muted-foreground">·</span>
                <span className="text-muted-foreground">{relTime(item.created_at)}</span>
              </>
            )}
          </div>
          <p className="mt-1 text-sm font-medium">
            {typeof item.subject === 'string' && item.subject ? item.subject : '(no subject)'}
          </p>
          {typeof item.body === 'string' && item.body && (
            <p className="mt-1 line-clamp-4 whitespace-pre-wrap text-xs leading-relaxed text-muted-foreground">
              {item.body}
            </p>
          )}
          <div className="mt-3 flex items-center gap-2">
            <input
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Reason (only used on reject)"
              className="qr-input flex-1 text-xs h-7 py-0"
            />
            <button
              onClick={() => act('approve')}
              disabled={!!busy}
              className="inline-flex items-center gap-1.5 rounded-md border border-emerald-500/40 bg-emerald-500/10 px-3 py-1 text-xs font-medium text-emerald-500 hover:bg-emerald-500/20 disabled:opacity-50"
            >
              {busy === 'approve' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <CheckCircle2 className="h-3.5 w-3.5" />}
              Approve
            </button>
            <button
              onClick={() => act('reject')}
              disabled={!!busy}
              className="inline-flex items-center gap-1.5 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-1 text-xs font-medium text-destructive hover:bg-destructive/20 disabled:opacity-50"
            >
              {busy === 'reject' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <XCircle className="h-3.5 w-3.5" />}
              Reject
            </button>
          </div>
          {err && <p className="mt-1 text-2xs text-destructive">{err}</p>}
        </div>
      </div>
    </li>
  );
}

// ───────────────────────────────────────────────────────────────────
// Payload renderers. Each action type has a lightly-opinionated view
// so the human isn't just staring at raw JSON. Unknown types fall
// back to a pretty-printed JSON block, capped at 800 chars.
// ───────────────────────────────────────────────────────────────────
function PayloadView({ kind, payload }: { kind: string; payload: unknown }) {
  const p = (payload ?? {}) as Record<string, unknown>;
  if (kind === 'email_send') {
    return (
      <div className="mt-2 space-y-1 text-xs">
        <div>
          <span className="text-muted-foreground">To:</span>{' '}
          <span className="font-mono">{String(p.to ?? '—')}</span>
        </div>
        <div>
          <span className="text-muted-foreground">Subject:</span>{' '}
          <span className="font-medium">{String(p.subject ?? '—')}</span>
        </div>
        {typeof p.body === 'string' && (
          <p className="mt-1 line-clamp-4 whitespace-pre-wrap leading-relaxed text-muted-foreground">
            {p.body}
          </p>
        )}
      </div>
    );
  }
  if (kind === 'telegram_send') {
    return (
      <div className="mt-2 space-y-1 text-xs">
        <div>
          <span className="text-muted-foreground">To:</span>{' '}
          <span className="font-mono">{String(p.chat_id ?? p.to ?? '—')}</span>
        </div>
        {typeof p.text === 'string' && (
          <p className="mt-1 line-clamp-4 whitespace-pre-wrap leading-relaxed">{p.text}</p>
        )}
      </div>
    );
  }
  if (kind === 'social_post') {
    return (
      <div className="mt-2 space-y-1 text-xs">
        <div>
          <span className="text-muted-foreground">Platform:</span>{' '}
          <span className="font-mono">{String(p.platform ?? p.provider ?? '—')}</span>
        </div>
        {typeof p.content === 'string' && (
          <p className="mt-1 line-clamp-5 whitespace-pre-wrap leading-relaxed">{p.content}</p>
        )}
      </div>
    );
  }
  if (kind === 'webhook') {
    return (
      <div className="mt-2 space-y-1 text-xs">
        <div className="flex items-center gap-1">
          <span className="font-mono font-semibold text-foreground">{String(p.method ?? 'POST')}</span>
          <span className="truncate font-mono text-muted-foreground">{String(p.url ?? '—')}</span>
        </div>
      </div>
    );
  }
  // Fallback
  const j = JSON.stringify(payload ?? {}, null, 2);
  const trimmed = j.length > 800 ? j.slice(0, 800) + '\n… [truncated]' : j;
  return (
    <pre className="mt-2 overflow-x-auto rounded-md border border-border/60 bg-background/50 px-2 py-1.5 font-mono text-2xs text-muted-foreground">
      {trimmed}
    </pre>
  );
}

function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
      <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
      <span>{message}</span>
    </div>
  );
}

function relTime(iso: string): string {
  if (!iso) return '—';
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return iso;
  const diff = Date.now() - t;
  if (diff < 60_000) return `${Math.round(diff / 1000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
  return new Date(iso).toLocaleDateString();
}
