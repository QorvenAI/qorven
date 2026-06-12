'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { calendarApi, type TimelineItem, type ScheduledRun } from '@/lib/api-workspace';
import { request } from '@/lib/api-core';

export function ItemDetail({ item, onClose, onChanged }: { item: TimelineItem; onClose: () => void; onChanged: () => void }) {
  const [run, setRun] = useState<ScheduledRun | null>(null);
  useEffect(() => {
    if (item.source === 'run') calendarApi.run(item.source_id).then(setRun).catch(() => setRun(null));
  }, [item]);

  const pause = async () => { await request(`/cron-jobs/${item.source_id}/pause`, { method: 'POST' }).catch(() => {}); onChanged(); onClose(); };
  const resume = async () => { await request(`/cron-jobs/${item.source_id}/resume`, { method: 'POST' }).catch(() => {}); onChanged(); onClose(); };
  const del = async () => { await request(`/cron-jobs/${item.source_id}`, { method: 'DELETE' }).catch(() => {}); onChanged(); onClose(); };

  return (
    <div className="fixed inset-0 z-[120] flex items-center justify-center bg-black/40" onClick={onClose}>
      <div role="dialog" className="w-[420px] rounded-xl border border-border bg-card p-5 shadow-lg" onClick={(e) => e.stopPropagation()}>
        <div className="mb-1 text-xs font-medium uppercase text-muted-foreground">{item.source} · {item.status}</div>
        <h2 className="mb-2 text-base font-semibold">{item.title}</h2>
        <p className="mb-3 text-xs text-muted-foreground">{item.agent_name || 'Unassigned'} · {new Date(item.when).toLocaleString()}</p>
        {item.detail && <p className="mb-3 whitespace-pre-wrap text-sm">{item.detail}</p>}
        {run && (
          <div className="mb-3 rounded-md border border-border p-2 text-xs">
            <div className="text-muted-foreground">Result</div>
            <p className="whitespace-pre-wrap">{run.result_snippet || run.error || '—'}</p>
            <div className="mt-1 text-xs text-muted-foreground">{run.tokens} tokens · {(run.cost_cents / 100).toFixed(2)} USD</div>
          </div>
        )}
        {item.source === 'cron' && (
          <div className="flex justify-end gap-2">
            {item.status === 'paused'
              ? <button onClick={resume} className="qr-btn qr-btn-outline">Resume</button>
              : <button onClick={pause} className="qr-btn qr-btn-outline">Pause</button>}
            <button onClick={del} className="qr-btn qr-btn-destructive">Delete</button>
          </div>
        )}
      </div>
    </div>
  );
}
