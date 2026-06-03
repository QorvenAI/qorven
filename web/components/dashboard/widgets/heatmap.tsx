'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
import { useDashboardData } from '@/contexts/dashboard-data';
import { registerWidget } from '@/components/dashboard/widget-registry';
import type { WidgetConfig } from '@/components/dashboard/widget-registry';
import { cn } from '@/lib/utils';

const HOURS = Array.from({ length: 24 }, (_, i) => i);
const DAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

function HeatmapWidget({ config }: { config: WidgetConfig }) {
  const { data } = useDashboardData();
  const raw = data[config.dataSource];
  // Expected: array of { day: 0-6, hour: 0-23, value: number }
  const cells = Array.isArray(raw) ? raw as { day: number; hour: number; value: number }[] : [];
  const maxVal = cells.reduce((m, c) => Math.max(m, c.value ?? 0), 1);
  const cellMap = new Map(cells.map(c => [`${c.day}-${c.hour}`, c.value]));

  return (
    <div className="flex flex-col h-full p-1">
      <p className="text-xs font-medium text-muted-foreground mb-2 truncate">{config.title}</p>
      <div className="flex-1 min-h-0 overflow-hidden">
        <div className="grid gap-px" style={{ gridTemplateColumns: `28px repeat(24, 1fr)` }}>
          {/* Hour labels */}
          <div />
          {HOURS.map(h => (
            <div key={h} className="text-[8px] text-muted-foreground/50 text-center">{h % 6 === 0 ? h : ''}</div>
          ))}
          {/* Rows */}
          {DAYS.map((day, di) => (
            <>
              <div key={`d-${di}`} className="text-[9px] text-muted-foreground/60 flex items-center">{day}</div>
              {HOURS.map(h => {
                const v = cellMap.get(`${di}-${h}`) ?? 0;
                const intensity = maxVal > 0 ? v / maxVal : 0;
                return (
                  <div key={`${di}-${h}`} title={`${day} ${h}:00 — ${v}`}
                    className="rounded-[2px] aspect-square"
                    style={{ background: `rgba(82,113,255,${0.1 + intensity * 0.8})` }}
                  />
                );
              })}
            </>
          ))}
        </div>
      </div>
    </div>
  );
}

registerWidget('heatmap', HeatmapWidget);
export { HeatmapWidget };
