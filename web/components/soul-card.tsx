'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import Link from 'next/link';
import { SoulPulseRing } from '@/components/soul-pulse-ring';
import { ChannelBadge } from '@/components/channel-badge';
import { useSoulRun } from '@/hooks/use-soul';
import { cn } from '@/lib/utils';
import { modelDisplayName } from '@/lib/model-names';
import type { Soul, ChannelType } from '@/types';

interface SoulCardProps {
  soul: Soul;
  channels?: { channel_type: ChannelType; status: string }[];
}

// Deterministic gradient colors based on soul name
const gradients = [
  'from-primary to-primary/80',
  'from-emerald-500 to-teal-600',
  'from-orange-500 to-red-600',
  'from-pink-500 to-rose-600',
  'from-cyan-500 to-blue-600',
  'from-amber-500 to-yellow-600',
  'from-fuchsia-500 to-purple-600',
  'from-lime-500 to-green-600',
];

export function soulGradient(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash);
  return gradients[Math.abs(hash) % gradients.length]!;
}

const activityBorder: Record<string, string> = {
  thinking: 'ring-2 ring-amber-400/50 animate-pulse',
  running: 'ring-2 ring-emerald-400/50',
  error: 'ring-2 ring-destructive/50',
};

export function SoulCard({ soul, channels = [] }: SoulCardProps) {
  const { activity, lastEvent, tokensToday } = useSoulRun(soul.id);

  return (
    <Link
      href={`/qors/${soul.id}`}
      className={cn(
        'group block rounded-xl border border-border bg-card p-4 transition-all hover:border-primary/30 cursor-pointer',
        activityBorder[activity] ?? '',
      )}
    >
      <div className="flex items-center gap-3">
        <SoulPulseRing activity={activity} size="md" />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{soul.display_name}</p>
          <p className="truncate text-2xs text-muted-foreground">{soul.title || soul.role}</p>
        </div>
        <span className="rounded-md bg-muted px-1.5 py-0.5 text-2xs text-muted-foreground">{modelDisplayName(soul.model)}</span>
      </div>

      {channels.length > 0 && (
        <div className="mt-3 flex flex-wrap gap-1.5">
          {channels.map((ch) => (
            <ChannelBadge key={ch.channel_type} type={ch.channel_type} status={ch.status === 'running' ? 'live' : 'offline'} />
          ))}
        </div>
      )}

      <div className="mt-3 flex items-center justify-between text-2xs text-muted-foreground">
        <span className="truncate">{lastEvent ?? 'No recent activity'}</span>
        <span className="shrink-0 rounded-md bg-primary/10 px-1.5 py-0.5 text-primary text-2xs font-medium">
          Chat →
        </span>
      </div>
    </Link>
  );
}
