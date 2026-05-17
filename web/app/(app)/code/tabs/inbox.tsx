'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useCallback, useEffect, useState } from 'react';
import {
  ShieldAlert, GitBranch, Mail, Globe, Megaphone, Webhook,
  MessageSquare, Wrench, Loader2, CheckCircle2, XCircle,
  AlertCircle, RefreshCw, RotateCcw, ChevronDown, ChevronUp,
  Play, Ban, CheckCheck,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import {
  plans as plansApi, approvals as approvalsApi, outbound,
  supervisor as supervisorApi, permissions,
  type Plan, type ApprovalItem, type OutboundAction, type MailApproval, type SupervisorMessage,
} from '@/lib/api';
import { useStore } from '@/store';

// ─── Types ────────────────────────────────────────────────────────────────────

type ItemKind = 'plan' | 'permission' | 'outbound' | 'mail' | 'escalation';

interface InboxItem {
  id: string;
  kind: ItemKind;
  createdAt: string;
  raw: Plan | ApprovalItem | OutboundAction | MailApproval | SupervisorMessage;
}

function relTime(iso: string): string {
  if (!iso) return '—';
  const diff = Date.now() - Date.parse(iso);
  if (!Number.isFinite(diff)) return iso;
  if (diff < 60_000) return `${Math.round(diff / 1_000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
  return new Date(iso).toLocaleDateString();
}

const KIND_META: Record<ItemKind, { icon: typeof ShieldAlert; label: string; cls: string }> = {
  plan:       { icon: GitBranch,   label: 'Plan gate',   cls: 'text-primary' },
  permission: { icon: ShieldAlert, label: 'Tool access',  cls: 'text-amber-500' },
  outbound:   { icon: Globe,       label: 'Outbound',     cls: 'text-emerald-500' },
  mail:       { icon: Mail,        label: 'Email',        cls: 'text-blue-400' },
  escalation: { icon: AlertCircle, label: 'Escalation',   cls: 'text-orange-400' },
};

const OUTBOUND_CHANNEL: Record<string, typeof Mail> = {
  email_send:    Mail,
  telegram_send: MessageSquare,
  social_post:   Megaphone,
  webhook:       Webhook,
};

// ─── Row components ───────────────────────────────────────────────────────────

function PlanRow({ item, onDone }: { item: InboxItem; onDone: () => void }) {
  const plan = item.raw as Plan;
  const [busy, setBusy] = useState<string | null>(null);
  const [comment, setComment] = useState('');
  const [expanded, setExpanded] = useState(false);

  const act = async (action: 'approve' | 'reject' | 'revise') => {
    setBusy(action);
    try {
      if (action === 'approve') await plansApi.approve(plan.id, comment || undefined);
      else if (action === 'reject') await plansApi.reject(plan.id, comment || 'Rejected');
      else await plansApi.revise(plan.id, comment || 'Please revise');
      onDone();
    } finally { setBusy(null); }
  };

  return (
    <div className="space-y-2">
      <p className="text-sm font-medium leading-snug">{plan.title || plan.id}</p>
      {plan.summary && <p className="text-xs text-muted-foreground line-clamp-2">{plan.summary}</p>}
      {expanded && (
        <textarea
          value={comment} onChange={e => setComment(e.target.value)}
          placeholder="Comment (optional)…"
          rows={2}
          className="qr-textarea text-xs resize-none"
        />
      )}
      <div className="flex items-center gap-1.5">
        <button onClick={() => act('approve')} disabled={!!busy}
          className="flex items-center gap-1 rounded-md bg-emerald-500/10 border border-emerald-500/30 text-emerald-600 text-xs font-medium px-2.5 py-1 hover:bg-emerald-500/20 disabled:opacity-50 transition-colors">
          {busy === 'approve' ? <Loader2 className="h-3 w-3 animate-spin" /> : <CheckCircle2 className="h-3 w-3" />}Approve
        </button>
        <button onClick={() => act('reject')} disabled={!!busy}
          className="flex items-center gap-1 rounded-md bg-destructive/10 border border-destructive/30 text-destructive text-xs font-medium px-2.5 py-1 hover:bg-destructive/20 disabled:opacity-50 transition-colors">
          {busy === 'reject' ? <Loader2 className="h-3 w-3 animate-spin" /> : <XCircle className="h-3 w-3" />}Reject
        </button>
        <button onClick={() => act('revise')} disabled={!!busy}
          className="flex items-center gap-1 rounded-md bg-muted border border-border text-muted-foreground text-xs font-medium px-2.5 py-1 hover:bg-accent disabled:opacity-50 transition-colors">
          {busy === 'revise' ? <Loader2 className="h-3 w-3 animate-spin" /> : <RotateCcw className="h-3 w-3" />}Revise
        </button>
        <button onClick={() => setExpanded(!expanded)} className="ml-auto text-muted-foreground hover:text-foreground transition-colors">
          {expanded ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
        </button>
      </div>
    </div>
  );
}

function PermissionRow({ item, onDone }: { item: InboxItem; onDone: () => void }) {
  const a = item.raw as ApprovalItem;
  const [busy, setBusy] = useState<string | null>(null);

  const act = async (decision: 'allow' | 'deny') => {
    setBusy(decision);
    try {
      await permissions.reply(a.id, { decision });
      onDone();
    } finally { setBusy(null); }
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <code className="rounded bg-muted px-1.5 py-0.5 text-xs font-mono font-semibold">{a.tool_name || 'unknown tool'}</code>
        {a.agent_id && <span className="text-xs text-muted-foreground">by agent {a.agent_id.slice(0, 8)}</span>}
      </div>
      {a.reason && <p className="text-xs text-muted-foreground">{a.reason}</p>}
      <div className="flex gap-1.5">
        <button onClick={() => act('allow')} disabled={!!busy}
          className="flex items-center gap-1 rounded-md bg-emerald-500/10 border border-emerald-500/30 text-emerald-600 text-xs font-medium px-2.5 py-1 hover:bg-emerald-500/20 disabled:opacity-50 transition-colors">
          {busy === 'allow' ? <Loader2 className="h-3 w-3 animate-spin" /> : <CheckCircle2 className="h-3 w-3" />}Allow
        </button>
        <button onClick={() => act('deny')} disabled={!!busy}
          className="flex items-center gap-1 rounded-md bg-destructive/10 border border-destructive/30 text-destructive text-xs font-medium px-2.5 py-1 hover:bg-destructive/20 disabled:opacity-50 transition-colors">
          {busy === 'deny' ? <Loader2 className="h-3 w-3 animate-spin" /> : <XCircle className="h-3 w-3" />}Deny
        </button>
      </div>
    </div>
  );
}

function OutboundRow({ item, onDone }: { item: InboxItem; onDone: () => void }) {
  const a = item.raw as OutboundAction;
  const [busy, setBusy] = useState<string | null>(null);
  const [note, setNote] = useState('');
  const ChannelIcon = OUTBOUND_CHANNEL[a.action_type] ?? Globe;
  const payload = a.payload as any;

  const act = async (action: 'approve' | 'reject') => {
    setBusy(action);
    try {
      if (action === 'approve') await outbound.approve(a.id, note);
      else await outbound.reject(a.id, note);
      onDone();
    } finally { setBusy(null); }
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <ChannelIcon className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
        <span className="text-xs font-medium">{a.action_type.replace('_', ' ')}</span>
        {payload?.to && <span className="text-xs text-muted-foreground truncate">→ {payload.to}</span>}
      </div>
      {payload?.subject && <p className="text-xs text-muted-foreground truncate">Subject: {payload.subject}</p>}
      {payload?.body && <p className="text-xs text-muted-foreground line-clamp-2 italic">{payload.body}</p>}
      <input value={note} onChange={e => setNote(e.target.value)}
        placeholder="Note (optional)…"
        className="qr-input text-xs h-7 py-0" />
      <div className="flex gap-1.5">
        <button onClick={() => act('approve')} disabled={!!busy}
          className="flex items-center gap-1 rounded-md bg-emerald-500/10 border border-emerald-500/30 text-emerald-600 text-xs font-medium px-2.5 py-1 hover:bg-emerald-500/20 disabled:opacity-50 transition-colors">
          {busy === 'approve' ? <Loader2 className="h-3 w-3 animate-spin" /> : <CheckCircle2 className="h-3 w-3" />}Send
        </button>
        <button onClick={() => act('reject')} disabled={!!busy}
          className="flex items-center gap-1 rounded-md bg-destructive/10 border border-destructive/30 text-destructive text-xs font-medium px-2.5 py-1 hover:bg-destructive/20 disabled:opacity-50 transition-colors">
          {busy === 'reject' ? <Loader2 className="h-3 w-3 animate-spin" /> : <Ban className="h-3 w-3" />}Block
        </button>
      </div>
    </div>
  );
}

function MailRow({ item, onDone }: { item: InboxItem; onDone: () => void }) {
  const m = item.raw as MailApproval;
  const [busy, setBusy] = useState<string | null>(null);

  const act = async (action: 'approve' | 'reject') => {
    setBusy(action);
    try {
      if (action === 'approve') await outbound.mailApprove(m.id);
      else await outbound.mailReject(m.id);
      onDone();
    } finally { setBusy(null); }
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-xs">
        <span className="font-medium truncate">{m.subject || '(no subject)'}</span>
        {m.to && <span className="text-muted-foreground shrink-0">→ {Array.isArray(m.to) ? m.to.join(', ') : m.to}</span>}
      </div>
      {m.body && <p className="text-xs text-muted-foreground line-clamp-2 italic">{m.body}</p>}
      <div className="flex gap-1.5">
        <button onClick={() => act('approve')} disabled={!!busy}
          className="flex items-center gap-1 rounded-md bg-emerald-500/10 border border-emerald-500/30 text-emerald-600 text-xs font-medium px-2.5 py-1 hover:bg-emerald-500/20 disabled:opacity-50 transition-colors">
          {busy === 'approve' ? <Loader2 className="h-3 w-3 animate-spin" /> : <CheckCircle2 className="h-3 w-3" />}Send
        </button>
        <button onClick={() => act('reject')} disabled={!!busy}
          className="flex items-center gap-1 rounded-md bg-destructive/10 border border-destructive/30 text-destructive text-xs font-medium px-2.5 py-1 hover:bg-destructive/20 disabled:opacity-50 transition-colors">
          {busy === 'reject' ? <Loader2 className="h-3 w-3 animate-spin" /> : <Ban className="h-3 w-3" />}Block
        </button>
      </div>
    </div>
  );
}

function EscalationRow({ item, onDone }: { item: InboxItem; onDone: () => void }) {
  const msg = item.raw as SupervisorMessage;
  const [busy, setBusy] = useState<string | null>(null);
  const [reason, setReason] = useState('');

  const act = async (action: 'approve' | 'reject') => {
    setBusy(action);
    try {
      if (action === 'approve') await supervisorApi.approve(msg.id, reason);
      else await supervisorApi.reject(msg.id, reason);
      onDone();
    } finally { setBusy(null); }
  };

  return (
    <div className="space-y-2">
      <p className="text-xs font-medium">{msg.intent?.replace(/_/g, ' ')}</p>
      {msg.content && <p className="text-xs text-muted-foreground leading-relaxed line-clamp-3">{msg.content}</p>}
      <input value={reason} onChange={e => setReason(e.target.value)}
        placeholder="Resolution note (optional)…"
        className="qr-input text-xs h-7 py-0" />
      <div className="flex gap-1.5">
        <button onClick={() => act('approve')} disabled={!!busy}
          className="flex items-center gap-1 rounded-md bg-emerald-500/10 border border-emerald-500/30 text-emerald-600 text-xs font-medium px-2.5 py-1 hover:bg-emerald-500/20 disabled:opacity-50 transition-colors">
          {busy === 'approve' ? <Loader2 className="h-3 w-3 animate-spin" /> : <CheckCircle2 className="h-3 w-3" />}Resolve
        </button>
        <button onClick={() => act('reject')} disabled={!!busy}
          className="flex items-center gap-1 rounded-md bg-destructive/10 border border-destructive/30 text-destructive text-xs font-medium px-2.5 py-1 hover:bg-destructive/20 disabled:opacity-50 transition-colors">
          {busy === 'reject' ? <Loader2 className="h-3 w-3 animate-spin" /> : <XCircle className="h-3 w-3" />}Reject
        </button>
      </div>
    </div>
  );
}

// ─── Main Inbox Tab ───────────────────────────────────────────────────────────

export function InboxTab() {
  const [items, setItems] = useState<InboxItem[]>([]);
  const [loading, setLoading] = useState(true);
  const liveApprovals = useStore((s) => s.approvals);

  const load = useCallback(async () => {
    setLoading(true);
    const all: InboxItem[] = [];

    await Promise.allSettled([
      // 1. Plan gates
      plansApi.list().then((ps) => {
        ps.filter(p => p.status === 'pending_approval').forEach(p =>
          all.push({ id: p.id, kind: 'plan', createdAt: p.created_at || '', raw: p })
        );
      }),

      // 2. Generic approvals (tool gates from orchestrator)
      approvalsApi.list().then((as) => {
        as.filter(a => a.state === 'pending').forEach(a =>
          all.push({ id: a.id, kind: 'permission', createdAt: a.created_at || '', raw: a })
        );
      }),

      // 3. Outbound actions
      outbound.pending().then((r) => {
        (r?.pending ?? []).forEach(o =>
          all.push({ id: o.id, kind: 'outbound', createdAt: o.requested_at || '', raw: o })
        );
      }),

      // 4. Mail approvals
      outbound.mailPending().then((ms) => {
        (Array.isArray(ms) ? ms : []).forEach(m =>
          all.push({ id: m.id, kind: 'mail', createdAt: (m.created_at as string) || '', raw: m })
        );
      }),

      // 5. Supervisor escalations
      supervisorApi.escalations().then((r) => {
        (r?.escalations ?? []).forEach(e =>
          all.push({ id: e.id, kind: 'escalation', createdAt: e.timestamp || '', raw: e })
        );
      }),
    ]);

    // Also add live WS permission requests not yet in the approvals API.
    // Read via getState() to avoid making `load` depend on `liveApprovals`,
    // which would recreate it on every approval event and cause a double-fetch storm.
    const currentApprovals = useStore.getState().approvals;
    Object.values(currentApprovals).filter(a => !a.resolved).forEach(a => {
      if (!all.find(i => i.id === a.request.request_id)) {
        all.push({
          id: a.request.request_id, kind: 'permission',
          createdAt: new Date(a.createdAt).toISOString(),
          raw: { id: a.request.request_id, kind: 'tool', state: 'pending', tool_name: a.request.tool, reason: a.request.reason, agent_id: a.request.agent_key ?? '' } as ApprovalItem,
        });
      }
    });

    all.sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt));
    setItems(all);
    setLoading(false);
  }, []);

  useEffect(() => { load(); }, [load]);

  // Auto-refresh when live approvals change — single effect, single re-fetch.
  const liveApprovalCount = Object.keys(liveApprovals).length;
  useEffect(() => { load(); }, [liveApprovalCount, load]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="flex shrink-0 items-center gap-2 border-b border-border px-4 py-2.5">
        <span className="text-xs text-muted-foreground">
          {items.length === 0 ? 'All clear' : `${items.length} item${items.length !== 1 ? 's' : ''} need your decision`}
        </span>
        <button onClick={load} className="ml-auto flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors">
          <RefreshCw className="h-3.5 w-3.5" />Refresh
        </button>
      </div>

      {/* List */}
      <div className="flex-1 overflow-y-auto divide-y divide-border/50">
        {items.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 gap-2 text-center">
            <CheckCheck className="h-10 w-10 text-emerald-500/30" />
            <p className="text-sm font-medium text-muted-foreground">All clear</p>
            <p className="text-xs text-muted-foreground/60">No pending approvals or escalations</p>
          </div>
        ) : (
          items.map((item) => {
            const meta = KIND_META[item.kind];
            const Icon = meta.icon;
            return (
              <div key={`${item.kind}-${item.id}`} className="px-4 py-4 hover:bg-accent/20 transition-colors">
                <div className="flex items-center gap-2 mb-2.5">
                  <Icon className={cn('h-3.5 w-3.5 shrink-0', meta.cls)} />
                  <span className={cn('text-xs font-semibold', meta.cls)}>{meta.label}</span>
                  <span className="ml-auto text-xs text-muted-foreground/50 tabular-nums shrink-0">
                    {relTime(item.createdAt)}
                  </span>
                </div>
                {item.kind === 'plan'       && <PlanRow       item={item} onDone={load} />}
                {item.kind === 'permission' && <PermissionRow item={item} onDone={load} />}
                {item.kind === 'outbound'   && <OutboundRow   item={item} onDone={load} />}
                {item.kind === 'mail'       && <MailRow       item={item} onDone={load} />}
                {item.kind === 'escalation' && <EscalationRow item={item} onDone={load} />}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
