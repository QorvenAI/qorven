'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { cn } from '@/lib/utils';
import { Plug, Plus } from 'lucide-react';
import { SidebarMenuItem, SidebarDivider, SidebarGroupTitle } from './sidebar-primitives';

export function McpSidebar() {
  const router = useRouter();
  const [servers, setServers] = useState<any[]>([]);
  useEffect(() => {
    fetch('/api/v1/mcp/servers', { headers: { Authorization: `Bearer ${typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : ''}` } })
      .then(r => r.json()).then(d => setServers(Array.isArray(d) ? d : d?.servers ?? [])).catch(() => {});
  }, []);
  return (
    <>
      <SidebarGroupTitle>MCP Servers ({servers.length})</SidebarGroupTitle>
      <ul className="flex flex-col gap-px px-2.5">
        {servers.map((s: any) => (
          <SidebarMenuItem key={s.id || s.name} icon={Plug}
            label={<span className="flex items-center gap-1.5">{s.name} <span className={cn('h-1.5 w-1.5 rounded-full', s.status === 'connected' ? 'bg-emerald-400' : 'bg-muted-foreground/30')} /></span> as any}
            onClick={() => router.push('/mcp')} />
        ))}
        {servers.length === 0 && <li className="px-3 py-2 text-xs text-muted-foreground">No servers connected</li>}
      </ul>
      <SidebarDivider />
      <ul className="flex flex-col gap-px px-2.5">
        <SidebarMenuItem icon={Plus} label="Add Server" onClick={() => router.push('/mcp')} />
      </ul>
    </>
  );
}
