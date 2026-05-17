'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { createContext, useContext, useMemo } from 'react';
import type { AppPageDef, AppTabDef } from '@/lib/api-apps';

export interface AppTabEntry {
  appId: string;
  appDisplayName: string;
  id: string;
  label: string;
  icon: string;
  order: number;
  component: (props: AppComponentProps) => React.ReactElement;
}

export interface AppPageEntry {
  appId: string;
  id: string;
  label: string;
  path: string;
  component: (props: AppComponentProps) => React.ReactElement;
}

export interface AppComponentProps {
  React: typeof import('react');
  request: (path: string, init?: RequestInit) => Promise<unknown>;
  token: string;
  appId: string;
}

export interface RegisteredApp {
  id: string;
  pages?: { id: string; path: string; component: (props: AppComponentProps) => React.ReactElement }[];
  agentTabs?: { id: string; component: (props: AppComponentProps) => React.ReactElement }[];
  settingTabs?: { id: string; component: (props: AppComponentProps) => React.ReactElement }[];
}

export interface AppRegistry {
  entries: Record<string, RegisteredApp>;
  // metadata from backend manifests (for labels/icons)
  pageMeta: Record<string, AppPageDef[]>;      // appId → pages
  agentTabMeta: Record<string, AppTabDef[]>;   // appId → agent_tabs
  settingTabMeta: Record<string, AppTabDef[]>; // appId → setting_tabs
}

export const AppRegistryContext = createContext<AppRegistry>({
  entries: {},
  pageMeta: {},
  agentTabMeta: {},
  settingTabMeta: {},
});

export function useAppRegistry(): AppRegistry {
  return useContext(AppRegistryContext);
}

export function useAppAgentTabs(): AppTabEntry[] {
  const { entries, agentTabMeta } = useContext(AppRegistryContext);
  return useMemo(() => {
    const result: AppTabEntry[] = [];
    for (const [appId, app] of Object.entries(entries)) {
      if (!app.agentTabs) continue;
      const meta = agentTabMeta[appId] ?? [];
      for (const tab of app.agentTabs) {
        const m = meta.find((t) => t.id === tab.id);
        result.push({
          appId,
          appDisplayName: appId,
          id: `app-${appId}-${tab.id}`,
          label: m?.label ?? tab.id,
          icon: m?.icon ?? 'Package',
          order: m?.order ?? 999,
          component: tab.component,
        });
      }
    }
    return result.sort((a, b) => a.order - b.order);
  }, [entries, agentTabMeta]);
}

export function useAppSettingTabs(): AppTabEntry[] {
  const { entries, settingTabMeta } = useContext(AppRegistryContext);
  return useMemo(() => {
    const result: AppTabEntry[] = [];
    for (const [appId, app] of Object.entries(entries)) {
      if (!app.settingTabs) continue;
      const meta = settingTabMeta[appId] ?? [];
      for (const tab of app.settingTabs) {
        const m = meta.find((t) => t.id === tab.id);
        result.push({
          appId,
          appDisplayName: appId,
          id: `app-${appId}-${tab.id}`,
          label: m?.label ?? tab.id,
          icon: m?.icon ?? 'Settings',
          order: m?.order ?? 999,
          component: tab.component,
        });
      }
    }
    return result.sort((a, b) => a.order - b.order);
  }, [entries, settingTabMeta]);
}

export function useAppPages(): AppPageEntry[] {
  const { entries, pageMeta } = useContext(AppRegistryContext);
  return useMemo(() => {
    const result: AppPageEntry[] = [];
    for (const [appId, app] of Object.entries(entries)) {
      if (!app.pages) continue;
      const meta = pageMeta[appId] ?? [];
      for (const page of app.pages) {
        const m = meta.find((p) => p.id === page.id);
        result.push({
          appId,
          id: page.id,
          label: m?.label ?? page.id,
          path: page.path,
          component: page.component,
        });
      }
    }
    return result;
  }, [entries, pageMeta]);
}
