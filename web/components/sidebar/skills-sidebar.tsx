'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { Link2, Sparkles } from 'lucide-react';
import { SidebarMenuItem, SidebarDivider, SidebarGroupTitle } from './sidebar-primitives';
import { SidebarLayout } from './sidebar-layout';

const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

export function SkillsSidebar() {
  const router = useRouter();
  const [marketplace, setMarketplace] = useState<any[]>([]);
  useEffect(() => {
    fetch('/api/v1/marketplace/skills', { headers: { Authorization: `Bearer ${getToken()}` } })
      .then((r) => r.json()).then((d) => setMarketplace(d.skills || [])).catch(() => {});
  }, []);

  const mcp = marketplace.filter((s: any) => s.type === 'mcp');
  const skills = marketplace.filter((s: any) => s.type !== 'mcp' && s.type !== 'crystallized');

  return (
    <SidebarLayout
      section3={
        <>
          <SidebarGroupTitle>MCP Plugins ({mcp.length})</SidebarGroupTitle>
          <ul className="flex flex-col gap-px px-2.5">
            {mcp.slice(0, 8).map((s: any) => (
              <SidebarMenuItem key={s.id} icon={Link2} label={s.name}
                onClick={() => router.push('/skills')} />
            ))}
          </ul>
          <SidebarDivider />
          <SidebarGroupTitle>Skills ({skills.length})</SidebarGroupTitle>
          <ul className="flex flex-col gap-px px-2.5">
            {skills.slice(0, 8).map((s: any) => (
              <SidebarMenuItem key={s.id} icon={Sparkles} label={s.name}
                onClick={() => router.push('/skills')} />
            ))}
          </ul>
          <SidebarDivider />
          <div className="px-2.5">
            <SidebarMenuItem icon={Sparkles} label="Browse All Skills"
              badge={String(marketplace.length)} badgeColor="bg-primary/10 text-primary"
              onClick={() => router.push('/skills')} />
          </div>
        </>
      }
    />
  );
}
