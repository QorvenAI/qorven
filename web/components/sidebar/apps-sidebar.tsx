'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useRouter } from 'next/navigation';
import { Package, LayoutGrid } from 'lucide-react';
import { SidebarMenuItem, SidebarDivider, SidebarGroupTitle } from './sidebar-primitives';
import { useAppPages } from '@/components/apps/app-registry-context';

export function AppsSidebar() {
  const router = useRouter();
  const pages = useAppPages();

  return (
    <>
      <SidebarMenuItem
        icon={LayoutGrid}
        label="Installed Apps"
        onClick={() => router.push('/apps')}
      />
      {pages.length > 0 && (
        <>
          <SidebarDivider />
          <SidebarGroupTitle>App Pages ({pages.length})</SidebarGroupTitle>
          <ul className="flex flex-col gap-px px-2.5">
            {pages.map((p) => (
              <SidebarMenuItem
                key={`${p.appId}-${p.id}`}
                icon={Package}
                label={p.label}
                onClick={() => router.push(`/apps/${p.appId}/${p.path}`)}
              />
            ))}
          </ul>
        </>
      )}
    </>
  );
}
