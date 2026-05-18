'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import { Calendar as CalIcon, Plus, Clock, Megaphone, ChevronLeft, ChevronRight, ExternalLink } from 'lucide-react';
import { calendarApi, social as socialApi } from '@/lib/api';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import Link from 'next/link';

const MONTHS = ['January','February','March','April','May','June','July','August','September','October','November','December'];
const DAYS = ['Sun','Mon','Tue','Wed','Thu','Fri','Sat'];

type CalEvent = { id: string; title: string; start_time: string; end_time?: string; agent_id?: string; description?: string; event_type?: string };
type SocialPost = { id: string; content: string; platforms: string[]; status: string; scheduled_at?: string; agent_id?: string };

type DayEntry = {
  date: string;
  events: CalEvent[];
  posts: SocialPost[];
};

export default function CalendarPage() {
  const [today] = useState(() => new Date());
  const [current, setCurrent] = useState(() => new Date());
  const [events, setEvents] = useState<CalEvent[]>([]);
  const [socialEntries, setSocialEntries] = useState<Record<string, SocialPost[]>>({});
  const [loading, setLoading] = useState(true);
  const [selectedDay, setSelectedDay] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState({ title: '', start_time: '', end_time: '', description: '' });
  const [view, setView] = useState<'month' | 'list'>('month');

  const calFilter = useStore(s => s.calSoulFilter);
  const souls = useStore(s => s.souls);

  const year = current.getFullYear();
  const month = current.getMonth();
  const firstDay = new Date(year, month, 1).getDay();
  const daysInMonth = new Date(year, month + 1, 0).getDate();

  const load = useCallback(() => {
    setLoading(true);
    const start = new Date(year, month, 1).toISOString();
    const end = new Date(year, month + 1, 0, 23, 59, 59).toISOString();

    Promise.all([
      calendarApi.list(start, end, calFilter ?? undefined).catch(() => []),
      socialApi.calendar(calFilter ?? undefined).catch(() => ({ entries: [] })),
    ]).then(([evts, socialCal]) => {
      setEvents(Array.isArray(evts) ? evts : []);
      // Build date → posts map
      const postMap: Record<string, SocialPost[]> = {};
      ((socialCal as any)?.entries ?? []).forEach((e: any) => {
        postMap[e.date] = e.posts || [];
      });
      setSocialEntries(postMap);
      setLoading(false);
    });
  }, [year, month, calFilter]);

  useEffect(() => { load(); }, [load]);

  const createEvent = async () => {
    if (!form.title) return;
    await calendarApi.create({
      title: form.title,
      start_time: form.start_time,
      end_time: form.end_time || undefined,
      description: form.description || undefined,
      agent_id: calFilter || undefined,
    });
    setShowCreate(false);
    setForm({ title: '', start_time: '', end_time: '', description: '' });
    load();
  };

  // Build event map by date
  const eventsByDate: Record<string, CalEvent[]> = {};
  events.forEach(e => {
    if (e.start_time) {
      const d = e.start_time.slice(0, 10);
      eventsByDate[d] = eventsByDate[d] ?? [];
      eventsByDate[d].push(e);
    }
  });

  const todayKey = today.toISOString().slice(0, 10);

  const cells: (number | null)[] = [
    ...Array(firstDay).fill(null),
    ...Array.from({ length: daysInMonth }, (_, i) => i + 1),
  ];
  while (cells.length % 7 !== 0) cells.push(null);

  const selectedDateKey = selectedDay;
  const selectedEvents = selectedDateKey ? (eventsByDate[selectedDateKey] ?? []) : [];
  const selectedPosts = selectedDateKey ? (socialEntries[selectedDateKey] ?? []) : [];

  // Upcoming events for the next 7 days
  const upcoming: DayEntry[] = [];
  for (let i = 0; i < 7; i++) {
    const d = new Date(today);
    d.setDate(d.getDate() + i);
    const key = d.toISOString().slice(0, 10);
    const evts = eventsByDate[key] ?? [];
    const posts = socialEntries[key] ?? [];
    if (evts.length > 0 || posts.length > 0) {
      upcoming.push({ date: key, events: evts, posts });
    }
  }

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold flex items-center gap-2">
            <CalIcon className="h-5 w-5" /> Calendar
          </h1>
          <p className="text-sm text-muted-foreground">
            {calFilter ? souls.find(s => s.id === calFilter)?.display_name ?? 'Agent' : 'All Agents'} · events and social posts
          </p>
        </div>
        <div className="flex items-center gap-2">
          {/* View toggle */}
          <div className="flex rounded-lg border border-border overflow-hidden">
            <button onClick={() => setView('month')}
              className={cn('px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer', view === 'month' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent')}>
              Month
            </button>
            <button onClick={() => setView('list')}
              className={cn('px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer', view === 'list' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent')}>
              List
            </button>
          </div>
          <button onClick={() => setShowCreate(v => !v)}
            className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 cursor-pointer">
            <Plus className="h-4 w-4" /> New Event
          </button>
        </div>
      </div>

      {/* Create form */}
      {showCreate && (
        <div className="rounded-xl border border-border bg-card p-4 space-y-3">
          <input placeholder="Event title *" value={form.title} onChange={e => setForm(f => ({ ...f, title: e.target.value }))}
            className="qr-input" autoFocus />
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-muted-foreground">Start</label>
              <input type="datetime-local" value={form.start_time} onChange={e => setForm(f => ({ ...f, start_time: e.target.value }))}
                className="mt-1 qr-input" />
            </div>
            <div>
              <label className="text-xs text-muted-foreground">End</label>
              <input type="datetime-local" value={form.end_time} onChange={e => setForm(f => ({ ...f, end_time: e.target.value }))}
                className="mt-1 qr-input" />
            </div>
          </div>
          <textarea placeholder="Description (optional)" value={form.description} onChange={e => setForm(f => ({ ...f, description: e.target.value }))}
            rows={2} className="qr-textarea resize-none" />
          <div className="flex gap-2">
            <button onClick={createEvent} disabled={!form.title}
              className="rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
              Create Event
            </button>
            <button onClick={() => setShowCreate(false)}
              className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer">
              Cancel
            </button>
          </div>
        </div>
      )}

      {view === 'month' ? (
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
                if (!day) return <div key={i} className="bg-background/50 min-h-[80px] p-1" />;
                const key = `${year}-${String(month + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
                const dayEvents = eventsByDate[key] ?? [];
                const dayPosts = socialEntries[key] ?? [];
                const total = dayEvents.length + dayPosts.length;
                const isToday = key === todayKey;
                const isSelected = key === selectedDay;

                return (
                  <div key={i} onClick={() => setSelectedDay(isSelected ? null : key)}
                    className={cn(
                      'bg-background min-h-[80px] p-1.5 cursor-pointer hover:bg-accent/30 transition-colors',
                      isSelected && 'bg-primary/5 ring-1 ring-inset ring-primary/20',
                    )}>
                    <div className={cn(
                      'text-xs font-medium w-6 h-6 flex items-center justify-center rounded-full mb-1',
                      isToday ? 'bg-primary text-primary-foreground' : 'text-muted-foreground',
                    )}>
                      {day}
                    </div>
                    <div className="space-y-0.5">
                      {dayEvents.slice(0, 2).map((e, ei) => (
                        <div key={ei} className="text-xs rounded px-1 py-0.5 truncate bg-primary/10 text-primary">
                          <Clock className="inline h-2.5 w-2.5 mr-0.5" />{e.title}
                        </div>
                      ))}
                      {dayPosts.slice(0, 2).map((p, pi) => (
                        <div key={pi} className={cn(
                          'text-xs rounded px-1 py-0.5 truncate',
                          p.status === 'scheduled' ? 'bg-blue-500/10 text-blue-500' :
                          p.status === 'published' ? 'bg-emerald-500/10 text-emerald-500' :
                          'bg-muted text-muted-foreground'
                        )}>
                          <Megaphone className="inline h-2.5 w-2.5 mr-0.5" />{p.content?.slice(0, 16)}
                        </div>
                      ))}
                      {total > 3 && <div className="text-xs text-muted-foreground pl-1">+{total - 3} more</div>}
                    </div>
                  </div>
                );
              })}
            </div>

            {loading && (
              <div className="flex items-center justify-center py-4 gap-2 text-sm text-muted-foreground">
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
                Loading…
              </div>
            )}
          </div>

          {/* Day detail panel */}
          <div className="w-72 shrink-0 space-y-3">
            {selectedDay ? (
              <>
                <div className="rounded-xl border border-border bg-card overflow-hidden">
                  <div className="px-4 py-3 border-b border-border bg-muted/20">
                    <p className="text-sm font-semibold">
                      {new Date(selectedDay + 'T00:00:00').toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric' })}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {selectedEvents.length} event{selectedEvents.length !== 1 ? 's' : ''} · {selectedPosts.length} social post{selectedPosts.length !== 1 ? 's' : ''}
                    </p>
                  </div>
                  <div className="divide-y divide-border/50 max-h-96 overflow-y-auto">
                    {selectedEvents.map((e, i) => (
                      <div key={i} className="px-4 py-2.5">
                        <div className="flex items-start gap-2">
                          <Clock className="h-3.5 w-3.5 text-primary shrink-0 mt-0.5" />
                          <div className="min-w-0">
                            <p className="text-sm font-medium">{e.title}</p>
                            <p className="text-xs text-muted-foreground">
                              {new Date(e.start_time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                              {e.end_time && ` – ${new Date(e.end_time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`}
                            </p>
                          </div>
                        </div>
                      </div>
                    ))}
                    {selectedPosts.map((p, i) => (
                      <div key={i} className="px-4 py-2.5">
                        <div className="flex items-start gap-2">
                          <Megaphone className="h-3.5 w-3.5 text-blue-500 shrink-0 mt-0.5" />
                          <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-1.5 mb-0.5">
                              <span className={cn('text-xs px-1.5 py-0.5 rounded font-medium',
                                p.status === 'scheduled' ? 'bg-blue-500/10 text-blue-500' :
                                p.status === 'published' ? 'bg-emerald-500/10 text-emerald-500' :
                                'bg-muted text-muted-foreground'
                              )}>{p.status}</span>
                              <span className="text-xs text-muted-foreground">{(p.platforms || []).join(', ')}</span>
                            </div>
                            <p className="text-xs text-foreground/80 line-clamp-2">{p.content}</p>
                          </div>
                        </div>
                      </div>
                    ))}
                    {selectedEvents.length === 0 && selectedPosts.length === 0 && (
                      <p className="text-sm text-muted-foreground text-center py-6">Nothing scheduled this day</p>
                    )}
                  </div>
                </div>
              </>
            ) : (
              <>
                {/* Upcoming widget */}
                <div className="rounded-xl border border-border bg-card overflow-hidden">
                  <div className="px-4 py-3 border-b border-border bg-muted/20 flex items-center justify-between">
                    <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Upcoming 7 days</p>
                    <Link href="/social?tab=calendar" className="text-xs text-primary hover:underline flex items-center gap-1">
                      Social calendar <ExternalLink className="h-3 w-3" />
                    </Link>
                  </div>
                  <div className="divide-y divide-border/50 max-h-80 overflow-y-auto">
                    {upcoming.length === 0 ? (
                      <p className="text-sm text-muted-foreground text-center py-6">Nothing in the next 7 days</p>
                    ) : upcoming.map(({ date, events: evts, posts }) => (
                      <div key={date} className="px-4 py-2.5">
                        <p className="text-xs font-medium text-muted-foreground mb-1.5">
                          {new Date(date + 'T00:00:00').toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })}
                        </p>
                        {evts.map((e, i) => (
                          <div key={i} className="flex items-center gap-1.5 text-xs mb-1">
                            <Clock className="h-3 w-3 text-primary shrink-0" />
                            <span className="truncate">{e.title}</span>
                          </div>
                        ))}
                        {posts.map((p, i) => (
                          <div key={i} className="flex items-center gap-1.5 text-xs mb-1">
                            <Megaphone className="h-3 w-3 text-blue-400 shrink-0" />
                            <span className="truncate text-muted-foreground">{p.content?.slice(0, 40)}</span>
                            <span className={cn('ml-auto shrink-0 text-xs',
                              p.status === 'scheduled' ? 'text-blue-400' : 'text-emerald-400'
                            )}>{p.status}</span>
                          </div>
                        ))}
                      </div>
                    ))}
                  </div>
                </div>
              </>
            )}
          </div>
        </div>
      ) : (
        /* List view — chronological all entries */
        <div className="space-y-2">
          {[...events]
            .sort((a, b) => new Date(a.start_time).getTime() - new Date(b.start_time).getTime())
            .map(e => (
              <div key={e.id} className="flex items-center gap-4 rounded-xl border border-border bg-card px-4 py-3">
                <Clock className="h-4 w-4 text-primary shrink-0" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium">{e.title}</p>
                  <p className="text-xs text-muted-foreground">
                    {new Date(e.start_time).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}
                    {e.end_time && ` → ${new Date(e.end_time).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`}
                  </p>
                </div>
                {e.event_type && <span className="text-xs bg-muted px-2 py-0.5 rounded">{e.event_type}</span>}
              </div>
            ))}
          {events.length === 0 && !loading && (
            <div className="rounded-xl border border-dashed border-border py-12 text-center">
              <CalIcon className="h-8 w-8 mx-auto mb-2 text-muted-foreground/30" />
              <p className="text-sm text-muted-foreground">No events this month</p>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
