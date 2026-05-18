'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { cron as cronApi } from '@/lib/api-agents';
import { cn } from '@/lib/utils';
import { CalendarClock, Play, Pause, Trash2, Plus, Loader2 } from 'lucide-react';

const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

function cleanJobName(name: string): string {
  return name
    .replace(/_/g, ' ')
    .replace(/\s+\d{3,}$/, '') // strip trailing numbers like _1435
    .replace(/\b\w/g, (c) => c.toUpperCase())
    .trim();
}

type Freq = 'hourly' | 'daily' | 'weekly' | 'monthly' | 'custom';

interface RawJob {
  id: string;
  name: string;
  cron_expression: string;
  enabled: boolean;
  last_run_at: string | null;
  next_run_at: string | null;
  agent_id: string | null;
}

function fmtTime(h: number, m: number): string {
  const ampm = h >= 12 ? 'PM' : 'AM';
  const displayH = h === 0 ? 12 : h > 12 ? h - 12 : h;
  const displayM = m === 0 ? '' : `:${String(m).padStart(2, '0')}`;
  return `${displayH}${displayM} ${ampm}`;
}

function humanizeCron(expr: string): string {
  const parts = expr.trim().split(/\s+/);
  if (parts.length !== 5) return expr;
  const [minS, hourS, domS, , dowS] = parts as [string, string, string, string, string];
  const isNum = (s: string) => /^\d+$/.test(s);
  if (!isNum(minS) || !isNum(hourS)) {
    // handles */N patterns or ranges — just format what we can
    if (minS === '0' && hourS === '*' && domS === '*' && dowS === '*') return 'Every hour';
    return expr;
  }
  const h = parseInt(hourS, 10);
  const m = parseInt(minS, 10);
  const t = fmtTime(h, m);
  if (domS === '*' && dowS === '*') return `Every day at ${t}`;
  if (domS === '*' && isNum(dowS)) return `Every ${DAYS[parseInt(dowS, 10)]} at ${t}`;
  if (dowS === '*' && isNum(domS)) return `Monthly on day ${domS} at ${t}`;
  return expr;
}

function buildExpression(freq: Freq, hour: number, dayOfWeek: number, dayOfMonth: number): string {
  if (freq === 'hourly') return '0 * * * *';
  if (freq === 'daily') return `0 ${hour} * * *`;
  if (freq === 'weekly') return `0 ${hour} * * ${dayOfWeek}`;
  if (freq === 'monthly') return `0 ${hour} ${dayOfMonth} * *`;
  return '';
}

function formatTime(dateStr: string | null | undefined): string {
  if (!dateStr) return '—';
  try {
    return new Date(dateStr).toLocaleString();
  } catch {
    return '—';
  }
}

function HourSelect({ value, onChange }: { value: number; onChange: (h: number) => void }) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(parseInt(e.target.value, 10))}
      className="qr-select"
    >
      {Array.from({ length: 24 }, (_, i) => {
        const ampm = i >= 12 ? 'PM' : 'AM';
        const display = i === 0 ? 12 : i > 12 ? i - 12 : i;
        return (
          <option key={i} value={i}>
            {display}:00 {ampm}
          </option>
        );
      })}
    </select>
  );
}

export function SchedulesTab({ agentId }: { agentId: string }) {
  const [jobs, setJobs] = useState<RawJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAdd, setShowAdd] = useState(false);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [togglingId, setTogglingId] = useState<string | null>(null);

  // Form state
  const [freq, setFreq] = useState<Freq>('daily');
  const [hour, setHour] = useState(9);
  const [dayOfWeek, setDayOfWeek] = useState(1);
  const [dayOfMonth, setDayOfMonth] = useState(1);
  const [customExpr, setCustomExpr] = useState('');
  const [task, setTask] = useState('');
  const [saving, setSaving] = useState(false);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await cronApi.list();
      const all: RawJob[] = data as unknown as RawJob[];
      setJobs(all.filter((j) => j.agent_id === agentId));
    } catch {
      toast.error('Failed to load schedules');
    } finally {
      setLoading(false);
    }
  }, [agentId]);

  useEffect(() => {
    load();
  }, [load]);

  const expression = freq === 'custom' ? customExpr : buildExpression(freq, hour, dayOfWeek, dayOfMonth);

  const handleSave = async () => {
    if (!expression.trim() || !task.trim()) return;
    setSaving(true);
    try {
      await cronApi.create({ agent_id: agentId, expression, task });
      toast.success('Schedule added');
      setTask('');
      setShowAdd(false);
      await load();
    } catch {
      toast.error('Failed to create schedule');
    } finally {
      setSaving(false);
    }
  };

  const handleToggle = async (job: RawJob) => {
    setTogglingId(job.id);
    try {
      if (job.enabled) {
        await cronApi.pause(job.id);
      } else {
        await cronApi.resume(job.id);
      }
      await load();
    } catch {
      toast.error('Failed to update schedule');
    } finally {
      setTogglingId(null);
    }
  };

  const handleDelete = async (id: string) => {
    setConfirmDeleteId(null);
    setDeletingId(id);
    try {
      await cronApi.delete(id);
      setJobs((prev) => prev.filter((j) => j.id !== id));
      toast.success('Schedule removed');
    } catch {
      toast.error('Failed to remove schedule');
    } finally {
      setDeletingId(null);
    }
  };

  const freqOptions: { id: Freq; label: string }[] = [
    { id: 'hourly', label: 'Hourly' },
    { id: 'daily', label: 'Daily' },
    { id: 'weekly', label: 'Weekly' },
    { id: 'monthly', label: 'Monthly' },
    { id: 'custom', label: 'Custom' },
  ];

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">Schedules</h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            Recurring tasks run automatically on this Qor.
          </p>
        </div>
        <button
          onClick={() => setShowAdd(!showAdd)}
          className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90 cursor-pointer"
        >
          <Plus className="h-3.5 w-3.5" />
          Add Schedule
        </button>
      </div>

      {/* Add Schedule form */}
      {showAdd && (
        <div className="rounded-xl border border-border p-5 space-y-5">
          <h3 className="text-sm font-medium">New Schedule</h3>

          {/* Frequency picker */}
          <div className="space-y-1.5">
            <label className="text-xs text-muted-foreground">Frequency</label>
            <div className="flex flex-wrap gap-2">
              {freqOptions.map((opt) => (
                <button
                  key={opt.id}
                  onClick={() => setFreq(opt.id)}
                  className={cn(
                    'rounded-full px-3 py-1 text-xs font-medium border transition-colors cursor-pointer',
                    freq === opt.id
                      ? 'border-primary bg-primary/10 text-primary'
                      : 'border-border bg-muted/30 text-muted-foreground hover:bg-accent',
                  )}
                >
                  {opt.label}
                </button>
              ))}
            </div>
          </div>

          {/* Contextual time/day selector */}
          {freq === 'daily' && (
            <div className="space-y-1.5">
              <label className="text-xs text-muted-foreground">Time</label>
              <HourSelect value={hour} onChange={setHour} />
            </div>
          )}

          {freq === 'weekly' && (
            <div className="space-y-3">
              <div className="space-y-1.5">
                <label className="text-xs text-muted-foreground">Day of week</label>
                <div className="flex flex-wrap gap-1.5">
                  {DAYS.map((day, i) => (
                    <button
                      key={day}
                      onClick={() => setDayOfWeek(i)}
                      className={cn(
                        'rounded-lg px-3 py-1.5 text-xs font-medium border transition-colors cursor-pointer',
                        dayOfWeek === i
                          ? 'border-primary bg-primary/10 text-primary'
                          : 'border-border bg-muted/30 text-muted-foreground hover:bg-accent',
                      )}
                    >
                      {day}
                    </button>
                  ))}
                </div>
              </div>
              <div className="space-y-1.5">
                <label className="text-xs text-muted-foreground">Time</label>
                <HourSelect value={hour} onChange={setHour} />
              </div>
            </div>
          )}

          {freq === 'monthly' && (
            <div className="space-y-3">
              <div className="space-y-1.5">
                <label className="text-xs text-muted-foreground">Day of month</label>
                <div className="flex flex-wrap gap-1.5">
                  {Array.from({ length: 28 }, (_, i) => i + 1).map((d) => (
                    <button
                      key={d}
                      onClick={() => setDayOfMonth(d)}
                      className={cn(
                        'rounded-lg w-9 py-1.5 text-xs font-medium border transition-colors cursor-pointer',
                        dayOfMonth === d
                          ? 'border-primary bg-primary/10 text-primary'
                          : 'border-border bg-muted/30 text-muted-foreground hover:bg-accent',
                      )}
                    >
                      {d}
                    </button>
                  ))}
                </div>
              </div>
              <div className="space-y-1.5">
                <label className="text-xs text-muted-foreground">Time</label>
                <HourSelect value={hour} onChange={setHour} />
              </div>
            </div>
          )}

          {freq === 'custom' && (
            <div className="space-y-1.5">
              <label className="text-xs text-muted-foreground">Cron expression</label>
              <input
                value={customExpr}
                onChange={(e) => setCustomExpr(e.target.value)}
                placeholder="e.g. 0 9 * * 1-5"
                className="qr-textarea resize-none text-xs font-mono"
              />
            </div>
          )}

          {/* Task description */}
          <div className="space-y-1.5">
            <label className="text-xs text-muted-foreground">What should the agent do?</label>
            <textarea
              value={task}
              onChange={(e) => setTask(e.target.value)}
              rows={3}
              placeholder="e.g. Send me a motivational quote, Check and summarize new emails, Post a status update..."
              className="qr-textarea resize-none text-xs"
            />
          </div>

          {/* Preview */}
          {expression && (
            <div className="rounded-lg bg-muted/40 border border-border px-3 py-2.5 text-xs space-y-0.5">
              <p className="text-muted-foreground">
                Runs: <span className="text-foreground font-medium">{humanizeCron(expression)}</span>
              </p>
              <p className="font-mono text-muted-foreground/70">{expression}</p>
            </div>
          )}

          {/* Actions */}
          <div className="flex gap-2">
            <button
              onClick={handleSave}
              disabled={saving || !expression.trim() || !task.trim()}
              className="flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50 cursor-pointer"
            >
              {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
              {saving ? 'Saving…' : 'Add Schedule'}
            </button>
            <button
              onClick={() => setShowAdd(false)}
              className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Job list */}
      {loading ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      ) : jobs.length === 0 ? (
        <div className="text-center py-12">
          <CalendarClock className="h-10 w-10 text-muted-foreground/30 mx-auto" />
          <p className="mt-3 text-sm text-muted-foreground">No schedules yet. Add one to get started.</p>
        </div>
      ) : (
        <div className="space-y-2">
          {jobs.map((job) => (
            <div
              key={job.id}
              className="bg-card border border-border rounded-xl px-4 py-3 flex items-start gap-3"
            >
              {/* Left: icon + schedule info */}
              <CalendarClock className="h-4 w-4 text-muted-foreground mt-0.5 shrink-0" />
              <div className="flex-1 min-w-0 space-y-1">
                <p className="text-sm font-semibold leading-tight">
                  {cleanJobName(job.name)}
                </p>
                <p className="text-xs text-muted-foreground">{humanizeCron(job.cron_expression)}</p>
                <div className="flex flex-wrap items-center gap-3 mt-0.5">
                  <span
                    className={cn(
                      'inline-flex items-center rounded-full px-2 py-0.5 text-2xs font-medium',
                      job.enabled
                        ? 'bg-emerald-500/10 text-emerald-500'
                        : 'bg-muted text-muted-foreground',
                    )}
                  >
                    {job.enabled ? 'Active' : 'Paused'}
                  </span>
                  <span className="text-2xs text-muted-foreground">
                    Last: {formatTime(job.last_run_at)}
                  </span>
                  <span className="text-2xs text-muted-foreground">
                    Next: {formatTime(job.next_run_at)}
                  </span>
                </div>
              </div>

              {/* Right: action buttons */}
              <div className="flex items-center gap-1.5 shrink-0">
                <button
                  onClick={() => handleToggle(job)}
                  disabled={togglingId === job.id}
                  title={job.enabled ? 'Pause' : 'Resume'}
                  className={cn(
                    'rounded-md p-1.5 transition-colors cursor-pointer',
                    job.enabled
                      ? 'text-amber-500 hover:bg-amber-500/10'
                      : 'text-emerald-500 hover:bg-emerald-500/10',
                    togglingId === job.id && 'opacity-50',
                  )}
                >
                  {togglingId === job.id ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : job.enabled ? (
                    <Pause className="h-3.5 w-3.5" />
                  ) : (
                    <Play className="h-3.5 w-3.5" />
                  )}
                </button>

                {confirmDeleteId === job.id ? (
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => handleDelete(job.id)}
                      disabled={deletingId === job.id}
                      className="rounded-md px-2 py-1 text-2xs font-medium bg-destructive text-destructive-foreground hover:bg-destructive/90 transition-colors cursor-pointer"
                    >
                      {deletingId === job.id ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Delete'}
                    </button>
                    <button
                      onClick={() => setConfirmDeleteId(null)}
                      className="rounded-md px-2 py-1 text-2xs font-medium border border-border hover:bg-accent transition-colors cursor-pointer"
                    >
                      Cancel
                    </button>
                  </div>
                ) : (
                  <button
                    onClick={() => setConfirmDeleteId(job.id)}
                    disabled={deletingId === job.id}
                    title="Delete"
                    className={cn(
                      'rounded-md p-1.5 text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors cursor-pointer',
                      deletingId === job.id && 'opacity-50',
                    )}
                  >
                    {deletingId === job.id ? (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <Trash2 className="h-3.5 w-3.5" />
                    )}
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
