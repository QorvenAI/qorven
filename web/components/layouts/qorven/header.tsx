'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import Link from 'next/link';
import { useStore } from '@/store';
import { usePathname, useRouter } from 'next/navigation';
import useSWR from 'swr';
import { cn } from '@/lib/utils';
import { IconBadge } from '@/components/ui/badge';
import {
  PanelLeftClose, PanelLeft, Bell, MessageSquare, Activity,
  SquareTerminal, PanelRight, PanelRightClose, ChevronDown, Radio, Package, Menu, Search,
} from 'lucide-react';
import { useState, useEffect } from 'react';
import type { SoulActivity } from '@/types';
import { notifications as notifApi, providers as providersApi } from '@/lib/api';
import { listApps, type QorvenApp } from '@/lib/api-apps';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/qor/tooltip';

const ORG_ROLE_COLORS: Record<string, string> = {
  caio:  'bg-violet-500/15 text-violet-500 border-violet-500/20',
  coo:   'bg-amber-500/15 text-amber-500 border-amber-500/20',
  cto:   'bg-blue-500/15 text-blue-500 border-blue-500/20',
  cmo:   'bg-pink-500/15 text-pink-500 border-pink-500/20',
  cso:   'bg-emerald-500/15 text-emerald-500 border-emerald-500/20',
  cco:   'bg-cyan-500/15 text-cyan-500 border-cyan-500/20',
  chro:  'bg-orange-500/15 text-orange-500 border-orange-500/20',
  ciso:  'bg-red-500/15 text-red-500 border-red-500/20',
  cko:   'bg-teal-500/15 text-teal-500 border-teal-500/20',
  cfo:   'bg-lime-500/15 text-lime-600 border-lime-500/20',
};

// Page labels for breadcrumb
const pageLabels: Record<string, string> = {
  '/': 'Dashboard', '/qors': 'Qors', '/code': 'Code', '/terminal': 'Terminal',
  '/channels': 'Channels', '/connectors': 'Connectors', '/cron': 'Schedules',
  '/workflows': 'Flows', '/skills': 'Skills', '/analytics': 'Analytics',
  '/settings': 'Settings', '/hubs': 'Hubs', '/sessions': 'Chats',
  '/mail': 'Chat', '/drive': 'Drive', '/tasks': 'Tasks', '/schedule': 'Calendar',
  '/teams': 'Teams', '/mcp': 'MCP', '/knowledge-graph': 'Knowledge',
  '/heartbeat': 'Health', '/supervisor': 'Supervisor', '/models-hub': 'Models',
};

function TopbarAppBtn({ app }: { app: QorvenApp }) {
  const router = useRouter();
  const pathname = usePathname();
  const href = `/apps/${app.slug}`;
  const isActive = pathname?.startsWith(href);

  const iconContent = (() => {
    if (app.icon && /^\p{Emoji}/u.test(app.icon) && app.icon.length <= 4) {
      return <span className="text-base leading-none">{app.icon}</span>;
    }
    if (app.icon_url) {
      return <img src={app.icon_url} alt={app.display_name} className="h-[18px] w-[18px] rounded object-cover" />;
    }
    return <Package className="h-[18px] w-[18px]" strokeWidth={2.5} />;
  })();

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          onClick={() => router.push(href)}
          className={cn(
            'h-9 w-9 flex items-center justify-center rounded-md transition-colors cursor-pointer',
            isActive ? 'bg-primary/15 text-primary' : 'text-muted-foreground hover:bg-accent hover:text-foreground',
          )}
        >
          {iconContent}
        </button>
      </TooltipTrigger>
      <TooltipContent side="bottom" sideOffset={4}>{app.display_name}</TooltipContent>
    </Tooltip>
  );
}

export function Header() {
  const sidebarCollapsed = useStore((s) => s.sidebarCollapsed);
  const toggleSidebar = useStore((s) => s.toggleSidebar);
  const setMobileNavOpen = useStore((s) => s.setMobileNavOpen);
  const setCommandPaletteOpen = useStore((s) => s.setCommandPaletteOpen);
  const openCommandPalette = () => setCommandPaletteOpen(true);
  const activeChatId = useStore((s) => s.activeChatId);
  const souls = useStore((s) => s.souls);
  const activeSessions = useStore((s) => s.activeSessions);
  const wsConnected = useStore((s) => s.wsConnected);
  const pathname = usePathname();
  const router = useRouter();

  // Code page state — terminal toggle is mirrored into the page via
  // the store; Prime Coder button uses the global right-panel chat tab
  // so the single "chat lives in the right sidebar" rule holds.
  const codeProjectName = useStore((s) => s.codeProjectName);
  const codeTermOpen = useStore((s) => s.codeTermOpen);
  const setCodeTermOpen = useStore((s) => s.setCodeTermOpen);
  const isCodePage = pathname?.startsWith('/code');

  // Right panel
  const rightPanelOpen = useStore((s) => s.rightPanelOpen);
  const rightPanelTab = useStore((s) => s.rightPanelTab);
  const openRightPanel = useStore((s) => s.openRightPanel);
  const closeRightPanel = useStore((s) => s.closeRightPanel);

  const { data: appsData } = useSWR('apps-list', listApps, { refreshInterval: 30_000 });
  const pinnedTopbarApps = (appsData?.apps ?? [] as QorvenApp[])
    .filter((a: QorvenApp) => a.enabled && a.pinned_topbar)
    .sort((a: QorvenApp, b: QorvenApp) => a.topbar_order - b.topbar_order);

  const isSoulWorkspace = pathname?.match(/^\/(?:souls|qors)\/[^/]+$/);
  const isChat = pathname?.startsWith('/sessions/');
  const isSoulPage = isSoulWorkspace || isChat;

  const activeSoul = isSoulPage && activeChatId ? souls.find((s) => s.id === activeChatId) : null;
  const urlSoul = !activeSoul && isSoulWorkspace ? (() => { const id = pathname?.split(/\/(?:qors|souls)\//)[1]; return id ? souls.find((s) => s.id === id) : null; })() : null;
  const sessionSoul = !activeSoul && !urlSoul && isChat ? (() => { const id = pathname?.split('/sessions/')[1]; const sess = id ? activeSessions[id] : null; return sess ? souls.find((s) => s.id === sess.agent_id) : null; })() : null;
  const soul = activeSoul || urlSoul || sessionSoul || null;

  const getTitle = () => {
    if (soul) return null;
    for (const [path, label] of Object.entries(pageLabels)) {
      if (path === '/' ? pathname === '/' : pathname?.startsWith(path)) return label;
    }
    return 'Qorven';
  };
  const title = getTitle();

  const handlePanelIcon = (tab: 'chat' | 'notifications' | 'activity') => {
    if (rightPanelOpen && rightPanelTab === tab) closeRightPanel();
    else openRightPanel(tab);
  };

  return (
    <header className="header fixed top-0 right-0 z-10 flex items-stretch border-b border-border bg-background/95 backdrop-blur-sm">
      <div className="flex flex-1 items-stretch justify-between px-3 gap-2.5">

        {/* LEFT */}
        <div className="flex items-stretch gap-2.5">
          <button onClick={() => setMobileNavOpen(true)}
            aria-label="Open navigation"
            className="lg:hidden flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent transition-colors shrink-0 self-center">
            <Menu className="h-[18px] w-[18px]" />
          </button>
          <button onClick={toggleSidebar}
            className="hidden lg:flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent transition-colors shrink-0 self-center">
            {sidebarCollapsed ? <PanelLeft className="h-[18px] w-[18px]" /> : <PanelLeftClose className="h-[18px] w-[18px]" />}
          </button>

          {soul ? (
            <div className="flex flex-col justify-center min-w-0">
              <div className="flex items-center gap-1.5 text-2sm text-muted-foreground leading-none flex-wrap">
                <Link href="/qors" className="hover:text-foreground transition-colors">Qors</Link>
                <span className="text-muted-foreground/50">/</span>
                <span className="text-foreground font-medium truncate max-w-[140px]">{soul.display_name}</span>
                {(soul as any).org_role && ORG_ROLE_COLORS[(soul as any).org_role] && (
                  <span className={`inline-flex items-center rounded border px-1 py-0.5 text-2xs font-bold uppercase tracking-wide leading-none ${ORG_ROLE_COLORS[(soul as any).org_role]}`}>
                    {((soul as any).org_role as string).toUpperCase()}
                  </span>
                )}
                {(soul as any).org_level === 'l1' && (
                  <span className="inline-flex items-center rounded border px-1 py-0.5 text-2xs font-medium uppercase tracking-wide leading-none bg-amber-500/10 text-amber-500 border-amber-500/20">
                    Executive
                  </span>
                )}
              </div>
            </div>
          ) : isCodePage ? (
            <nav className="flex items-center gap-1.5 text-2sm">
              <Link href="/" className="text-muted-foreground hover:text-foreground transition-colors">Home</Link>
              <span className="text-muted-foreground/50">/</span>
              <Link href="/code" className={cn('transition-colors', codeProjectName ? 'text-muted-foreground hover:text-foreground' : 'text-foreground font-medium')}>Code</Link>
              {codeProjectName && (<>
                <span className="text-muted-foreground/50">/</span>
                <button className="flex items-center gap-1 text-foreground font-medium hover:text-primary transition-colors">
                  {codeProjectName}<ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
                </button>
              </>)}
            </nav>
          ) : (
            <nav className="flex items-center gap-1.5 text-2sm">
              <Link href="/" className="text-muted-foreground hover:text-foreground transition-colors">Home</Link>
              <span className="text-muted-foreground/50">/</span>
              <span className="text-foreground font-medium">{title}</span>
            </nav>
          )}
        </div>

        {/* CENTER: global search trigger */}
        <div className="flex flex-1 items-center justify-center px-2 min-w-0">
          <button
            onClick={openCommandPalette}
            title="Search agents, hubs, files… (⌘K)"
            className="hidden md:flex h-8.5 w-full max-w-md items-center gap-2 rounded-md border border-border bg-muted/40 px-3 text-2sm text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
          >
            <Search className="h-3.5 w-3.5 shrink-0" />
            <span className="flex-1 text-left truncate">Search agents, hubs, anything…</span>
            <kbd className="shrink-0 rounded border border-border bg-background px-1.5 py-0.5 text-2xs font-medium">⌘K</kbd>
          </button>
          <button
            onClick={openCommandPalette}
            title="Search (⌘K)"
            className="md:hidden flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground transition-colors"
          >
            <Search className="h-[18px] w-[18px]" />
          </button>
        </div>

        {/* pinned topbar apps */}
        {pinnedTopbarApps.length > 0 && (
          <TooltipProvider delayDuration={200}>
            <nav className="flex items-center gap-1 shrink-0">
              <div className="w-px h-5 bg-border mx-0.5" />
              {pinnedTopbarApps.map((app: QorvenApp) => <TopbarAppBtn key={app.id} app={app} />)}
              <div className="w-px h-5 bg-border mx-0.5" />
            </nav>
          </TooltipProvider>
        )}

        {/* RIGHT: 6 icon buttons */}
        <nav className="flex items-center gap-1.5 shrink-0">
          {isCodePage && (<>
            <IconBtn icon={SquareTerminal} label="Terminal (⌘`)" active={codeTermOpen} onClick={() => setCodeTermOpen(!codeTermOpen)} />
            <div className="w-px h-5 bg-border mx-0.5" />
          </>)}

          <IconBtn icon={MessageSquare} label="Chat panel" active={rightPanelOpen && rightPanelTab === 'chat'} onClick={() => handlePanelIcon('chat')} />
          {!isCodePage && <IconBtn icon={SquareTerminal} label="Terminal" onClick={() => router.push('/terminal')} />}
          <NotificationBtn active={rightPanelOpen && rightPanelTab === 'notifications'} onOpen={() => handlePanelIcon('notifications')} />
          <IconBtn icon={Activity} label="Activity" active={rightPanelOpen && rightPanelTab === 'activity'} onClick={() => handlePanelIcon('activity')} />
          <ConnectionStatus connected={wsConnected} />
          <button title={rightPanelOpen ? "Close panel" : "Expand panel"} onClick={() => rightPanelOpen ? closeRightPanel() : openRightPanel(rightPanelTab ?? "activity")} className="hidden lg:flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent shrink-0 self-center">{rightPanelOpen ? <PanelRightClose className="h-[18px] w-[18px]" /> : <PanelRight className="h-[18px] w-[18px]" />}</button>
        </nav>
      </div>
    </header>
  );
}

function IconBtn({ icon: Icon, label, active, onClick }: { icon: typeof PanelRight; label: string; active?: boolean; onClick?: () => void }) {
  return (
    <button title={label} onClick={onClick}
      className={cn('h-9 w-9 flex items-center justify-center rounded-md transition-colors cursor-pointer',
        active ? 'bg-primary/15 text-primary' : 'text-muted-foreground hover:bg-accent hover:text-foreground')}>
      <Icon className="h-[18px] w-[18px]" strokeWidth={2.5} />
    </button>
  );
}

function ConnectionStatus({ connected }: { connected: boolean }) {
  return (
    <div title={connected ? 'Connected' : 'Disconnected — reconnecting…'}
      className={cn('h-8.5 px-2.5 flex items-center gap-1.5 rounded-md text-xs font-medium',
        connected ? 'text-muted-foreground' : 'text-destructive')}>
      {connected ? (
        <><span className="relative flex h-2 w-2"><span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75" /><span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500" /></span><span className="hidden lg:inline">Live</span></>
      ) : (
        <><Radio className="h-3.5 w-3.5 opacity-30" /><span className="hidden lg:inline">Offline</span></>
      )}
    </div>
  );
}

function NotificationBtn({ active, onOpen }: { active: boolean; onOpen: () => void }) {
  const [count, setCount] = useState(0);
  const [discoveredCount, setDiscoveredCount] = useState(0);
  const approvals = useStore((s) => s.approvals);
  const pendingApprovalCount = Object.values(approvals).filter((a) => !a.resolved).length;

  const refresh = () => {
    notifApi.list().then((d) => setCount((Array.isArray(d) ? d : []).filter((n: any) => !n.read_at).length)).catch(() => {});
    providersApi.discoveredModels(true).then((d) => setDiscoveredCount(Array.isArray(d) ? d.length : 0)).catch(() => {});
  };

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 60_000);
    return () => clearInterval(interval);
  }, []);

  const total = count + discoveredCount + pendingApprovalCount;
  const title = pendingApprovalCount > 0
    ? `${pendingApprovalCount} agent tool request${pendingApprovalCount !== 1 ? 's' : ''} waiting for approval`
    : discoveredCount > 0
    ? `Notifications · ${discoveredCount} new model${discoveredCount !== 1 ? 's' : ''} discovered`
    : 'Notifications';

  const badgeVariant = pendingApprovalCount > 0 || discoveredCount > 0 ? 'warning' : 'destructive';

  return (
    <IconBadge count={total} variant={badgeVariant} pulse={pendingApprovalCount > 0}>
      <button title={title} onClick={onOpen}
        className={cn('h-9 w-9 flex items-center justify-center rounded-md transition-colors cursor-pointer',
          active ? 'bg-primary/15 text-primary' : 'text-muted-foreground hover:bg-accent hover:text-foreground')}>
        <Bell className="h-[18px] w-[18px]" />
      </button>
    </IconBadge>
  );
}
