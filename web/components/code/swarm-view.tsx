'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback, useRef } from 'react';
import { Loader2, GitMerge, Tag, ChevronDown, ChevronRight, Play, Zap } from 'lucide-react';
import { projectBriefs } from '@/lib/api-workspace';
import { BudgetBar } from './budget-bar';
import { ProjectTimeline } from './project-timeline';
import type { MergeQueueItem, ReleaseGate } from '@/types';
import { cn } from '@/lib/utils';

interface Props {
  briefId: string;
}

const POLL_MS = 10_000;
const DEBOUNCE_MS = 1_000;

// ── Status chip helpers ───────────────────────────────────────────────────────

const MERGE_STATUS_CLS: Record<MergeQueueItem['status'], string> = {
  queued:   'bg-muted text-muted-foreground',
  merging:  'bg-amber-500/10 text-amber-600 border border-amber-500/20',
  conflict: 'bg-destructive/10 text-destructive border border-destructive/20',
  merged:   'bg-emerald-500/10 text-emerald-700 border border-emerald-500/20',
  failed:   'bg-destructive/10 text-destructive border border-destructive/20',
};

const MERGE_STATUS_LABEL: Record<MergeQueueItem['status'], string> = {
  queued:   'Queued',
  merging:  'Merging',
  conflict: 'Conflict',
  merged:   'Merged',
  failed:   'Failed',
};

const RELEASE_STATUS_CLS: Record<ReleaseGate['status'], string> = {
  proposed: 'bg-amber-500/10 text-amber-600 border border-amber-500/20',
  approved: 'bg-primary/10 text-primary border border-primary/20',
  released: 'bg-emerald-500/10 text-emerald-700 border border-emerald-500/20',
  rejected: 'bg-destructive/10 text-destructive border border-destructive/20',
};

const RELEASE_STATUS_LABEL: Record<ReleaseGate['status'], string> = {
  proposed: 'Proposed',
  approved: 'Approved',
  released: 'Released',
  rejected: 'Rejected',
};

// ── Sub-components ────────────────────────────────────────────────────────────

function MergeQueueRow({ item }: { item: MergeQueueItem }) {
  return (
    <div className="flex items-start gap-3 py-2 border-b border-border last:border-0">
      <GitMerge className="h-4 w-4 mt-0.5 shrink-0 text-muted-foreground" />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="text-2sm font-medium text-foreground">#{item.pr_number}</span>
          <span className="text-xs text-muted-foreground truncate max-w-48">{item.branch}</span>
          <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium', MERGE_STATUS_CLS[item.status])}>
            {MERGE_STATUS_LABEL[item.status]}
          </span>
          {item.attempt > 1 && (
            <span className="text-xs text-muted-foreground">attempt {item.attempt}</span>
          )}
        </div>
        {item.detail && (
          <p className="text-xs text-muted-foreground mt-0.5 truncate">{item.detail}</p>
        )}
      </div>
    </div>
  );
}

function ReleaseGateRow({
  gate,
  briefId,
  onAction,
}: {
  gate: ReleaseGate;
  briefId: string;
  onAction: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [approving, setApproving] = useState(false);
  const [approveError, setApproveError] = useState<string | null>(null);

  const handleApprove = async () => {
    setApproving(true);
    setApproveError(null);
    try {
      await projectBriefs.approveRelease(briefId, gate.id);
      onAction();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Approval failed';
      setApproveError(msg);
    } finally {
      setApproving(false);
    }
  };

  return (
    <div className="border-b border-border last:border-0 py-2">
      <div className="flex items-center gap-2 flex-wrap">
        <Tag className="h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="text-2sm font-medium text-foreground">{gate.version}</span>
        <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium', RELEASE_STATUS_CLS[gate.status])}>
          {RELEASE_STATUS_LABEL[gate.status]}
        </span>
        {gate.changelog_md && (
          <button
            onClick={() => setExpanded((v) => !v)}
            className="ml-auto flex items-center gap-0.5 text-xs text-muted-foreground hover:text-foreground transition-colors"
          >
            {expanded ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
            Changelog
          </button>
        )}
        {gate.status === 'proposed' && (
          <button
            onClick={handleApprove}
            disabled={approving}
            className={cn(
              'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
              'bg-primary/10 text-primary hover:bg-primary/20 disabled:opacity-50 disabled:pointer-events-none',
            )}
          >
            {approving ? (
              <Loader2 className="h-3 w-3 animate-spin" />
            ) : (
              'Approve & release'
            )}
          </button>
        )}
      </div>
      {approveError && (
        <p className="mt-1 text-xs bg-destructive/10 text-destructive rounded px-2 py-1">{approveError}</p>
      )}
      {expanded && gate.changelog_md && (
        <pre className="mt-2 text-xs text-muted-foreground bg-muted rounded-md p-3 whitespace-pre-wrap overflow-x-auto">
          {gate.changelog_md}
        </pre>
      )}
    </div>
  );
}

// ── Main component ────────────────────────────────────────────────────────────

export function SwarmView({ briefId }: Props) {
  const [building, setBuilding] = useState(false);
  const [buildError, setBuildError] = useState<string | null>(null);

  const [queue, setQueue]       = useState<MergeQueueItem[] | null>(null);
  const [releases, setReleases] = useState<ReleaseGate[] | null>(null);
  const [loadingQ, setLoadingQ] = useState(true);
  const [loadingR, setLoadingR] = useState(true);

  const [proposing, setProposing] = useState(false);
  const [proposeError, setProposeError] = useState<string | null>(null);

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // ── Fetchers ────────────────────────────────────────────────────────────────

  const fetchQueue = useCallback(async () => {
    try {
      const r = await projectBriefs.mergeQueue(briefId);
      setQueue(r.queue);
    } catch {
      /* non-fatal */
    } finally {
      setLoadingQ(false);
    }
  }, [briefId]);

  const fetchReleases = useCallback(async () => {
    try {
      const r = await projectBriefs.releases(briefId);
      setReleases(r.releases);
    } catch {
      /* non-fatal */
    } finally {
      setLoadingR(false);
    }
  }, [briefId]);

  const refetchAll = useCallback(() => {
    fetchQueue();
    fetchReleases();
  }, [fetchQueue, fetchReleases]);

  // ── Initial + polling ───────────────────────────────────────────────────────

  useEffect(() => {
    refetchAll();
    const timer = setInterval(refetchAll, POLL_MS);
    return () => clearInterval(timer);
  }, [refetchAll]);

  // ── WS live-update subscription ─────────────────────────────────────────────

  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent<{ project_id?: string }>).detail;
      if (detail?.project_id && detail.project_id !== briefId) return;
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(refetchAll, DEBOUNCE_MS);
    };
    window.addEventListener('qorven:project_updated', handler);
    return () => {
      window.removeEventListener('qorven:project_updated', handler);
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [briefId, refetchAll]);

  // ── Actions ─────────────────────────────────────────────────────────────────

  const handleBuild = async () => {
    setBuilding(true);
    setBuildError(null);
    try {
      await projectBriefs.build(briefId);
      // success — backend transitions stage; live-update will refresh queue
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Build failed to start';
      setBuildError(msg);
      setBuilding(false);
    }
  };

  const handleProposeRelease = async () => {
    setProposing(true);
    setProposeError(null);
    try {
      await projectBriefs.proposeRelease(briefId);
      await fetchReleases();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to propose release';
      setProposeError(msg);
    } finally {
      setProposing(false);
    }
  };

  // ── Render ──────────────────────────────────────────────────────────────────

  return (
    <div className="overflow-y-auto h-full px-4 py-4 space-y-4">

      {/* Start Build */}
      <section className="rounded-lg border border-border bg-card p-4 space-y-3">
        <div className="flex items-center justify-between gap-4">
          <div>
            <h3 className="text-sm font-semibold text-foreground">Build</h3>
            <p className="text-xs text-muted-foreground mt-0.5">
              Trigger the autonomous code-build swarm for this project.
            </p>
          </div>
          <button
            onClick={handleBuild}
            disabled={building}
            className={cn(
              'shrink-0 flex items-center gap-1.5 rounded-md px-3 py-1.5 text-2sm font-medium transition-colors',
              'bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50 disabled:pointer-events-none',
            )}
          >
            {building ? (
              <>
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                Building…
              </>
            ) : (
              <>
                <Play className="h-3.5 w-3.5" />
                Start build
              </>
            )}
          </button>
        </div>

        {buildError && (
          <div className="rounded-md bg-destructive/10 text-destructive px-3 py-2 text-xs">
            {buildError}
          </div>
        )}

        {building && (
          <div className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
            Build started — agents are spinning up. The merge queue and releases sections will update as work lands.
          </div>
        )}

        {/* Live cost gauge */}
        <BudgetBar projectId={briefId} />
      </section>

      {/* Merge Queue */}
      <section className="rounded-lg border border-border bg-card p-4 space-y-3">
        <h3 className="text-sm font-semibold text-foreground">Merge Queue</h3>
        {loadingQ ? (
          <div className="flex items-center gap-2 text-muted-foreground py-2">
            <Loader2 className="h-4 w-4 animate-spin" />
            <span className="text-xs">Loading…</span>
          </div>
        ) : queue && queue.length > 0 ? (
          <div>
            {queue.map((item) => (
              <MergeQueueRow key={item.id} item={item} />
            ))}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">No merges queued.</p>
        )}
      </section>

      {/* Releases */}
      <section className="rounded-lg border border-border bg-card p-4 space-y-3">
        <div className="flex items-center justify-between gap-3">
          <h3 className="text-sm font-semibold text-foreground">Releases</h3>
          <button
            onClick={handleProposeRelease}
            disabled={proposing}
            className={cn(
              'flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
              'bg-muted text-foreground hover:bg-muted/70 disabled:opacity-50 disabled:pointer-events-none',
            )}
          >
            {proposing ? (
              <Loader2 className="h-3 w-3 animate-spin" />
            ) : (
              <Zap className="h-3 w-3" />
            )}
            Propose release
          </button>
        </div>

        {proposeError && (
          <div className="rounded-md bg-destructive/10 text-destructive px-3 py-2 text-xs">
            {proposeError}
          </div>
        )}

        {loadingR ? (
          <div className="flex items-center gap-2 text-muted-foreground py-2">
            <Loader2 className="h-4 w-4 animate-spin" />
            <span className="text-xs">Loading…</span>
          </div>
        ) : releases && releases.length > 0 ? (
          <div>
            {releases.map((gate) => (
              <ReleaseGateRow
                key={gate.id}
                gate={gate}
                briefId={briefId}
                onAction={refetchAll}
              />
            ))}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">No release gates yet.</p>
        )}
      </section>

      {/* Activity timeline */}
      <section className="rounded-lg border border-border bg-card overflow-hidden">
        <div className="px-4 pt-4 pb-2">
          <h3 className="text-sm font-semibold text-foreground">Activity</h3>
        </div>
        <div className="h-48 min-h-0">
          <ProjectTimeline briefId={briefId} />
        </div>
      </section>

    </div>
  );
}
