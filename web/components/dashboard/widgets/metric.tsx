'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useRef } from 'react';
import { TrendingDown, TrendingUp } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { useDashboardData } from '@/contexts/dashboard-data';
import { registerWidget, type WidgetConfig } from '../widget-registry';

function formatValue(raw: unknown, prefix?: string, suffix?: string): string {
  const num =
    typeof raw === 'number'
      ? raw
      : typeof raw === 'string'
      ? parseFloat(raw)
      : NaN;
  const formatted = isNaN(num) ? String(raw ?? '—') : num.toLocaleString();
  return `${prefix ?? ''}${formatted}${suffix ?? ''}`;
}

function MetricWidget({ config }: { config: WidgetConfig }) {
  const { data } = useDashboardData();
  const prevRef = useRef<number | null>(null);

  const raw = data[config.dataSource];
  const current =
    typeof raw === 'number'
      ? raw
      : typeof raw === 'string'
      ? parseFloat(raw)
      : NaN;

  let trendDir: 'up' | 'down' | null = null;
  if (config.config?.showTrend && !isNaN(current) && prevRef.current !== null) {
    trendDir = current >= prevRef.current ? 'up' : 'down';
  }
  if (!isNaN(current)) prevRef.current = current;

  const display = formatValue(raw, config.config?.prefix, config.config?.suffix);

  return (
    <Card className="h-full flex flex-col">
      <CardHeader className="min-h-10 px-4 py-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{config.title}</CardTitle>
      </CardHeader>
      <CardContent className="flex-1 flex items-center gap-2 px-4 pb-4 pt-0">
        <span className="text-3xl font-bold tracking-tight truncate">{display}</span>
        {config.config?.showTrend && trendDir && (
          <span
            className={cn(
              'flex items-center text-sm font-medium',
              trendDir === 'up' ? 'text-emerald-500' : 'text-rose-500',
            )}
          >
            {trendDir === 'up' ? <TrendingUp className="size-4" /> : <TrendingDown className="size-4" />}
          </span>
        )}
      </CardContent>
    </Card>
  );
}

registerWidget('metric', MetricWidget);
export { MetricWidget };
