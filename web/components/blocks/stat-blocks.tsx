'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { cn } from '@/lib/utils';
import { TrendingUp, TrendingDown, Minus } from 'lucide-react';

interface StatCardProps {
  value: string | number;
  label: string;
  change?: string;
  changeType?: 'up' | 'down' | 'neutral';
  icon?: string;
  color?: string;
  title?: string;
}

export function StatCard({ value, label, change, changeType = 'neutral', color }: StatCardProps) {
  const trendColor = changeType === 'up' ? 'text-emerald-400' : changeType === 'down' ? 'text-red-400' : 'text-muted-foreground';
  const TrendIcon = changeType === 'up' ? TrendingUp : changeType === 'down' ? TrendingDown : Minus;

  return (
    <div className={cn('rounded-lg border border-border bg-card p-4 space-y-2', color && `border-l-4`)} style={color ? { borderLeftColor: color } : undefined}>
      <p className="text-sm text-muted-foreground">{label}</p>
      <div className="flex items-end justify-between">
        <p className="text-2xl font-semibold tabular-nums">{value}</p>
        {change && (
          <span className={cn('flex items-center gap-1 text-xs font-medium', trendColor)}>
            <TrendIcon className="h-3 w-3" />{change}
          </span>
        )}
      </div>
    </div>
  );
}

export function StatRow({ stats }: { stats: StatCardProps[] }) {
  return (
    <div className={cn('grid gap-4', stats.length <= 3 ? `grid-cols-${stats.length}` : 'grid-cols-2 md:grid-cols-4')}>
      {stats.map((s, i) => <StatCard key={i} {...s} />)}
    </div>
  );
}
