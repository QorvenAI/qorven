'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { soulGradient } from '@/components/soul-card';
import { ListChecks, ListTodo, CircleDot, CheckCircle2, ChevronsUpDown, Users } from 'lucide-react';
import { SidebarMenuItem } from './sidebar-primitives';
import { SidebarLayout } from './sidebar-layout';

export function TasksSidebar() {
  const souls = useStore((s) => s.souls);
  const taskAgentFilter = useStore((s) => s.taskAgentFilter);
  const setTaskAgentFilter = useStore((s) => s.setTaskAgentFilter);
  const [pickerOpen, setPickerOpen] = useState(false);
  const activeSoul = taskAgentFilter ? souls.find((s) => s.id === taskAgentFilter) : null;

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
          <button onClick={() => { setTaskAgentFilter(null); setPickerOpen(false); }}
            className={cn('flex w-full items-center gap-2.5 px-3 py-2 text-2sm hover:bg-accent', !taskAgentFilter && 'bg-accent font-medium')}>
            <Users className="h-4 w-4 text-muted-foreground" /> All Agents
          </button>
          <div className="my-1 border-t border-border" />
          {souls.map((s) => (
            <button key={s.id} onClick={() => { setTaskAgentFilter(s.id); setPickerOpen(false); }}
              className={cn('flex w-full items-center gap-2.5 px-3 py-2 text-2sm hover:bg-accent', taskAgentFilter === s.id && 'bg-accent font-medium')}>
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
        <ul className="flex flex-col gap-px px-2.5">
          <SidebarMenuItem icon={ListChecks} label="All Tasks" active />
          <SidebarMenuItem icon={ListTodo} label="To Do" />
          <SidebarMenuItem icon={CircleDot} label="In Progress" />
          <SidebarMenuItem icon={CheckCircle2} label="Done" />
        </ul>
      }
    />
  );
}
