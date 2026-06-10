'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// SidebarPinned — the always-present zone pinned above the COO dock on every
// page: Hubs (company room first) + Recent chats. Each section shows ~3 rows by
// default and grows with screen height; longer lists scroll within the section
// and expose a search box, so the zone never pushes the contextual content off
// screen or runs under the dock.

import { useEffect, useMemo, useState } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { useStore } from '@/store';
import { rooms as roomsApi } from '@/lib/api';
import { SidebarHubRow, type HubMember } from './sidebar-hub-row';
import { Plus, MessageSquare, Search } from 'lucide-react';

// A list longer than this gets its own search box and a scroll area.
const SEARCH_THRESHOLD = 3;
// Each section's scroll area: ~3 rows on short screens, more when tall.
const LIST_MAX_H = 'max-h-[clamp(108px,16vh,260px)]';

export function SidebarPinned() {
  const router = useRouter();
  const pathname = usePathname();
  const souls = useStore((s) => s.souls);

  const [hubs, setHubs] = useState<any[]>([]);
  const [hubMembers, setHubMembers] = useState<Record<string, HubMember[]>>({});
  const [hubQuery, setHubQuery] = useState('');
  const [chatQuery, setChatQuery] = useState('');

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

  const hubLabel = (h: any) => String(h.display_name || h.name || '');
  const chatLabel = (s: any) => String(s.display_name || s.agent_key || '');

  const filteredHubs = useMemo(() => {
    const q = hubQuery.trim().toLowerCase();
    return q ? hubs.filter((h) => hubLabel(h).toLowerCase().includes(q)) : hubs;
  }, [hubs, hubQuery]);

  const filteredChats = useMemo(() => {
    const q = chatQuery.trim().toLowerCase();
    return q ? souls.filter((s) => chatLabel(s).toLowerCase().includes(q)) : souls;
  }, [souls, chatQuery]);

  return (
    <div className="shrink-0 border-t border-border bg-muted/40 px-2 py-2">
      {/* Hubs */}
      <div className="flex items-center justify-between px-1 pb-1">
        <span className="text-2xs font-semibold uppercase tracking-wide text-muted-foreground/70">Hubs</span>
        <button onClick={() => router.push('/rooms')} title="All hubs" className="text-muted-foreground hover:text-foreground">
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>
      {hubs.length > SEARCH_THRESHOLD && (
        <SearchBox value={hubQuery} onChange={setHubQuery} placeholder="Search hubs" />
      )}
      <div className={`${LIST_MAX_H} overflow-y-auto scrollbar-thin flex flex-col gap-px`}>
        {filteredHubs.map((h) => (
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
        {hubs.length > 0 && filteredHubs.length === 0 && (
          <p className="px-1 py-1 text-2xs text-muted-foreground">No matching hubs.</p>
        )}
      </div>

      {/* Recent chats */}
      <div className="px-1 pt-3 pb-1">
        <span className="text-2xs font-semibold uppercase tracking-wide text-muted-foreground/70">Recent chats</span>
      </div>
      {souls.length > SEARCH_THRESHOLD && (
        <SearchBox value={chatQuery} onChange={setChatQuery} placeholder="Search chats" />
      )}
      <div className={`${LIST_MAX_H} overflow-y-auto scrollbar-thin flex flex-col gap-px`}>
        {filteredChats.map((s) => (
          <button
            key={s.id}
            onClick={() => router.push(`/qors/${s.id}`)}
            className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-2sm text-muted-foreground hover:bg-muted/60 hover:text-foreground transition-colors"
          >
            <MessageSquare className="h-3.5 w-3.5 shrink-0" />
            <span className="flex-1 truncate">{chatLabel(s)}</span>
          </button>
        ))}
        {souls.length === 0 && <p className="px-1 py-1 text-2xs text-muted-foreground">No chats yet.</p>}
        {souls.length > 0 && filteredChats.length === 0 && (
          <p className="px-1 py-1 text-2xs text-muted-foreground">No matching chats.</p>
        )}
      </div>
    </div>
  );
}

function SearchBox({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder: string }) {
  return (
    <div className="relative mb-1 px-1">
      <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3 w-3 -translate-y-1/2 text-muted-foreground/60" />
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="w-full rounded-md border border-border bg-background py-1 pl-7 pr-2 text-2xs text-foreground placeholder:text-muted-foreground/60 focus:outline-none focus:ring-1 focus:ring-ring"
      />
    </div>
  );
}
