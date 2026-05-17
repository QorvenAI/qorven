// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { cn } from '@/lib/utils';

function Bone({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded-md bg-muted', className)} />;
}

/** Generic page loading skeleton — grid of cards */
export function PageSkeleton({ cols = 3, rows = 2 }: { cols?: number; rows?: number }) {
  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <Bone className="h-7 w-48" />
        <Bone className="h-4 w-64" />
      </div>
      <div className={cn('grid gap-4', cols === 2 && 'sm:grid-cols-2', cols === 3 && 'sm:grid-cols-2 lg:grid-cols-3', cols === 4 && 'sm:grid-cols-2 lg:grid-cols-4')}>
        {Array.from({ length: cols * rows }).map((_, i) => (
          <div key={i} className="rounded-xl border border-border bg-card p-5 space-y-3">
            <div className="flex items-center gap-3">
              <Bone className="h-10 w-10 rounded-full" />
              <div className="flex-1 space-y-2">
                <Bone className="h-4 w-32" />
                <Bone className="h-3 w-20" />
              </div>
            </div>
            <Bone className="h-3 w-full" />
            <Bone className="h-3 w-4/5" />
          </div>
        ))}
      </div>
    </div>
  );
}

/** Table loading skeleton */
export function TableSkeleton({ rows = 6 }: { rows?: number }) {
  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <Bone className="h-7 w-48" />
        <Bone className="h-4 w-64" />
      </div>
      <div className="rounded-xl border border-border overflow-hidden">
        <div className="bg-muted/50 px-4 py-3 flex gap-4">
          {[3, 2, 2, 1].map((w, i) => <Bone key={i} className={`h-3 w-${w * 8}`} />)}
        </div>
        {Array.from({ length: rows }).map((_, i) => (
          <div key={i} className="border-t border-border px-4 py-3 flex gap-4">
            {[3, 2, 2, 1].map((w, j) => <Bone key={j} className={`h-4 w-${w * 8}`} />)}
          </div>
        ))}
      </div>
    </div>
  );
}

/** Detail page loading skeleton */
export function DetailSkeleton() {
  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Bone className="h-16 w-16 rounded-full" />
        <div className="space-y-2">
          <Bone className="h-6 w-48" />
          <Bone className="h-4 w-32" />
        </div>
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        {[1, 2, 3].map((i) => <div key={i} className="rounded-xl border border-border bg-card p-4 space-y-2"><Bone className="h-3 w-16" /><Bone className="h-5 w-24" /></div>)}
      </div>
      <div className="rounded-xl border border-border bg-card p-5 space-y-3">
        {[1, 2, 3, 4].map((i) => <Bone key={i} className={`h-4 w-${i % 2 === 0 ? 'full' : '4/5'}`} />)}
      </div>
    </div>
  );
}
