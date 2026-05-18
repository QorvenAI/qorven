'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { cn } from '@/lib/utils';
import { Clock, Play, Pause, Trash2, Plus, X } from 'lucide-react';

interface Props { agentId: string }

export function SchedulesTab({ agentId }: Props) {
  const [jobs, setJobs] = useState<any[]>([]);
  const [showCreate, setShowCreate] = useState(false);
  const [task, setTask] = useState('');
  const [expression, setExpression] = useState('');
  const [creating, setCreating] = useState(false);
  const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

  const refreshJobs = () => {
    fetch(`/api/v1/cron-jobs`, { headers: { Authorization: `Bearer ${getToken()}` } })
      .then((r) => r.json())
      .then((d) => {
        const all = Array.isArray(d) ? d : Object.values(d).find(Array.isArray) ?? [];
        setJobs(all.filter((j: any) => j.agent_id === agentId));
      }).catch(() => {});
  };

  useEffect(() => { refreshJobs(); }, [agentId, getToken()]);

  const handleCreate = async () => {
    if (!task || !expression) return;
    setCreating(true);
    try {
      await fetch('/api/v1/cron-jobs', {
        method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${getToken()}` },
        body: JSON.stringify({ agent_id: agentId, expression, task }),
      });
      refreshJobs();
      setShowCreate(false); setTask(''); setExpression('');
    } catch { alert('Failed to create schedule'); }
    setCreating(false);
  };

  const cronToHuman = (expr: string) => {
    const parts = expr.split(' ');
    if (parts.length < 5) return expr;
    const [min, hour] = parts as [string, string];
    if (hour !== '*' && min !== '*') return `Daily at ${hour}:${min.padStart(2, '0')}`;
    if (hour === '*' && min !== '*') return `Every hour at :${min.padStart(2, '0')}`;
    return expr;
  };

  return (
    <div className="max-w-3xl mx-auto">
      <div className="flex items-center justify-between mb-4">
        <div>
          <p className="text-sm font-medium">Schedules</p>
          <p className="text-2xs text-muted-foreground">{jobs.length} scheduled task{jobs.length !== 1 ? 's' : ''}</p>
        </div>
        <button onClick={() => setShowCreate(true)} className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90">
          <Plus className="h-3.5 w-3.5" /> New Schedule
        </button>
      </div>

      {/* Create form */}
      {showCreate && (
        <div className="rounded-xl border border-border bg-card p-4 mb-4">
          <div className="flex items-center justify-between mb-3">
            <p className="text-sm font-medium">New Schedule</p>
            <button onClick={() => setShowCreate(false)}><X className="h-4 w-4 text-muted-foreground" /></button>
          </div>
          <input value={task} onChange={(e) => setTask(e.target.value)} placeholder="What should this Qor do? e.g. Send me a weather report"
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm mb-2" />
          <input value={expression} onChange={(e) => setExpression(e.target.value)} placeholder="Cron expression e.g. 0 9 * * * (daily at 9am)"
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm mb-3 font-mono" />
          <button onClick={handleCreate} disabled={creating || !task || !expression}
            className="rounded-lg bg-primary px-4 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50">
            {creating ? 'Creating...' : 'Create Schedule'}
          </button>
        </div>
      )}

      {jobs.length === 0 ? (
        <p className="py-12 text-center text-sm text-muted-foreground">No schedules yet — create one to automate tasks</p>
      ) : (
        <div className="space-y-2">
          {jobs.map((job: any) => {
            const isActive = job.status === 'active' || job.enabled !== false;
            return (
              <div key={job.id} className="rounded-xl border border-border bg-card p-4 group">
                <div className="flex items-start gap-3">
                  <div className={cn('mt-0.5 flex h-8 w-8 items-center justify-center rounded-lg', isActive ? 'bg-emerald-400/10 text-emerald-400' : 'bg-muted text-muted-foreground')}>
                    <Clock className="h-4 w-4" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <p className="text-sm font-medium truncate">{job.task || job.name || 'Scheduled task'}</p>
                      <span className={cn('rounded-full px-2 py-0.5 text-2xs font-medium',
                        isActive ? 'bg-emerald-400/10 text-emerald-400' : 'bg-muted text-muted-foreground')}>
                        {isActive ? 'Active' : 'Paused'}
                      </span>
                    </div>
                    <p className="text-xs text-muted-foreground mt-0.5">{cronToHuman(job.expression || job.cron || '')}</p>
                    {job.last_run && (
                      <p className="text-2xs text-muted-foreground mt-1">Last run: {new Date(job.last_run).toLocaleString()}</p>
                    )}
                  </div>
                  <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                    <button title={isActive ? 'Pause' : 'Resume'}
                      className="h-7 w-7 flex items-center justify-center rounded-lg text-muted-foreground hover:bg-accent hover:text-foreground">
                      {isActive ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
                    </button>
                    <button title="Delete"
                      className="h-7 w-7 flex items-center justify-center rounded-lg text-muted-foreground hover:bg-accent hover:text-destructive">
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
