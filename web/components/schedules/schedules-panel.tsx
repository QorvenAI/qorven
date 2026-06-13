'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useCallback, useRef } from 'react';
import { cron as cronApi, agents as agentsApi } from '@/lib/api';
import type { CronJob, CronRun } from '@/types';
import { cronToHuman, timeUntil } from '@/components/cron/cron-utils';
import { ScheduleBuilder, type ScheduleValue } from './schedule-builder';
import { cn } from '@/lib/utils';
import {
  Plus,
  Play,
  Pause,
  Trash2,
  Pencil,
  Clock,
  ChevronDown,
  ChevronRight,
  Loader2,
  CalendarClock,
  TriangleAlert,
} from 'lucide-react';
import { toast } from 'sonner';

// ---------------------------------------------------------------------------
// Drawer FSM
// ---------------------------------------------------------------------------
type DrawerMode = { kind: 'closed' } | { kind: 'create' } | { kind: 'edit'; job: CronJob };

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
function fmtDate(s: string | null): string {
  if (!s) return '—';
  try {
    return new Date(s).toLocaleString([], {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  } catch {
    return s;
  }
}

function statusColor(status: string): string {
  switch (status) {
    case 'completed': return 'text-emerald-500';
    case 'failed': return 'text-destructive';
    case 'running': return 'text-amber-500';
    default: return 'text-muted-foreground';
  }
}

// ---------------------------------------------------------------------------
// DrawerForm — isolated so it can be reset cleanly
// ---------------------------------------------------------------------------
interface DrawerFormProps {
  mode: 'create' | 'edit';
  job?: CronJob;
  agentId?: string;
  agents: Array<{ id: string; display_name?: string; name?: string }>;
  onClose: () => void;
  onSaved: () => void;
}

function DrawerForm({ mode, job, agentId, agents, onClose, onSaved }: DrawerFormProps) {
  const [fName, setFName] = useState(job?.name ?? '');
  const [fAgent, setFAgent] = useState(job?.agent_id ?? agentId ?? '');
  const [fInstr, setFInstr] = useState('');
  const [fChannel, setFChannel] = useState(job?.delivery_channel ?? '');
  const [fSched, setFSched] = useState<ScheduleValue>({
    cron_expression: job?.cron_expression ?? '0 9 * * *',
    one_shot: false,
  });
  const [saving, setSaving] = useState(false);

  // Escape to close
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onClose]);

  const save = async () => {
    if (!fName.trim() || !fAgent || !fSched.cron_expression) return;
    setSaving(true);
    try {
      const body = {
        agent_id: fAgent,
        name: fName.trim(),
        cron_expression: fSched.cron_expression,
        instruction: fInstr.trim() || fName.trim(),
        delivery_channel: fChannel.trim() || undefined,
      };
      if (mode === 'create') {
        await cronApi.create({ ...body, one_shot: fSched.one_shot });
      } else if (mode === 'edit' && job) {
        await cronApi.update(job.id, body);
      }
      toast.success(mode === 'create' ? 'Schedule created' : 'Schedule updated');
      onSaved();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Save failed';
      toast.error(msg);
    } finally {
      setSaving(false);
    }
  };

  const canSave = fName.trim().length > 0 && fAgent.length > 0 && fSched.cron_expression.length > 0;

  return (
    // Overlay
    <div
      className="fixed inset-0 z-50 flex justify-end bg-black/30"
      role="dialog"
      aria-modal="true"
      onClick={onClose}
    >
      {/* Drawer panel */}
      <div
        className="w-full max-w-md h-full bg-background border-l border-border shadow-xl overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="sticky top-0 z-10 flex items-center justify-between px-5 py-4 border-b border-border bg-background">
          <h3 className="text-sm font-semibold">
            {mode === 'create' ? 'New Schedule' : 'Edit Schedule'}
          </h3>
          <button
            onClick={onClose}
            className="qr-btn qr-btn-ghost qr-btn-icon"
            aria-label="Close"
          >
            ×
          </button>
        </div>

        <div className="p-5 space-y-4">
          {/* Name */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Name</label>
            <input
              className="qr-input"
              value={fName}
              onChange={(e) => setFName(e.target.value)}
              placeholder="Daily digest, Weekly report…"
              autoFocus
            />
          </div>

          {/* Agent picker (hidden when agentId is pre-set) */}
          {!agentId && (
            <div className="space-y-1">
              <label className="text-xs font-medium text-muted-foreground">Agent</label>
              <select
                className="qr-select"
                value={fAgent}
                onChange={(e) => setFAgent(e.target.value)}
              >
                <option value="">Select agent…</option>
                {agents.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.display_name || a.name || a.id}
                  </option>
                ))}
              </select>
            </div>
          )}

          {/* Instruction */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">
              Instruction{' '}
              <span className="text-muted-foreground/60">(what should the agent do?)</span>
            </label>
            <textarea
              className="qr-textarea resize-none"
              rows={3}
              value={fInstr}
              onChange={(e) => setFInstr(e.target.value)}
              placeholder="Summarize the latest news, send me a morning briefing…"
            />
          </div>

          {/* Delivery channel */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">
              Delivery channel{' '}
              <span className="text-muted-foreground/60">(optional, e.g. telegram)</span>
            </label>
            <input
              className="qr-input"
              value={fChannel}
              onChange={(e) => setFChannel(e.target.value)}
              placeholder="telegram, email, slack…"
            />
          </div>

          {/* Schedule builder */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-muted-foreground">Schedule</label>
            <ScheduleBuilder value={fSched} onChange={setFSched} />
          </div>
        </div>

        {/* Footer */}
        <div className="sticky bottom-0 flex gap-2 justify-end px-5 py-4 border-t border-border bg-background">
          <button onClick={onClose} className="qr-btn qr-btn-outline">
            Cancel
          </button>
          <button
            onClick={save}
            disabled={!canSave || saving}
            className="qr-btn qr-btn-primary"
          >
            {saving && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {mode === 'create' ? 'Create' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// DeleteConfirm inline modal
// ---------------------------------------------------------------------------
function DeleteConfirm({
  job,
  onCancel,
  onConfirm,
}: {
  job: CronJob;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  // Escape to cancel
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onCancel(); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onCancel]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      role="alertdialog"
      aria-modal="true"
      onClick={onCancel}
    >
      <div
        className="bg-background border border-border rounded-xl p-6 w-full max-w-sm shadow-xl space-y-4"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start gap-3">
          <TriangleAlert className="h-5 w-5 text-destructive shrink-0 mt-0.5" />
          <div>
            <p className="text-sm font-semibold">Delete schedule?</p>
            <p className="text-xs text-muted-foreground mt-1">
              &ldquo;{job.name}&rdquo; will be permanently removed.
            </p>
          </div>
        </div>
        <div className="flex gap-2 justify-end">
          <button onClick={onCancel} className="qr-btn qr-btn-outline qr-btn-sm">
            Cancel
          </button>
          <button onClick={onConfirm} className="qr-btn qr-btn-destructive qr-btn-sm">
            Delete
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// RunHistory — expandable per-job history panel
// ---------------------------------------------------------------------------
function RunHistory({ jobId }: { jobId: string }) {
  const [runs, setRuns] = useState<CronRun[] | null>(null);

  useEffect(() => {
    cronApi.runs(jobId).then(setRuns).catch(() => setRuns([]));
  }, [jobId]);

  if (runs === null) {
    return (
      <div className="flex items-center gap-2 px-4 py-3 text-xs text-muted-foreground">
        <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading run history…
      </div>
    );
  }

  if (runs.length === 0) {
    return (
      <p className="px-4 py-3 text-xs text-muted-foreground">No runs recorded yet.</p>
    );
  }

  return (
    <div className="divide-y divide-border/50">
      {runs.map((rn) => (
        <div key={rn.id} className="flex items-center gap-3 px-4 py-2.5 text-xs">
          <span className={cn('font-medium capitalize shrink-0', statusColor(rn.status))}>
            {rn.status}
          </span>
          <span className="text-muted-foreground shrink-0">{fmtDate(rn.scheduled_for)}</span>
          {rn.error ? (
            <span className="text-destructive/70 truncate">{rn.error}</span>
          ) : rn.result_snippet ? (
            <span className="text-muted-foreground truncate">{rn.result_snippet}</span>
          ) : null}
          {(rn.cost_cents > 0 || rn.tokens > 0) && (
            <span className="ml-auto shrink-0 text-muted-foreground/60">
              {rn.tokens.toLocaleString()} tok · ${(rn.cost_cents / 100).toFixed(2)}
            </span>
          )}
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// SchedulesPanel — the one shared surface
// ---------------------------------------------------------------------------
export function SchedulesPanel({ agentId }: { agentId?: string }) {
  const [jobs, setJobs] = useState<CronJob[]>([]);
  const [loadingJobs, setLoadingJobs] = useState(true);
  const [agents, setAgents] = useState<Array<{ id: string; display_name?: string; name?: string }>>([]);
  const [drawer, setDrawer] = useState<DrawerMode>({ kind: 'closed' });
  const [expanded, setExpanded] = useState<string | null>(null);
  const [togglingId, setTogglingId] = useState<string | null>(null);
  const [runningId, setRunningId] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<CronJob | null>(null);

  const load = useCallback(async () => {
    setLoadingJobs(true);
    try {
      const all = await cronApi.list();
      setJobs(agentId ? all.filter((j) => j.agent_id === agentId) : all);
    } catch {
      toast.error('Failed to load schedules');
    } finally {
      setLoadingJobs(false);
    }
  }, [agentId]);

  useEffect(() => {
    load();
    agentsApi.list().then(setAgents).catch(() => {});
  }, [load]);

  const handleToggle = async (j: CronJob) => {
    setTogglingId(j.id);
    try {
      await (j.enabled ? cronApi.pause(j.id) : cronApi.resume(j.id));
      await load();
      toast.success(j.enabled ? `${j.name} paused` : `${j.name} resumed`);
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : 'Failed to update');
    } finally {
      setTogglingId(null);
    }
  };

  const handleRunNow = async (j: CronJob) => {
    setRunningId(j.id);
    try {
      await cronApi.runNow(j.id);
      toast.success(`${j.name} queued to run`);
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : 'Failed to queue run');
    } finally {
      setRunningId(null);
    }
  };

  const handleDelete = async (j: CronJob) => {
    setConfirmDelete(null);
    try {
      await cronApi.delete(j.id);
      toast.success('Schedule removed');
      await load();
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : 'Failed to delete');
    }
  };

  const toggleExpand = (id: string) => {
    setExpanded((prev) => (prev === id ? null : id));
  };

  return (
    <div className="space-y-3">
      {/* Toolbar */}
      <div className="flex justify-end">
        <button
          onClick={() => setDrawer({ kind: 'create' })}
          className="qr-btn qr-btn-primary qr-btn-sm"
        >
          <Plus className="h-3.5 w-3.5" />
          New Schedule
        </button>
      </div>

      {/* Loading skeletons */}
      {loadingJobs ? (
        <div className="space-y-2">
          {[1, 2, 3].map((i) => (
            <div
              key={i}
              className="rounded-xl border border-border bg-card p-4 flex items-center gap-4 animate-pulse"
            >
              <div className="h-4 w-4 rounded-full bg-muted shrink-0" />
              <div className="flex-1 space-y-1.5">
                <div className="h-4 w-48 rounded bg-muted" />
                <div className="h-3 w-32 rounded bg-muted" />
              </div>
            </div>
          ))}
        </div>
      ) : jobs.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border py-12 text-center space-y-2">
          <CalendarClock className="h-10 w-10 text-muted-foreground/30 mx-auto" />
          <p className="text-sm font-medium text-muted-foreground">No schedules yet</p>
          <p className="text-xs text-muted-foreground/70">
            Create one to run agent tasks automatically.
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {jobs.map((j) => {
            const isExpanded = expanded === j.id;
            const isToggling = togglingId === j.id;
            const isRunning = runningId === j.id;

            return (
              <div key={j.id} className="rounded-xl border border-border bg-card overflow-hidden">
                {/* Job row */}
                <div className="flex items-center gap-3 px-4 py-3">
                  {/* Expand toggle */}
                  <button
                    onClick={() => toggleExpand(j.id)}
                    className="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
                    aria-label={isExpanded ? 'Collapse' : 'Expand'}
                  >
                    {isExpanded ? (
                      <ChevronDown className="h-4 w-4" />
                    ) : (
                      <ChevronRight className="h-4 w-4" />
                    )}
                  </button>

                  {/* Status dot */}
                  <span
                    className={cn(
                      'h-2 w-2 rounded-full shrink-0',
                      j.enabled ? 'bg-emerald-500' : 'bg-muted-foreground',
                    )}
                  />

                  {/* Info */}
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium truncate">{j.name}</p>
                    <div className="flex items-center gap-1.5 mt-0.5 text-xs text-muted-foreground">
                      <Clock className="h-3 w-3 shrink-0" />
                      <span className="font-mono">{j.cron_expression}</span>
                      <span>·</span>
                      <span>{cronToHuman(j.cron_expression)}</span>
                      {j.agent_name && (
                        <>
                          <span>·</span>
                          <span className="truncate">{j.agent_name}</span>
                        </>
                      )}
                      {j.next_run_at && (
                        <>
                          <span>·</span>
                          <span>Next {timeUntil(j.next_run_at)}</span>
                        </>
                      )}
                    </div>
                  </div>

                  {/* Status badge */}
                  <span
                    className={cn(
                      'shrink-0 inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                      j.enabled
                        ? 'bg-emerald-500/10 text-emerald-500'
                        : 'bg-muted text-muted-foreground',
                    )}
                  >
                    {j.enabled ? 'Active' : 'Paused'}
                  </span>

                  {/* Action buttons */}
                  <div className="flex items-center gap-1 shrink-0">
                    {/* Run now */}
                    <button
                      onClick={() => handleRunNow(j)}
                      disabled={isRunning}
                      title="Run now"
                      className="qr-btn qr-btn-ghost qr-btn-icon"
                    >
                      {isRunning ? (
                        <Loader2 className="h-3.5 w-3.5 animate-spin" />
                      ) : (
                        <Play className="h-3.5 w-3.5" />
                      )}
                    </button>

                    {/* Pause/Resume */}
                    <button
                      onClick={() => handleToggle(j)}
                      disabled={isToggling}
                      title={j.enabled ? 'Pause' : 'Resume'}
                      className="qr-btn qr-btn-ghost qr-btn-icon"
                    >
                      {isToggling ? (
                        <Loader2 className="h-3.5 w-3.5 animate-spin" />
                      ) : j.enabled ? (
                        <Pause className="h-3.5 w-3.5" />
                      ) : (
                        <Play className="h-3.5 w-3.5" />
                      )}
                    </button>

                    {/* Edit */}
                    <button
                      onClick={() => setDrawer({ kind: 'edit', job: j })}
                      title="Edit"
                      className="qr-btn qr-btn-ghost qr-btn-icon"
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </button>

                    {/* Delete */}
                    <button
                      onClick={() => setConfirmDelete(j)}
                      title="Delete"
                      className="qr-btn qr-btn-ghost qr-btn-icon text-muted-foreground hover:text-destructive"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>

                {/* Expanded: detail + run history */}
                {isExpanded && (
                  <div className="border-t border-border">
                    {/* Job meta */}
                    <div className="grid grid-cols-2 sm:grid-cols-3 gap-3 px-4 py-3 bg-muted/20">
                      <div>
                        <p className="text-xs text-muted-foreground">Last run</p>
                        <p className="text-xs font-medium">{fmtDate(j.last_run_at)}</p>
                      </div>
                      <div>
                        <p className="text-xs text-muted-foreground">Next run</p>
                        <p className="text-xs font-medium">
                          {j.next_run_at ? fmtDate(j.next_run_at) : '—'}
                        </p>
                      </div>
                      {j.delivery_channel && (
                        <div>
                          <p className="text-xs text-muted-foreground">Channel</p>
                          <p className="text-xs font-medium">{j.delivery_channel}</p>
                        </div>
                      )}
                    </div>

                    {/* Run history */}
                    <div>
                      <p className="px-4 pt-3 pb-1 text-xs font-semibold text-muted-foreground uppercase tracking-wide">
                        Run History
                      </p>
                      <RunHistory jobId={j.id} />
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {/* Create / Edit drawer */}
      {drawer.kind !== 'closed' && (
        <DrawerForm
          mode={drawer.kind}
          job={drawer.kind === 'edit' ? drawer.job : undefined}
          agentId={agentId}
          agents={agents}
          onClose={() => setDrawer({ kind: 'closed' })}
          onSaved={() => {
            setDrawer({ kind: 'closed' });
            load();
          }}
        />
      )}

      {/* Delete confirmation */}
      {confirmDelete && (
        <DeleteConfirm
          job={confirmDelete}
          onCancel={() => setConfirmDelete(null)}
          onConfirm={() => handleDelete(confirmDelete)}
        />
      )}
    </div>
  );
}
