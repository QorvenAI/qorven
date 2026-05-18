// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { cn } from '@/lib/utils';

function Bone({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded-md bg-muted dark:bg-muted', className)} />;
}

export function SoulCardSkeleton() {
  return (
    <div className="qr-card p-4">
      <div className="flex items-center gap-3">
        <Bone className="h-10 w-10 rounded-full" />
        <div className="flex-1 space-y-2">
          <Bone className="h-4 w-24" />
          <Bone className="h-3 w-16" />
        </div>
      </div>
      <div className="mt-3 flex gap-2">
        <Bone className="h-5 w-20 rounded-full" />
        <Bone className="h-5 w-16 rounded-full" />
      </div>
      <Bone className="mt-3 h-3 w-full" />
    </div>
  );
}

export function ChatBubbleSkeleton({ align = 'left' }: { align?: 'left' | 'right' }) {
  return (
    <div className={cn('flex gap-2', align === 'right' && 'flex-row-reverse')}>
      <Bone className="h-7 w-7 shrink-0 rounded-full" />
      <div className={cn('space-y-1.5', align === 'right' ? 'items-end' : 'items-start')}>
        <Bone className="h-4 w-48" />
        <Bone className="h-4 w-32" />
      </div>
    </div>
  );
}

export function TableRowSkeleton({ cols = 5 }: { cols?: number }) {
  return (
    <div className="flex items-center gap-4 border-b border-border px-4 py-3">
      {Array.from({ length: cols }).map((_, i) => (
        <Bone key={i} className={cn('h-4', i === 0 ? 'w-32' : 'w-20', 'flex-shrink-0')} />
      ))}
    </div>
  );
}
