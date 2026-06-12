'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import type { TimelineItem } from '@/lib/api-workspace';
import { TimelineItemBlock } from './timeline-item';

function dayKey(iso: string) {
  return new Date(iso).toISOString().slice(0, 10);
}

function dayLabel(key: string) {
  return new Date(key + 'T00:00:00').toLocaleDateString('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
  });
}

export function AgendaView({
  items,
  onSelect,
}: {
  items: TimelineItem[];
  onSelect: (i: TimelineItem) => void;
}) {
  const sorted = [...items].sort(
    (a, b) => new Date(a.when).getTime() - new Date(b.when).getTime()
  );
  const groups = new Map<string, TimelineItem[]>();
  for (const it of sorted) {
    const k = dayKey(it.when);
    if (!groups.has(k)) groups.set(k, []);
    groups.get(k)!.push(it);
  }

  if (sorted.length === 0) {
    return (
      <p className="px-4 py-12 text-center text-sm text-muted-foreground">
        Nothing scheduled in this range.
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-4 p-4">
      {[...groups.entries()].map(([k, its]) => (
        <div key={k}>
          <h3 className="mb-2 text-xs font-semibold text-muted-foreground">
            {dayLabel(k)}
          </h3>
          <div className="flex flex-col gap-1.5">
            {its.map((it) => (
              <TimelineItemBlock
                key={it.id}
                item={it}
                onClick={() => onSelect(it)}
              />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
