'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useCallback } from 'react';
import { useStore } from '@/store';
import type { DaemonPlan } from '@/hooks/use-agents-stream';
import { BASE, getToken } from '@/lib/api-core';
import { cn } from '@/lib/utils';
import { CheckCircle2, XCircle, Clock, Loader2, ChevronDown, ChevronUp } from 'lucide-react';
import { toast } from 'sonner';

// ── Single plan card ──────────────────────────────────────────────────────────

function PlanCard({ plan }: { plan: DaemonPlan }) {
  const [expanded, setExpanded] = useState(true);
  const [approving, setApproving] = useState(false);
  const [rejecting, setRejecting] = useState(false);
  const [mods, setMods] = useState('');

  const act = useCallback(async (action: 'approve' | 'reject') => {
    const busy = action === 'approve' ? setApproving : setRejecting;
    busy(true);
    try {
      const res = await fetch(`${BASE}/daemon/plans/${plan.id}/${action}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${getToken()}` },
        body: JSON.stringify(action === 'approve' ? { modifications: mods } : { reason: mods }),
      });
      if (!res.ok) throw new Error(await res.text());
      toast.success(action === 'approve' ? 'Plan approved — tasks queued' : 'Plan rejected');
    } catch (e: any) {
      toast.error(`Failed to ${action}: ${e.message}`);
    } finally {
      busy(false);
    }
  }, [plan.id, mods]);

  const isPending = plan.status === 'pending';

  return (
    <div className={cn(
      'rounded-xl border bg-card overflow-hidden',
      isPending ? 'border-amber-500/40' : 'border-border',
    )}>
      {/* Header */}
      <button className="w-full flex items-center justify-between px-4 py-3 text-left"
        onClick={() => setExpanded(v => !v)}>
        <div className="flex items-center gap-2 min-w-0">
          {plan.status === 'pending'   && <Clock className="h-4 w-4 text-amber-400 shrink-0" />}
          {plan.status === 'approved'  && <CheckCircle2 className="h-4 w-4 text-emerald-400 shrink-0" />}
          {plan.status === 'rejected'  && <XCircle className="h-4 w-4 text-red-400 shrink-0" />}
          {plan.status === 'executing' && <Loader2 className="h-4 w-4 text-primary animate-spin shrink-0" />}
          {plan.status === 'done'      && <CheckCircle2 className="h-4 w-4 text-emerald-500 shrink-0" />}
          <div className="min-w-0">
            <p className="text-sm font-medium truncate">{plan.title}</p>
            <p className="text-2xs text-muted-foreground">
              {plan.tasks.length} task{plan.tasks.length !== 1 ? 's' : ''} · proposed by {plan.proposed_by}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2 shrink-0 ml-2">
          <span className={cn('text-2xs font-medium px-1.5 py-0.5 rounded-full',
            plan.status === 'pending'   && 'bg-amber-500/10 text-amber-400',
            plan.status === 'approved'  && 'bg-emerald-500/10 text-emerald-400',
            plan.status === 'rejected'  && 'bg-red-500/10 text-red-400',
            plan.status === 'executing' && 'bg-primary/10 text-primary',
            plan.status === 'done'      && 'bg-emerald-500/10 text-emerald-500',
          )}>{plan.status}</span>
          {expanded ? <ChevronUp className="h-4 w-4 text-muted-foreground" /> : <ChevronDown className="h-4 w-4 text-muted-foreground" />}
        </div>
      </button>

      {expanded && (
        <div className="px-4 pb-4 space-y-3 border-t border-border/50 pt-3">
          {/* Description */}
          {plan.description && (
            <p className="text-xs text-muted-foreground">{plan.description}</p>
          )}

          {/* Task list */}
          <div className="space-y-1.5">
            {plan.tasks.map((t, i) => (
              <div key={t.id} className="flex items-center gap-2 text-xs">
                <span className="text-muted-foreground shrink-0 w-4">{i + 1}.</span>
                <span className="flex-1 truncate">{t.title}</span>
                <span className="text-2xs text-muted-foreground shrink-0">{t.owner}</span>
                {t.estimated_minutes && (
                  <span className="text-2xs text-muted-foreground shrink-0">~{t.estimated_minutes}m</span>
                )}
              </div>
            ))}
          </div>

          {/* Approval actions — only for pending plans */}
          {isPending && (
            <div className="space-y-2 pt-1">
              <textarea
                value={mods}
                onChange={e => setMods(e.target.value)}
                placeholder="Optional: notes or requested changes…"
                rows={2}
                className="w-full rounded-lg border border-border bg-background px-3 py-2 text-xs placeholder:text-muted-foreground resize-none focus:outline-none focus:ring-1 focus:ring-ring"
              />
              <div className="flex gap-2">
                <button
                  onClick={() => act('approve')}
                  disabled={approving || rejecting}
                  className="flex items-center gap-1.5 rounded-lg bg-emerald-500 px-3 py-1.5 text-xs font-medium text-white hover:bg-emerald-600 disabled:opacity-50 transition-colors">
                  {approving ? <Loader2 className="h-3 w-3 animate-spin" /> : <CheckCircle2 className="h-3 w-3" />}
                  Approve & queue
                </button>
                <button
                  onClick={() => act('reject')}
                  disabled={approving || rejecting}
                  className="flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs font-medium hover:bg-accent disabled:opacity-50 transition-colors">
                  {rejecting ? <Loader2 className="h-3 w-3 animate-spin" /> : <XCircle className="h-3 w-3" />}
                  Reject
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ── Plan approval feed ────────────────────────────────────────────────────────

export function PlanApproval() {
  const plans = useStore(s => Object.values(s.daemonPlans));

  // Pending first, then by status
  const sorted = [...plans].sort((a, b) => {
    if (a.status === 'pending' && b.status !== 'pending') return -1;
    if (b.status === 'pending' && a.status !== 'pending') return 1;
    return 0;
  });

  const pendingCount = plans.filter(p => p.status === 'pending').length;

  if (plans.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-border py-10 text-center">
        <CheckCircle2 className="h-8 w-8 text-muted-foreground/40" />
        <p className="text-sm font-medium">No plans</p>
        <p className="text-xs text-muted-foreground">Plans proposed by agents will appear here for approval.</p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {pendingCount > 0 && (
        <p className="text-xs text-amber-400 font-medium">
          {pendingCount} plan{pendingCount !== 1 ? 's' : ''} awaiting your approval
        </p>
      )}
      {sorted.map(plan => <PlanCard key={plan.id} plan={plan} />)}
    </div>
  );
}
