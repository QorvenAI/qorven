'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// CanvasHeader — standard page header for every main canvas page.
// Like pageheader.php: title left, description below, actions pinned right.
// Every page that has a title must use this — never inline h1/flex headers.

import type { ReactNode } from 'react';

export function CanvasHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
}) {
  return (
    <div className="flex items-start justify-between px-6 py-5 shrink-0">
      <div className="min-w-0">
        <h1 className="text-xl font-semibold leading-tight">{title}</h1>
        {description && (
          <p className="text-sm text-muted-foreground mt-0.5">{description}</p>
        )}
      </div>
      {actions && (
        <div className="flex items-center gap-2 shrink-0 ml-4">
          {actions}
        </div>
      )}
    </div>
  );
}
