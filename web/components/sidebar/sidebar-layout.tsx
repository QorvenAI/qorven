'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// SidebarLayout — the slot contract every left-sidebar file must use.
// section2: optional 44px filter/search row — sticky so it never scrolls away
// section3: optional nav content — scrolls with the sidebar parent
//
// Usage:
//   <SidebarLayout
//     section2={<AgentFilterDropdown />}
//     section3={<NavItemsList />}
//   />

import type { ReactNode } from 'react';

export function SidebarLayout({
  section2,
  section3,
}: {
  section2?: ReactNode;
  section3?: ReactNode;
}) {
  return (
    <>
      {section2 && (
        <div className="sticky top-0 z-10 h-[44px] shrink-0 flex items-center border-b border-border px-2 bg-muted">
          {section2}
        </div>
      )}
      {section3 && (
        <div className="py-1">
          {section3}
        </div>
      )}
    </>
  );
}
