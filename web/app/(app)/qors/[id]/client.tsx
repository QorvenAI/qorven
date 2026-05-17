'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import React, { useEffect, useState, useCallback, useMemo, useRef } from 'react';
import { brand } from '@/lib/branding';
import { toast } from 'sonner';
import { useParams, useSearchParams } from 'next/navigation';
import { agents, sessions as sessionsApi, channels as channelsApi, tools as toolsApi } from '@/lib/api';
import { approvals as approvalsApi, permissions as permissionsApi, type ApprovalItem } from '@/lib/api';

type RuntimeState = 'idle' | 'working' | 'suspended';

type LiveTask = {
  id: string;
  title: string;
  status: string;
  iteration: number;
  scratchpad: string;
  last_tool?: string;
  updated_at: string;
};
import { DollarSign, ShieldAlert, ShieldCheck, ThumbsUp, ThumbsDown } from 'lucide-react';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { soulGradient } from '@/components/soul-card';
import { ChatPlayground } from '@/components/chat-v2/chat-playground';
import { SoulSettingsPage } from '@/components/soul-settings-page';
import { MailTab } from '@/components/qors/mail-tab';
import { PermissionsTab } from '@/components/qors/permissions-tab';
import { SchedulesTab } from '@/components/qors/schedules-tab';
import { ErrorBoundary } from '@/components/error-boundary';
import { useAppAgentTabs } from '@/components/apps/app-registry-context';
import { request as apiRequest, getToken } from '@/lib/api-core';
import { useSelectedModels } from '@/hooks/use-selected-models';
import { SearchableSelect } from '@/components/searchable-select';
import { useToolbarContent } from '@/hooks/use-toolbar-content';
import {
  ProfileSkillsTab, ProfileMemoryTab, ProfileBackgroundTab,
  ProfileMetricsTab,
} from '@/components/qor-profile-tabs';
import {
  Plus, MessageSquare, Trash2, Download, ChevronRight,
  Save, ToggleLeft, ToggleRight, Search, Globe, Brain, Wrench,
  Loader2, Check, AlertCircle, X, Hash,
  Settings2, Sparkles, Activity, BarChart3, Radio, Zap, Package, Mail, CalendarClock,
} from 'lucide-react';
import type { Soul, Session, Channel, Message } from '@/types';

const workspaceTabs = [
  { id: 'chat',       icon: MessageSquare, label: 'Chat' },
  { id: 'config',     icon: Settings2,     label: 'Config' },
  { id: 'skills',     icon: Sparkles,      label: 'Skills' },
  { id: 'memory',     icon: Brain,         label: 'Memory' },
  { id: 'background', icon: Activity,      label: 'Background' },
  { id: 'metrics',    icon: BarChart3,     label: 'Metrics' },
  { id: 'channels',   icon: Radio,         label: 'Channels' },
  { id: 'mail',       icon: Mail,          label: 'Mail' },
  { id: 'tools',        icon: Wrench,        label: 'Tools' },
  { id: 'permissions',  icon: ShieldCheck,   label: 'Permissions' },
  { id: 'schedules',    icon: CalendarClock, label: 'Schedules' },
  { id: 'settings',     icon: Settings2,     label: 'Settings' },
] as const;

// One-Qor-one-chat: the 'sessions' tab and SessionSidebar are gone.
// The Qor always opens on its single canonical chat. Long-running
// context flows through the 'memory' tab instead.
type TabId =
  | 'chat' | 'config' | 'channels' | 'mail' | 'tools' | 'permissions' | 'schedules' | 'settings'
  | 'skills' | 'memory' | 'background' | 'metrics';

function TaskCard({ task }: { task: LiveTask }) {
  const statusColor = task.status === 'done'
    ? 'border-emerald-500/30 bg-emerald-500/5'
    : task.status === 'blocked'
    ? 'border-amber-500/30 bg-amber-500/5'
    : 'border-blue-500/30 bg-blue-500/5';
  const icon = task.status === 'done' ? '✓' : task.status === 'blocked' ? '⚠' : '⚡';
  const lastLine = task.scratchpad?.split('\n').filter(Boolean).pop() ?? '';
  return (
    <div className={cn('mx-4 my-2 rounded-xl border px-4 py-3 text-sm', statusColor)}>
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium">{icon} {task.title}</span>
        {task.status === 'in_progress' && (
          <span className="text-xs text-muted-foreground tabular-nums">Iteration {task.iteration}</span>
        )}
      </div>
      {lastLine && <p className="mt-1 text-xs text-muted-foreground/80 truncate">▸ {lastLine}</p>}
      <a href="/tasks" className="mt-1.5 block text-xs text-primary/70 hover:text-primary transition-colors">
        View in Tasks →
      </a>
    </div>
  );
}

export default function QorDetailPage() {
  const params = useParams<{ id: string }>();
  // Static export: generateStaticParams emits '__dynamic__' as the placeholder.
  // useParams returns that placeholder during SSR/hydration; fall back to the
  // real UUID from window.location so the data fetch fires immediately.
  const id = (params.id === '__dynamic__' || !params.id)
    ? (typeof window !== 'undefined'
        ? window.location.pathname.split('/').filter(Boolean).pop() ?? params.id
        : params.id)
    : params.id;
  const searchParams = useSearchParams();
  const [soul, setSoul] = useState<Soul | null>(null);
  const [allSessions, setAllSessions] = useState<Session[]>([]);
  const [activeSession, setActiveSession] = useState<Session | null>(null);
  const [soulChannels, setSoulChannels] = useState<Channel[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [pendingApprovals, setPendingApprovals] = useState<ApprovalItem[]>([]);
  const [decidingId, setDecidingId] = useState<string | null>(null);
  // Live approvals from WS hub (permission.requested events keyed by request_id)
  const storeApprovals = useStore((s) => s.approvals);

  const [runtimeState, setRuntimeState] = useState<RuntimeState | null>(null);
  const [liveTasks, setLiveTasks] = useState<Record<string, LiveTask>>({});

  const activeTab = useStore((s) => s.workspaceTab) as TabId | string;
  const setWorkspaceTab = useStore((s) => s.setWorkspaceTab);
  const appAgentTabs = useAppAgentTabs();
  const setActiveChat = useStore((s) => s.setActiveChat);
  const setSessionStore = useStore((s) => s.setSession);
  const setLiveTaskStore = useStore((s) => s.setLiveTask);
  const clearLiveTasksStore = useStore((s) => s.clearLiveTasks);
  const setLiveTaskAgentIdStore = useStore((s) => s.setLiveTaskAgentId);
  const openRightPanel = useStore((s) => s.openRightPanel);

  // Set initial tab from URL param
  useEffect(() => {
    const tab = searchParams?.get('tab');
    if (tab) setWorkspaceTab(tab);
    else setWorkspaceTab('chat');
  }, [searchParams, setWorkspaceTab]);

  // Subscribe to runtime events from the WS hub via window CustomEvents.
  useEffect(() => {
    if (!id) return;
    const onStateChanged = (e: Event) => {
      const d = (e as CustomEvent).detail as Record<string, string>;
      if (d?.agent_id === id) setRuntimeState(d.state as RuntimeState);
    };
    window.addEventListener('qorven:runtime_state_changed', onStateChanged);
    return () => {
      window.removeEventListener('qorven:runtime_state_changed', onStateChanged);
    };
  }, [id]);

  // Live task cards — WebSocket subscription for task events
  useEffect(() => {
    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const ws = new WebSocket(`${proto}://${window.location.host}/ws/realtime`);
    ws.onmessage = (e) => {
      try {
        const evt = JSON.parse(e.data);
        if (['task_iteration_start','task_tool_call','task_progress','task_done','task_blocked'].includes(evt.type)) {
          const d = evt.data ?? {};
          const taskID = d.task_id;
          if (!taskID) return;
          setLiveTasks(prev => {
            const current: LiveTask = prev[taskID] ?? { id: taskID, title: d.title ?? '', status: 'in_progress', iteration: 0, scratchpad: '', last_tool: undefined, updated_at: new Date().toISOString() };
            const updated: LiveTask = {
              ...current,
              status: evt.type === 'task_done' ? 'done' : evt.type === 'task_blocked' ? 'blocked' : 'in_progress',
              iteration: d.iteration ?? current.iteration,
              scratchpad: d.scratchpad ?? current.scratchpad,
              last_tool: d.tool ?? current.last_tool,
              updated_at: new Date().toISOString(),
            };
            setLiveTaskStore(updated);
            return { ...prev, [taskID]: updated };
          });
        }
      } catch {}
    };
    return () => {
      ws.close();
      clearLiveTasksStore();
    };
    // setLiveTasks is a stable React setState ref — intentionally omitted from deps
  }, [setLiveTaskStore, clearLiveTasksStore]);

  // Register workspace tabs in the toolbar
  const tabBar = useMemo(() => (
    <nav className="flex items-stretch self-stretch gap-0 -mx-1">
      {workspaceTabs.map((tab) => {
        const Icon = tab.icon;
        return (
          <button key={tab.id} onClick={() => setWorkspaceTab(tab.id)}
            className={cn('flex items-center gap-1.5 px-3 text-xs font-medium border-b-2 whitespace-nowrap transition-colors h-full',
              activeTab === tab.id
                ? 'border-primary text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground')}>
            <Icon className="h-3.5 w-3.5" />
            {tab.label}
          </button>
        );
      })}
      {appAgentTabs.map((tab) => (
        <button key={tab.id} onClick={() => setWorkspaceTab(tab.id)}
          className={cn('flex items-center gap-1.5 px-3 text-xs font-medium border-b-2 whitespace-nowrap transition-colors h-full',
            activeTab === tab.id
              ? 'border-primary text-foreground'
              : 'border-transparent text-muted-foreground hover:text-foreground')}>
          <Package className="h-3.5 w-3.5" />
          {tab.label}
        </button>
      ))}
    </nav>
  ), [activeTab, setWorkspaceTab, appAgentTabs]);
  const activeLiveTaskCount = useMemo(
    () => Object.values(liveTasks).filter(t => t.status === 'in_progress').length,
    [liveTasks]
  );

  const orbNode = useMemo(() => {
    if (soul?.runtime_mode !== 'persistent') return null;
    return (
      <StateOrb
        state={runtimeState}
        taskCount={activeLiveTaskCount}
        onOpen={() => openRightPanel('tasks')}
      />
    );
  }, [soul?.runtime_mode, runtimeState, activeLiveTaskCount, openRightPanel]);

  useToolbarContent(tabBar, orbNode);

  // One-Qor-one-chat: the Qor page always resolves to its single
  // canonical chat. The backend's POST /v1/sessions is idempotent for
  // chat-family channels (web, tui, telegram, whatsapp, slack_dm,
  // discord_dm) — it returns the existing active session if one
  // already exists, otherwise mints a new one.
  const loadCanonicalSession = useCallback(async (agentID: string) => {
    const sess = await sessionsApi.create({ agent_id: agentID, channel: 'web' });
    const full = await sessionsApi.get(sess.id);
    return full;
  }, []);

  useEffect(() => {
    if (!id || id === '__dynamic__') return;
    Promise.all([
      agents.get(id),
      channelsApi.list().then((chs) => chs.filter((c: Channel) => c.agent_id === id)),
    ]).then(async ([soulData, chs]) => {
      setSoul(soulData);
      setActiveChat(soulData.id, 'soul');
      setLiveTaskAgentIdStore(soulData.id);

      // Redirect system agents
      if (soulData.agent_key?.startsWith('__') || soulData.model === 'system') {
        window.location.href = '/qors';
        return;
      }

      try {
        const full = await loadCanonicalSession(soulData.id);
        setActiveSession(full);
        setSessionStore(full);
        setAllSessions([full]); // kept for components that still accept a list prop
      } catch {
        // non-fatal; user sees empty-state and can retry
      }

      setSoulChannels(chs);
      setLoading(false);
    }).catch(async (e) => {
      // Agent not found — redirect to the first available agent rather than
      // showing a dead error page. This handles stale UUIDs from old DB state.
      try {
        const list = await agents.list();
        const valid = (Array.isArray(list) ? list : []).filter((a: any) => !a.agent_key?.startsWith('__'));
        if (valid.length > 0 && valid[0]) { window.location.replace(`/qors/${valid[0].id}`); return; }
      } catch { /* fall through to error */ }
      setLoadError(e instanceof Error ? e.message : 'Failed to load agent');
      setLoading(false);
    });
    return () => {
      setLiveTaskAgentIdStore(null);
    };
  }, [id, setActiveChat, setSessionStore, loadCanonicalSession, setLiveTaskAgentIdStore]);

  // One-time REST fetch for approvals that landed before this page opened.
  // Ongoing updates come from the WS store (no polling needed).
  const refreshApprovals = useCallback(async () => {
    if (!id) return;
    try {
      const all = await approvalsApi.list();
      setPendingApprovals(all.filter((a) => a.agent_id === id && a.status === 'pending'));
    } catch { /* non-fatal */ }
  }, [id]);

  useEffect(() => {
    if (!loading && soul) refreshApprovals();
  }, [loading, soul, refreshApprovals]);

  // Merge WS-pushed permission.requested events into the banner.
  // These use agent_key (not agent_id), so match against soul.agent_key too.
  useEffect(() => {
    if (!soul) return;
    const wsItems: ApprovalItem[] = Object.values(storeApprovals)
      .filter((e) => e.resolved === null && (e.request.agent_key === soul.agent_key || e.request.session_id))
      .map((e) => ({
        id: e.request.request_id,
        kind: 'tool',
        state: 'pending',
        status: 'pending',
        agent_id: id,
        tool_name: e.request.tool,
        tool_args: e.request.args,
        reason: e.request.reason,
      } as ApprovalItem));
    if (wsItems.length > 0) {
      setPendingApprovals((prev) => {
        const existingIds = new Set(prev.map((a) => a.id));
        const newOnes = wsItems.filter((w) => !existingIds.has(w.id));
        return newOnes.length ? [...prev, ...newOnes] : prev;
      });
    }
    // Remove resolved items from the banner
    setPendingApprovals((prev) =>
      prev.filter((a) => {
        const wsEntry = storeApprovals[a.id];
        return !wsEntry || wsEntry.resolved === null;
      })
    );
  }, [storeApprovals, soul, id]);

  const handleDecide = useCallback(async (approvalId: string, decision: 'approve' | 'reject') => {
    setDecidingId(approvalId);
    try {
      // WS-sourced permission requests use the permissions API; REST approvals use approvals API
      if (storeApprovals[approvalId]) {
        await permissionsApi.reply(approvalId, { decision: decision === 'approve' ? 'allow' : 'deny' });
      } else {
        await approvalsApi.decide(approvalId, decision);
      }
      // Optimistically remove from banner; WS will confirm resolution
      setPendingApprovals((prev) => prev.filter((a) => a.id !== approvalId));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Decision failed');
    } finally {
      setDecidingId(null);
    }
  }, [storeApprovals]);

  if (loading) {
    return (
      <div className="full-bleed flex h-full items-center justify-center" style={{ minHeight: 'calc(100vh - var(--header-height) - var(--toolbar-height))' }}>
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (loadError || !soul) {
    return (
      <div className="full-bleed flex h-full flex-col items-center justify-center gap-3" style={{ minHeight: 'calc(100vh - var(--header-height) - var(--toolbar-height))' }}>
        <AlertCircle className="h-8 w-8 text-destructive" />
        <p className="text-sm text-destructive">{loadError || 'Agent not found'}</p>
        <button onClick={() => window.location.reload()} className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90">Retry</button>
      </div>
    );
  }

  return (
    <ErrorBoundary fallbackTitle={`${brand.agentName} workspace failed`}>
      <div className="full-bleed flex flex-col" style={{ height: 'calc(100vh - var(--header-height) - var(--toolbar-height) - var(--status-bar-height, 24px))' }}>
        <div className="flex-1 overflow-hidden">
          {/* Chat tab — one Qor, one chat. No session sidebar; the
              Qor's canonical chat fills the area. Long-running context
              flows through the Memory tab. */}
          {activeTab === 'chat' && (
            <div className="flex h-full flex-col">
              {/* Pending approvals banner */}
              {pendingApprovals.length > 0 && (
                <div className="shrink-0 border-b border-amber-500/30 bg-amber-500/8">
                  {pendingApprovals.map((a) => (
                    <div key={a.id} className="flex items-start gap-3 px-4 py-2.5">
                      <ShieldAlert className="h-4 w-4 text-amber-500 mt-0.5 shrink-0" />
                      <div className="flex-1 min-w-0">
                        <p className="text-xs font-medium text-amber-600">
                          Waiting for approval — <span className="font-mono">{a.tool_name || 'action'}</span>
                        </p>
                        {a.reason && (
                          <p className="text-xs text-muted-foreground mt-0.5 truncate">{a.reason}</p>
                        )}
                      </div>
                      <div className="flex items-center gap-1.5 shrink-0">
                        <button
                          onClick={() => handleDecide(a.id, 'approve')}
                          disabled={decidingId === a.id}
                          className="inline-flex items-center gap-1 rounded-md bg-emerald-500/10 px-2.5 py-1 text-xs font-medium text-emerald-600 hover:bg-emerald-500/20 disabled:opacity-50"
                        >
                          {decidingId === a.id ? <Loader2 className="h-3 w-3 animate-spin" /> : <ThumbsUp className="h-3 w-3" />}
                          Approve
                        </button>
                        <button
                          onClick={() => handleDecide(a.id, 'reject')}
                          disabled={decidingId === a.id}
                          className="inline-flex items-center gap-1 rounded-md bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive hover:bg-destructive/20 disabled:opacity-50"
                        >
                          {decidingId === a.id ? <Loader2 className="h-3 w-3 animate-spin" /> : <ThumbsDown className="h-3 w-3" />}
                          Reject
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
              {activeSession ? (
                <>
                  <ChatPlayground
                    key={activeSession.id}
                    sessionId={activeSession.id}
                    agentId={soul.id}
                    agentName={soul.display_name}
                    initialThinkingLevel={(soul.thinking_level as 'off' | 'medium' | 'high') || 'off'}
                    className="flex-1 min-h-0"
                  />
                  {/* Live task cards */}
                  {Object.values(liveTasks).length > 0 && (
                    <div className="shrink-0 mt-2 pb-2">
                      {Object.values(liveTasks).map(task => (
                        <TaskCard key={task.id} task={task} />
                      ))}
                    </div>
                  )}
                </>
              ) : (
                <div className="flex flex-1 items-center justify-center">
                  <div className="text-center space-y-3">
                    <MessageSquare className="h-10 w-10 text-muted-foreground/30 mx-auto" />
                    <p className="text-sm text-muted-foreground">Preparing chat…</p>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Config tab */}
          {activeTab === 'config' && (
            <div className="h-full overflow-y-auto p-5">
              <ConfigTab soul={soul} onSaved={(updated) => setSoul(updated)} />
            </div>
          )}

          {/* Channels tab */}
          {activeTab === 'channels' && (
            <div className="h-full overflow-y-auto p-5">
              <ChannelsTab
                agentId={soul.id}
                channels={soulChannels}
                onRefresh={() => channelsApi.list().then((chs) => setSoulChannels(chs.filter((c: Channel) => c.agent_id === id)))}
              />
            </div>
          )}

          {/* Tools tab */}
          {activeTab === 'tools' && (
            <div className="h-full overflow-y-auto p-5">
              <ToolsTab soul={soul} onSaved={(updated) => setSoul(updated)} />
            </div>
          )}

          {/* Settings tab (full settings page) */}
          {activeTab === 'settings' && (
            <div className="h-full overflow-y-auto p-5">
              <SoulSettingsPage soul={soul} />
            </div>
          )}

          {/* P9 T3.1 new tabs — operational surfaces per-agent */}
          {activeTab === 'skills' && (
            <div className="h-full overflow-y-auto p-5">
              <ProfileSkillsTab agentId={soul.id} />
            </div>
          )}
          {activeTab === 'memory' && (
            <div className="h-full overflow-y-auto p-5">
              <ProfileMemoryTab agentId={soul.id} agentName={soul.display_name} />
            </div>
          )}
          {activeTab === 'background' && (
            <div className="h-full overflow-y-auto p-5">
              <ProfileBackgroundTab agentId={soul.id} />
            </div>
          )}
          {activeTab === 'metrics' && (
            <div className="h-full overflow-y-auto p-5">
              <ProfileMetricsTab agentId={soul.id} />
            </div>
          )}
          {activeTab === 'mail' && (
            <div className="flex h-full flex-col overflow-hidden">
              <MailTab agentId={soul.id} />
            </div>
          )}
          {activeTab === 'permissions' && (
            <div className="h-full overflow-y-auto p-5">
              <PermissionsTab agentId={soul.id} />
            </div>
          )}
          {activeTab === 'schedules' && (
            <div className="h-full overflow-y-auto p-5">
              <SchedulesTab agentId={soul.id} />
            </div>
          )}
          {appAgentTabs.map((tab) =>
            activeTab === tab.id ? (
              <div key={tab.id} className="flex-1 overflow-y-auto">
                <ErrorBoundary>
                  {React.createElement(tab.component, {
                    React,
                    request: (path: string, init?: RequestInit) => apiRequest(path, init),
                    token: getToken(),
                    appId: tab.appId,
                  })}
                </ErrorBoundary>
              </div>
            ) : null
          )}
        </div>
      </div>
    </ErrorBoundary>
  );
}

// ─── Config Tab ───
function ConfigTab({ soul, onSaved }: { soul: Soul; onSaved: (s: Soul) => void }) {
  const { models } = useSelectedModels();
  const isPrime = soul.role === 'prime' || soul.agent_key === '__prime__';
  const [form, setForm] = useState({
    system_prompt: soul.system_prompt,
    model: soul.model,
    temperature: soul.temperature,
    tool_profile: soul.tool_profile,
    web_search_enabled: soul.web_search_enabled,
    max_tool_iterations: soul.max_tool_iterations,
    thinking_level: (soul.thinking_level || 'off') as 'off' | 'medium' | 'high',
    outbound_approval: soul.outbound_approval || 'supervisor',
  });
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  const handleModelChange = (modelId: string) => {
    setForm((f) => ({ ...f, model: modelId }));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const updated = await agents.update(soul.id, form as any);
      onSaved(updated);
      setSaved(true);
      toast.success('Configuration saved');
      setTimeout(() => setSaved(false), 2000);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">Configuration</h2>
          {isPrime && (
            <p className="text-xs text-muted-foreground mt-0.5">
              Prime agent — some settings are locked to ensure platform stability.
            </p>
          )}
        </div>
        <button
          onClick={handleSave}
          disabled={saving}
          className={cn(
            'flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium transition-colors cursor-pointer',
            saved
              ? 'bg-emerald-500/20 text-emerald-400'
              : 'bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50'
          )}
        >
          {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : saved ? <Check className="h-4 w-4" /> : <Save className="h-4 w-4" />}
          {saving ? 'Saving...' : saved ? 'Saved' : 'Save Changes'}
        </button>
      </div>

      {/* Model */}
      <section className="rounded-xl border border-border p-5 space-y-4">
        <h3 className="text-sm font-medium">Model Settings</h3>
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="text-xs text-muted-foreground">Model</label>
            <div className="mt-1">
              <SearchableSelect
                value={form.model}
                onChange={handleModelChange}
                options={[
                  { value: form.model, label: form.model },
                  ...models.filter((m) => m.model_id !== form.model).map((m) => ({ value: m.model_id, label: m.model_id })),
                ]}
              />
            </div>
          </div>
          <div>
            <label className="text-xs text-muted-foreground">Context Window</label>
            <div className="mt-1 rounded-lg border border-border bg-muted/10 px-3 py-2 text-sm text-muted-foreground font-mono">
              {(() => {
                const m = models.find((x) => x.model_id === form.model);
                const cw = m?.context_window ?? soul.context_window;
                return cw ? `${(cw / 1000).toFixed(0)}K tokens` : '—';
              })()}
              <span className="ml-2 text-xs text-muted-foreground/50">model max</span>
            </div>
          </div>
        </div>
        <div>
          <label className="text-xs text-muted-foreground">
            Temperature: {form.temperature.toFixed(2)}
          </label>
          <input
            type="range"
            min="0"
            max="1"
            step="0.05"
            value={form.temperature}
            onChange={(e) => setForm({ ...form, temperature: parseFloat(e.target.value) })}
            className="mt-1 w-full accent-primary"
          />
          <div className="flex justify-between text-2xs text-muted-foreground mt-1">
            <span>Precise (0)</span>
            <span>Creative (1)</span>
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="text-xs text-muted-foreground">Thinking</label>
            <div className="mt-1 flex gap-1.5">
              {(['off', 'medium', 'high'] as const).map((level) => (
                <button key={level} onClick={() => setForm({ ...form, thinking_level: level })}
                  className={cn('flex-1 rounded-lg border px-2 py-1.5 text-xs transition-colors capitalize cursor-pointer',
                    form.thinking_level === level
                      ? 'border-primary bg-primary/10 text-primary font-medium'
                      : 'border-border bg-muted/30 text-muted-foreground hover:bg-accent')}>
                  {level === 'off' ? 'Off' : level === 'medium' ? 'Normal' : 'High'}
                </button>
              ))}
            </div>
          </div>
          <div>
            <label className="text-xs text-muted-foreground">Max Tool Iterations</label>
            <input type="number" min="1" max="50" value={form.max_tool_iterations}
              onChange={(e) => setForm({ ...form, max_tool_iterations: parseInt(e.target.value) || 10 })}
              className="mt-1 qr-input"
            />
          </div>
        </div>
      </section>

      {/* System Prompt */}
      <section className="rounded-xl border border-border p-5 space-y-3">
        <h3 className="text-sm font-medium">System Prompt</h3>
        <textarea
          value={form.system_prompt}
          onChange={(e) => setForm({ ...form, system_prompt: e.target.value })}
          rows={8}
          placeholder="Instructions for this agent..."
          className="qr-textarea font-mono text-xs"
        />
      </section>

      {/* Capabilities */}
      <section className="rounded-xl border border-border p-5 space-y-4">
        <h3 className="text-sm font-medium">Capabilities</h3>
        <ToggleRow
          label="Web Search"
          description={isPrime ? 'Always enabled for Prime' : 'Agent can search the internet in real time'}
          icon={<Globe className="h-4 w-4" />}
          checked={isPrime ? true : form.web_search_enabled}
          onChange={(v) => { if (!isPrime) setForm({ ...form, web_search_enabled: v }); }}
          locked={isPrime}
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="text-xs text-muted-foreground">Tool Profile</label>
            <div className="mt-1">
              <SearchableSelect
                value={form.tool_profile}
                onChange={(v) => setForm({ ...form, tool_profile: v })}
                options={[
                  { value: 'full', label: 'Full — all tools enabled' },
                  { value: 'minimal', label: 'Minimal — core tools only' },
                  { value: 'none', label: 'None — no tools' },
                  { value: 'custom', label: 'Custom — manually selected' },
                ]}
              />
            </div>
          </div>
          <div>
            <label className="text-xs text-muted-foreground">Outbound Approval</label>
            <div className="mt-1">
              <SearchableSelect
                value={form.outbound_approval}
                onChange={(v) => setForm({ ...form, outbound_approval: v })}
                options={[
                  { value: 'none', label: 'None — auto-send' },
                  { value: 'supervisor', label: 'Supervisor review' },
                  { value: 'user', label: 'User approval' },
                  { value: 'both', label: 'Both — supervisor + user' },
                ]}
              />
            </div>
          </div>
        </div>
      </section>

      {/* Budget */}
      <BudgetSection soul={soul} onSaved={onSaved} />
    </div>
  );
}

// ─── Budget Section ───
function BudgetSection({ soul, onSaved }: { soul: Soul; onSaved: (s: Soul) => void }) {
  const [dollars, setDollars] = useState(
    soul.credit_budget_cents != null && soul.credit_budget_cents > 0
      ? (soul.credit_budget_cents / 100).toFixed(2)
      : '',
  );
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  const usedDollars = (soul.credit_used_cents ?? 0) / 100;
  const budgetDollars = soul.credit_budget_cents != null && soul.credit_budget_cents > 0
    ? soul.credit_budget_cents / 100
    : null;
  const pct = budgetDollars && budgetDollars > 0 ? Math.min((usedDollars / budgetDollars) * 100, 100) : 0;

  const handleSave = async () => {
    setSaving(true);
    try {
      const cents = Math.round(parseFloat(dollars || '0') * 100);
      await agents.setBudget(soul.id, cents);
      onSaved({ ...soul, credit_budget_cents: cents });
      setSaved(true);
      toast.success('Budget updated');
      setTimeout(() => setSaved(false), 2000);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to update budget');
    } finally {
      setSaving(false);
    }
  };

  return (
    <section className="rounded-xl border border-border p-5 space-y-4">
      <h3 className="text-sm font-medium flex items-center gap-2">
        <DollarSign className="h-4 w-4 text-emerald-500" />
        Budget
      </h3>

      <div className="grid gap-4 sm:grid-cols-2 items-end">
        <div>
          <label className="text-xs text-muted-foreground">Monthly budget (USD)</label>
          <div className="relative mt-1">
            <span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">$</span>
            <input
              type="number"
              min="0"
              step="0.01"
              value={dollars}
              onChange={(e) => setDollars(e.target.value)}
              placeholder="No limit"
              className="qr-input pl-6"
            />
          </div>
          <p className="mt-1 text-2xs text-muted-foreground">Leave blank for no limit.</p>
        </div>
        <button
          onClick={handleSave}
          disabled={saving}
          className="flex items-center justify-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50 cursor-pointer h-10"
        >
          {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : saved ? <Check className="h-4 w-4" /> : <Save className="h-4 w-4" />}
          {saving ? 'Saving…' : saved ? 'Saved' : 'Set Budget'}
        </button>
      </div>

      {(usedDollars > 0 || budgetDollars != null) && (
        <div className="space-y-1.5">
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">Used this period</span>
            <span className="font-mono">
              ${usedDollars.toFixed(4)}
              {budgetDollars != null && <span className="text-muted-foreground"> / ${budgetDollars.toFixed(2)}</span>}
            </span>
          </div>
          {budgetDollars != null && (
            <div className="h-2 w-full rounded-full bg-muted overflow-hidden">
              <div
                className={cn(
                  'h-full rounded-full transition-all',
                  pct >= 90 ? 'bg-destructive' : pct >= 70 ? 'bg-amber-400' : 'bg-emerald-500',
                )}
                style={{ width: `${pct}%` }}
              />
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function ToggleRow({ label, description, icon, checked, onChange, locked }: {
  label: string; description: string; icon: React.ReactNode; checked: boolean; onChange: (v: boolean) => void; locked?: boolean;
}) {
  return (
    <div className="flex items-center justify-between">
      <div className="flex items-center gap-3">
        <div className="text-muted-foreground">{icon}</div>
        <div>
          <p className="text-sm font-medium">{label}</p>
          <p className="text-2xs text-muted-foreground">{description}</p>
        </div>
      </div>
      <button
        onClick={() => { if (!locked) onChange(!checked); }}
        disabled={locked}
        title={locked ? 'Locked for Prime agent' : undefined}
        className={cn('flex h-6 w-10 items-center rounded-full px-0.5 transition-colors',
          locked ? 'cursor-not-allowed opacity-60' : 'cursor-pointer',
          checked ? 'bg-primary' : 'bg-muted')}
      >
        <div className={cn('h-5 w-5 rounded-full bg-background shadow-sm transition-transform', checked && 'translate-x-4')} />
      </button>
    </div>
  );
}

// ─── Channels Tab ───

// Channel icon as coloured SVG emoji-style text — no external deps
function ChannelIcon({ type, className }: { type: string; className?: string }) {
  const map: Record<string, { bg: string; text: string; label: string }> = {
    telegram:   { bg: 'bg-[#229ED9]/15', text: 'text-[#229ED9]', label: '✈' },
    discord:    { bg: 'bg-[#5865F2]/15', text: 'text-[#5865F2]', label: '🎮' },
    slack:      { bg: 'bg-[#4A154B]/15', text: 'text-[#E01E5A]', label: '#' },
    whatsapp:   { bg: 'bg-[#25D366]/15', text: 'text-[#25D366]', label: '💬' },
    email:      { bg: 'bg-amber-500/15', text: 'text-amber-500', label: '✉' },
    webchat:    { bg: 'bg-primary/15',   text: 'text-primary',   label: '🌐' },
    webhook:    { bg: 'bg-muted',        text: 'text-muted-foreground', label: '⚡' },
    zalo:       { bg: 'bg-[#0068FF]/15', text: 'text-[#0068FF]', label: 'Z' },
    wecom:      { bg: 'bg-[#07C160]/15', text: 'text-[#07C160]', label: '企' },
    dingtalk:   { bg: 'bg-[#1677FF]/15', text: 'text-[#1677FF]', label: '钉' },
    feishu:     { bg: 'bg-[#3370FF]/15', text: 'text-[#3370FF]', label: '飞' },
    facebook:   { bg: 'bg-[#1877F2]/15', text: 'text-[#1877F2]', label: 'f' },
    teams:      { bg: 'bg-[#6264A7]/15', text: 'text-[#6264A7]', label: 'T' },
    line:       { bg: 'bg-[#06C755]/15', text: 'text-[#06C755]', label: 'L' },
    sms:        { bg: 'bg-emerald-500/15', text: 'text-emerald-500', label: '📱' },
    signal:     { bg: 'bg-[#3A76F0]/15', text: 'text-[#3A76F0]', label: '🔒' },
    imessage:   { bg: 'bg-[#34C759]/15', text: 'text-[#34C759]', label: '' },
    matrix:     { bg: 'bg-neutral-500/15', text: 'text-neutral-400', label: '[]' },
    mattermost: { bg: 'bg-[#0058CC]/15', text: 'text-[#0058CC]', label: 'M' },
  };
  const cfg = map[type] ?? { bg: 'bg-muted', text: 'text-muted-foreground', label: type.slice(0, 2).toUpperCase() };
  return (
    <span className={cn('flex h-9 w-9 items-center justify-center rounded-lg text-sm font-bold', cfg.bg, cfg.text, className)}>
      {cfg.label}
    </span>
  );
}

const ALL_CHANNEL_TYPES = [
  { type: 'telegram',   label: 'Telegram',       singleton: true,  fields: ['bot_token', 'chat_id'] },
  { type: 'discord',    label: 'Discord',         singleton: true,  fields: ['bot_token'] },
  { type: 'whatsapp',   label: 'WhatsApp',        singleton: true,  fields: ['phone_id', 'access_token'] },
  { type: 'slack',      label: 'Slack',           singleton: false, fields: ['webhook_url', 'bot_token'] },
  // Email moved to Mail tab (IMAP/SMTP setup lives there)
  { type: 'webchat',    label: 'Webchat',         singleton: true,  fields: [] },
  { type: 'webhook',    label: 'Webhook',         singleton: false, fields: ['secret'] },
  { type: 'zalo',       label: 'Zalo',            singleton: true,  fields: ['app_id', 'secret'] },
  { type: 'wecom',      label: 'WeChat Work',     singleton: true,  fields: ['corp_id', 'agent_id', 'secret'] },
  { type: 'dingtalk',   label: 'DingTalk',        singleton: true,  fields: ['app_key', 'app_secret'] },
  { type: 'feishu',     label: 'Feishu / Lark',   singleton: true,  fields: ['app_id', 'app_secret'] },
  { type: 'facebook',   label: 'Facebook Messenger', singleton: true, fields: ['page_id', 'access_token', 'verify_token'] },
  { type: 'teams',      label: 'Microsoft Teams', singleton: true,  fields: ['app_id', 'app_password', 'tenant_id'] },
  { type: 'line',       label: 'LINE',            singleton: true,  fields: ['channel_access_token', 'channel_secret'] },
  { type: 'sms',        label: 'SMS',             singleton: false, fields: ['provider', 'account_sid', 'auth_token', 'from_number'] },
  { type: 'signal',     label: 'Signal',          singleton: true,  fields: ['phone_number'] },
  { type: 'imessage',   label: 'iMessage',        singleton: true,  fields: [] },
  { type: 'matrix',     label: 'Matrix',          singleton: false, fields: ['homeserver', 'access_token', 'room_id'] },
  { type: 'mattermost', label: 'Mattermost',      singleton: false, fields: ['server_url', 'bot_token', 'channel_id'] },
] as const;

function ChannelsTab({ agentId, channels, onRefresh }: { agentId: string; channels: Channel[]; onRefresh: () => void }) {
  const [showAdd, setShowAdd] = useState(false);
  const [newChannel, setNewChannel] = useState({ channel_type: 'telegram', config: {} as Record<string, string> });
  const [saving, setSaving] = useState(false);

  const existingTypes = new Set(channels.map((c) => c.channel_type));
  const availableTypes = ALL_CHANNEL_TYPES.filter(
    (ct) => !ct.singleton || !existingTypes.has(ct.type)
  );
  const selectedType = ALL_CHANNEL_TYPES.find((ct) => ct.type === newChannel.channel_type);

  const handleAdd = async () => {
    setSaving(true);
    try {
      await channelsApi.create({
        agent_id: agentId,
        channel_type: newChannel.channel_type as any,
        name: newChannel.channel_type,
        config: newChannel.config,
        enabled: true,
      });
      toast.success('Channel added');
      setShowAdd(false);
      setNewChannel({ channel_type: 'telegram', config: {} });
      onRefresh();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to add channel');
    } finally {
      setSaving(false);
    }
  };

  const handleToggle = async (ch: Channel) => {
    try {
      if (ch.enabled) await channelsApi.stop(ch.id);
      else await channelsApi.start(ch.id);
      onRefresh();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to toggle channel');
    }
  };

  const handleDelete = async (ch: Channel) => {
    const label = ALL_CHANNEL_TYPES.find(t => t.type === ch.channel_type)?.label ?? ch.channel_type;
    if (!confirm(`Remove ${label}?`)) return;
    try {
      await channelsApi.delete(ch.id);
      onRefresh();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to remove channel');
    }
  };

  const statusDot: Record<string, string> = {
    running:   'bg-emerald-400',
    connected: 'bg-emerald-400',
    stopped:   'bg-muted-foreground/50',
    error:     'bg-destructive',
  };

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Channels</h2>
        {availableTypes.length > 0 && (
          <button onClick={() => setShowAdd(!showAdd)} className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90 cursor-pointer">
            <Plus className="h-3.5 w-3.5" />
            Add Channel
          </button>
        )}
      </div>

      {showAdd && availableTypes.length > 0 && (
        <div className="rounded-xl border border-border p-5 space-y-4">
          <div>
            <label className="text-xs text-muted-foreground">Provider</label>
            <div className="mt-1">
              <SearchableSelect
                value={newChannel.channel_type}
                onChange={(v) => setNewChannel({ channel_type: v, config: {} })}
                options={availableTypes.map(ct => ({
                  value: ct.type,
                  label: ct.label,
                  icon: <ChannelIcon type={ct.type} className="h-5 w-5 text-xs" />,
                }))}
              />
            </div>
          </div>
          {selectedType && selectedType.fields.length > 0 && (
            <div className="grid gap-3">
              {selectedType.fields.map((field) => (
                <div key={field}>
                  <label className="text-xs text-muted-foreground capitalize">{field.replace(/_/g, ' ')}</label>
                  <input
                    value={newChannel.config[field] ?? ''}
                    onChange={(e) => setNewChannel({ ...newChannel, config: { ...newChannel.config, [field]: e.target.value } })}
                    className="mt-1 qr-input font-mono"
                    placeholder={field}
                  />
                </div>
              ))}
            </div>
          )}
          <div className="flex gap-2">
            <button onClick={handleAdd} disabled={saving} className="rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
              {saving ? 'Adding…' : 'Add'}
            </button>
            <button onClick={() => setShowAdd(false)} className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer">
              Cancel
            </button>
          </div>
        </div>
      )}

      {channels.length === 0 && !showAdd ? (
        <div className="text-center py-12">
          <Hash className="h-10 w-10 text-muted-foreground/30 mx-auto" />
          <p className="mt-3 text-sm text-muted-foreground">No channels connected</p>
          <p className="text-2xs text-muted-foreground mt-1">Add a provider so this agent can send and receive messages externally</p>
        </div>
      ) : (
        <div className="space-y-2">
          {channels.map((ch) => {
            const meta = ALL_CHANNEL_TYPES.find((t) => t.type === ch.channel_type);
            return (
              <div key={ch.id} className="flex items-center justify-between rounded-xl border border-border p-4">
                <div className="flex items-center gap-3 min-w-0">
                  <ChannelIcon type={ch.channel_type} />
                  <div className="min-w-0">
                    <p className="text-sm font-medium">{meta?.label ?? ch.channel_type}</p>
                    <div className="flex items-center gap-1.5 text-2xs text-muted-foreground">
                      <span className={cn('h-1.5 w-1.5 rounded-full', statusDot[ch.status] ?? 'bg-muted-foreground/50')} />
                      {ch.status || 'unknown'}
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => handleToggle(ch)}
                    className={cn('flex items-center gap-1 rounded-md px-2 py-1 text-2xs font-medium transition-colors cursor-pointer',
                      ch.enabled ? 'bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20' : 'bg-muted text-muted-foreground hover:bg-muted/80'
                    )}
                  >
                    {ch.enabled ? <ToggleRight className="h-3.5 w-3.5" /> : <ToggleLeft className="h-3.5 w-3.5" />}
                    {ch.enabled ? 'On' : 'Off'}
                  </button>
                  <button onClick={() => handleDelete(ch)} className="text-muted-foreground hover:text-destructive cursor-pointer">
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ─── Tools Tab ───
function ToolsTab({ soul, onSaved }: { soul: Soul; onSaved: (s: Soul) => void }) {
  const [builtinTools, setBuiltinTools] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [denied, setDenied] = useState<Set<string>>(new Set(soul.tools_denied ?? []));
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let active = true;
    toolsApi.builtin()
      .then((t) => { if (active) { setBuiltinTools(Array.isArray(t) ? t : []); setLoading(false); } })
      .catch(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, []);

  const filtered = builtinTools.filter((t: any) => {
    if (!search) return true;
    const name = (t.name || t.function?.name || '').toLowerCase();
    const desc = (t.description || t.function?.description || '').toLowerCase();
    return name.includes(search.toLowerCase()) || desc.includes(search.toLowerCase());
  });

  const handleToggle = (toolName: string) => {
    setDenied((prev) => {
      const next = new Set(prev);
      if (next.has(toolName)) next.delete(toolName);
      else next.add(toolName);
      return next;
    });
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const updated = await agents.update(soul.id, {
        tools_denied: Array.from(denied),
        tool_profile: 'custom',
      } as any);
      onSaved(updated);
      toast.success('Tools saved');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to save tools');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="max-w-3xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Tools ({builtinTools.length})</h2>
        <button onClick={handleSave} disabled={saving} className="flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
          {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
          Save
        </button>
      </div>

      <p className="text-sm text-muted-foreground">
        Profile: <span className="font-medium text-foreground">{soul.tool_profile}</span> — toggle individual tools off to restrict access.
      </p>

      {/* Search */}
      <div className="relative max-w-sm">
        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search tools..."
          className="qr-input pl-9"
        />
      </div>

      {loading ? (
        <div className="space-y-2">
          {Array.from({ length: 8 }).map((_, i) => (
            <div key={i} className="h-14 rounded-xl bg-muted animate-pulse" />
          ))}
        </div>
      ) : (
        <div className="space-y-1.5">
          {filtered.map((tool: any) => {
            const name = tool.name || tool.function?.name || 'unknown';
            const desc = tool.description || tool.function?.description || '';
            const isEnabled = !denied.has(name);
            return (
              <div key={name} className="flex items-center justify-between rounded-xl border border-border p-3">
                <div className="flex items-center gap-3 min-w-0">
                  <Wrench className="h-4 w-4 text-muted-foreground shrink-0" />
                  <div className="min-w-0">
                    <p className="text-sm font-medium font-mono">{name}</p>
                    <p className="text-2xs text-muted-foreground truncate max-w-md">{desc}</p>
                  </div>
                </div>
                <button
                  onClick={() => handleToggle(name)}
                  className={cn('flex h-6 w-10 items-center rounded-full px-0.5 transition-colors cursor-pointer',
                    isEnabled ? 'bg-primary' : 'bg-muted')}
                >
                  <div className={cn('h-5 w-5 rounded-full bg-background shadow-sm transition-transform', isEnabled && 'translate-x-4')} />
                </button>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ─── State Orb ───
function StateOrb({ state, taskCount, onOpen }: {
  state: RuntimeState | null;
  taskCount: number;
  onOpen: () => void;
}) {
  const isWorking = state === 'working';
  const isSuspended = state === 'suspended';

  return (
    <button
      onClick={onOpen}
      title={`Agent ${state ?? 'offline'} · ${taskCount} task${taskCount !== 1 ? 's' : ''}`}
      className="relative flex items-center gap-2 rounded-full px-2.5 py-1 text-xs font-medium transition-colors hover:bg-accent"
    >
      <span className={cn(
        'h-2.5 w-2.5 rounded-full',
        isWorking && 'bg-green-400 animate-pulse',
        isSuspended && 'bg-yellow-400',
        state === 'idle' && 'bg-zinc-400',
        state === null && 'bg-zinc-600',
      )} />
      <span className={cn(
        'hidden sm:inline',
        isWorking && 'text-green-400',
        isSuspended && 'text-yellow-400',
        state === 'idle' && 'text-zinc-400',
        state === null && 'text-zinc-500',
      )}>
        {isWorking ? 'Working' : isSuspended ? 'Paused' : state === 'idle' ? 'Idle' : 'Offline'}
      </span>
      {taskCount > 0 && (
        <span className="flex h-4 min-w-4 items-center justify-center rounded-full bg-primary/20 px-1 text-2xs font-bold text-primary">
          {taskCount}
        </span>
      )}
    </button>
  );
}
