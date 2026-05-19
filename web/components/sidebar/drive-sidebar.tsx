'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { soulGradient } from '@/components/soul-card';
import {
  HardDrive, FolderOpen, Image, FileCode, FileSpreadsheet, Upload,
  ChevronsUpDown, Users,
} from 'lucide-react';
import { SidebarMenuItem, SidebarDivider, SidebarGroupTitle } from './sidebar-primitives';

export function DriveSidebar() {
  const souls = useStore((s) => s.souls);
  const driveSoulFilter = useStore((s) => s.driveSoulFilter);
  const setDriveSoulFilter = useStore((s) => s.setDriveSoulFilter);
  const [pickerOpen, setPickerOpen] = useState(false);
  const router = useRouter();
  const activeSoul = driveSoulFilter ? souls.find((s) => s.id === driveSoulFilter) : null;

  return (
    <>
      <div className="relative flex h-[44px] shrink-0 items-center border-b border-border px-2">
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
            <button onClick={() => { setDriveSoulFilter(null); setPickerOpen(false); }}
              className={cn('flex w-full items-center gap-2.5 px-3 py-2 text-2sm hover:bg-accent', !driveSoulFilter && 'bg-accent font-medium')}>
              <Users className="h-4 w-4 text-muted-foreground" />All Agents
            </button>
            <div className="my-1 border-t border-border" />
            {souls.map((s) => (
              <button key={s.id} onClick={() => { setDriveSoulFilter(s.id); setPickerOpen(false); }}
                className={cn('flex w-full items-center gap-2.5 px-3 py-2 text-2sm hover:bg-accent', driveSoulFilter === s.id && 'bg-accent font-medium')}>
                <div className={cn('flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br text-2xs font-semibold text-white', soulGradient(s.display_name))}>
                  {s.display_name.charAt(0)}
                </div>
                {s.display_name}
              </button>
            ))}
          </div>
        )}
      </div>

      <SidebarGroupTitle>Browse</SidebarGroupTitle>
      <ul className="flex flex-col gap-px px-2.5">
        <SidebarMenuItem icon={HardDrive} label="All Files" active onClick={() => router.push('/drive')} />
        <SidebarMenuItem icon={FolderOpen} label="Folders" onClick={() => router.push('/drive?view=folders')} />
        <SidebarMenuItem icon={Image} label="Images" onClick={() => router.push('/drive?type=image')} />
        <SidebarMenuItem icon={FileCode} label="Code" onClick={() => router.push('/drive?type=code')} />
        <SidebarMenuItem icon={FileSpreadsheet} label="Documents" onClick={() => router.push('/drive?type=document')} />
      </ul>

      <SidebarDivider />
      <SidebarGroupTitle>Quick Actions</SidebarGroupTitle>
      <ul className="flex flex-col gap-px px-2.5">
        <SidebarMenuItem icon={Upload} label="Upload File" />
        <SidebarMenuItem icon={FolderOpen} label="New Folder" />
      </ul>
    </>
  );
}
