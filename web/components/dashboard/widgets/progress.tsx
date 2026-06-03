'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
import { useDashboardData } from '@/contexts/dashboard-data';
import { registerWidget } from '@/components/dashboard/widget-registry';
import type { WidgetConfig } from '@/components/dashboard/widget-registry';

function ProgressWidget({ config }: { config: WidgetConfig }) {
  const { data } = useDashboardData();
  const raw = data[config.dataSource];

  // Expected: { value: number, max: number, label?: string } or just a number (treated as 0-100 percentage)
  let value = 0, max = 100, label = '';
  if (typeof raw === 'number') {
    value = raw;
  } else if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
    const r = raw as { value?: number; max?: number; label?: string };
    value = r.value ?? 0;
    max = r.max ?? 100;
    label = r.label ?? '';
  }

  const pct = max > 0 ? Math.min(100, Math.round(value / max * 100)) : 0;
  const color = pct >= 90 ? 'bg-red-500' : pct >= 75 ? 'bg-amber-400' : 'bg-primary';

  return (
    <div className="flex flex-col h-full p-1">
      <p className="text-xs font-medium text-muted-foreground mb-2 truncate">{config.title}</p>
      <div className="flex-1 flex flex-col items-center justify-center gap-3">
        {/* Circular progress — pure CSS */}
        <div className="relative" style={{ width: 72, height: 72 }}>
          <svg viewBox="0 0 36 36" className="w-full h-full -rotate-90">
            <circle cx="18" cy="18" r="15.9" fill="none" stroke="var(--border)" strokeWidth="3" />
            <circle cx="18" cy="18" r="15.9" fill="none"
              stroke={pct >= 90 ? '#ef4444' : pct >= 75 ? '#ffab40' : 'var(--primary)'}
              strokeWidth="3"
              strokeDasharray={`${pct} ${100 - pct}`}
              strokeLinecap="round"
              style={{ transition: 'stroke-dasharray 0.5s ease' }}
            />
          </svg>
          <div className="absolute inset-0 flex items-center justify-center">
            <span className="text-sm font-bold">{pct}%</span>
          </div>
        </div>
        <div className="text-center">
          <p className="text-xs text-foreground font-medium">
            {config.config?.prefix}{value}{config.config?.suffix} / {config.config?.prefix}{max}{config.config?.suffix}
          </p>
          {label && <p className="text-[10px] text-muted-foreground mt-0.5">{label}</p>}
        </div>
      </div>
    </div>
  );
}

registerWidget('progress', ProgressWidget);
export { ProgressWidget };
