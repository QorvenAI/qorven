'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useState } from 'react';
import Link from 'next/link';
import { useSearchParams, useRouter } from 'next/navigation';
import {
  Workflow, CheckCircle2, XCircle, Loader2, RefreshCw,
  Clock, Mail, Globe, Megaphone, Webhook, MessageSquare,
  ShieldCheck, ShieldAlert, ArrowRight, GitBranch, Wrench,
  ListChecks, Play, Ban, CircleDot, CheckCheck, AlertCircle,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import {
  plans as plansApi, approvals as approvalsApi, outbound,
  type Plan, type PlanStatus,
  type ApprovalItem, type OutboundAction, type OutboundActionKind, type MailApproval,
} from '@/lib/api';
import { useStore } from '@/store';
import SupervisorPage from '@/app/(app)/supervisor/page';

// ─── helpers ────────────────────────────────────────────────────────

function relTime(iso: string): string {
  if (!iso) return '—';
  const diff = Date.now() - Date.parse(iso);
  if (!Number.isFinite(diff)) return iso;
  if (diff < 60_000)    return `${Math.round(diff / 1_000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
  return new Date(iso).toLocaleDateString();
}

const STATUS_META: Record<PlanStatus, { label: string; icon: typeof Workflow; cls: string }> = {
  draft:               { label: 'Draft',            icon: CircleDot,    cls: 'text-muted-foreground' },
  pending_approval:    { label: 'Needs Approval',   icon: ShieldCheck,  cls: 'text-amber-400' },
  approved:            { label: 'Approved',         icon: CheckCircle2, cls: 'text-emerald-400' },
  rejected:            { label: 'Rejected',         icon: XCircle,      cls: 'text-destructive' },
  revision_requested:  { label: 'Revision Needed',  icon: AlertCircle,  cls: 'text-orange-400' },
  running:             { label: 'Running',          icon: Play,         cls: 'text-primary' },
  done:                { label: 'Done',             icon: CheckCheck,   cls: 'text-emerald-500' },
  failed:              { label: 'Failed',           icon: AlertCircle,  cls: 'text-destructive' },
  cancelled:           { label: 'Cancelled',        icon: Ban,          cls: 'text-muted-foreground' },
};

const OUTBOUND_META: Record<string, { icon: typeof Mail; label: string; tone: string }> = {
  email_send:    { icon: Mail,          label: 'Email',    tone: 'text-blue-400' },
  telegram_send: { icon: MessageSquare, label: 'Telegram', tone: 'text-cyan-400' },
  social_post:   { icon: Megaphone,     label: 'Social',   tone: 'text-fuchsia-400' },
  webhook:       { icon: Webhook,       label: 'Webhook',  tone: 'text-emerald-400' },
};
const outboundMeta = (k: OutboundActionKind) =>
  OUTBOUND_META[k] ?? { icon: Globe, label: String(k), tone: 'text-muted-foreground' };

// ─── approval row ────────────────────────────────────────────────────

function ApprovalRow({ item, onResolved }: { item: ApprovalItem; onResolved: () => void }) {
  const [busy, setBusy] = useState<'approve' | 'reject' | null>(null);
  const [done, setDone] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const act = async (decision: 'approve' | 'reject') => {
    setBusy(decision);
    setErr(null);
    try {
      await approvalsApi.decide(item.id, decision);
      setDone(decision === 'approve' ? 'Approved.' : 'Rejected.');
      setTimeout(onResolved, 500);
    } catch (e) {
      setErr(e instanceof Error ? e.message : `${decision} failed`);
      setBusy(null);
    }
  };

  const Icon = item.kind === 'tool' ? Wrench : GitBranch;
  const tone = item.kind === 'tool' ? 'text-violet-400' : 'text-primary';

  return (
    <li className="rounded-xl border border-border bg-card p-4">
      <div className="flex items-start gap-3">
        <Icon className={cn('mt-0.5 h-4 w-4 shrink-0', tone)} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2 text-2xs">
            <span className="rounded-sm bg-muted px-1.5 py-0.5 font-mono uppercase text-muted-foreground">
              {item.kind === 'tool' ? 'Tool gate' : 'Plan node'}
            </span>
            {item.tool_name && <span className="font-mono text-foreground/80">{item.tool_name}</span>}
            {item.requested_by && <span className="text-muted-foreground">{item.requested_by}</span>}
            {item.created_at && (
              <span className="flex items-center gap-1 text-muted-foreground">
                <Clock className="h-3 w-3" />{relTime(item.created_at)}
              </span>
            )}
            {item.plan_id && (
              <Link href={`/plans/${item.plan_id}`} className="text-primary hover:underline font-mono">
                view plan →
              </Link>
            )}
          </div>
          {item.reason && <p className="mt-1 text-xs text-muted-foreground leading-relaxed">{item.reason}</p>}
          {item.tool_args != null && (
            <pre className="mt-2 overflow-x-auto rounded-md border border-border/60 bg-background/50 px-2 py-1.5 font-mono text-2xs text-muted-foreground">
              {JSON.stringify(item.tool_args, null, 2).slice(0, 400)}
            </pre>
          )}
          {done ? (
            <p className="mt-3 text-2xs text-emerald-400">{done}</p>
          ) : (
            <div className="mt-3 flex items-center gap-2">
              <ActionBtn busy={busy === 'approve'} onClick={() => act('approve')} tone="emerald" icon={<CheckCircle2 className="h-3.5 w-3.5" />} label="Approve" />
              <ActionBtn busy={busy === 'reject'}  onClick={() => act('reject')}  tone="red"    icon={<XCircle className="h-3.5 w-3.5" />}      label="Reject"  />
            </div>
          )}
          {err && <p className="mt-1 text-2xs text-destructive">{err}</p>}
        </div>
      </div>
    </li>
  );
}

// ─── outbound row ─────────────────────────────────────────────────────

function OutboundRow({ action, onResolved }: { action: OutboundAction; onResolved: () => void }) {
  const souls = useStore((s) => s.souls);
  const soul = souls.find((s) => s.id === action.agent_id);
  const { icon: Icon, label, tone } = outboundMeta(action.action_type);
  const [busy, setBusy] = useState<'approve' | 'reject' | null>(null);
  const [done, setDone] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const p = (action.payload ?? {}) as Record<string, unknown>;

  const act = async (kind: 'approve' | 'reject') => {
    setBusy(kind);
    setErr(null);
    try {
      if (kind === 'approve') await outbound.approve(action.id);
      else await outbound.reject(action.id);
      setDone(kind === 'approve' ? 'Approved.' : 'Rejected.');
      setTimeout(onResolved, 500);
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
            <span className="rounded-sm bg-muted px-1.5 py-0.5 font-mono uppercase text-muted-foreground">{label}</span>
            {soul && <span className="text-muted-foreground">{soul.display_name}</span>}
            <span className="flex items-center gap-1 text-muted-foreground">
              <Clock className="h-3 w-3" />{relTime(action.requested_at)}
            </span>
          </div>
          {action.action_type === 'email_send' ? (
            <div className="mt-1 space-y-0.5 text-xs">
              <div><span className="text-muted-foreground">To:</span> <span className="font-mono">{String(p.to ?? '—')}</span></div>
              <div><span className="text-muted-foreground">Subject:</span> <span className="font-medium">{String(p.subject ?? '—')}</span></div>
            </div>
          ) : (
            <pre className="mt-2 max-h-24 overflow-hidden rounded-md border border-border/60 bg-background/50 px-2 py-1.5 font-mono text-2xs text-muted-foreground">
              {JSON.stringify(action.payload, null, 2).slice(0, 300)}
            </pre>
          )}
          {done ? (
            <p className="mt-3 text-2xs text-emerald-400">{done}</p>
          ) : (
            <div className="mt-3 flex items-center gap-2">
              <ActionBtn busy={busy === 'approve'} onClick={() => act('approve')} tone="emerald" icon={<CheckCircle2 className="h-3.5 w-3.5" />} label="Approve" />
              <ActionBtn busy={busy === 'reject'}  onClick={() => act('reject')}  tone="red"    icon={<XCircle className="h-3.5 w-3.5" />}      label="Reject"  />
            </div>
          )}
          {err && <p className="mt-1 text-2xs text-destructive">{err}</p>}
        </div>
      </div>
    </li>
  );
}

// ─── mail row ─────────────────────────────────────────────────────────

function MailRow({ item, onResolved }: { item: MailApproval; onResolved: () => void }) {
  const [busy, setBusy] = useState<'approve' | 'reject' | null>(null);
  const [done, setDone] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const to = Array.isArray(item.to) ? item.to.join(', ') : (item.to ?? '(no recipient)');

  const act = async (kind: 'approve' | 'reject') => {
    setBusy(kind);
    setErr(null);
    try {
      if (kind === 'approve') await outbound.mailApprove(item.id);
      else await outbound.mailReject(item.id);
      setDone(kind === 'approve' ? 'Approved.' : 'Rejected.');
      setTimeout(onResolved, 500);
    } catch (e) {
      setErr(e instanceof Error ? e.message : `${kind} failed`);
      setBusy(null);
    }
  };

  return (
    <li className="rounded-xl border border-border bg-card p-4">
      <div className="flex items-start gap-3">
        <Mail className="mt-0.5 h-4 w-4 shrink-0 text-blue-400" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2 text-2xs">
            <span className="rounded-sm bg-muted px-1.5 py-0.5 font-mono uppercase text-muted-foreground">Mail</span>
            <span className="font-mono text-muted-foreground">→ {to}</span>
            {item.created_at && <span className="text-muted-foreground">{relTime(item.created_at)}</span>}
          </div>
          <p className="mt-1 text-sm font-medium">{typeof item.subject === 'string' && item.subject ? item.subject : '(no subject)'}</p>
          {typeof item.body === 'string' && item.body && (
            <p className="mt-1 line-clamp-3 whitespace-pre-wrap text-xs leading-relaxed text-muted-foreground">{item.body}</p>
          )}
          {done ? (
            <p className="mt-3 text-2xs text-emerald-400">{done}</p>
          ) : (
            <div className="mt-3 flex items-center gap-2">
              <ActionBtn busy={busy === 'approve'} onClick={() => act('approve')} tone="emerald" icon={<CheckCircle2 className="h-3.5 w-3.5" />} label="Approve" />
              <ActionBtn busy={busy === 'reject'}  onClick={() => act('reject')}  tone="red"    icon={<XCircle className="h-3.5 w-3.5" />}      label="Reject"  />
            </div>
          )}
          {err && <p className="mt-1 text-2xs text-destructive">{err}</p>}
        </div>
      </div>
    </li>
  );
}

// ─── shared button ────────────────────────────────────────────────────

function ActionBtn({ busy, onClick, tone, icon, label }: {
  busy: boolean; onClick: () => void; tone: 'emerald' | 'red'; icon: React.ReactNode; label: string;
}) {
  const emerald = 'border-emerald-500/40 bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20';
  const red     = 'border-destructive/40 bg-destructive/10 text-destructive hover:bg-destructive/20';
  return (
    <button onClick={onClick} disabled={busy}
      className={cn('inline-flex items-center gap-1.5 rounded-md border px-3 py-1 text-xs font-medium disabled:opacity-50', tone === 'emerald' ? emerald : red)}>
      {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : icon}
      {label}
    </button>
  );
}

// ─── plan card (All Plans tab) ────────────────────────────────────────

function PlanCard({ plan }: { plan: Plan }) {
  const meta = STATUS_META[plan.status] ?? STATUS_META.draft;
  const Icon = meta.icon;
  return (
    <Link href={`/plans/${plan.id}`}
      className="group flex items-start gap-3 rounded-xl border border-border bg-card p-4 hover:border-primary/30 transition-colors">
      <Icon className={cn('mt-0.5 h-4 w-4 shrink-0', meta.cls)} />
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium truncate group-hover:text-primary transition-colors">{plan.title || '(untitled)'}</p>
        {plan.summary && <p className="mt-0.5 text-xs text-muted-foreground line-clamp-2">{plan.summary}</p>}
        <div className="mt-2 flex flex-wrap items-center gap-2 text-2xs text-muted-foreground">
          <span className={cn('font-medium', meta.cls)}>{meta.label}</span>
          <span>·</span>
          <span className="flex items-center gap-1"><Clock className="h-3 w-3" />{relTime(plan.created_at)}</span>
          {plan.session_id && <span className="font-mono truncate max-w-[100px]">{plan.session_id.slice(0, 8)}</span>}
        </div>
      </div>
      <ArrowRight className="mt-1 h-3.5 w-3.5 shrink-0 text-muted-foreground/40 group-hover:text-foreground transition-colors" />
    </Link>
  );
}

// ─── inbox tab ────────────────────────────────────────────────────────

function InboxTab() {
  const [planItems, setPlanItems] = useState<ApprovalItem[]>([]);
  const [actions, setActions]     = useState<OutboundAction[]>([]);
  const [mailItems, setMailItems] = useState<MailApproval[]>([]);
  const [loading, setLoading]     = useState(true);

  const refresh = useCallback(async () => {
    setLoading(true);
    const safe = <T,>(p: Promise<T>, fb: T) => p.catch(() => fb);
    const [plan, act, mail] = await Promise.all([
      safe(approvalsApi.list(), [] as ApprovalItem[]),
      safe(outbound.pending(), { pending: [] as OutboundAction[] }),
      safe(outbound.mailPending(), [] as MailApproval[]),
    ]);
    setPlanItems((Array.isArray(plan) ? plan : []).filter((i) => i.state === 'pending'));
    setActions(act.pending ?? []);
    setMailItems(Array.isArray(mail) ? mail : []);
    setLoading(false);
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  const total = planItems.length + actions.length + mailItems.length;

  if (loading) return <div className="flex items-center gap-2 py-10 justify-center text-sm text-muted-foreground"><Loader2 className="h-4 w-4 animate-spin" /> Loading…</div>;

  if (total === 0) return (
    <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border/60 bg-card/40 px-6 py-14 text-center">
      <CheckCheck className="h-9 w-9 text-emerald-500/70" />
      <div>
        <p className="text-sm font-medium">All clear</p>
        <p className="mt-0.5 text-xs text-muted-foreground max-w-xs mx-auto">
          No pending decisions. Permission prompts appear inline in chat.
        </p>
      </div>
    </div>
  );

  const MAX = 10;
  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <button onClick={refresh} className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-accent">
          <RefreshCw className="h-3.5 w-3.5" /> Refresh ({total} pending)
        </button>
      </div>

      {planItems.length > 0 && (
        <section className="space-y-2">
          <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-2">
            <ShieldCheck className="h-3.5 w-3.5 text-primary" /> Plan & Tool Gates ({planItems.length})
          </h2>
          <ul className="space-y-2">
            {planItems.slice(0, MAX).map((item) => <ApprovalRow key={item.id} item={item} onResolved={refresh} />)}
            {planItems.length > MAX && <li className="text-center text-2xs text-muted-foreground">+{planItems.length - MAX} more</li>}
          </ul>
        </section>
      )}

      {actions.length > 0 && (
        <section className="space-y-2">
          <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-2">
            <ShieldAlert className="h-3.5 w-3.5 text-amber-400" /> Outbound Actions ({actions.length})
          </h2>
          <ul className="space-y-2">
            {actions.slice(0, MAX).map((a) => <OutboundRow key={a.id} action={a} onResolved={refresh} />)}
            {actions.length > MAX && <li className="text-center text-2xs text-muted-foreground">+{actions.length - MAX} more</li>}
          </ul>
        </section>
      )}

      {mailItems.length > 0 && (
        <section className="space-y-2">
          <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground flex items-center gap-2">
            <Mail className="h-3.5 w-3.5 text-blue-400" /> Mail Approvals ({mailItems.length})
          </h2>
          <ul className="space-y-2">
            {mailItems.slice(0, MAX).map((m) => <MailRow key={m.id} item={m} onResolved={refresh} />)}
            {mailItems.length > MAX && <li className="text-center text-2xs text-muted-foreground">+{mailItems.length - MAX} more</li>}
          </ul>
        </section>
      )}
    </div>
  );
}

// ─── all plans tab ────────────────────────────────────────────────────

const STATUS_FILTERS = ['all', 'pending_approval', 'running', 'done', 'failed'] as const;

function AllPlansTab() {
  const [allPlans, setAllPlans] = useState<Plan[]>([]);
  const [loading, setLoading]   = useState(true);
  const [filter, setFilter]     = useState<typeof STATUS_FILTERS[number]>('all');

  const refresh = useCallback(async () => {
    setLoading(true);
    const data = await plansApi.list().catch(() => [] as Plan[]);
    setAllPlans(data);
    setLoading(false);
  }, []);

  useEffect(() => { refresh(); }, [refresh]);

  const filtered = filter === 'all' ? allPlans : allPlans.filter((p) => p.status === filter);

  const counts = STATUS_FILTERS.reduce((acc, s) => {
    acc[s] = s === 'all' ? allPlans.length : allPlans.filter((p) => p.status === s).length;
    return acc;
  }, {} as Record<string, number>);

  if (loading) return <div className="flex items-center gap-2 py-10 justify-center text-sm text-muted-foreground"><Loader2 className="h-4 w-4 animate-spin" /> Loading…</div>;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex flex-wrap gap-1.5">
          {STATUS_FILTERS.map((s) => (
            <button key={s} onClick={() => setFilter(s)}
              className={cn('rounded-md px-2.5 py-1 text-xs font-medium border transition-colors',
                filter === s ? 'bg-primary text-primary-foreground border-primary' : 'border-border text-muted-foreground hover:text-foreground hover:bg-accent')}>
              {s === 'all' ? 'All' : STATUS_META[s as PlanStatus]?.label ?? s} {(counts[s] ?? 0) > 0 && `(${counts[s] ?? 0})`}
            </button>
          ))}
        </div>
        <button onClick={refresh} className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground hover:bg-accent">
          <RefreshCw className="h-3.5 w-3.5" />
        </button>
      </div>

      {filtered.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-border/60 bg-card/40 px-6 py-12 text-center">
          <ListChecks className="h-8 w-8 text-muted-foreground/40" />
          <p className="text-sm text-muted-foreground">{allPlans.length === 0 ? 'No plans yet — Prime creates these automatically.' : `No plans with status "${filter}".`}</p>
        </div>
      ) : (
        <div className="space-y-2">
          {filtered.map((p) => <PlanCard key={p.id} plan={p} />)}
        </div>
      )}
    </div>
  );
}

// ─── page ─────────────────────────────────────────────────────────────

type PlanTab = 'inbox' | 'all' | 'supervisor';

const PLAN_TABS: { id: PlanTab; label: string }[] = [
  { id: 'inbox',      label: 'Inbox'      },
  { id: 'all',        label: 'All Plans'  },
  { id: 'supervisor', label: 'Supervisor' },
];

export default function PlansPage() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const tab = (searchParams.get('tab') ?? 'inbox') as PlanTab;

  const setTab = (t: PlanTab) => {
    router.replace(t === 'inbox' ? '/plans' : `/plans?tab=${t}`);
  };

  return (
    <div className="mx-auto max-w-6xl space-y-6 p-4 lg:p-6">
      <header className="flex items-start gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0">
          <Workflow className="h-5 w-5" />
        </div>
        <div>
          <h1 className="text-lg font-semibold tracking-tight">Plans</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            Approval inbox, plan history, and team health monitoring.
          </p>
        </div>
      </header>

      <div className="flex gap-1 border-b border-border">
        {PLAN_TABS.map((t) => (
          <button key={t.id} onClick={() => setTab(t.id)}
            className={cn('px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors',
              tab === t.id ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground')}>
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'inbox'      && <InboxTab />}
      {tab === 'all'        && <AllPlansTab />}
      {tab === 'supervisor' && <SupervisorPage />}
    </div>
  );
}
