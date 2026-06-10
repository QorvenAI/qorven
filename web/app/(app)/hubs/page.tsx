'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { rooms } from '@/lib/api';
import { Loader2, MessageSquare, Plus, X } from 'lucide-react';

function HubsWelcome({ onCreateClick }: { onCreateClick: () => void }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 p-8 text-center">
      <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/10">
        <MessageSquare className="h-7 w-7 text-primary" />
      </div>
      <div>
        <p className="text-base font-semibold">Select a hub</p>
        <p className="mt-1 text-sm text-muted-foreground max-w-xs">
          Pick a hub from the sidebar to chat, review decisions, and coordinate your agent team.
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

export default function HubsPage() {
  const router = useRouter();
  // The hub list lives in the contextual sidebar; selecting a hub there routes
  // to /hubs/[id]. This index page is just the empty state + create flow.
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState('');
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

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
      if (created?.id) router.push(`/hubs/${created.id}`);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="flex h-[calc(100vh-var(--header-height,56px)-1px)] flex-col overflow-hidden">
      <HubsWelcome onCreateClick={() => setShowCreate(true)} />

      {/* Create-hub modal */}
      {showCreate && (
        <div className="fixed inset-0 z-[100] flex items-start justify-center pt-[20vh] bg-black/60 backdrop-blur-sm"
          onClick={() => { setShowCreate(false); setNewName(''); }}>
          <div className="w-full max-w-sm rounded-xl border border-border bg-popover p-4 shadow-2xl space-y-3"
            onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center justify-between">
              <p className="text-sm font-semibold">New hub</p>
              <button onClick={() => { setShowCreate(false); setNewName(''); }}
                className="flex h-6 w-6 items-center justify-center rounded-md text-muted-foreground hover:bg-accent">
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
            <input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && createRoom()}
              placeholder="Hub name"
              autoFocus
              className="qr-input text-2sm"
            />
            {error && <p className="text-2xs text-destructive">{error}</p>}
            <button
              onClick={createRoom}
              disabled={creating || !newName.trim()}
              className="flex w-full items-center justify-center gap-1.5 rounded-md bg-primary px-3 py-2 text-2sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {creating ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
              Create hub
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
