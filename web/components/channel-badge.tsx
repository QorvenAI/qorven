// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { cn } from '@/lib/utils';
import { BrandIcon, getBrandColor } from '@/components/brand-icon';
import type { ChannelType } from '@/types';

interface ChannelBadgeProps {
  type: ChannelType;
  status?: 'live' | 'offline';
}

export function ChannelBadge({ type, status }: ChannelBadgeProps) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-border bg-card px-2 py-0.5 text-2xs">
      <BrandIcon name={type} size={12} />
      <span className="capitalize">{type}</span>
      {status && (
        <span className={cn('h-1.5 w-1.5 rounded-full', status === 'live' ? 'bg-emerald-400' : 'bg-muted-foreground/30')} />
      )}
    </span>
  );
}
