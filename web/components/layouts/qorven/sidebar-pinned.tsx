'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// SidebarPinned — the always-present zone pinned above the COO dock on every
// page: Hubs (company room first) + Recent chats. Capped height with its own
// scroll so it never pushes the contextual content off-screen.

import { useEffect, useState } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { useStore } from '@/store';
import { rooms as roomsApi } from '@/lib/api';
import { SidebarHubRow, type HubMember } from './sidebar-hub-row';
import { Plus, MessageSquare } from 'lucide-react';

export function SidebarPinned() {
  const router = useRouter();
  const pathname = usePathname();
  const souls = useStore((s) => s.souls);

  const [hubs, setHubs] = useState<any[]>([]);
  const [hubMembers, setHubMembers] = useState<Record<string, HubMember[]>>({});

  useEffect(() => {
    let cancelled = false;
    roomsApi.list().then((d: any) => {
      const list: any[] = Array.isArray(d?.rooms) ? d.rooms : Array.isArray(d) ? d : [];
      if (cancelled) return;
      setHubs(list);
      list.forEach((h: any) => {
        roomsApi.org(h.id).then((org: any) => {
          if (cancelled) return;
          const members: HubMember[] = (org?.members ?? []).map((m: any) => ({
            id: m.id ?? m.agent_id,
            display_name: m.display_name ?? m.agent_key ?? 'Agent',
            avatar: m.avatar,
          }));
          setHubMembers((prev) => ({ ...prev, [h.id]: members }));
        }).catch(() => {});
      });
    }).catch(() => {});
    return () => { cancelled = true; };
  }, []);

  const currentHubId = pathname?.match(/^\/rooms\/([^/]+)/)?.[1] ?? null;
  const recent = [...souls].slice(0, 5);

  return (
    <div className="shrink-0 border-t border-border bg-muted/40 max-h-[44vh] overflow-y-auto px-2 py-2">
      <div className="flex items-center justify-between px-1 pb-1">
        <span className="text-2xs font-semibold uppercase tracking-wide text-muted-foreground/70">Hubs</span>
        <button onClick={() => router.push('/rooms')} title="All hubs" className="text-muted-foreground hover:text-foreground">
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>
      <div className="flex flex-col gap-px">
        {hubs.map((h) => (
          <SidebarHubRow
            key={h.id}
            id={h.id}
            name={h.name}
            displayName={h.display_name}
            members={hubMembers[h.id] ?? []}
            isActive={currentHubId === h.id}
            onClick={() => router.push(`/rooms/${h.id}`)}
          />
        ))}
        {hubs.length === 0 && <p className="px-1 py-1 text-2xs text-muted-foreground">No hubs yet.</p>}
      </div>

      <div className="px-1 pt-3 pb-1">
        <span className="text-2xs font-semibold uppercase tracking-wide text-muted-foreground/70">Recent chats</span>
      </div>
      <div className="flex flex-col gap-px">
        {recent.map((s) => (
          <button
            key={s.id}
            onClick={() => router.push(`/qors/${s.id}`)}
            className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-2sm text-muted-foreground hover:bg-muted/60 hover:text-foreground transition-colors"
          >
            <MessageSquare className="h-3.5 w-3.5 shrink-0" />
            <span className="flex-1 truncate">{s.display_name || s.agent_key}</span>
          </button>
        ))}
        {recent.length === 0 && <p className="px-1 py-1 text-2xs text-muted-foreground">No chats yet.</p>}
      </div>
    </div>
  );
}
