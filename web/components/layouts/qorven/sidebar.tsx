'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { CreateSoulSheet } from '@/components/forms/create-soul-sheet';
import {
  ChevronDown, Plus, Settings2, Key, Palette, LogOut, BarChart3,
  MessageSquare, CheckSquare, Search, Sparkles, Lock, User,
  ShieldCheck, Zap, Cpu, Hash, ArrowLeft,
} from 'lucide-react';
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem,
  DropdownMenuSeparator, DropdownMenuTrigger,
} from '@/components/qor/dropdown-menu';
import { useActiveRail } from '@/hooks/use-active-rail';
import { rooms as roomsApi, sessions as sessionsApi } from '@/lib/api';
import { SoulPulseRing } from '@/components/soul-pulse-ring';
import { SidebarAgentRow } from './sidebar-agent-row';
import { SidebarHubRow, type HubMember } from './sidebar-hub-row';

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
import { RoomsSidebar }     from '@/components/sidebar/rooms-sidebar';
import { TaskCountBadge }   from './task-count-badge';

export const statusColor: Record<string, string> = {
  idle: 'bg-emerald-500', thinking: 'bg-amber-400 animate-pulse',
  running: 'bg-emerald-400', offline: 'bg-muted-foreground/20', error: 'bg-destructive',
};

/* ─── Main Sidebar ─────────────────────────────────────────────────────────── */
export function Sidebar() {
  const activeRail = useActiveRail();
  const pathname = usePathname();
  const souls = useStore((s) => s.souls);
  const soulStates = useStore((s) => s.soulStates);
  const liveEvents = useStore((s) => s.liveEvents);
  const [showCreateSoul, setShowCreateSoul] = useState(false);

  const codeSidebarContent = (() => {
    if (pathname?.startsWith('/tasks')) return <TasksSidebar />;
    if (pathname?.startsWith('/approvals')) return <WorkflowsSidebar />;
    if (pathname?.startsWith('/workflows')) return <WorkflowsSidebar />;
    if (pathname?.startsWith('/plans')) return <WorkflowsSidebar />;
    return <CodeSidebar />;
  })();

  // When inside a specific agent page, show agent detail profile
  const agentDetailMatch = pathname?.match(/^\/qors\/([^/]+)/);
  const agentDetailId = agentDetailMatch?.[1];
  const detailSoul = agentDetailId
    ? souls.find((s) => s.id === agentDetailId)
    : null;

  return (
    <div
      className="sidebar fixed top-0 bottom-0 z-20 flex flex-col overflow-hidden border-e border-border bg-muted"
      style={{ left: 'var(--rail-width)' }}
    >
      <div className="w-(--sidebar-default-width) flex flex-col h-full overflow-hidden">
        <SidebarHeader />
        {/* pb accounts for the agent pill height */}
        <div className="flex-1 overflow-y-auto pb-[56px]">
          {activeRail === 'dashboard' && <HomeSidebar events={liveEvents} />}
          {activeRail === 'souls' && (
            detailSoul
              ? <AgentDetailSidebar soul={detailSoul} soulState={soulStates[detailSoul.id]} />
              : <SoulsSidebar souls={souls} soulStates={soulStates} onNewSoul={() => setShowCreateSoul(true)} />
          )}
          {activeRail === 'sessions' && <MailSidebar />}
          {activeRail === 'live' && <CalendarSidebar />}
          {activeRail === 'drive' && <DriveSidebar />}
          {activeRail === 'connectors' && <ChannelsSidebar />}
          {(activeRail as string) === 'rooms' && <RoomsSidebar />}
          {(activeRail as string) === 'social' && <SocialSidebar />}
          {(activeRail as string) === 'skills' && <SkillsSidebar />}
          {(activeRail as string) === 'teams' && <TeamsSidebar />}
          {(activeRail as string) === 'org-chart' && <TeamsSidebar />}
          {(activeRail as string) === 'mcp' && <McpSidebar />}
          {(activeRail as string) === 'kg' && <KnowledgeSidebar />}
          {(activeRail as string) === 'heartbeat' && <HeartbeatSidebar />}
          {activeRail === 'models' && <ModelsSidebar />}
          {activeRail === 'settings' && <SettingsSidebar />}
          {(activeRail as string) === 'apps' && <AppsSidebar />}
          {(activeRail as string) === 'code' && codeSidebarContent}
        </div>
        {/* Voice widget removed — voice is now per-agent in SoulsSidebar */}
      </div>
      <CreateSoulSheet open={showCreateSoul} onClose={() => setShowCreateSoul(false)} />
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

/* ─── Home / Dashboard Sidebar ─────────────────────────────────────────────── */
function HomeSidebar({ events }: { events: any[] }) {
  const router = useRouter();
  const pathname = usePathname();
  const [feedOpen, setFeedOpen] = useState(true);

  const navItems = [
    { icon: BarChart3,     label: 'Dashboard', href: '/' },
    { icon: Sparkles,      label: 'Agents',    href: '/qors' },
    { icon: MessageSquare, label: 'Inbox',     href: '/mail' },
    { icon: CheckSquare,   label: 'Tasks',     href: '/tasks' },
    { icon: ShieldCheck,   label: 'Approvals', href: '/approvals' },
    { icon: Zap,           label: 'Channels',  href: '/channels' },
    { icon: Cpu,           label: 'Models',    href: '/models-hub' },
    { icon: Settings2,     label: 'Settings',  href: '/settings' },
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

/* ─── Agents List Sidebar ───────────────────────────────────────────────────── */
function SoulsSidebar({
  souls,
  soulStates,
  onNewSoul,
}: {
  souls: any[];
  soulStates: Record<string, any>;
  onNewSoul: () => void;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const [hubs, setHubs] = useState<any[]>([]);
  const [hubMembers, setHubMembers] = useState<Record<string, HubMember[]>>({});
  const [addOpen, setAddOpen] = useState(false);
  const [soulsOpen, setSoulsOpen] = useState(true);
  const [hubsOpen, setHubsOpen] = useState(true);
  const [search, setSearch] = useState('');

  useEffect(() => {
    roomsApi.list().then((d: any) => {
      const list: any[] = Array.isArray(d?.rooms) ? d.rooms : Array.isArray(d) ? d : [];
      setHubs(list);
      list.forEach((h: any) => {
        roomsApi.org(h.id).then((org: any) => {
          const members: HubMember[] = (org?.members ?? []).map((m: any) => ({
            id: m.id ?? m.agent_id,
            display_name: m.display_name ?? m.agent_key ?? 'Agent',
            avatar: m.avatar,
          }));
          setHubMembers((prev) => ({ ...prev, [h.id]: members }));
        }).catch(() => {});
      });
    }).catch(() => {});
  }, []);

  const filtered = search
    ? souls.filter((s: any) =>
        (s.display_name ?? '').toLowerCase().includes(search.toLowerCase()))
    : souls;

  return (
    <>
      <div className="relative flex h-[44px] shrink-0 items-center gap-1 border-b border-border px-2">
        <div className="flex flex-1 items-center h-8 rounded-md border border-input bg-transparent px-2.5 text-2sm min-w-0">
          <Search className="h-3.5 w-3.5 text-muted-foreground mr-1.5 shrink-0" />
          <input
            type="text"
            placeholder="Search agents…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="flex-1 bg-transparent text-2sm text-foreground placeholder:text-muted-foreground outline-none min-w-0"
          />
        </div>
        <button
          onClick={() => setAddOpen(!addOpen)}
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground hover:bg-primary/90"
        >
          <Plus className="h-4 w-4" />
        </button>
        {addOpen && (
          <div
            className="fixed z-[100] w-44 rounded-lg border border-border bg-popover shadow-lg py-1"
            style={{
              left: 'calc(var(--rail-width) + var(--sidebar-default-width) + 4px)',
              top: 'calc(var(--header-height) + 8px)',
            }}
          >
            <button
              onClick={() => { onNewSoul(); setAddOpen(false); }}
              className="flex w-full items-center gap-2.5 px-3 py-2 text-2sm hover:bg-accent"
            >
              <Sparkles className="h-4 w-4 text-muted-foreground" />New Agent
            </button>
            <button
              onClick={() => { router.push('/rooms'); setAddOpen(false); }}
              className="flex w-full items-center gap-2.5 px-3 py-2 text-2sm hover:bg-accent"
            >
              <Hash className="h-4 w-4 text-muted-foreground" />New Hub
            </button>
          </div>
        )}
      </div>

      <div className="px-1.5 pt-2">
        <button
          onClick={() => setSoulsOpen(!soulsOpen)}
          className="flex w-full items-center gap-1.5 px-2 py-1 rounded-md hover:bg-muted transition-colors mb-0.5"
        >
          <ChevronDown className={cn('h-3.5 w-3.5 text-muted-foreground transition-transform shrink-0', !soulsOpen && '-rotate-90')} />
          <span className="text-[10px] font-semibold text-muted-foreground/60 flex-1 text-left uppercase tracking-wider">
            Agents {souls.length > 0 ? `· ${souls.length}` : ''}
          </span>
        </button>
        {soulsOpen && (
          <div className="flex flex-col gap-0.5 max-h-[55vh] overflow-y-auto">
            {filtered.map((soul: any) => {
              const state = soulStates[soul.id];
              return (
                <SidebarAgentRow
                  key={soul.id}
                  soul={soul}
                  activity={state?.activity ?? 'offline'}
                  lastEvent={state?.lastEvent}
                  isActive={pathname?.startsWith(`/qors/${soul.id}`) ?? false}
                  onClick={() => router.push(`/qors/${soul.id}`)}
                />
              );
            })}
            {filtered.length === 0 && (
              <p className="px-2.5 py-3 text-[12px] text-muted-foreground">No agents found</p>
            )}
          </div>
        )}
      </div>

      {hubs.length > 0 && (
        <>
          <SidebarDivider />
          <div className="px-1.5">
            <div className="flex items-center gap-1 px-2 py-1 mb-0.5">
              <button
                onClick={() => setHubsOpen(!hubsOpen)}
                className="flex flex-1 items-center gap-1.5 rounded-md hover:bg-muted transition-colors"
              >
                <ChevronDown className={cn('h-3.5 w-3.5 text-muted-foreground transition-transform shrink-0', !hubsOpen && '-rotate-90')} />
                <span className="text-[10px] font-semibold text-muted-foreground/60 flex-1 text-left uppercase tracking-wider">
                  Hubs · {hubs.length}
                </span>
              </button>
              <button
                onClick={() => router.push('/rooms')}
                className="flex h-5 w-5 items-center justify-center rounded text-muted-foreground/50 hover:text-muted-foreground hover:bg-muted"
                title="Go to Hubs"
              >
                <Hash className="h-3 w-3" />
              </button>
            </div>
            {hubsOpen && (
              <div className="flex flex-col gap-0.5 max-h-[30vh] overflow-y-auto">
                {hubs.map((h: any) => (
                  <SidebarHubRow
                    key={h.id}
                    id={h.id}
                    name={h.name}
                    displayName={h.display_name}
                    members={hubMembers[h.id] ?? []}
                    messageCount={h.message_count}
                    isActive={pathname?.startsWith(`/rooms/${h.id}`) ?? false}
                    onClick={() => router.push(`/rooms/${h.id}`)}
                  />
                ))}
              </div>
            )}
          </div>
        </>
      )}
    </>
  );
}

/* ─── Agent Detail Sidebar (shown when inside /qors/:id) ────────────────────── */
function AgentDetailSidebar({
  soul,
  soulState,
}: {
  soul: any;
  soulState?: { activity: string; lastEvent?: string; tokensToday: number };
}) {
  const router = useRouter();
  const [sessions, setSessions] = useState<any[]>([]);
  const [tab, setTab] = useState<'sessions' | 'tasks' | 'channels'>('sessions');

  useEffect(() => {
    sessionsApi.listByAgent(soul.id).then((d: any) => {
      setSessions(Array.isArray(d?.sessions) ? d.sessions.slice(0, 5) : []);
    }).catch(() => {});
  }, [soul.id]);

  const activity = (soulState?.activity ?? 'offline') as any;

  const GRADIENTS = [
    'from-primary to-primary/80', 'from-emerald-500 to-teal-600',
    'from-orange-500 to-red-600', 'from-pink-500 to-rose-600',
    'from-cyan-500 to-blue-600', 'from-amber-500 to-yellow-600',
    'from-fuchsia-500 to-purple-600', 'from-lime-500 to-green-600',
  ];
  function gradientFor(id: string) {
    let hash = 0;
    for (let i = 0; i < id.length; i++) hash = id.charCodeAt(i) + ((hash << 5) - hash);
    return GRADIENTS[Math.abs(hash) % GRADIENTS.length]!;
  }

  return (
    <div className="flex flex-col overflow-hidden">
      <button
        onClick={() => router.push('/qors')}
        className="flex items-center gap-1.5 px-3 py-2.5 text-[12px] text-muted-foreground hover:text-foreground transition-colors border-b border-border"
      >
        <ArrowLeft className="h-3.5 w-3.5" />
        All Agents
      </button>

      <div className="flex flex-col items-center gap-2 px-3 pt-5 pb-4 border-b border-border">
        <div className="relative">
          {soul.avatar ? (
            <img src={soul.avatar} alt={soul.display_name}
              className="h-14 w-14 rounded-full object-cover ring-2 ring-border" />
          ) : (
            <div className={cn(
              'flex h-14 w-14 items-center justify-center rounded-full bg-gradient-to-br text-lg font-bold text-white ring-2 ring-border',
              gradientFor(soul.id)
            )}>
              {(soul.display_name?.[0] ?? '?').toUpperCase()}
            </div>
          )}
          <span className="absolute -bottom-0.5 -right-0.5">
            <SoulPulseRing activity={activity} size="md" />
          </span>
        </div>

        <div className="text-center">
          <p className="text-[14px] font-semibold leading-tight">{soul.display_name}</p>
          <p className="text-[11px] text-muted-foreground mt-0.5">
            {soul.title || soul.role || soul.org_role || 'Agent'}
          </p>
          {soulState?.lastEvent && (
            <p className="text-[11px] text-amber-400/80 mt-1 line-clamp-2">
              {soulState.lastEvent}
            </p>
          )}
        </div>

        <div className="flex gap-4 mt-1">
          <div className="text-center">
            <p className="text-[13px] font-semibold">{soulState?.tokensToday?.toLocaleString() ?? '0'}</p>
            <p className="text-[10px] text-muted-foreground">tokens today</p>
          </div>
          <div className="text-center">
            <p className="text-[13px] font-semibold">{sessions.length}</p>
            <p className="text-[10px] text-muted-foreground">recent chats</p>
          </div>
        </div>
      </div>

      <div className="flex border-b border-border">
        {(['sessions', 'tasks', 'channels'] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={cn(
              'flex-1 py-2 text-[11px] font-medium transition-colors capitalize',
              tab === t
                ? 'text-foreground border-b-2 border-primary'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {t}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto px-1.5 py-1">
        {tab === 'sessions' && (
          <div className="flex flex-col gap-0.5">
            {sessions.length === 0 ? (
              <p className="px-2 py-4 text-[12px] text-muted-foreground text-center">No recent conversations</p>
            ) : (
              sessions.map((s: any) => (
                <button
                  key={s.id}
                  onClick={() => router.push(`/qors/${soul.id}?session=${s.id}`)}
                  className="flex flex-col items-start rounded-lg px-2.5 py-2 text-left hover:bg-muted transition-colors w-full"
                >
                  <span className="text-[12px] font-medium truncate w-full">
                    {s.title || s.summary || 'Conversation'}
                  </span>
                  <span className="text-[10px] text-muted-foreground">
                    {s.created_at ? new Date(s.created_at).toLocaleDateString() : ''}
                  </span>
                </button>
              ))
            )}
          </div>
        )}
        {tab === 'tasks' && (
          <p className="px-2 py-4 text-[12px] text-muted-foreground text-center">
            Tasks shown in the Tasks view
          </p>
        )}
        {tab === 'channels' && (
          <p className="px-2 py-4 text-[12px] text-muted-foreground text-center">
            Channels shown in the Channels view
          </p>
        )}
      </div>
    </div>
  );
}
