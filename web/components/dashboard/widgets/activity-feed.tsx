'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useDashboardData } from '@/contexts/dashboard-data';
import { registerWidget, type WidgetConfig } from '../widget-registry';

interface ActivityItem {
  label: string;
  time: string;
  icon?: string;
}

function relativeTime(time: string): string {
  try {
    const diff = Math.floor((Date.now() - new Date(time).getTime()) / 1000);
    if (diff < 60) return `${diff}s ago`;
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
    return `${Math.floor(diff / 86400)}d ago`;
  } catch {
    return time;
  }
}

function ActivityFeedWidget({ config }: { config: WidgetConfig }) {
  const { data } = useDashboardData();
  const raw = data[config.dataSource];
  const items: ActivityItem[] = Array.isArray(raw)
    ? (raw as ActivityItem[]).slice(0, 8)
    : [];

  return (
    <Card className="h-full flex flex-col">
      <CardHeader className="min-h-10 px-4 py-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{config.title}</CardTitle>
      </CardHeader>
      <CardContent className="flex-1 overflow-y-auto px-4 pb-3 pt-0">
        {items.length === 0 ? (
          <p className="text-xs text-muted-foreground py-2">No recent activity</p>
        ) : (
          <ul className="space-y-2">
            {items.map((item, i) => (
              <li key={i} className="flex items-start gap-2">
                <span className="mt-1 shrink-0 size-1.5 rounded-full bg-primary" />
                <span className="flex-1 text-xs leading-relaxed text-foreground">
                  {item.icon && <span className="mr-1">{item.icon}</span>}
                  {item.label}
                </span>
                <span className="shrink-0 text-xs text-muted-foreground whitespace-nowrap">
                  {relativeTime(item.time)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

registerWidget('activity', ActivityFeedWidget);
export { ActivityFeedWidget };
