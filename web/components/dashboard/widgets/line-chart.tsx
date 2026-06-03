'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useDashboardData } from '@/contexts/dashboard-data';
import { registerWidget, type WidgetConfig } from '../widget-registry';

function LineChartWidget({ config }: { config: WidgetConfig }) {
  const { data } = useDashboardData();
  const raw = data[config.dataSource];
  const chartData = Array.isArray(raw) ? (raw as Record<string, unknown>[]) : [];
  const xKey = config.config?.xKey ?? 'x';
  const yKey = config.config?.yKey ?? 'y';
  const color = config.config?.color ?? 'var(--primary)';

  return (
    <Card className="h-full flex flex-col">
      <CardHeader className="min-h-10 px-4 py-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{config.title}</CardTitle>
      </CardHeader>
      <CardContent className="flex-1 px-2 pb-3 pt-0">
        {chartData.length === 0 ? (
          <div className="h-full flex items-center justify-center text-xs text-muted-foreground">
            No data
          </div>
        ) : (
          <ResponsiveContainer width="100%" height={180}>
            <AreaChart data={chartData} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
              <defs>
                <linearGradient id={`fill-${config.id}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor={color} stopOpacity={0.2} />
                  <stop offset="95%" stopColor={color} stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
              <XAxis
                dataKey={xKey}
                tick={{ fontSize: 10, fill: 'var(--muted-foreground)' }}
                tickLine={false}
                axisLine={false}
              />
              <YAxis
                tick={{ fontSize: 10, fill: 'var(--muted-foreground)' }}
                tickLine={false}
                axisLine={false}
                width={36}
              />
              <Tooltip
                contentStyle={{
                  background: 'var(--background)',
                  border: '1px solid var(--border)',
                  borderRadius: 8,
                  fontSize: 12,
                }}
              />
              <Area
                type="monotone"
                dataKey={yKey}
                stroke={color}
                strokeWidth={2}
                fill={`url(#fill-${config.id})`}
                dot={false}
              />
            </AreaChart>
          </ResponsiveContainer>
        )}
      </CardContent>
    </Card>
  );
}

registerWidget('line', LineChartWidget);
registerWidget('area', LineChartWidget);
export { LineChartWidget };
