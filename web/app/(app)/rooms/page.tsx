'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState } from 'react';
import { rooms } from '@/lib/api';
import { cn } from '@/lib/utils';
import { AlertCircle, Hash, Loader2, MessageSquare, Plus, Search, Users, X } from 'lucide-react';
import { RoomDetail } from './[id]/client';

interface Room {
  id: string;
  name: string;
  display_name?: string;
  description?: string;
  member_count?: number;
  message_count?: number;
}

function HubsWelcome({ onCreateClick }: { onCreateClick: () => void }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 p-8 text-center">
      <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/10">
        <MessageSquare className="h-7 w-7 text-primary" />
      </div>
      <div>
        <p className="text-base font-semibold">Select a hub</p>
        <p className="mt-1 text-sm text-muted-foreground max-w-xs">
          Pick a hub from the left to chat, review decisions, and coordinate your agent team.
        </p>
      </div>
      <button
        onClick={onCreateClick}
        className="flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
      >
        <Plus className="h-4 w-4" />
        New Hub
      </button>
    </div>
  );
}

export default function RoomsPage() {
  const [list, setList] = useState<Room[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [query, setQuery] = useState('');
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState('');
  const [creating, setCreating] = useState(false);

  const load = () => {
    setLoading(true);
    setError(null);
    rooms
      .list()
      .then((d: any) => {
        const items: Room[] = Array.isArray(d?.rooms) ? d.rooms
          : Array.isArray(d) ? d
          : [];
        setList(items);
        setLoading(false);
      })
      .catch((e) => { setError(e.message); setLoading(false); });
  };

  useEffect(load, []);

  const createRoom = async () => {
    if (!newName.trim()) return;
    setCreating(true);
    try {
      const created: any = await rooms.create({
        name: newName.trim().toLowerCase().replace(/\s+/g, '-'),
        display_name: newName.trim(),
      });
      setShowCreate(false);
      setNewName('');
      load();
      if (created?.id) setSelectedId(created.id);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setCreating(false);
    }
  };

  const filtered = list.filter(r => {
    const q = query.toLowerCase();
    return !q || (r.display_name || r.name).toLowerCase().includes(q);
  });

  return (
    <div className="flex h-[calc(100vh-var(--header-height,56px)-1px)] overflow-hidden">
      {/* ── Left sidebar ── */}
      <div className="flex w-60 shrink-0 flex-col border-r border-border bg-card/50">
        {/* Sidebar header */}
        <div className="flex items-center justify-between px-3 pt-3 pb-2 shrink-0">
          <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Hubs</span>
          <button
            onClick={() => setShowCreate(v => !v)}
            title="New Hub"
            className="flex h-6 w-6 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            <Plus className="h-3.5 w-3.5" />
          </button>
        </div>

        {/* Create form */}
        {showCreate && (
          <div className="mx-2 mb-2 rounded-lg border border-border bg-card p-2.5 space-y-2 shrink-0">
            <input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && createRoom()}
              placeholder="Hub name"
              autoFocus
              className="w-full rounded-md border border-input bg-transparent px-2 py-1 text-xs placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/40"
            />
            <div className="flex gap-1.5">
              <button
                onClick={createRoom}
                disabled={creating || !newName.trim()}
                className="flex-1 rounded-md bg-primary px-2 py-1 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              >
                {creating ? <Loader2 className="h-3 w-3 animate-spin mx-auto" /> : 'Create'}
              </button>
              <button
                onClick={() => { setShowCreate(false); setNewName(''); }}
                className="flex h-6 w-6 items-center justify-center rounded-md text-muted-foreground hover:bg-accent"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
        )}

        {/* Search */}
        <div className="relative mx-2 mb-1 shrink-0">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-muted-foreground pointer-events-none" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Find a hub…"
            className="w-full rounded-md border border-border bg-background pl-6 pr-2 py-1 text-xs placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/40"
          />
        </div>

        {/* Hub list */}
        <div className="flex-1 overflow-y-auto py-1">
          {loading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            </div>
          ) : error ? (
            <div className="flex flex-col items-center gap-2 px-3 py-6 text-center">
              <AlertCircle className="h-5 w-5 text-destructive" />
              <p className="text-xs text-destructive">{error}</p>
              <button onClick={load} className="text-xs text-primary hover:underline">Retry</button>
            </div>
          ) : filtered.length === 0 ? (
            <p className="px-3 py-4 text-xs text-muted-foreground text-center">
              {query ? 'No matching hubs' : 'No hubs yet'}
            </p>
          ) : (
            filtered.map((room) => (
              <button
                key={room.id}
                onClick={() => setSelectedId(room.id)}
                className={cn(
                  'flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 mx-1 text-left transition-colors text-xs',
                  selectedId === room.id
                    ? 'bg-primary/10 text-primary font-medium'
                    : 'text-muted-foreground hover:bg-accent hover:text-foreground',
                )}
                style={{ width: 'calc(100% - 8px)' }}
              >
                <Hash className="h-3 w-3 shrink-0 opacity-60" />
                <span className="flex-1 truncate">{room.display_name || room.name}</span>
                {(room.member_count ?? 0) > 0 && (
                  <span className="flex items-center gap-0.5 text-2xs shrink-0 opacity-50">
                    <Users className="h-2.5 w-2.5" />
                    {room.member_count}
                  </span>
                )}
              </button>
            ))
          )}
        </div>
      </div>

      {/* ── Right panel ── */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {selectedId
          ? <RoomDetail roomId={selectedId} showBack={false} />
          : <HubsWelcome onCreateClick={() => setShowCreate(true)} />
        }
      </div>
    </div>
  );
}
