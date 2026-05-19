'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useRouter, useSearchParams } from 'next/navigation';
import { Package, LayoutGrid, Sparkles, Layers } from 'lucide-react';
import { SidebarMenuItem, SidebarDivider, SidebarGroupTitle } from './sidebar-primitives';
import { SidebarLayout } from './sidebar-layout';
import { useAppPages } from '@/components/apps/app-registry-context';

export function AppsSidebar() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const pages = useAppPages();
  const section = searchParams.get('section') ?? 'apps';

  return (
    <SidebarLayout
      section3={
        <>
          <ul className="flex flex-col gap-px px-2.5">
            <SidebarMenuItem icon={LayoutGrid} label="Apps"
              active={section === 'apps'}
              onClick={() => router.push('/apps?section=apps')} />
            <SidebarMenuItem icon={Sparkles} label="Skills"
              active={section === 'skills'}
              onClick={() => router.push('/apps?section=skills')} />
            <SidebarMenuItem icon={Layers} label="Blueprints"
              active={section === 'blueprints'}
              onClick={() => router.push('/apps?section=blueprints')} />
          </ul>
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
      }
    />
  );
}
