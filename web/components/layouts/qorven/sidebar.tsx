'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { soulGradient } from '@/components/soul-card';
import { CreateSoulSheet } from '@/components/forms/create-soul-sheet';
import {
  ChevronDown, Plus, Settings2, Key, Palette, LogOut, Users, BarChart3,
  MessageSquare, CheckSquare, Search, Sparkles, Pin, Lock, User,
  ShieldCheck, Zap, Cpu, Bell,
} from 'lucide-react';
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem,
  DropdownMenuSeparator, DropdownMenuTrigger,
} from '@/components/qor/dropdown-menu';
import type { Session } from '@/types';

import { useActiveRail } from '@/hooks/use-active-rail';
import { PrimeVoiceWidget } from '@/components/prime-voice-widget';
import { useVoiceEnabled } from '@/hooks/use-voice-enabled';
import { rooms as roomsApi } from '@/lib/api';

import { MailSidebar }      from '@/components/sidebar/mail-sidebar';
import { CalendarSidebar }  from '@/components/sidebar/calendar-sidebar';
import { DriveSidebar }     from '@/components/sidebar/drive-sidebar';
import { TasksSidebar }     from '@/components/sidebar/tasks-sidebar';
import { ChannelsSidebar }  from '@/components/sidebar/channels-sidebar';
import { WorkflowsSidebar } from '@/components/sidebar/workflows-sidebar';
import { SocialSidebar }    from '@/components/sidebar/social-sidebar';
import { SkillsSidebar }    from '@/components/sidebar/skills-sidebar';
import { ModelsSidebar }    from '@/components/sidebar/models-sidebar';
import { SettingsSidebar }  from '@/components/sidebar/settings-sidebar';
import { TeamsSidebar }     from '@/components/sidebar/teams-sidebar';
import { McpSidebar }       from '@/components/sidebar/mcp-sidebar';
import { KnowledgeSidebar } from '@/components/sidebar/knowledge-sidebar';
import { HeartbeatSidebar } from '@/components/sidebar/heartbeat-sidebar';
import { CodeSidebar }      from './sidebar-code';
import { SidebarDivider }   from '@/components/sidebar/sidebar-primitives';
import { AppsSidebar }      from '@/components/sidebar/apps-sidebar';
import { TaskCountBadge }   from './task-count-badge';

const sidebarGetToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

export const statusColor: Record<string, string> = {
  idle: 'bg-emerald-500', thinking: 'bg-amber-400 animate-pulse',
  running: 'bg-emerald-400', offline: 'bg-muted-foreground/20', error: 'bg-destructive',
};

/* ─── Main Sidebar — the single "left context slot" (P9 T1.5) ───
 * Exactly one left-aligned context pane per page. New pages pick the
 * right `activeRail` section; content is dispatched below. DO NOT
 * mount page-local <aside> / drawer / sidebar components next to or
 * in place of this component — that was the layout pollution the
 * four-zone rule exists to prevent.
 *
 * To add a new section: add a route → rail id mapping in
 * hooks/use-active-rail.ts, add a case in the switch below, and
 * build its content as a function-component that reads from the
 * store (preferred) or plain props. */
export function Sidebar() {
  const voice = useVoiceEnabled();
  const activeRail = useActiveRail();
  const pathname = usePathname();
  const souls = useStore((s) => s.souls);
  const soulStates = useStore((s) => s.soulStates);
  const liveEvents = useStore((s) => s.liveEvents);
  const [showCreateSoul, setShowCreateSoul] = useState(false);

  // Within the code rail, dispatch sidebar by exact pathname so /tasks, /approvals, etc.
  // show their purpose-built sidebars rather than CodeSidebar.
  const codeSidebarContent = (() => {
    if (pathname?.startsWith('/tasks')) return <TasksSidebar />;
    if (pathname?.startsWith('/approvals')) return <WorkflowsSidebar />;
    if (pathname?.startsWith('/workflows')) return <WorkflowsSidebar />;
    if (pathname?.startsWith('/plans')) return <WorkflowsSidebar />;
    return <CodeSidebar />;
  })();

  return (
    <div
      className="sidebar fixed top-0 bottom-0 z-20 flex flex-col overflow-hidden border-e border-border bg-muted"
      style={{ left: 'var(--rail-width)' }}>
      <div className="w-(--sidebar-default-width) flex flex-col h-full overflow-hidden">
        <SidebarHeader />
        <div className={cn('flex-1 overflow-y-auto', (activeRail as string) !== 'code' && 'pt-1')}>
          {activeRail === 'dashboard' && <HomeSidebar events={liveEvents} />}
          {activeRail === 'souls' && <SoulsSidebar souls={souls} soulStates={soulStates} onNewSoul={() => setShowCreateSoul(true)} />}
          {activeRail === 'sessions' && <MailSidebar />}
          {activeRail === 'live' && <CalendarSidebar />}
          {activeRail === 'drive' && <DriveSidebar />}
          {activeRail === 'connectors' && <ChannelsSidebar />}
          {activeRail === 'workflows' && <WorkflowsSidebar />}
          {(activeRail as string) === 'social' && <SocialSidebar />}
          {(activeRail as string) === 'skills' && <SkillsSidebar />}
          {(activeRail as string) === 'teams' && <TeamsSidebar />}
          {(activeRail as string) === 'mcp' && <McpSidebar />}
          {(activeRail as string) === 'kg' && <KnowledgeSidebar />}
          {(activeRail as string) === 'heartbeat' && <HeartbeatSidebar />}
          {activeRail === 'models' && <ModelsSidebar />}
          {activeRail === 'settings' && <SettingsSidebar />}
          {(activeRail as string) === 'apps' && <AppsSidebar />}
          {(activeRail as string) === 'code' && codeSidebarContent}
        </div>

        <div className="px-2 pb-2">
          <PrimeVoiceWidget />
        </div>
      </div>

      <CreateSoulSheet open={showCreateSoul} onClose={() => setShowCreateSoul(false)} />
    </div>
  );
}

/* ─── Sidebar Header (User profile) ─── */
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
  const role = user?.role ?? '';

  const handleLogout = () => {
    localStorage.removeItem('qorven_token');
    localStorage.removeItem('qorven_user');
    document.cookie = 'qorven_token=; path=/; max-age=0';
    router.push('/login');
  };

  const handleLock = () => {
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
          <DropdownMenuItem onClick={handleLock}>
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

/* ─── Home / Dashboard ─── */
function HomeSidebar({ events }: { events: any[] }) {
  const router = useRouter();
  const pathname = usePathname();
  const [feedOpen, setFeedOpen] = useState(true);

  const navItems = [
    { icon: BarChart3,    label: 'Dashboard',  href: '/' },
    { icon: Sparkles,     label: 'Qors',       href: '/qors' },
    { icon: MessageSquare,label: 'Inbox',      href: '/mail' },
    { icon: CheckSquare,  label: 'Tasks',      href: '/tasks' },
    { icon: ShieldCheck,  label: 'Approvals',  href: '/approvals' },
    { icon: Zap,          label: 'Channels',   href: '/channels' },
    { icon: Cpu,          label: 'Models',     href: '/models-hub' },
    { icon: Settings2,    label: 'Settings',   href: '/settings' },
  ] as const;

  return (
    <>
      <div className="flex flex-col gap-px px-2.5 pt-2">
        {navItems.map(({ icon: Icon, label, href }) => {
          const isActive = pathname === href || (href !== '/' && pathname?.startsWith(href));
          return (
            <button key={href} onClick={() => router.push(href)}
              className={cn('flex w-full items-center gap-2.5 h-8.5 px-2.5 rounded-md text-2sm transition-colors',
                isActive
                  ? 'bg-accent text-foreground font-medium'
                  : 'font-normal text-muted-foreground hover:bg-muted hover:text-foreground')}>
              <Icon className={cn('h-4 w-4 shrink-0', isActive ? 'opacity-80' : 'opacity-50')} />
              <span className="truncate flex-1 text-left">{label}</span>
              {href === '/tasks' && <TaskCountBadge />}
            </button>
          );
        })}
      </div>
      <SidebarDivider />
      <div className="px-2.5">
        <button onClick={() => setFeedOpen(!feedOpen)}
          className="flex w-full items-center gap-1.5 px-2 py-1.5 rounded-md hover:bg-muted transition-colors">
          <ChevronDown className={cn('h-3.5 w-3.5 text-muted-foreground transition-transform shrink-0', !feedOpen && '-rotate-90')} />
          <span className="text-2xs font-medium text-muted-foreground/60 flex-1 text-left uppercase tracking-wider">Live Feed</span>
        </button>
        {feedOpen && (
          <div className="mt-px space-y-px">
            {events.length === 0
              ? <p className="px-2.5 py-4 text-xs text-muted-foreground">No activity yet</p>
              : events.slice(0, 20).map((e, i) => (
                <div key={e.id ?? i} className="rounded-md px-2.5 py-1.5 hover:bg-muted/40 transition-colors">
                  <span className="text-2xs text-muted-foreground">{new Date(e.timestamp).toLocaleTimeString()}</span>
                  {e.soul_key && <span className="text-2xs font-medium ml-1">@{e.soul_key}</span>}
                  <span className="text-2xs text-muted-foreground ml-1">{e.detail ?? e.type}</span>
                </div>
              ))}
          </div>
        )}
      </div>
    </>
  );
}

/* ─── Souls ─── */

const soulIcons: Record<string, string> = {
  chief: '👑', research: '🔬', researcher: '🔬', devops: '⚙️', writer: '✍️',
  content: '📝', data: '📊', analyst: '📊', support: '💬', customer: '💬',
  marketing: '📣', sales: '💰', design: '🎨', security: '🛡️', hr: '👥',
  finance: '💳', legal: '⚖️', product: '🚀', engineering: '🔧', qa: '🧪',
};

function soulIcon(key: string): string {
  const lower = (key || '').toLowerCase();
  for (const [k, v] of Object.entries(soulIcons)) {
    if (lower.includes(k)) return v;
  }
  const icons = ['🤖', '🧠', '⚡', '🎯', '🔮', '💡', '🌟', '🎪'];
  let hash = 0;
  for (let i = 0; i < lower.length; i++) hash = ((hash << 5) - hash + lower.charCodeAt(i)) | 0;
  return icons[Math.abs(hash) % icons.length]!;
}

function SoulsSidebar({ souls, soulStates, onNewSoul }: { souls: any[]; soulStates: Record<string, any>; onNewSoul: () => void }) {
  const router = useRouter();
  const pathname = usePathname();
  const [rooms, setHubs] = useState<any[]>([]);
  const [addOpen, setAddOpen] = useState(false);
  const [soulsOpen, setSoulsOpen] = useState(true);
  const [roomsOpen, setHubsOpen] = useState(true);
  const [search, setSearch] = useState('');
  const [sessionResults, setSessionResults] = useState<any[]>([]);
  useEffect(() => { roomsApi?.list?.().then(setHubs).catch(() => {}); }, []);

  useEffect(() => {
    if (search.length < 3) { setSessionResults([]); return; }
    const timer = setTimeout(() => {
      fetch(`/api/v1/sessions/search?q=${encodeURIComponent(search)}`, {
        headers: { Authorization: `Bearer ${sidebarGetToken()}` }
      }).then(r => r.json()).then(d => setSessionResults(d.sessions || [])).catch(() => {});
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);

  const filtered = search ? souls.filter((s) => s.display_name?.toLowerCase().includes(search.toLowerCase())) : souls;

  return (
    <>
      <div className="relative flex items-center gap-1 px-3 pt-4 pb-2">
        <div className="flex flex-1 items-center h-8 rounded-md border border-input bg-transparent px-2.5 text-2sm min-w-0">
          <Search className="h-3.5 w-3.5 text-muted-foreground mr-1.5 shrink-0" />
          <input type="text" placeholder="Search" value={search} onChange={(e) => setSearch(e.target.value)}
            className="flex-1 bg-transparent text-2sm text-foreground placeholder:text-muted-foreground outline-none min-w-0" />
        </div>
        <button onClick={() => setAddOpen(!addOpen)}
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground hover:bg-primary/90">
          <Plus className="h-4 w-4" />
        </button>
        {addOpen && (
          <div className="fixed z-[100] w-44 rounded-lg border border-border bg-popover shadow-lg py-1" style={{ left: 'calc(var(--rail-width) + var(--sidebar-default-width) + 4px)', top: 'calc(var(--header-height) + 8px)' }}>
            <button onClick={() => { onNewSoul(); setAddOpen(false); }}
              className="flex w-full items-center gap-2.5 px-3 py-2 text-2sm hover:bg-accent">
              <Sparkles className="h-4 w-4 text-muted-foreground" />Soul
            </button>
            <button onClick={() => { router.push('/rooms/new'); setAddOpen(false); }}
              className="flex w-full items-center gap-2.5 px-3 py-2 text-2sm hover:bg-accent">
              <Users className="h-4 w-4 text-muted-foreground" />Hub
            </button>
          </div>
        )}
      </div>

      <SidebarDivider />

      <div className="px-2.5">
        <button onClick={() => setSoulsOpen(!soulsOpen)}
          className="flex w-full items-center gap-1.5 px-2 py-1.5 rounded-md hover:bg-muted transition-colors">
          <ChevronDown className={cn('h-3.5 w-3.5 text-muted-foreground transition-transform shrink-0', !soulsOpen && '-rotate-90')} />
          <span className="text-2xs font-medium text-muted-foreground/60 flex-1 text-left uppercase tracking-wider">Qors ({souls.length})</span>
        </button>
        {soulsOpen && (
          <div className="mt-px max-h-[55vh] overflow-y-auto flex flex-col gap-px">
            {filtered.map((soul) => {
              const isActive = pathname?.startsWith(`/qors/${soul.id}`);
              return (
                <button key={soul.id} onClick={() => router.push(`/qors/${soul.id}`)}
                  className={cn('group/qor flex w-full items-center gap-2 h-8.5 rounded-md px-2.5 text-2sm text-left transition-colors',
                    isActive ? 'bg-accent text-foreground font-medium' : 'font-normal text-muted-foreground hover:bg-muted hover:text-foreground')}>
                  <span className={cn('shrink-0', isActive ? 'opacity-80' : 'opacity-50')}>{soulIcon(soul.agent_key || soul.display_name)}</span>
                  <span className="truncate flex-1">{soul.display_name}</span>
                  <Pin className="h-3 w-3 shrink-0 ml-auto text-muted-foreground/60 opacity-0 group-hover/qor:opacity-100 transition-opacity" />
                </button>
              );
            })}
            {filtered.length === 0 && <p className="px-2.5 py-3 text-2sm text-muted-foreground">No matches</p>}
          </div>
        )}
      </div>

      {rooms.length > 0 && (
        <>
          <SidebarDivider />
          <div className="px-2.5">
            <button onClick={() => setHubsOpen(!roomsOpen)}
              className="flex w-full items-center gap-1.5 px-2 py-1.5 rounded-md hover:bg-muted transition-colors">
              <ChevronDown className={cn('h-3.5 w-3.5 text-muted-foreground transition-transform shrink-0', !roomsOpen && '-rotate-90')} />
              <span className="text-2xs font-medium text-muted-foreground/60 flex-1 text-left uppercase tracking-wider">Hubs ({rooms.length})</span>
            </button>
            {roomsOpen && (
              <div className="mt-px flex flex-col gap-px max-h-[30vh] overflow-y-auto">
                {rooms.map((r: any) => (
                  <button key={r.id} onClick={() => router.push(`/rooms/${r.id}`)}
                    className="flex w-full items-center gap-2.5 h-8.5 px-2.5 rounded-md text-2sm font-normal transition-colors text-muted-foreground hover:bg-muted hover:text-foreground">
                    <Users className="h-4 w-4 shrink-0 opacity-50" />
                    <span className="truncate flex-1 text-left">{r.name || `Room ${r.id.slice(0, 6)}`}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        </>
      )}
    </>
  );
}
