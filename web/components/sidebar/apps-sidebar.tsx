'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useRouter, usePathname } from 'next/navigation';
import { Package, LayoutGrid, Sparkles, Layers, ChevronLeft, Settings } from 'lucide-react';
import { SidebarMenuItem, SidebarDivider, SidebarGroupTitle } from './sidebar-primitives';
import { SidebarLayout } from './sidebar-layout';
import { useAppDisplayName, useAppPagesForSlug } from '@/components/apps/app-registry-context';
import useSWR from 'swr';
import { listApps } from '@/lib/api-apps';

// When inside /apps/{slug}/*, show that app's pages as the sidebar nav.
// When at /apps or /apps/{slug} with no sub-page, show the launcher nav.

function AppNavSidebar({ slug }: { slug: string }) {
  const router = useRouter();
  const pathname = usePathname();
  const displayName = useAppDisplayName(slug);
  const pages = useAppPagesForSlug(slug);
  const isSettings = pathname?.startsWith(`/apps/${slug}/settings`);

  const { data } = useSWR('apps-list', listApps, { refreshInterval: 60_000 });
  const appData = data?.apps?.find(a => a.slug === slug);
  const icon = appData?.icon;
  const iconUrl = appData?.icon_url;
  const hasSettings = (appData?.settings_schema?.length ?? 0) > 0 || pages.some(p => p.path === 'settings');

  const appIcon = (() => {
    if (icon && /^\p{Emoji}/u.test(icon) && icon.length <= 4) {
      return <span className="text-base leading-none">{icon}</span>;
    }
    if (iconUrl) {
      return <img src={iconUrl} alt={displayName} className="h-4 w-4 rounded object-cover" />;
    }
    return <Package className="h-4 w-4" />;
  })();

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
          <SidebarGroupTitle>
            <span className="flex items-center gap-1.5">
              {appIcon}
              {displayName}
            </span>
          </SidebarGroupTitle>
          <ul className="flex flex-col gap-px px-2.5">
            {pages.filter(p => p.path !== 'settings').map((p) => (
              <SidebarMenuItem
                key={p.id}
                icon={Package}
                label={p.label}
                active={pathname === `/apps/${slug}/${p.path}` || pathname?.startsWith(`/apps/${slug}/${p.path}/`)}
                onClick={() => router.push(`/apps/${slug}/${p.path}`)}
              />
            ))}
            {pages.filter(p => p.path !== 'settings').length === 0 && (
              <SidebarMenuItem
                icon={Package}
                label="Home"
                active={!isSettings}
                onClick={() => router.push(`/apps/${slug}`)}
              />
            )}
            {hasSettings && (
              <>
                <SidebarDivider />
                <SidebarMenuItem
                  icon={Settings}
                  label="Settings"
                  active={isSettings ?? false}
                  onClick={() => router.push(`/apps/${slug}/settings`)}
                />
              </>
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
