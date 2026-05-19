'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { SearchProvidersTab } from '../search-tab';
import { CanvasHeader } from '@/components/layouts/canvas-header';

export default function SearchPage() {
  return (
    <div className="space-y-5">
      <CanvasHeader title="Search Providers" description="Web search grounding — Brave, Tavily, Exa, Serper and more" />
      <SearchProvidersTab />
    </div>
  );
}
