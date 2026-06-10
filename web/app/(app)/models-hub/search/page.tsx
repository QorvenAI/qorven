'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { SearchProvidersTab } from '../search-tab';
import { PageShell } from '@/components/layouts/page-shell';

export default function SearchPage() {
  return (
    <PageShell title="Search Providers" description="Web search grounding — Brave, Tavily, Exa, Serper and more">
      <SearchProvidersTab />
    </PageShell>
  );
}
