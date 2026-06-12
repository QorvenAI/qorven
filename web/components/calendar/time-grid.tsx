'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { cn } from '@/lib/utils';
import { soulGradient } from '@/components/soul-card';
import type { TimelineItem } from '@/lib/api-workspace';

const HOURS = Array.from({ length: 24 }, (_, h) => h);

function sameDay(a: Date, b: Date) {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

// TimeGrid renders `days` columns (1 for day view, 7 for week) with hour rows.
// Items are positioned by their start hour; a readable approximation (not
// pixel-perfect overlap resolution — that's a v2 nicety).
export function TimeGrid({
  days,
  items,
  now,
  onSelect,
}: {
  days: Date[];
  items: TimelineItem[];
  now: Date;
  onSelect: (i: TimelineItem) => void;
}) {
  return (
    <div className="flex flex-1 overflow-auto">
      <div className="w-14 shrink-0">
        <div className="h-10" />
        {HOURS.map((h) => (
          <div key={h} className="h-12 pr-2 text-right text-2xs text-muted-foreground">
            {h}:00
          </div>
        ))}
      </div>
      <div
        className="grid flex-1"
        style={{ gridTemplateColumns: `repeat(${days.length}, minmax(0, 1fr))` }}
      >
        {days.map((day, di) => {
          const dayItems = items.filter((it) => sameDay(new Date(it.when), day));
          const isToday = sameDay(day, now);
          return (
            <div key={di} className="relative border-l border-border">
              <div
                className={cn(
                  'flex h-10 items-center justify-center border-b border-border text-xs font-medium',
                  isToday && 'text-primary',
                )}
              >
                {day.toLocaleDateString('en-US', { weekday: 'short', day: 'numeric' })}
              </div>
              <div className="relative">
                {HOURS.map((h) => (
                  <div key={h} className="h-12 border-b border-border/50" />
                ))}
                {dayItems.map((it) => {
                  const d = new Date(it.when);
                  const top = 40 + d.getHours() * 48 + (d.getMinutes() / 60) * 48;
                  const name = it.agent_name || 'Unassigned';
                  return (
                    <button
                      key={it.id}
                      onClick={() => onSelect(it)}
                      className="absolute left-1 right-1 flex items-center gap-1 rounded-md border border-border bg-card px-1.5 py-1 text-2xs hover:bg-accent"
                      style={{ top: `${top}px` }}
                    >
                      <span
                        className={cn(
                          'flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-full bg-gradient-to-br text-2xs font-semibold text-white',
                          soulGradient(name),
                        )}
                      >
                        {name.charAt(0).toUpperCase()}
                      </span>
                      <span className="truncate">{it.title}</span>
                    </button>
                  );
                })}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
