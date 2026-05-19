'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { Users, GitBranch, Shield } from 'lucide-react';
import { SidebarMenuItem, SidebarDivider, SidebarGroupTitle } from './sidebar-primitives';
import { SidebarLayout } from './sidebar-layout';

export function TeamsSidebar() {
  const router = useRouter();
  const [teams, setTeams] = useState<any[]>([]);
  useEffect(() => {
    fetch('/api/v1/teams', { headers: { Authorization: `Bearer ${typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : ''}` } })
      .then(r => r.json()).then(d => setTeams(Array.isArray(d) ? d : d?.teams ?? [])).catch(() => {});
  }, []);
  return (
    <SidebarLayout
      section3={
        <>
          <SidebarGroupTitle>Teams ({teams.length})</SidebarGroupTitle>
          <ul className="flex flex-col gap-px px-2.5">
            {teams.slice(0, 10).map((t: any) => (
              <SidebarMenuItem key={t.id} icon={Users} label={`${t.name} (${t.member_count ?? 0})`} onClick={() => router.push('/teams')} />
            ))}
            {teams.length === 0 && <li className="px-3 py-2 text-xs text-muted-foreground">No teams yet</li>}
          </ul>
          <SidebarDivider />
          <SidebarGroupTitle>Navigation</SidebarGroupTitle>
          <ul className="flex flex-col gap-px px-2.5">
            <SidebarMenuItem icon={Users} label="Rooms" onClick={() => router.push('/rooms')} />
            <SidebarMenuItem icon={GitBranch} label="Org Chart" onClick={() => router.push('/org-chart')} />
            <SidebarMenuItem icon={Shield} label="Supervisor" onClick={() => router.push('/supervisor')} />
          </ul>
        </>
      }
    />
  );
}
