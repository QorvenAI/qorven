'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState, useCallback } from 'react';
import { CalendarDays, Clock, Play, Pause, Plus, ChevronLeft, ChevronRight, Loader2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { cron as cronApi, calendarApi } from '@/lib/api';
import { useStore } from '@/store';
import { ErrorBoundary } from '@/components/error-boundary';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { toast } from 'sonner';
import type { CronJob } from '@/types';

const MONTHS = ['January','February','March','April','May','June','July','August','September','October','November','December'];
const DAYS = ['Sun','Mon','Tue','Wed','Thu','Fri','Sat'];

const EVENT_COLORS = [
  'bg-primary/20 text-primary border-primary/40',
  'bg-emerald-500/20 text-emerald-400 border-emerald-500/40',
  'bg-amber-500/20 text-amber-400 border-amber-500/40',
  'bg-blue-500/20 text-blue-400 border-blue-500/40',
  'bg-purple-500/20 text-purple-400 border-purple-500/40',
  'bg-rose-500/20 text-rose-400 border-rose-500/40',
];

function colorForAgent(id: string) {
  let h = 0;
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) & 0xffffffff;
  return EVENT_COLORS[Math.abs(h) % EVENT_COLORS.length]!;
}

export default function SchedulePage() {
  const calAgentFilter = useStore((s) => s.calSoulFilter);
  const souls = useStore((s) => s.souls);

  const [tab, setTab] = useState<'calendar' | 'cron'>('calendar');
  const [today] = useState(() => new Date());
  const [current, setCurrent] = useState(() => new Date());
  const [events, setEvents] = useState<any[]>([]);
  const [jobs, setJobs] = useState<CronJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedDay, setSelectedDay] = useState<string | null>(null);

  const year = current.getFullYear();
  const month = current.getMonth();
  const firstDay = new Date(year, month, 1).getDay();
  const daysInMonth = new Date(year, month + 1, 0).getDate();

  const agentFilter = calAgentFilter;
  const agentLabel = agentFilter ? (souls.find(s => s.id === agentFilter)?.display_name ?? 'Agent') : 'All Agents';

  const loadEvents = useCallback(() => {
    const start = new Date(year, month, 1).toISOString();
    const end = new Date(year, month + 1, 0, 23, 59, 59).toISOString();
    setLoading(true);
    Promise.all([
      calendarApi.list(start, end, agentFilter ?? undefined).catch(() => []),
      cronApi.list().catch(() => []),
    ]).then(([evts, jobs]) => {
      setEvents(Array.isArray(evts) ? evts : []);
      setJobs(Array.isArray(jobs) ? jobs : []);
      setLoading(false);
    });
  }, [year, month, agentFilter]);

  useEffect(() => { loadEvents(); }, [loadEvents]);

  const toggleJob = async (job: CronJob) => {
    try {
      if (job.status === 'active') {
        await cronApi.pause(job.id);
        setJobs(prev => prev.map(j => j.id === job.id ? { ...j, status: 'paused' } : j));
        toast.success(`${job.task} paused`);
      } else {
        await cronApi.resume(job.id);
        setJobs(prev => prev.map(j => j.id === job.id ? { ...j, status: 'active' } : j));
        toast.success(`${job.task} resumed`);
      }
    } catch { toast.error('Failed to update job'); }
  };

  // Group events by date key YYYY-MM-DD
  const byDate = events.reduce<Record<string, any[]>>((acc, e) => {
    const d = e.start_time ? e.start_time.slice(0, 10) : e.date?.slice(0, 10);
    if (d) { acc[d] = acc[d] ?? []; acc[d].push(e); }
    return acc;
  }, {});

  const todayKey = today.toISOString().slice(0, 10);
  const selectedEvents = selectedDay ? (byDate[selectedDay] ?? []) : [];

  const cells: (number | null)[] = [
    ...Array(firstDay).fill(null),
    ...Array.from({ length: daysInMonth }, (_, i) => i + 1),
  ];
  while (cells.length % 7 !== 0) cells.push(null);

  return (
    <ErrorBoundary fallbackTitle="Failed to load schedule">
      <div className="space-y-5">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold flex items-center gap-2">
              <CalendarDays className="h-5 w-5" /> Schedule
            </h1>
            <p className="text-sm text-muted-foreground">
              {agentFilter ? `Showing events for ${agentLabel}` : 'All agents · events and cron jobs'}
            </p>
          </div>
          <div className="flex gap-1">
            {(['calendar', 'cron'] as const).map(t => (
              <button key={t} onClick={() => setTab(t)}
                className={cn('px-3 py-1.5 rounded-lg text-sm font-medium transition-colors cursor-pointer capitalize',
                  tab === t ? 'bg-primary text-primary-foreground' : 'border border-border text-muted-foreground hover:text-foreground hover:bg-accent')}>
                {t === 'cron' ? `Cron (${jobs.length})` : 'Calendar'}
              </button>
            ))}
          </div>
        </div>

        {tab === 'calendar' ? (
          <div className="flex gap-5">
            {/* Calendar grid */}
            <div className="flex-1 min-w-0">
              {/* Month nav */}
              <div className="flex items-center justify-between mb-3">
                <button onClick={() => setCurrent(new Date(year, month - 1, 1))}
                  className="h-8 w-8 flex items-center justify-center rounded-lg hover:bg-accent cursor-pointer transition-colors">
                  <ChevronLeft className="h-4 w-4" />
                </button>
                <h2 className="text-base font-semibold">{MONTHS[month]} {year}</h2>
                <button onClick={() => setCurrent(new Date(year, month + 1, 1))}
                  className="h-8 w-8 flex items-center justify-center rounded-lg hover:bg-accent cursor-pointer transition-colors">
                  <ChevronRight className="h-4 w-4" />
                </button>
              </div>

              {/* Day headers */}
              <div className="grid grid-cols-7 mb-1">
                {DAYS.map(d => (
                  <div key={d} className="text-center text-xs font-medium text-muted-foreground py-1">{d}</div>
                ))}
              </div>

              {/* Cells */}
              <div className="grid grid-cols-7 gap-px bg-border rounded-xl overflow-hidden border border-border">
                {cells.map((day, i) => {
                  if (day === null) return <div key={i} className="bg-background/50 min-h-[80px] p-1" />;
                  const key = `${year}-${String(month + 1).padStart(2,'0')}-${String(day).padStart(2,'0')}`;
                  const dayEvents = byDate[key] ?? [];
                  const isToday = key === todayKey;
                  const isSelected = key === selectedDay;
                  return (
                    <div key={i} onClick={() => setSelectedDay(isSelected ? null : key)}
                      className={cn(
                        'bg-background min-h-[80px] p-1.5 cursor-pointer hover:bg-accent/30 transition-colors',
                        isSelected && 'bg-primary/5',
                      )}>
                      <div className={cn(
                        'text-xs font-medium w-6 h-6 flex items-center justify-center rounded-full mb-1',
                        isToday ? 'bg-primary text-primary-foreground' : 'text-muted-foreground',
                      )}>
                        {day}
                      </div>
                      <div className="space-y-0.5">
                        {dayEvents.slice(0, 3).map((e, ei) => {
                          const color = colorForAgent(e.agent_id || '0');
                          return (
                            <div key={ei} className={cn('text-xs rounded px-1 py-0.5 truncate border', color)}>
                              {e.title || e.event_type || 'Event'}
                            </div>
                          );
                        })}
                        {dayEvents.length > 3 && (
                          <div className="text-xs text-muted-foreground pl-1">+{dayEvents.length - 3} more</div>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>

              {loading && (
                <div className="flex items-center justify-center py-4 gap-2 text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" /> Loading events…
                </div>
              )}
            </div>

            {/* Day detail panel */}
            <div className="w-72 shrink-0">
              {selectedDay ? (
                <div className="rounded-xl border border-border bg-card overflow-hidden">
                  <div className="px-4 py-3 border-b border-border bg-muted/20">
                    <p className="text-sm font-semibold">
                      {new Date(selectedDay + 'T00:00:00').toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric' })}
                    </p>
                    <p className="text-xs text-muted-foreground">{selectedEvents.length} event{selectedEvents.length !== 1 ? 's' : ''}</p>
                  </div>
                  <div className="divide-y divide-border/50">
                    {selectedEvents.length === 0 ? (
                      <p className="text-sm text-muted-foreground px-4 py-6 text-center">No events this day</p>
                    ) : selectedEvents.map((e, i) => {
                      const soul = souls.find(s => s.id === e.agent_id);
                      const color = colorForAgent(e.agent_id || '0');
                      return (
                        <div key={i} className="px-4 py-3">
                          <div className="flex items-start gap-2">
                            <div className={cn('w-1.5 h-1.5 rounded-full mt-1.5 shrink-0', color.replace('bg-', 'bg-').split(' ')[0] ?? '')} />
                            <div className="min-w-0">
                              <p className="text-sm font-medium">{e.title || e.event_type || 'Event'}</p>
                              {soul && <p className="text-xs text-muted-foreground">{soul.display_name}</p>}
                              {e.start_time && (
                                <p className="text-xs text-muted-foreground">
                                  {new Date(e.start_time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                                  {e.end_time && ` – ${new Date(e.end_time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`}
                                </p>
                              )}
                              {e.description && <p className="text-xs text-muted-foreground mt-0.5 line-clamp-2">{e.description}</p>}
                            </div>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </div>
              ) : (
                <div className="rounded-xl border border-dashed border-border p-6 text-center">
                  <CalendarDays className="h-8 w-8 mx-auto mb-2 text-muted-foreground/30" />
                  <p className="text-sm text-muted-foreground">Click a day to see its events</p>
                </div>
              )}

              {/* Upcoming */}
              {events.length > 0 && (
                <div className="mt-4 space-y-2">
                  <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">Upcoming</p>
                  {events
                    .filter(e => e.start_time && new Date(e.start_time) >= today)
                    .sort((a, b) => new Date(a.start_time).getTime() - new Date(b.start_time).getTime())
                    .slice(0, 5)
                    .map((e, i) => {
                      const soul = souls.find(s => s.id === e.agent_id);
                      return (
                        <div key={i} className="flex items-center gap-2.5 rounded-lg border border-border bg-card px-3 py-2">
                          <div className={cn('w-1.5 h-8 rounded-full shrink-0', colorForAgent(e.agent_id || '0').split(' ')[0] ?? '')} />
                          <div className="min-w-0">
                            <p className="text-xs font-medium truncate">{e.title || e.event_type}</p>
                            <p className="text-xs text-muted-foreground">
                              {new Date(e.start_time).toLocaleDateString([], { month: 'short', day: 'numeric' })}
                              {soul ? ` · ${soul.display_name}` : ''}
                            </p>
                          </div>
                        </div>
                      );
                    })}
                </div>
              )}
            </div>
          </div>
        ) : (
          /* Cron tab */
          <div className="space-y-2">
            {loading ? (
              Array.from({ length: 3 }).map((_, i) => (
                <div key={i} className="flex items-center gap-4 rounded-xl border border-border bg-card p-4">
                  <div className="h-5 w-5 animate-pulse rounded bg-muted" />
                  <div className="flex-1 space-y-1.5">
                    <div className="h-4 w-48 animate-pulse rounded bg-muted" />
                    <div className="h-3 w-32 animate-pulse rounded bg-muted" />
                  </div>
                </div>
              ))
            ) : jobs.length === 0 ? (
              <EmptyState
                icon={emptyStates.cron.icon}
                title="No cron jobs"
                description="Cron jobs let agents run tasks on a schedule. Create one from an agent's settings."
              />
            ) : jobs.map(j => {
              const soul = souls.find(s => s.id === j.agent_id);
              const isActive = j.status === 'active';
              return (
                <div key={j.id} className="flex items-center gap-4 rounded-xl border border-border bg-card p-4">
                  <button onClick={() => toggleJob(j)} className="cursor-pointer shrink-0">
                    {isActive
                      ? <Play className="h-5 w-5 text-emerald-500" />
                      : <Pause className="h-5 w-5 text-muted-foreground" />}
                  </button>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium truncate">{j.task || 'Unnamed job'}</span>
                      {soul && (
                        <span className="text-xs text-muted-foreground shrink-0">· {soul.display_name}</span>
                      )}
                    </div>
                    <div className="text-xs text-muted-foreground flex items-center gap-1.5 mt-0.5">
                      <Clock className="h-3 w-3" />
                      <code className="font-mono">{j.expression}</code>
                    </div>
                  </div>
                  <div className="text-right text-xs text-muted-foreground shrink-0 space-y-0.5">
                    {j.last_run && <div>Last: {new Date(j.last_run).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</div>}
                    {j.next_run && <div>Next: {new Date(j.next_run).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</div>}
                  </div>
                  <span className={cn('text-xs px-2 py-0.5 rounded-full shrink-0',
                    isActive ? 'bg-emerald-500/10 text-emerald-500' : 'bg-muted text-muted-foreground')}>
                    {isActive ? 'Active' : 'Paused'}
                  </span>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </ErrorBoundary>
  );
}
