'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { SearchProvidersTab } from '../search-tab';

export default function SearchPage() {
  return (
    <div className="space-y-5">
      <div className="pb-2">
        <h1 className="text-lg font-semibold leading-none">Search Providers</h1>
        <p className="text-sm text-muted-foreground mt-1">Web search grounding — Brave, Tavily, Exa, Serper and more</p>
      </div>
      <SearchProvidersTab />
    </div>
  );
}
