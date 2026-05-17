'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useStore } from '@/store';
import { X, Zap } from 'lucide-react';

export function ActivityPulse() {
  const liveEvents = useStore((s) => s.liveEvents);
  const openContextPanel = useStore((s) => s.openContextPanel);
  const hasActivity = liveEvents.length > 0;

  return (
    <button onClick={() => openContextPanel('activity', null)} title="Live Activity"
      className="relative flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground hover:bg-accent hover:text-foreground">
      <Zap className="h-[18px] w-[18px]" />
      {hasActivity && <span className="absolute top-1 right-1 h-2 w-2 rounded-full bg-emerald-400 animate-pulse" />}
    </button>
  );
}

export function ActivityPanel() {
  const liveEvents = useStore((s) => s.liveEvents);
  const close = useStore((s) => s.closeContextPanel);
  const souls = useStore((s) => s.souls);
  const soulMap = Object.fromEntries(souls.map((s) => [s.id, s]));

  return (
    <>
      <div className="flex items-center justify-between border-b border-border px-4 h-[var(--header-height)]">
        <span className="text-sm font-medium">Live Activity</span>
        <button onClick={close} className="h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground hover:bg-accent"><X className="h-4 w-4" /></button>
      </div>
      <div className="overflow-y-auto p-3 space-y-0.5">
        {liveEvents.length === 0 ? <p className="py-8 text-center text-sm text-muted-foreground">No activity yet</p> :
          liveEvents.slice(0, 50).map((e) => {
            const soul = e.agent_id ? soulMap[e.agent_id] : null;
            return (
              <div key={e.id} className="rounded-lg px-2.5 py-1.5 text-2xs hover:bg-accent/50">
                <span className="text-muted-foreground">{new Date(e.timestamp).toLocaleTimeString()}</span>{' '}
                {soul && <span className="font-medium">@{soul.display_name}</span>}{' '}
                <span className="text-muted-foreground">{e.detail ?? e.type}</span>
              </div>
            );
          })}
      </div>
    </>
  );
}
