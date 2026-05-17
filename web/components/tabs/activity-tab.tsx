'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState, useMemo } from 'react';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { Terminal, Globe, Brain, MessageSquare, AlertCircle, Settings2, Wrench, ChevronDown, Download } from 'lucide-react';

const eventIcons: Record<string, { icon: typeof Terminal; color: string }> = {
  exec: { icon: Terminal, color: 'text-primary/70 bg-primary/10' },
  web_search: { icon: Globe, color: 'text-emerald-400 bg-emerald-400/10' },
  web_fetch: { icon: Globe, color: 'text-emerald-400 bg-emerald-400/10' },
  memory_search: { icon: Brain, color: 'text-pink-400 bg-pink-400/10' },
  memory_get: { icon: Brain, color: 'text-pink-400 bg-pink-400/10' },
  new_message: { icon: MessageSquare, color: 'text-blue-400 bg-blue-400/10' },
  soul_activity: { icon: Settings2, color: 'text-amber-400 bg-amber-400/10' },
  error: { icon: AlertCircle, color: 'text-destructive bg-destructive/10' },
};

interface Props { agentId: string }

export function ActivityTab({ agentId }: Props) {
  const liveEvents = useStore((s) => s.liveEvents);
  const [filter, setFilter] = useState('all');
  const events = useMemo(() => {
    const filtered = liveEvents.filter((e) => e.agent_id === agentId);
    if (filter === 'all') return filtered;
    return filtered.filter((e) => e.type === filter || e.detail?.includes(filter));
  }, [liveEvents, agentId, filter]);

  // Group by date
  const grouped = useMemo(() => {
    const groups: Record<string, typeof events> = {};
    events.forEach((e) => {
      const d = new Date(e.timestamp);
      const today = new Date();
      const yesterday = new Date(today); yesterday.setDate(today.getDate() - 1);
      let label = d.toLocaleDateString();
      if (d.toDateString() === today.toDateString()) label = 'Today';
      else if (d.toDateString() === yesterday.toDateString()) label = 'Yesterday';
      if (!groups[label]) groups[label] = [];
      groups[label]!.push(e);
    });
    return groups;
  }, [events]);

  return (
    <div className="max-w-3xl mx-auto">
      {/* Header */}
      <div className="flex items-center gap-3 mb-4">
        <select value={filter} onChange={(e) => setFilter(e.target.value)}
          className="rounded-lg border border-border bg-background px-3 py-1.5 text-xs">
          <option value="all">All Events</option>
          <option value="exec">Tools</option>
          <option value="new_message">Messages</option>
          <option value="memory">Memory</option>
          <option value="error">Errors</option>
        </select>
        <div className="flex-1" />
        <span className="text-2xs text-muted-foreground">{events.length} events</span>
      </div>

      {/* Timeline */}
      {events.length === 0 ? (
        <p className="py-12 text-center text-sm text-muted-foreground">No activity yet — events appear as this Qor works</p>
      ) : (
        <div className="space-y-6">
          {Object.entries(grouped).map(([date, items]) => (
            <div key={date}>
              <p className="text-2xs font-medium uppercase tracking-wider text-muted-foreground mb-3">{date}</p>
              <div className="relative border-l-2 border-border ml-3 space-y-0.5">
                {items.map((e) => {
                  const meta = eventIcons[e.type] ?? { icon: Wrench, color: 'text-muted-foreground bg-muted' };
                  const Icon = meta.icon;
                  return (
                    <div key={e.id} className="relative pl-6 py-1.5 group hover:bg-accent/30 rounded-r-lg">
                      <div className={cn('absolute -left-[9px] top-2.5 h-4 w-4 rounded-full flex items-center justify-center', meta.color)}>
                        <Icon className="h-2.5 w-2.5" />
                      </div>
                      <div className="flex items-baseline gap-2">
                        <span className="text-2xs text-muted-foreground w-12 shrink-0">{new Date(e.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                        <span className="text-xs">{e.detail || e.type}</span>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
