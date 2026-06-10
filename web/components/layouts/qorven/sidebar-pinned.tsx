'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// SidebarPinned — the always-present zone pinned above the COO dock on every
// page. Three collapsible groups, top→bottom: ★ Pinned (the user's pinned hubs
// + chats, shown only when non-empty), Hubs, and Recent chats. Built from the
// same primitives as the contextual sidebars so it reads as one design: muted
// group headers, a subtle inline search on long lists, dense rows with a
// glyph/avatar + name + a right-aligned indicator, and a hover star to pin.

import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { useStore } from '@/store';
import { rooms as roomsApi, pins as pinsApi, type SidebarPin } from '@/lib/api';
import { soulGradient } from '@/components/soul-card';
import { cn } from '@/lib/utils';
import { Plus, Hash, MessageSquare, Star, ChevronDown } from 'lucide-react';

// Each group's scroll area: ~3 rows on short screens, more when tall.
const LIST_MAX_H = 'max-h-[clamp(108px,18vh,280px)]';

type PinKey = string; // `${type}:${id}`
const keyOf = (type: 'hub' | 'chat', id: string): PinKey => `${type}:${id}`;

export function SidebarPinned() {
  const router = useRouter();
  const pathname = usePathname();
  const souls = useStore((s) => s.souls);

  const [hubs, setHubs] = useState<any[]>([]);
  const [hubMembers, setHubMembers] = useState<Record<string, number>>({});
  const [pinned, setPinned] = useState<SidebarPin[]>([]);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  useEffect(() => {
    let cancelled = false;
    roomsApi.list().then((d: any) => {
      const list: any[] = Array.isArray(d?.rooms) ? d.rooms : Array.isArray(d) ? d : [];
      if (cancelled) return;
      setHubs(list);
      list.forEach((h: any) => {
        roomsApi.org(h.id).then((org: any) => {
          if (cancelled) return;
          setHubMembers((prev) => ({ ...prev, [h.id]: (org?.members ?? []).length }));
        }).catch(() => {});
      });
    }).catch(() => {});
    pinsApi.list().then((p) => { if (!cancelled) setPinned(Array.isArray(p) ? p : []); }).catch(() => {});
    return () => { cancelled = true; };
  }, []);

  const currentHubId = pathname?.match(/^\/hubs\/([^/]+)/)?.[1] ?? null;
  const currentChatId = pathname?.match(/^\/qors\/([^/]+)/)?.[1] ?? null;

  const pinnedKeys = useMemo(() => new Set(pinned.map((p) => keyOf(p.item_type, p.item_id))), [pinned]);
  const isPinned = (type: 'hub' | 'chat', id: string) => pinnedKeys.has(keyOf(type, id));

  const togglePin = useCallback((type: 'hub' | 'chat', id: string) => {
    const k = keyOf(type, id);
    // Read live state inside the updater so this stays correct regardless of
    // where the row that triggered it was memoised.
    setPinned((prev) => {
      if (prev.some((p) => keyOf(p.item_type, p.item_id) === k)) {
        pinsApi.unpin(type, id).catch(() => {});
        return prev.filter((p) => keyOf(p.item_type, p.item_id) !== k);
      }
      // Optimistic: append a provisional pin; reconcile from server response.
      const provisional: SidebarPin = { id: k, item_type: type, item_id: id, order_index: 999, created_at: '' };
      pinsApi.pin(type, id)
        .then((saved) => setPinned((cur) => cur.map((p) => (p.id === k ? saved : p))))
        .catch(() => setPinned((cur) => cur.filter((p) => p.id !== k)));
      return [...prev, provisional];
    });
  }, []);

  const hubLabel = (h: any) => String(h.display_name || h.name || '');
  const chatLabel = (s: any) => String(s.display_name || s.agent_key || '');

  // Pin order: a pinned item's rank = its index in `pinned`; unpinned sort after.
  const pinRank = useMemo(() => {
    const m = new Map<string, number>();
    pinned.forEach((p, i) => m.set(keyOf(p.item_type, p.item_id), i));
    return m;
  }, [pinned]);

  // Stable sort that floats pinned items (in pin order) to the top of a list;
  // unpinned items (rank Infinity) keep their original order after them.
  const pinnedFirst = useCallback(<T,>(items: T[], type: 'hub' | 'chat', idOf: (x: T) => string): T[] =>
    items
      .map((item, i) => ({ item, i }))
      .sort((a, b) => {
        const ra = pinRank.get(keyOf(type, idOf(a.item))) ?? Infinity;
        const rb = pinRank.get(keyOf(type, idOf(b.item))) ?? Infinity;
        return ra === rb ? a.i - b.i : ra - rb;
      })
      .map((x) => x.item), [pinRank]);

  const sortedHubs = useMemo(() => pinnedFirst(hubs, 'hub', (h) => h.id), [hubs, pinnedFirst]);
  const sortedChats = useMemo(() => pinnedFirst(souls, 'chat', (s) => s.id), [souls, pinnedFirst]);

  return (
    <div className="shrink-0 border-t border-border bg-muted/30">
      {/* Hubs — pinned hubs float to the top */}
      <Group
        title="Hubs"
        collapsed={!!collapsed.hubs}
        onToggle={() => setCollapsed((c) => ({ ...c, hubs: !c.hubs }))}
        action={<button onClick={() => router.push('/hubs')} title="All hubs" className="text-muted-foreground/70 hover:text-foreground"><Plus className="h-3.5 w-3.5" /></button>}
      >
        <div className={cn(LIST_MAX_H, 'overflow-y-auto scrollbar-thin flex flex-col gap-px px-2')}>
          {sortedHubs.map((h) => (
            <HubRow
              key={h.id}
              label={hubLabel(h)}
              members={hubMembers[h.id] ?? 0}
              active={currentHubId === h.id}
              pinned={isPinned('hub', h.id)}
              onOpen={() => router.push(`/hubs/${h.id}`)}
              onTogglePin={() => togglePin('hub', h.id)}
            />
          ))}
          {hubs.length === 0 && <Empty>No hubs yet.</Empty>}
        </div>
      </Group>

      <div className="mx-3 my-2 h-px bg-border/60" />

      {/* Recent chats — pinned chats float to the top */}
      <Group
        title="Recent chats"
        collapsed={!!collapsed.chats}
        onToggle={() => setCollapsed((c) => ({ ...c, chats: !c.chats }))}
      >
        <div className={cn(LIST_MAX_H, 'overflow-y-auto scrollbar-thin flex flex-col gap-px px-2')}>
          {sortedChats.map((s) => (
            <ChatRow
              key={s.id}
              label={chatLabel(s)}
              active={currentChatId === s.id}
              pinned={isPinned('chat', s.id)}
              onOpen={() => router.push(`/qors/${s.id}`)}
              onTogglePin={() => togglePin('chat', s.id)}
            />
          ))}
          {souls.length === 0 && <Empty>No chats yet.</Empty>}
        </div>
      </Group>
    </div>
  );
}

/* ─── Group (collapsible header + body) ─────────────────────────────────────── */
function Group({ title, icon, action, collapsed, onToggle, children }: {
  title: string; icon?: ReactNode; action?: ReactNode; collapsed: boolean; onToggle: () => void; children: ReactNode;
}) {
  return (
    <div className="pt-2">
      <div className="flex items-center gap-1.5 px-3 pb-1">
        <button onClick={onToggle} className="flex flex-1 items-center gap-1.5 text-2xs font-medium uppercase tracking-wider text-muted-foreground/60 hover:text-muted-foreground">
          <ChevronDown className={cn('h-3 w-3 transition-transform', collapsed && '-rotate-90')} />
          {icon}
          <span>{title}</span>
        </button>
        {action}
      </div>
      {!collapsed && children}
    </div>
  );
}

/* ─── Rows ──────────────────────────────────────────────────────────────────── */
function RowShell({ active, onOpen, pinned, onTogglePin, children }: {
  active: boolean; onOpen: () => void; pinned: boolean; onTogglePin: () => void; children: ReactNode;
}) {
  return (
    <div
      className={cn(
        'group/row flex w-full items-center gap-2.5 h-8.5 px-2.5 rounded-md transition-colors cursor-pointer',
        active ? 'bg-accent text-foreground font-medium' : 'text-muted-foreground hover:bg-muted hover:text-foreground',
      )}
      onClick={onOpen}
    >
      {children}
      <button
        onClick={(e) => { e.stopPropagation(); onTogglePin(); }}
        title={pinned ? 'Unpin' : 'Pin'}
        className={cn(
          'shrink-0 -mr-1 flex h-5 w-5 items-center justify-center rounded transition-opacity hover:bg-background/60',
          pinned ? 'opacity-100' : 'opacity-0 group-hover/row:opacity-100',
        )}
      >
        <Star className={cn('h-3.5 w-3.5', pinned ? 'fill-amber-500 text-amber-500' : 'text-muted-foreground')} />
      </button>
    </div>
  );
}

function HubRow({ label, members, active, pinned, onOpen, onTogglePin }: {
  label: string; members: number; active: boolean; pinned: boolean; onOpen: () => void; onTogglePin: () => void;
}) {
  return (
    <RowShell active={active} onOpen={onOpen} pinned={pinned} onTogglePin={onTogglePin}>
      <div className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <Hash className="h-3 w-3" />
      </div>
      <span className="flex-1 truncate text-2sm">{label}</span>
      {members > 0 && <span className="shrink-0 text-2xs tabular-nums text-muted-foreground/70">{members}</span>}
    </RowShell>
  );
}

function ChatRow({ label, active, pinned, onOpen, onTogglePin }: {
  label: string; active: boolean; pinned: boolean; onOpen: () => void; onTogglePin: () => void;
}) {
  return (
    <RowShell active={active} onOpen={onOpen} pinned={pinned} onTogglePin={onTogglePin}>
      <div className={cn('flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-gradient-to-br text-2xs font-semibold text-white', soulGradient(label || '?'))}>
        {(label?.[0] ?? '?').toUpperCase()}
      </div>
      <span className="flex-1 truncate text-2sm">{label}</span>
      <MessageSquare className="h-3 w-3 shrink-0 text-muted-foreground/40 opacity-0 group-hover/row:opacity-100" />
    </RowShell>
  );
}

/* ─── Bits ──────────────────────────────────────────────────────────────────── */
function Empty({ children }: { children: ReactNode }) {
  return <p className="px-2.5 py-1 text-2xs text-muted-foreground/70">{children}</p>;
}
