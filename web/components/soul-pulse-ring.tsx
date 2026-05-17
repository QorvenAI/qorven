'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { motion, type TargetAndTransition } from 'motion/react';
import { cn } from '@/lib/utils';
import type { SoulActivity } from '@/types';

const config: Record<SoulActivity, { color: string; animate: TargetAndTransition }> = {
  idle: {
    color: 'bg-soul-idle',
    animate: { scale: [1, 1.4, 1], opacity: [0.6, 0, 0.6] },
  },
  thinking: {
    color: 'bg-soul-thinking',
    animate: { scale: [1, 1.6, 1], opacity: [0.7, 0, 0.7] },
  },
  running: {
    color: 'bg-soul-running',
    animate: { rotate: 360, scale: [1, 1.3, 1], opacity: [0.7, 0.3, 0.7] },
  },
  offline: { color: 'bg-soul-offline', animate: {} },
  error: {
    color: 'bg-soul-error',
    animate: { scale: [1, 1.3, 1], opacity: [0.7, 0, 0.7] },
  },
};

interface SoulPulseRingProps {
  activity: SoulActivity;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

const sizes = { sm: 'h-2.5 w-2.5', md: 'h-4 w-4', lg: 'h-6 w-6' };
const ringSizes = { sm: 'h-5 w-5', md: 'h-7 w-7', lg: 'h-10 w-10' };

export function SoulPulseRing({ activity, size = 'md', className }: SoulPulseRingProps) {
  const { color, animate } = config[activity];
  const isAnimated = activity !== 'offline';

  return (
    <div className={cn('relative inline-flex items-center justify-center', ringSizes[size], className)}>
      {/* Pulse ring */}
      {isAnimated && (
        <motion.span
          className={cn('absolute rounded-full', color, sizes[size])}
          animate={animate}
          transition={{ duration: activity === 'running' ? 1.5 : 2, repeat: Infinity, ease: 'easeInOut' }}
        />
      )}
      {/* Solid dot */}
      <span className={cn('relative rounded-full', color, sizes[size])} />
    </div>
  );
}
