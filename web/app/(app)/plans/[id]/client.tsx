'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useState } from 'react';
import { useParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import {
  ArrowLeft, Loader2, RefreshCw, CheckCircle2, XCircle, AlertCircle,
  Play, CheckCheck, Ban, CircleDot, Clock, User,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import {
  plans as plansApi,
  type Plan, type PlanNode, type PlanEdge, type PlanStatus,
} from '@/lib/api';
import { PlanGraph } from '@/components/plans/plan-graph';

// ─── status meta ────────────────────────────────────────────────────

const PLAN_STATUS: Record<PlanStatus, { label: string; icon: typeof CircleDot; cls: string }> = {
  draft:              { label: 'Draft',           icon: CircleDot,    cls: 'text-muted-foreground' },
  pending_approval:   { label: 'Needs Approval',  icon: AlertCircle,  cls: 'text-amber-400' },
  approved:           { label: 'Approved',        icon: CheckCircle2, cls: 'text-emerald-400' },
  rejected:           { label: 'Rejected',        icon: XCircle,      cls: 'text-destructive' },
  revision_requested: { label: 'Revision Needed', icon: AlertCircle,  cls: 'text-orange-400' },
  running:            { label: 'Running',         icon: Play,         cls: 'text-primary' },
  done:               { label: 'Done',            icon: CheckCheck,   cls: 'text-emerald-500' },
  failed:             { label: 'Failed',          icon: AlertCircle,  cls: 'text-destructive' },
  cancelled:          { label: 'Cancelled',       icon: Ban,          cls: 'text-muted-foreground' },
};

function relTime(iso?: string): string {
  if (!iso) return '—';
  const diff = Date.now() - Date.parse(iso);
  if (!Number.isFinite(diff)) return iso;
  if (diff < 60_000)     return `${Math.round(diff / 1_000)}s ago`;
  if (diff < 3_600_000)  return `${Math.round(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
  return new Date(iso).toLocaleDateString();
}

// ─── approve / reject / revise panel ─────────────────────────────────

function ActionPanel({ plan, onDone }: { plan: Plan; onDone: () => void }) {
  const [comment, setComment] = useState('');
  const [busy, setBusy] = useState<'approve' | 'reject' | 'revise' | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const act = async (kind: 'approve' | 'reject' | 'revise') => {
    if ((kind === 'reject' || kind === 'revise') && !comment.trim()) {
      setErr('Comment required for reject and revise.');
      return;
    }
    setBusy(kind);
    setErr(null);
    try {
      if (kind === 'approve') await plansApi.approve(plan.id, comment || undefined);
      else if (kind === 'reject') await plansApi.reject(plan.id, comment);
      else await plansApi.revise(plan.id, comment);
      onDone();
    } catch (e) {
      setErr(e instanceof Error ? e.message : `${kind} failed`);
      setBusy(null);
    }
  };

  if (!['pending_approval', 'revision_requested'].includes(plan.status)) return null;

  return (
    <div className="rounded-xl border border-amber-500/30 bg-amber-500/5 p-4 space-y-3">
      <p className="text-sm font-medium text-amber-400 flex items-center gap-2">
        <AlertCircle className="h-4 w-4" />
        {plan.status === 'revision_requested' ? 'Revision requested — review and re-approve or reject' : 'Waiting for your decision'}
      </p>
      <textarea
        value={comment}
        onChange={(e) => setComment(e.target.value)}
        placeholder="Add a comment (required for reject / request revision)…"
        rows={3}
        className="qr-textarea resize-none"
      />
      {err && <p className="text-xs text-destructive">{err}</p>}
      <div className="flex flex-wrap items-center gap-2">
        <button onClick={() => act('approve')} disabled={!!busy}
          className="inline-flex items-center gap-1.5 rounded-md border border-emerald-500/40 bg-emerald-500/10 px-4 py-1.5 text-sm font-medium text-emerald-500 hover:bg-emerald-500/20 disabled:opacity-50">
          {busy === 'approve' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <CheckCircle2 className="h-3.5 w-3.5" />}
          Approve
        </button>
        <button onClick={() => act('revise')} disabled={!!busy}
          className="inline-flex items-center gap-1.5 rounded-md border border-orange-500/40 bg-orange-500/10 px-4 py-1.5 text-sm font-medium text-orange-400 hover:bg-orange-500/20 disabled:opacity-50">
          {busy === 'revise' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <AlertCircle className="h-3.5 w-3.5" />}
          Request Revision
        </button>
        <button onClick={() => act('reject')} disabled={!!busy}
          className="inline-flex items-center gap-1.5 rounded-md border border-destructive/40 bg-destructive/10 px-4 py-1.5 text-sm font-medium text-destructive hover:bg-destructive/20 disabled:opacity-50">
          {busy === 'reject' ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <XCircle className="h-3.5 w-3.5" />}
          Reject
        </button>
      </div>
    </div>
  );
}

// ─── page ─────────────────────────────────────────────────────────────

export default function PlanDetailClient() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const [plan, setPlan]   = useState<Plan | null>(null);
  const [nodes, setNodes] = useState<PlanNode[]>([]);
  const [edges, setEdges] = useState<PlanEdge[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setErr(null);
    try {
      const [p, graph] = await Promise.all([
        plansApi.get(id),
        plansApi.nodes(id).catch(() => ({ nodes: [], edges: [] })),
      ]);
      setPlan(p);
      setNodes(graph.nodes ?? []);
      setEdges(graph.edges ?? []);
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Failed to load plan');
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => { refresh(); }, [refresh]);

  if (loading) return (
    <div className="flex items-center justify-center py-20 text-muted-foreground gap-2 text-sm">
      <Loader2 className="h-5 w-5 animate-spin" /> Loading plan…
    </div>
  );

  if (err || !plan) return (
    <div className="mx-auto max-w-3xl p-6 space-y-4">
      <Link href="/plans" className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft className="h-3.5 w-3.5" /> Back to Plans
      </Link>
      <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
        {err ?? 'Plan not found.'}
      </div>
    </div>
  );

  const statusMeta = PLAN_STATUS[plan.status] ?? PLAN_STATUS.draft;
  const StatusIcon = statusMeta.icon;

  return (
    <div className="mx-auto max-w-5xl space-y-6 p-4 lg:p-6">
      <Link href="/plans" className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors">
        <ArrowLeft className="h-3.5 w-3.5" /> All Plans
      </Link>

      <header className="space-y-2">
        <div className="flex items-start gap-3">
          <div className="min-w-0 flex-1">
            <h1 className="text-xl font-semibold">{plan.title || '(untitled plan)'}</h1>
            {plan.summary && <p className="mt-1 text-sm text-muted-foreground">{plan.summary}</p>}
          </div>
          <button onClick={refresh} className="shrink-0 rounded-md border border-border p-1.5 text-muted-foreground hover:bg-accent">
            <RefreshCw className="h-4 w-4" />
          </button>
        </div>
        <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
          <span className={cn('flex items-center gap-1.5 font-medium', statusMeta.cls)}>
            <StatusIcon className="h-3.5 w-3.5" />{statusMeta.label}
          </span>
          <span className="flex items-center gap-1"><Clock className="h-3.5 w-3.5" />Created {relTime(plan.created_at)}</span>
          {plan.created_by && <span className="flex items-center gap-1"><User className="h-3.5 w-3.5" />{plan.created_by}</span>}
          {plan.session_id && (
            <Link href={`/sessions?id=${plan.session_id}`} className="font-mono hover:text-primary transition-colors">
              session {plan.session_id.slice(0, 8)}
            </Link>
          )}
        </div>
      </header>

      <ActionPanel plan={plan} onDone={refresh} />

      <section className="space-y-3">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          Plan Steps ({nodes.length})
        </h2>
        {nodes.length === 0 ? (
          <p className="text-sm text-muted-foreground py-4 text-center">No nodes yet.</p>
        ) : (
          <PlanGraph nodes={nodes} edges={edges} />
        )}
      </section>

      {!!plan.spec && (
        <details className="rounded-xl border border-border/60">
          <summary className="cursor-pointer px-4 py-2.5 text-xs text-muted-foreground hover:text-foreground select-none">
            Raw spec
          </summary>
          <pre className="overflow-x-auto px-4 pb-4 font-mono text-2xs text-muted-foreground">
            {JSON.stringify(plan.spec, null, 2)}
          </pre>
        </details>
      )}
    </div>
  );
}
