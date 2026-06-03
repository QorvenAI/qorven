'use client';
// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
import { PieChart, Pie, Cell, ResponsiveContainer, Tooltip } from 'recharts';
import { useDashboardData } from '@/contexts/dashboard-data';
import { registerWidget } from '@/components/dashboard/widget-registry';
import type { WidgetConfig } from '@/components/dashboard/widget-registry';

const COLORS = ['var(--primary)', '#7b92ff', '#a78bfa', '#2dd4bf', '#34d399'];

function DonutWidget({ config }: { config: WidgetConfig }) {
  const { data } = useDashboardData();
  const raw = data[config.dataSource];
  const items = Array.isArray(raw) ? raw as { name: string; value: number }[] : [];
  const total = items.reduce((s, i) => s + (i.value ?? 0), 0);

  return (
    <div className="flex flex-col h-full p-1">
      <p className="text-xs font-medium text-muted-foreground mb-2 truncate">{config.title}</p>
      {items.length === 0 ? (
        <div className="flex-1 flex items-center justify-center text-xs text-muted-foreground">No data</div>
      ) : (
        <div className="flex-1 min-h-0 flex items-center gap-3">
          <ResponsiveContainer width="50%" height="100%">
            <PieChart>
              <Pie data={items} cx="50%" cy="50%" innerRadius="55%" outerRadius="80%" dataKey="value" paddingAngle={2}>
                {items.map((_, i) => <Cell key={i} fill={COLORS[i % COLORS.length]} />)}
              </Pie>
              <Tooltip formatter={(v: number) => [config.config?.prefix ? `${config.config.prefix}${v}` : v, '']} />
            </PieChart>
          </ResponsiveContainer>
          <div className="flex-1 space-y-1 overflow-hidden">
            {items.slice(0, 5).map((item, i) => (
              <div key={i} className="flex items-center gap-1.5 text-xs">
                <span className="h-2 w-2 rounded-full flex-shrink-0" style={{ background: COLORS[i % COLORS.length] }} />
                <span className="truncate text-muted-foreground">{item.name}</span>
                <span className="ml-auto font-medium text-foreground">{total > 0 ? `${Math.round(item.value / total * 100)}%` : '—'}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

registerWidget('donut', DonutWidget);
export { DonutWidget };
