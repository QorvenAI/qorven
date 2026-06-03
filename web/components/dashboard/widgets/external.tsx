'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
import { useDashboardData } from '@/contexts/dashboard-data';
import { registerWidget } from '@/components/dashboard/widget-registry';
import type { WidgetConfig } from '@/components/dashboard/widget-registry';
import { ExternalLink } from 'lucide-react';

function ExternalWidget({ config }: { config: WidgetConfig }) {
  const { data } = useDashboardData();
  const raw = data[config.dataSource];

  // Expected: { value: any, label?: string, updated_at?: string, source?: string, url?: string }
  const payload = (raw && typeof raw === 'object' && !Array.isArray(raw))
    ? raw as { value?: unknown; label?: string; updated_at?: string; source?: string; url?: string }
    : null;

  const displayValue = payload?.value ?? raw ?? '—';
  const updatedAt = payload?.updated_at ? new Date(payload.updated_at).toLocaleTimeString() : null;

  return (
    <div className="flex flex-col h-full p-1">
      <div className="flex items-center justify-between mb-2">
        <p className="text-xs font-medium text-muted-foreground truncate">{config.title}</p>
        {payload?.source && (
          <span className="text-[10px] text-muted-foreground/50 flex items-center gap-0.5">
            <ExternalLink className="h-2.5 w-2.5" />{payload.source}
          </span>
        )}
      </div>
      <div className="flex-1 flex items-center justify-center">
        <div className="text-center">
          <p className="text-2xl font-bold text-foreground">
            {config.config?.prefix}{String(displayValue)}{config.config?.suffix}
          </p>
          {payload?.label && <p className="text-xs text-muted-foreground mt-1">{payload.label}</p>}
          {updatedAt && <p className="text-[10px] text-muted-foreground/40 mt-1">Updated {updatedAt}</p>}
        </div>
      </div>
    </div>
  );
}

registerWidget('external', ExternalWidget);
export { ExternalWidget };
