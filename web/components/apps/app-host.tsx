'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { getToken, request } from '@/lib/api-core';
import type { AppsListResponse, AppFrontendEntry } from '@/lib/api-apps';
import {
  AppRegistryContext,
  type AppRegistry,
  type RegisteredApp,
} from './app-registry-context';
import * as LucideReact from 'lucide-react';
import * as QorComponents from '@/components/qor';
import { cn } from '@/lib/utils';
import { useStore } from '@/store';
import type { Soul } from '@/types';

// Module-level registry — survives re-renders, populated by bundle.js calls
const appRegistry: Record<string, RegisteredApp> = {};
const pageMeta: AppRegistry['pageMeta'] = {};
const agentTabMeta: AppRegistry['agentTabMeta'] = {};
const settingTabMeta: AppRegistry['settingTabMeta'] = {};

// ── UI SDK types ────────────────────────────────────────────────────────────

export interface UISDKUser {
  id: string;
  email: string;
  display_name: string;
  role: 'admin' | 'user';
  tenant_id: string;
}

export type QorvenUIBus = {
  cn: typeof cn;
  icons: typeof LucideReact;
  useCurrentUser: () => UISDKUser | null;
  useActiveSoul: () => Soul | null;
  useWsConnected: () => boolean;
} & typeof QorComponents;

export type QorvenAppBus = {
  React: typeof React;
  request: (path: string, init?: RequestInit) => Promise<unknown>;
  getToken: () => string;
  register: (entry: RegisteredApp) => void;
};

// module-level cache for useCurrentUser
let _cachedUser: UISDKUser | null = null;

function useCurrentUser(): UISDKUser | null {
  const [user, setUser] = React.useState<UISDKUser | null>(_cachedUser);
  React.useEffect(() => {
    if (_cachedUser) { setUser(_cachedUser); return; }
    request('/auth/me')
      // Trust the server contract — request() throws on non-2xx, so u is always a valid session user here.
      .then((u: unknown) => { _cachedUser = u as UISDKUser; setUser(_cachedUser); })
      .catch(() => {});
  }, []);
  return user;
}

function useActiveSoul(): Soul | null {
  const activeChatId = useStore(s => s.activeChatId);
  const souls = useStore(s => s.souls);
  return activeChatId ? souls.find(soul => soul.id === activeChatId) ?? null : null;
}

function useWsConnected(): boolean {
  return useStore(s => s.wsConnected);
}

function initUiBus() {
  if (typeof window === 'undefined') return;
  if ((window as any).__QorvenUI) return;

  (window as any).__QorvenUI = {
    ...QorComponents,
    cn,
    icons: LucideReact,
    useCurrentUser,
    useActiveSoul,
    useWsConnected,
  } satisfies QorvenUIBus;
}

// The host bus exposed as window.__QorvenApp
function initHostBus() {
  if (typeof window === 'undefined') return;
  if ((window as any).__QorvenApp) return;

  (window as any).__QorvenApp = {
    React,
    request: (path: string, init?: RequestInit) => request(path, init),
    getToken: () => getToken(),
    register: (entry: RegisteredApp) => {
      appRegistry[entry.id] = entry;
    },
  };
}

export function AppHost({ children }: { children: React.ReactNode }) {
  const [manifests, setManifests] = useState<AppFrontendEntry[]>([]);
  const [registryVersion, setRegistryVersion] = useState(0);
  const injectedSlugs = useRef<Set<string>>(new Set());

  useEffect(() => {
    initHostBus();
    initUiBus();

    request<AppsListResponse>('/apps')
      .then((data) => {
        // Store metadata for context consumers
        for (const m of data.frontend_manifests) {
          pageMeta[m.app_id] = m.pages ?? [];
          agentTabMeta[m.app_id] = m.agent_tabs ?? [];
          settingTabMeta[m.app_id] = m.setting_tabs ?? [];
        }
        setManifests(data.frontend_manifests);
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    if (manifests.length === 0) return;

    let loaded = 0;
    for (const m of manifests) {
      if (injectedSlugs.current.has(m.slug)) continue;
      injectedSlugs.current.add(m.slug);

      const script = document.createElement('script');
      script.src = m.bundle_url;
      script.async = true;
      script.onload = () => {
        loaded++;
        if (loaded === manifests.filter((x) => !injectedSlugs.current.has(x.slug + '_done')).length) {
          setRegistryVersion((v) => v + 1);
        }
        injectedSlugs.current.add(m.slug + '_done');
      };
      script.onerror = () => {
        injectedSlugs.current.add(m.slug + '_done');
      };
      document.head.appendChild(script);
    }

    // Bump version after a short delay to pick up synchronously-registered apps
    const t = setTimeout(() => setRegistryVersion((v) => v + 1), 300);
    return () => clearTimeout(t);
  }, [manifests]);

  // Memoize so parent re-renders (e.g. Zustand store updates) do not propagate
  // a new context value — consumers only re-render when registryVersion changes.
  const ctxValue: AppRegistry = useMemo(() => ({
    entries: registryVersion >= 0 ? { ...appRegistry } : {},
    pageMeta,
    agentTabMeta,
    settingTabMeta,
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [registryVersion]);

  return (
    <AppRegistryContext.Provider value={ctxValue}>
      {children}
    </AppRegistryContext.Provider>
  );
}
