'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useRouter } from 'next/navigation';
import { useStore } from '@/store';
import { Activity, Shield, BarChart3 } from 'lucide-react';
import { SidebarMenuItem, SidebarDivider, SidebarGroupTitle } from './sidebar-primitives';

export function HeartbeatSidebar() {
  const router = useRouter();
  const souls = useStore((s) => s.souls);
  const soulStates = useStore((s) => s.soulStates);
  const healthy = souls.filter(s => (soulStates[s.id]?.activity ?? 'idle') !== 'error').length;
  const errors = souls.filter(s => soulStates[s.id]?.activity === 'error').length;
  return (
    <>
      <SidebarGroupTitle>System Health</SidebarGroupTitle>
      <div className="px-3 py-2 space-y-1.5">
        <div className="flex items-center justify-between text-xs">
          <span className="text-muted-foreground">Healthy</span>
          <span className="text-emerald-400 font-medium">{healthy}</span>
        </div>
        {errors > 0 && (
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">Errors</span>
            <span className="text-destructive font-medium">{errors}</span>
          </div>
        )}
        <div className="flex items-center justify-between text-xs">
          <span className="text-muted-foreground">Total Agents</span>
          <span className="font-medium">{souls.length}</span>
        </div>
      </div>
      <SidebarDivider />
      <SidebarGroupTitle>Monitoring</SidebarGroupTitle>
      <ul className="flex flex-col gap-px px-2.5">
        <SidebarMenuItem icon={Activity} label="Heartbeat" onClick={() => router.push('/heartbeat')} />
        <SidebarMenuItem icon={Shield} label="Supervisor" onClick={() => router.push('/supervisor')} />
        <SidebarMenuItem icon={BarChart3} label="Analytics" onClick={() => router.push('/analytics')} />
      </ul>
    </>
  );
}
