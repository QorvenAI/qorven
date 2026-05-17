'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import React, { useEffect, useRef, type ReactNode } from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { useStore } from '@/store';
import { agents } from '@/lib/api';
import { toast } from 'sonner';
import { Rail } from './rail';
import { Sidebar } from './sidebar';
import { Header } from './header';
import { ToolbarProvider, Toolbar, useToolbar } from './toolbar';
import { ContextPanel } from './context-panel';
import { RightPanel } from './right-panel';
import { BottomDrawer } from './bottom-drawer';
import { StatusBar } from './status-bar';
import { ReconnectBanner } from '@/components/reconnect-banner';
import { Home, MessageSquare, Bot, CheckSquare, Settings } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useActiveRail } from '@/hooks/use-active-rail';

const mobileNav = [
  { href: '/', icon: Home, label: 'Home' },
  { href: '/qors', icon: Bot, label: 'Qors' },
  { href: '/mail', icon: MessageSquare, label: 'Chat' },
  { href: '/tasks', icon: CheckSquare, label: 'Tasks' },
  { href: '/settings', icon: Settings, label: 'Settings' },
];

export function QorvenLayout({ children }: { children: ReactNode }) {
  return (
    <ToolbarProvider>
      <QorvenLayoutInner>{children}</QorvenLayoutInner>
    </ToolbarProvider>
  );
}

function QorvenLayoutInner({ children }: { children: ReactNode }) {
  const router = useRouter();
  const activeRail = useActiveRail();
  const sidebarCollapsed = useStore((s) => s.sidebarCollapsed);
  const rightPanelOpen = useStore((s) => s.rightPanelOpen);
  const contextPanelOpen = useStore((s) => s.contextPanelOpen);
  const bottomDrawerOpen = useStore((s) => s.bottomDrawerOpen);
  const bottomDrawerHeightPx = useStore((s) => s.bottomDrawerHeightPx);
  const bottomDrawerTabs = useStore((s) => s.bottomDrawerTabs);
  const setSouls = useStore((s) => s.setSouls);
  const souls = useStore((s) => s.souls);
  const pathname = usePathname();
  const rootRef = useRef<HTMLDivElement>(null);

  const fullBleedPage = pathname?.startsWith('/terminal');
  // /qors renders its own two-panel layout — suppress the global Sidebar to avoid double sidebar
  const noSidebarPage = fullBleedPage || pathname === '/qors';
  const effectiveCollapsed = sidebarCollapsed || noSidebarPage;

  const { left, right } = useToolbar();
  const hasToolbar = !!(left || right);

  useEffect(() => {
    if (souls.length === 0) {
      agents.list().then((data) => setSouls(Array.isArray(data) ? data : [])).catch(() => {});
    }
  }, [souls.length, setSouls]);

  // Global budget_warning toast — shows on any page when an agent approaches its budget
  useEffect(() => {
    const handler = (e: Event) => {
      const d = (e as CustomEvent<{ agent_id?: string; pct?: number; used?: number; budget?: number }>).detail;
      const pct = d?.pct ?? 0;
      const label = d?.agent_id ? `Agent ${d.agent_id.slice(0, 8)}` : 'An agent';
      if (pct >= 100) {
        toast.error(`${label} has exceeded its budget (${pct}%)`);
      } else {
        toast.warning(`${label} is at ${pct}% of its budget`);
      }
    };
    window.addEventListener('qorven:budget_warning', handler);
    return () => window.removeEventListener('qorven:budget_warning', handler);
  }, []);

  useEffect(() => {
    const el = rootRef.current?.closest('.qorven');
    if (!el) return;
    const timer = setTimeout(() => el.classList.add('layout-initialized'), 500);
    return () => clearTimeout(timer);
  }, []);

  useEffect(() => {
    const el = rootRef.current?.closest('.qorven');
    if (!el) return;
    el.classList.add('no-transition');
    el.classList.toggle('sidebar-collapse', !!effectiveCollapsed);
    const raf = requestAnimationFrame(() => el.classList.remove('no-transition'));
    return () => cancelAnimationFrame(raf);
  }, [effectiveCollapsed, fullBleedPage]);

  useEffect(() => {
    const el = rootRef.current?.closest('.qorven');
    if (el) el.classList.toggle('context-panel-open', contextPanelOpen);
  }, [contextPanelOpen]);

  useEffect(() => {
    const el = rootRef.current?.closest('.qorven');
    if (el) el.classList.toggle('right-panel-open', rightPanelOpen);
  }, [rightPanelOpen]);

  useEffect(() => {
    const el = rootRef.current?.closest('.qorven');
    if (el) el.classList.toggle('has-toolbar', hasToolbar);
  }, [hasToolbar]);

  useEffect(() => {
    const el = rootRef.current?.closest<HTMLElement>('.qorven');
    if (!el) return;
    const active = bottomDrawerOpen && bottomDrawerTabs.length > 0;
    el.classList.toggle('bottom-drawer-open', active);
    el.style.setProperty('--bottom-drawer-height', active ? `${bottomDrawerHeightPx}px` : '0px');
  }, [bottomDrawerOpen, bottomDrawerHeightPx, bottomDrawerTabs.length]);

  return (
    <div ref={rootRef} style={{ width: '100%', minHeight: '100vh', position: 'relative', '--status-bar-height': '24px' } as React.CSSProperties}>
      <ReconnectBanner />
      <Rail />
      <Sidebar />
      <div className="wrapper flex min-h-screen flex-col">
        <Header />
        <Toolbar />
        <main className="main-canvas flex-1 overflow-y-auto">
          {children}
        </main>
      </div>
      <ContextPanel />
      <RightPanel />
      <BottomDrawer />
      <StatusBar />
      <nav className="mobile-bottom-bar">
        {mobileNav.map(({ href, icon: Icon, label }) => {
          const active = href === '/' ? activeRail === 'dashboard' : href.includes(activeRail as string);
          return (
            <button key={href} onClick={() => router.push(href)}
              className={cn('flex flex-col items-center gap-0.5 px-3 py-1 text-xs transition-colors',
                active ? 'text-primary' : 'text-muted-foreground hover:text-foreground')}>
              <Icon className="h-5 w-5" />
              {label}
            </button>
          );
        })}
      </nav>
    </div>
  );
}
