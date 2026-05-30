'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { Hash, Plus } from 'lucide-react';
import { cn } from '@/lib/utils';
import { rooms as roomsApi } from '@/lib/api';
import { useStore } from '@/store';
import { SoulPulseRing } from '@/components/soul-pulse-ring';
import { SidebarHubRow, type HubMember } from '@/components/layouts/qorven/sidebar-hub-row';

const GRADIENTS = [
  'from-primary to-primary/80', 'from-emerald-500 to-teal-600',
  'from-orange-500 to-red-600', 'from-pink-500 to-rose-600',
  'from-cyan-500 to-blue-600', 'from-amber-500 to-yellow-600',
  'from-fuchsia-500 to-purple-600', 'from-lime-500 to-green-600',
];
function gradientFor(id: string) {
  let hash = 0;
  for (let i = 0; i < id.length; i++) hash = id.charCodeAt(i) + ((hash << 5) - hash);
  return GRADIENTS[Math.abs(hash) % GRADIENTS.length]!;
}

export function RoomsSidebar() {
  const router = useRouter();
  const pathname = usePathname();
  const souls = useStore((s) => s.souls);
  const soulStates = useStore((s) => s.soulStates);
  const [hubs, setHubs] = useState<any[]>([]);
  const [hubMembers, setHubMembers] = useState<Record<string, HubMember[]>>({});
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState('');
  const [showCreate, setShowCreate] = useState(false);

  // Currently open hub id from pathname /rooms/:id
  const currentHubId = pathname?.match(/^\/rooms\/([^/]+)/)?.[1] ?? null;
  // Members of the current hub (for inline member panel)
  const currentMembers = currentHubId ? (hubMembers[currentHubId] ?? []) : [];

  const load = () => {
    setLoading(true);
    roomsApi.list().then((d: any) => {
      const list: any[] = Array.isArray(d?.rooms) ? d.rooms : Array.isArray(d) ? d : [];
      setHubs(list);
      list.forEach((h: any) => {
        roomsApi.org(h.id).then((org: any) => {
          const members: HubMember[] = (org?.members ?? []).map((m: any) => ({
            id: m.id ?? m.agent_id,
            display_name: m.display_name ?? m.agent_key ?? 'Agent',
            avatar: m.avatar,
          }));
          setHubMembers((prev) => ({ ...prev, [h.id]: members }));
        }).catch(() => {});
      });
      setLoading(false);
    }).catch(() => setLoading(false));
  };

  useEffect(load, []);

  const create = async () => {
    if (!newName.trim()) return;
    setCreating(true);
    try {
      const created: any = await roomsApi.create({
        name: newName.trim().toLowerCase().replace(/\s+/g, '-'),
        display_name: newName.trim(),
      });
      setShowCreate(false);
      setNewName('');
      load();
      if (created?.id) router.push(`/rooms/${created.id}`);
    } catch {
      // ignore
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2.5 border-b border-border shrink-0">
        <span className="text-[10px] font-semibold text-muted-foreground/60 uppercase tracking-wider">Hubs</span>
        <button
          onClick={() => setShowCreate((v) => !v)}
          className="flex h-5 w-5 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <Plus className="h-3.5 w-3.5" />
        </button>
      </div>

      {/* Create form */}
      {showCreate && (
        <div className="mx-2 mt-1.5 mb-1 rounded-lg border border-border bg-card p-2 space-y-1.5 shrink-0">
          <input
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && create()}
            placeholder="Hub name"
            autoFocus
            className="w-full rounded-md border border-input bg-transparent px-2 py-1 text-[12px] placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/40"
          />
          <div className="flex gap-1.5">
            <button
              onClick={create}
              disabled={creating || !newName.trim()}
              className="flex-1 rounded-md bg-primary px-2 py-1 text-[11px] font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {creating ? 'Creating…' : 'Create'}
            </button>
            <button
              onClick={() => { setShowCreate(false); setNewName(''); }}
              className="px-2 py-1 text-[11px] text-muted-foreground hover:text-foreground"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Hub list */}
      <div className="flex-1 overflow-y-auto px-1.5 py-1">
        {loading ? (
          <div className="space-y-1 mt-1">
            {[1, 2, 3].map((i) => (
              <div key={i} className="h-9 rounded-lg bg-muted animate-pulse" />
            ))}
          </div>
        ) : hubs.length === 0 ? (
          <div className="flex flex-col items-center gap-2 py-8 text-center">
            <Hash className="h-6 w-6 text-muted-foreground/30" />
            <p className="text-[12px] text-muted-foreground">No hubs yet</p>
            <button
              onClick={() => setShowCreate(true)}
              className="text-[11px] text-primary hover:underline"
            >
              Create one
            </button>
          </div>
        ) : (
          hubs.map((h: any) => (
            <SidebarHubRow
              key={h.id}
              id={h.id}
              name={h.name}
              displayName={h.display_name}
              members={hubMembers[h.id] ?? []}
              messageCount={h.message_count}
              isActive={
                pathname === `/rooms/${h.id}` ||
                (pathname?.startsWith(`/rooms/${h.id}/`) ?? false)
              }
              onClick={() => router.push(`/rooms/${h.id}`)}
            />
          ))
        )}
      </div>

      {/* Current hub members panel — shown when inside a specific hub */}
      {currentHubId && currentMembers.length > 0 && (
        <div className="border-t border-border px-3 py-2 shrink-0">
          <p className="text-[10px] font-semibold text-muted-foreground/60 uppercase tracking-wider mb-2">
            Members
          </p>
          <div className="space-y-1.5">
            {currentMembers.map((m) => {
              const soul = souls.find((s) => s.id === m.id);
              const state = soulStates[m.id];
              return (
                <div key={m.id} className="flex items-center gap-2">
                  <div className="relative shrink-0">
                    {m.avatar ? (
                      <img
                        src={m.avatar}
                        alt={m.display_name}
                        className="h-6 w-6 rounded-full object-cover"
                      />
                    ) : (
                      <div
                        className={cn(
                          'flex h-6 w-6 items-center justify-center rounded-full bg-gradient-to-br text-[9px] font-bold text-white',
                          gradientFor(m.id),
                        )}
                      >
                        {(m.display_name?.[0] ?? '?').toUpperCase()}
                      </div>
                    )}
                    <span className="absolute -bottom-0.5 -right-0.5">
                      <SoulPulseRing activity={state?.activity ?? 'offline'} size="sm" />
                    </span>
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-[12px] font-medium leading-tight">
                      {m.display_name}
                    </p>
                    {soul && (
                      <p className="truncate text-[10px] text-muted-foreground/60">
                        {soul.title || soul.role || ''}
                      </p>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
