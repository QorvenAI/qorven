'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// PageShell — the single page skeleton every non-full-bleed canvas page uses.
// Header (via CanvasHeader) + optional toolbar row (tabs/filters/search) +
// content area with responsive, token-based padding. Full-bleed pages
// (chat/code/mail/knowledge-graph) do NOT use this — they fill 100% height.
//
// Usage:
//   <PageShell title="Tasks" description="…" actions={<Btn/>} toolbar={<Filters/>}>
//     <YourContent/>
//   </PageShell>

import type { ReactNode } from 'react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { cn } from '@/lib/utils';

export function PageShell({
  title,
  description,
  actions,
  toolbar,
  children,
  contentClassName = '',
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
  toolbar?: ReactNode;
  children: ReactNode;
  /** extra classes for the scrollable content region */
  contentClassName?: string;
}) {
  return (
    <div className="flex h-full flex-col">
      <CanvasHeader title={title} description={description} actions={actions} />
      {toolbar && (
        <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border px-4 py-2.5 sm:px-6">
          {toolbar}
        </div>
      )}
      <div className={cn('flex-1 overflow-y-auto px-4 py-4 sm:px-6', contentClassName)}>
        {children}
      </div>
    </div>
  );
}
