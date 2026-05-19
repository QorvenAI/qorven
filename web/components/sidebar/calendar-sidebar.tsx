'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { soulGradient } from '@/components/soul-card';
import { CalendarDays, Clock, AlertCircle, CheckSquare, ChevronsUpDown, Users } from 'lucide-react';
import { SidebarMenuItem, SidebarDivider, SidebarGroupTitle } from './sidebar-primitives';
import { SidebarLayout } from './sidebar-layout';

const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

export function CalendarSidebar() {
  const souls = useStore((s) => s.souls);
  const calSoulFilter = useStore((s) => s.calSoulFilter);
  const setCalSoulFilter = useStore((s) => s.setCalSoulFilter);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [jobs, setJobs] = useState<any[]>([]);
  const activeSoul = calSoulFilter ? souls.find((s) => s.id === calSoulFilter) : null;

  useEffect(() => {
    fetch('/api/v1/cron-jobs', { headers: { Authorization: `Bearer ${getToken()}` } })
      .then((r) => r.json()).then((d) => setJobs(Array.isArray(d) ? d : Object.values(d).find(Array.isArray) ?? [])).catch(() => {});
  }, []);

  const picker = (
    <div className="relative flex w-full items-center">
      <button onClick={() => setPickerOpen(!pickerOpen)}
        className="flex w-full items-center gap-2.5 h-8.5 rounded-md border border-input px-3 text-2sm font-medium hover:bg-accent transition-colors">
        {activeSoul ? (
          <>
            <div className={cn('flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br text-2xs font-semibold text-white', soulGradient(activeSoul.display_name))}>
              {activeSoul.display_name.charAt(0)}
            </div>
            <span className="flex-1 text-left truncate">{activeSoul.display_name}</span>
          </>
        ) : (
          <>
            <Users className="h-4 w-4 text-muted-foreground" />
            <span className="flex-1 text-left">All Agents</span>
          </>
        )}
        <ChevronsUpDown className="h-3.5 w-3.5 text-muted-foreground" />
      </button>
      {pickerOpen && (
        <div className="fixed z-[100] w-52 max-h-60 overflow-y-auto rounded-lg border border-border bg-popover shadow-lg py-1"
          style={{ left: 'calc(var(--rail-width) + var(--sidebar-default-width) + 4px)', top: 'calc(var(--header-height) + 8px)' }}>
          <button onClick={() => { setCalSoulFilter(null); setPickerOpen(false); }}
            className={cn('flex w-full items-center gap-2.5 px-3 py-2 text-2sm hover:bg-accent', !calSoulFilter && 'bg-accent font-medium')}>
            <Users className="h-4 w-4 text-muted-foreground" />All Agents
          </button>
          <div className="my-1 border-t border-border" />
          {souls.map((s) => (
            <button key={s.id} onClick={() => { setCalSoulFilter(s.id); setPickerOpen(false); }}
              className={cn('flex w-full items-center gap-2.5 px-3 py-2 text-2sm hover:bg-accent', calSoulFilter === s.id && 'bg-accent font-medium')}>
              <div className={cn('flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br text-2xs font-semibold text-white', soulGradient(s.display_name))}>
                {s.display_name.charAt(0)}
              </div>
              {s.display_name}
            </button>
          ))}
        </div>
      )}
    </div>
  );

  return (
    <SidebarLayout
      section2={picker}
      section3={
        <>
          <SidebarGroupTitle>Event Types</SidebarGroupTitle>
          <ul className="flex flex-col gap-px px-2.5">
            <SidebarMenuItem icon={CalendarDays} label="All Events" active />
            <SidebarMenuItem icon={Clock} label="Schedules" />
            <SidebarMenuItem icon={AlertCircle} label="Deadlines" />
            <SidebarMenuItem icon={CheckSquare} label="Tasks" />
          </ul>
          <SidebarDivider />
          <SidebarGroupTitle>Upcoming</SidebarGroupTitle>
          <div className="px-2.5 space-y-0.5">
            {jobs.slice(0, 8).map((j: any) => (
              <div key={j.id} className="flex items-center gap-2.5 h-8.5 px-2.5 rounded-md text-2sm text-muted-foreground">
                <Clock className="h-4 w-4 shrink-0" />
                <span className="truncate">{j.task?.slice(0, 25) || 'Task'}</span>
              </div>
            ))}
            {jobs.length === 0 && <p className="px-2.5 py-4 text-2sm text-muted-foreground">No upcoming</p>}
          </div>
        </>
      }
    />
  );
}
