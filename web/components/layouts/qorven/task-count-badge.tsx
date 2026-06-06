'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useStore } from '@/store';

export function TaskCountBadge() {
  // Read live task counts from the shared Zustand store instead of opening a
  // separate WebSocket. The existing websocket.ts handler writes task events
  // to the store's daemonTasks slice.
  const tasks = useStore(s => s.daemonTasks);
  const count = Object.values(tasks).filter(t => t.status === 'in_progress').length;

  if (count === 0) return null;
  return (
    <span className="min-w-[18px] h-[18px] rounded-full bg-blue-500 text-white text-2xs font-semibold inline-flex items-center justify-center px-1 tabular-nums ml-auto">
      {count > 99 ? '99+' : count}
    </span>
  );
}
