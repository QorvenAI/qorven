'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState, useEffect } from 'react';
import { request } from '@/lib/api-core';

export function TaskCountBadge() {
  const [count, setCount] = useState(0);

  useEffect(() => {
    request<{ count: number }>('/tasks?status=in_progress&count=true')
      .then(r => setCount(r.count ?? 0))
      .catch(() => {});
  }, []);

  useEffect(() => {
    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const ws = new WebSocket(`${proto}://${window.location.host}/ws/realtime`);
    ws.onmessage = (e) => {
      try {
        const evt = JSON.parse(e.data);
        if (evt.type === 'task_iteration_start') setCount(c => c + 1);
        if (['task_done', 'task_blocked'].includes(evt.type)) setCount(c => Math.max(0, c - 1));
      } catch {}
    };
    return () => ws.close();
  }, []);

  if (count === 0) return null;
  return (
    <span className="min-w-[18px] h-[18px] rounded-full bg-blue-500 text-white text-2xs font-semibold inline-flex items-center justify-center px-1 tabular-nums ml-auto">
      {count > 99 ? '99+' : count}
    </span>
  );
}
