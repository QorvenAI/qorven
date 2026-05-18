'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useStore } from '@/store';
import { usePathname, useRouter } from 'next/navigation';
import { cn } from '@/lib/utils';
import {
  PanelLeftClose, PanelLeft, Bell, MessageSquare, Activity,
  SquareTerminal, PanelRight, PanelRightClose, ChevronDown, Radio,
} from 'lucide-react';
import { useState, useEffect } from 'react';
import type { SoulActivity } from '@/types';
import { notifications as notifApi, providers as providersApi } from '@/lib/api';

// Page labels for breadcrumb
const pageLabels: Record<string, string> = {
  '/': 'Dashboard', '/qors': 'Qors', '/code': 'Code', '/terminal': 'Terminal',
  '/channels': 'Channels', '/connectors': 'Connectors', '/cron': 'Schedules',
  '/workflows': 'Flows', '/skills': 'Skills', '/analytics': 'Analytics',
  '/settings': 'Settings', '/rooms': 'Hubs', '/sessions': 'Chats',
  '/mail': 'Chat', '/drive': 'Drive', '/tasks': 'Tasks', '/schedule': 'Calendar',
  '/teams': 'Teams', '/mcp': 'MCP', '/knowledge-graph': 'Knowledge',
  '/heartbeat': 'Health', '/supervisor': 'Supervisor', '/models-hub': 'Models',
};

export function Header() {
  const sidebarCollapsed = useStore((s) => s.sidebarCollapsed);
  const toggleSidebar = useStore((s) => s.toggleSidebar);
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
          <button onClick={toggleSidebar}
            className="hidden lg:flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent transition-colors shrink-0 self-center">
            {sidebarCollapsed ? <PanelLeft className="h-[18px] w-[18px]" /> : <PanelLeftClose className="h-[18px] w-[18px]" />}
          </button>

          {soul ? (
            <div className="flex flex-col justify-center min-w-0">
              <div className="flex items-center gap-1.5 text-2sm text-muted-foreground leading-none">
                <a href="/qors" className="hover:text-foreground transition-colors">Qors</a>
                <span className="text-muted-foreground/50">/</span>
                <span className="text-foreground font-medium truncate max-w-[160px]">{soul.display_name}</span>
              </div>
            </div>
          ) : isCodePage ? (
            <nav className="flex items-center gap-1.5 text-2sm">
              <a href="/" className="text-muted-foreground hover:text-foreground transition-colors">Home</a>
              <span className="text-muted-foreground/50">/</span>
              <a href="/code" className="text-muted-foreground hover:text-foreground transition-colors">Code</a>
              {codeProjectName && (<>
                <span className="text-muted-foreground/50">/</span>
                <button className="flex items-center gap-1 text-foreground font-medium hover:text-primary transition-colors">
                  {codeProjectName}<ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
                </button>
              </>)}
            </nav>
          ) : (
            <nav className="flex items-center gap-1.5 text-2sm">
              <a href="/" className="text-muted-foreground hover:text-foreground transition-colors">Home</a>
              <span className="text-muted-foreground/50">/</span>
              <span className="text-foreground font-medium">{title}</span>
            </nav>
          )}
        </div>

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
      <Icon className="h-4 w-4" />
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

  return (
    <button title={title} onClick={onOpen}
      className={cn('h-9 w-9 flex items-center justify-center rounded-md transition-colors cursor-pointer relative',
        active ? 'bg-primary/15 text-primary' : 'text-muted-foreground hover:bg-accent hover:text-foreground')}>
      <Bell className="h-4 w-4" />
      {total > 0 && (
        <span className={cn(
          'absolute -top-1 -right-1 h-4 w-4 rounded-full text-xs font-bold text-white flex items-center justify-center',
          pendingApprovalCount > 0 ? 'bg-amber-500 animate-pulse' : discoveredCount > 0 ? 'bg-amber-500' : 'bg-destructive',
        )}>
          {total > 9 ? '9+' : total}
        </span>
      )}
    </button>
  );
}
