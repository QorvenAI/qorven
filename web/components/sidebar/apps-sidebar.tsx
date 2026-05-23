'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useRouter, usePathname } from 'next/navigation';
import { Package, LayoutGrid, Sparkles, Layers, ChevronLeft } from 'lucide-react';
import { SidebarMenuItem, SidebarDivider, SidebarGroupTitle } from './sidebar-primitives';
import { SidebarLayout } from './sidebar-layout';
import { useAppDisplayName, useAppPagesForSlug } from '@/components/apps/app-registry-context';

// When inside /apps/{slug}/*, show that app's pages as the sidebar nav.
// When at /apps or /apps/{slug} with no sub-page, show the launcher nav.

function AppNavSidebar({ slug }: { slug: string }) {
  const router = useRouter();
  const pathname = usePathname();
  const displayName = useAppDisplayName(slug);
  const pages = useAppPagesForSlug(slug);

  return (
    <SidebarLayout
      section3={
        <>
          <button
            onClick={() => router.push('/apps')}
            className="flex items-center gap-1.5 px-4 py-2 text-2xs text-muted-foreground hover:text-foreground transition-colors"
          >
            <ChevronLeft className="h-3.5 w-3.5" />
            All Apps
          </button>
          <SidebarGroupTitle>{displayName}</SidebarGroupTitle>
          <ul className="flex flex-col gap-px px-2.5">
            {pages.map((p) => (
              <SidebarMenuItem
                key={p.id}
                icon={Package}
                label={p.label}
                active={pathname === `/apps/${slug}/${p.path}` || pathname?.startsWith(`/apps/${slug}/${p.path}/`)}
                onClick={() => router.push(`/apps/${slug}/${p.path}`)}
              />
            ))}
            {pages.length === 0 && (
              <SidebarMenuItem
                icon={Package}
                label="Home"
                active={true}
                onClick={() => router.push(`/apps/${slug}/home`)}
              />
            )}
          </ul>
        </>
      }
    />
  );
}

function AppsLauncherSidebar() {
  const router = useRouter();
  const pathname = usePathname();

  return (
    <SidebarLayout
      section3={
        <ul className="flex flex-col gap-px px-2.5">
          <SidebarMenuItem icon={LayoutGrid} label="Apps"
            active={pathname === '/apps'}
            onClick={() => router.push('/apps')} />
          <SidebarMenuItem icon={Sparkles} label="Skills"
            active={pathname?.startsWith('/skills') ?? false}
            onClick={() => router.push('/skills')} />
          <SidebarMenuItem icon={Layers} label="Blueprints"
            active={pathname?.startsWith('/marketplace') ?? false}
            onClick={() => router.push('/marketplace')} />
        </ul>
      }
    />
  );
}

export function AppsSidebar() {
  const pathname = usePathname();

  // Extract slug from /apps/{slug}/...
  const appMatch = pathname?.match(/^\/apps\/([^/]+)(?:\/|$)/);
  const slug = appMatch?.[1];

  if (slug && slug !== '__app__') {
    return <AppNavSidebar slug={slug} />;
  }

  return <AppsLauncherSidebar />;
}
