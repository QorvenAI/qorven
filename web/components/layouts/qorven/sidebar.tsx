'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import {
  ChevronDown, Key, Palette, LogOut, Lock, User,
} from 'lucide-react';
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem,
  DropdownMenuSeparator, DropdownMenuTrigger,
} from '@/components/qor/dropdown-menu';
import { SidebarNav } from './sidebar-nav';
import { useActiveRail } from '@/hooks/use-active-rail';
import { SidebarPinned } from './sidebar-pinned';
import { CodeSidebar } from './sidebar-code';
import { MailSidebar } from '@/components/sidebar/mail-sidebar';
import { ChannelsSidebar } from '@/components/sidebar/channels-sidebar';
import { SocialSidebar } from '@/components/sidebar/social-sidebar';
import { DriveSidebar } from '@/components/sidebar/drive-sidebar';
import { TeamsSidebar } from '@/components/sidebar/teams-sidebar';
import { McpSidebar } from '@/components/sidebar/mcp-sidebar';
import { KnowledgeSidebar } from '@/components/sidebar/knowledge-sidebar';
import { ModelsSidebar } from '@/components/sidebar/models-sidebar';
import { SettingsSidebar } from '@/components/sidebar/settings-sidebar';
import { AppsSidebar } from '@/components/sidebar/apps-sidebar';

export const statusColor: Record<string, string> = {
  idle: 'bg-emerald-500', thinking: 'bg-amber-400 animate-pulse',
  running: 'bg-emerald-400', offline: 'bg-muted-foreground/20', error: 'bg-destructive',
};

/* ─── Main Sidebar ─────────────────────────────────────────────────────────── */
export function Sidebar() {
  const activeRail = useActiveRail();

  const contextual = (() => {
    switch (activeRail as string) {
      case 'code':       return <CodeSidebar />;
      case 'sessions':   return <MailSidebar />;
      case 'connectors': return <ChannelsSidebar />;
      case 'social':     return <SocialSidebar />;
      case 'drive':      return <DriveSidebar />;
      case 'org-chart':
      case 'teams':      return <TeamsSidebar />;
      case 'mcp':        return <McpSidebar />;
      case 'kg':         return <KnowledgeSidebar />;
      case 'models':     return <ModelsSidebar />;
      case 'settings':   return <SettingsSidebar />;
      case 'apps':       return <AppsSidebar />;
      default:           return <SidebarNav />;   // dashboard, rooms, souls, more, unknown
    }
  })();

  return (
    <div
      className="sidebar fixed top-0 bottom-0 z-20 flex flex-col overflow-hidden border-e border-border bg-muted"
      style={{ left: 'var(--rail-width)' }}
    >
      <div className="w-(--sidebar-default-width) flex flex-col h-full overflow-hidden">
        <SidebarHeader />
        <div className="flex-1 overflow-y-auto">
          {contextual}
        </div>
        <SidebarPinned />
      </div>
    </div>
  );
}

/* ─── Sidebar Header ────────────────────────────────────────────────────────── */
export function SidebarHeader() {
  const router = useRouter();
  const [user, setUser] = useState<{ username: string; role: string } | null>(null);

  useEffect(() => {
    try {
      const stored = localStorage.getItem('qorven_user');
      if (stored) { setUser(JSON.parse(stored)); return; }
    } catch {}
    const token = localStorage.getItem('qorven_token');
    if (token) {
      try {
        const payload = JSON.parse(atob(token.split('.')[1]!));
        setUser({ username: payload.username || payload.sub || 'User', role: payload.role || 'user' });
      } catch {}
    }
  }, []);

  const initial = (user?.username?.[0] ?? 'U').toUpperCase();
  const rawName = user?.username ?? 'User';
  const displayName = rawName.charAt(0).toUpperCase() + rawName.slice(1);

  const handleLogout = () => {
    localStorage.removeItem('qorven_token');
    localStorage.removeItem('qorven_user');
    document.cookie = 'qorven_token=; path=/; max-age=0';
    router.push('/login');
  };

  return (
    <div className="flex h-[var(--header-height)] w-full shrink-0 items-center border-b border-border px-2.5 gap-1.5">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button className="flex flex-1 min-w-0 items-center gap-2.5 px-1.5 py-1 rounded-md hover:bg-accent transition-colors">
            <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-primary/20 text-primary text-xs font-bold">
              {initial}
            </div>
            <div className="flex flex-col items-start min-w-0 flex-1">
              <span className="text-2sm font-medium text-foreground truncate leading-tight">{displayName}</span>
            </div>
            <ChevronDown className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent className="w-52" align="start" sideOffset={4}>
          <DropdownMenuItem onClick={() => router.push('/settings')}>
            <User className="h-4 w-4" /><span>Profile &amp; Settings</span>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => router.push('/provider-keys')}>
            <Key className="h-4 w-4" /><span>API Keys</span>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => router.push('/settings')}>
            <Palette className="h-4 w-4" /><span>Appearance</span>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={handleLogout}>
            <Lock className="h-4 w-4" /><span>Lock screen</span>
          </DropdownMenuItem>
          <DropdownMenuItem onClick={handleLogout} className="text-destructive focus:text-destructive">
            <LogOut className="h-4 w-4" /><span>Sign out</span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
