// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { create } from 'zustand';
import type { Soul, SoulActivity, Session, LiveEvent, RailSection } from '@/types';
import type { TelemetryEvent, PermissionRequestedProps } from '@/lib/graph-events';
import type { GitHubPRReadyProps, GitHubCommitPendingProps } from '@/lib/events';

/** Max telemetry events we keep per session. Hard cap prevents a
 *  long plan run from ballooning store memory — a single chat rarely
 *  needs more than a few dozen. Older events scroll off FIFO. */
const TELEMETRY_CAP_PER_SESSION = 200;

/** A pending or resolved permission-gate request. Mirrors backend
 *  PermissionRequestedProps plus client-side lifecycle bits. Stored
 *  in pendingApprovals keyed by request_id, so a duplicate
 *  permission.requested for the same id collapses cleanly. */
export interface ApprovalEntry {
  request: PermissionRequestedProps;
  /** Client-side monotonic timestamp we first observed it. */
  createdAt: number;
  /** Resolution state. null = still pending. Driven by user click
   *  OR an inbound permission.replied event. */
  resolved: null | { decision: 'allow' | 'deny'; actor?: string; note?: string };
}

interface SoulState {
  activity: SoulActivity;
  lastEvent?: string;
  tokensToday: number;
}

interface Store {
  // Souls slice
  souls: Soul[];
  soulStates: Record<string, SoulState>;
  setSouls: (souls: Soul[]) => void;
  updateSoulActivity: (agentId: string, activity: SoulActivity, detail?: string) => void;

  // Live events slice
  liveEvents: LiveEvent[];
  pushEvent: (event: LiveEvent) => void;

  // ───────── Telemetry slice (P9 Step 1) ─────────
  // Per-session log of orchestrator `graph.node_*` + `agent.progress`
  // events. Consumed by in-chat telemetry renderers. Each session's
  // buffer is capped so a runaway plan can't balloon memory.
  telemetryBySession: Record<string, TelemetryEvent[]>;
  pushTelemetry: (sessionId: string, event: TelemetryEvent) => void;
  clearTelemetry: (sessionId: string) => void;

  // ───────── Permission approvals slice (P9 Step 2) ─────────
  // Keyed by request_id so we can O(1) resolve on permission.replied.
  // The entry's request.session_id lets UIs filter to their own chat.
  // Deliberately named `approvals` — the legacy `pendingApprovals`
  // counter below drives the Settings/Security badge and is unrelated
  // to the permission-gate protocol; collapsing them caused a silent
  // type clash that wedged the map at runtime.
  approvals: Record<string, ApprovalEntry>;
  upsertApproval: (req: PermissionRequestedProps) => void;
  markApprovalResolved: (
    requestId: string,
    decision: 'allow' | 'deny',
    opts?: { actor?: string; note?: string },
  ) => void;

  // Active sessions slice
  activeSessions: Record<string, Session>;
  setSession: (session: Session) => void;
  removeSession: (id: string) => void;

  // ───────── Daemon agents slice (multi-agent dashboard) ─────────
  daemonAgents: Record<string, import('@/hooks/use-agents-stream').DaemonAgent>;
  daemonTasks: Record<string, import('@/hooks/use-agents-stream').DaemonTask>;
  daemonPlans: Record<string, import('@/hooks/use-agents-stream').DaemonPlan>;
  daemonConnected: boolean;
  dispatchDaemonEvent: (event: any) => void;
  setDaemonConnected: (v: boolean) => void;

  // Streaming state
  streamingTokens: Record<string, string>;
  appendToken: (msgId: string, token: string) => void;
  clearStream: (msgId: string) => void;

  // Incoming messages from WebSocket (cron, other agents, etc.)
  incomingMessages: { sessionId: string; agentId: string; role: string; content: string; source?: string }[];
  pushIncomingMessage: (msg: { sessionId: string; agentId: string; role: string; content: string; source?: string }) => void;
  clearIncomingMessages: (sessionId: string) => void;

  // Guaranteed delivery: WS state + catch-up trigger
  wsConnected: boolean;
  pendingApprovals: number;
  incrementPendingApprovals: () => void;
  clearPendingApprovals: () => void;
  setWsConnected: (v: boolean) => void;
  catchUpCounter: number;
  triggerCatchUp: () => void;

  // Resilience: tracks reconnection state for the banner UI. Reset to
  // 0/undefined on successful connect. See web/lib/websocket.ts.
  wsReconnectAttempt: number;
  wsLastDisconnectAt: number | undefined;
  setWsReconnecting: (attempt: number, since: number | undefined) => void;

  // Service health: set by service_health WS events from the health monitor
  // goroutine, and also by active probing during WS disconnect. Lets the
  // banner show a specific "database unavailable" message rather than the
  // generic "Reconnecting…" spinner when the issue is a DB outage.
  serviceHealth: { database: 'ok' | 'unavailable' | 'unknown'; status: 'healthy' | 'degraded' | 'unknown' };
  setServiceHealth: (h: { database: 'ok' | 'unavailable' | 'unknown'; status: 'healthy' | 'degraded' | 'unknown' }) => void;

  // Disconnect reason: set by active probing in websocket.ts when WS drops.
  // null = not yet probed / connected. 'backend_down' = process unreachable.
  // 'db_down' = process alive, DB offline. 'network' = local connectivity.
  wsDisconnectReason: 'backend_down' | 'db_down' | 'degraded' | 'network' | null;
  setWsDisconnectReason: (r: 'backend_down' | 'db_down' | 'degraded' | 'network' | null) => void;

  // Layout state
  activeRail: RailSection;
  setActiveRail: (section: RailSection) => void;
  sidebarCollapsed: boolean;
  toggleSidebar: () => void;
  activeChatId: string | null;
  activeChatType: 'soul' | 'room' | null;
  setActiveChat: (id: string | null, type: 'soul' | 'room' | null) => void;
  workspaceTab: string;
  setWorkspaceTab: (tab: string) => void;
  contextPanelOpen: boolean;
  contextPanelContent: { type: string; data: unknown } | null;
  openContextPanel: (type: string, data: unknown) => void;
  closeContextPanel: () => void;

  // GitHub page state
  githubSelectedRepo: string | null;
  setGithubSelectedRepo: (repo: string | null) => void;
  githubActiveTab: 'prs' | 'issues' | 'tasks' | 'repos';
  setGithubActiveTab: (tab: 'prs' | 'issues' | 'tasks' | 'repos') => void;

  // GitHub PR approval cards — keyed by approval_id
  prApprovals: Record<string, GitHubPRReadyProps>;
  upsertPRApproval: (props: GitHubPRReadyProps) => void;
  removePRApproval: (approvalId: string) => void;

  // GitHub commit pending cards — keyed by approval_id
  commitPendings: Record<string, GitHubCommitPendingProps>;
  upsertCommitPending: (props: GitHubCommitPendingProps) => void;
  removeCommitPending: (approvalId: string) => void;

  // Mail filter + folder
  mailSoulFilter: string | null;
  setMailSoulFilter: (id: string | null) => void;
  mailFolder: string;
  setMailFolder: (folder: string) => void;
  // Mail sidebar secondary view (contacts | mailboxes | null = folders)
  mailView: 'contacts' | 'mailboxes' | null;
  setMailView: (v: 'contacts' | 'mailboxes' | null) => void;

  // Tasks agent filter
  taskAgentFilter: string | null;
  setTaskAgentFilter: (id: string | null) => void;

  // Settings active tab
  settingsTab: string;
  setSettingsTab: (tab: string) => void;

  // Calendar filter
  calSoulFilter: string | null;
  setCalSoulFilter: (id: string | null) => void;

  // Drive filter
  driveSoulFilter: string | null;
  setDriveSoulFilter: (id: string | null) => void;

  // Code IDE state (shared between sidebar + code page)
  codeProjectName: string;
  setCodeProjectName: (name: string) => void;
  codeTermOpen: boolean;
  setCodeTermOpen: (v: boolean) => void;
  codeChatOpen: boolean;
  setCodeChatOpen: (v: boolean) => void;
  codeGitHubOpen: boolean;
  setCodeGitHubOpen: (v: boolean) => void;
  codeProjects: any[];
  setCodeProjects: (p: any[]) => void;
  codeActiveProjectId: string | null;
  setCodeActiveProjectId: (id: string | null) => void;
  codeTree: any[];
  setCodeTree: (t: any[]) => void;
  codeProjectPath: string;
  setCodeProjectPath: (p: string) => void;
  codeSidebarTab: 'explorer' | 'github';
  setCodeSidebarTab: (t: 'explorer' | 'github') => void;

  // Right panel (Chat / Notifications / Activity / Tasks)
  rightPanelOpen: boolean;
  rightPanelTab: 'chat' | 'notifications' | 'activity' | 'tasks' | null;
  openRightPanel: (tab: 'chat' | 'notifications' | 'activity' | 'tasks') => void;
  closeRightPanel: () => void;

  // ───────── Live task monitoring slice ─────────
  // Updated by the Qor detail page WebSocket. The sidebar reads these
  // without needing to be a child of the page component.
  liveTasks: Record<string, {
    id: string; title: string; status: string;
    iteration: number; scratchpad: string;
    last_tool?: string; updated_at: string;
  }>;
  liveTaskAgentId: string | null;
  setLiveTask: (task: {
    id: string; title: string; status: string;
    iteration: number; scratchpad: string;
    last_tool?: string; updated_at: string;
  }) => void;
  clearLiveTasks: () => void;
  setLiveTaskAgentId: (id: string | null) => void;

  // ───────── Rooms live state (P9 T3.3) ─────────
  // Driven by the realtime hub:
  //   • room_typing_start / room_typing_stop → updates roomTyping
  //   • room_message → pushes into roomIncomingMessages[roomId]
  // The /rooms/[id] page consumes these to paint typing chips and
  // append new messages without a manual refresh.
  roomTyping: Record<string, string[]>;     // roomId → agent ids currently typing
  roomIncomingMessages: Record<string, any[]>; // roomId → messages pushed via WS
  setRoomTyping: (roomId: string, agentId: string, typing: boolean) => void;
  pushRoomMessage: (roomId: string, msg: any) => void;
  updateRoomMessage: (roomId: string, msgId: string, patch: Partial<any>) => void;
  clearRoomIncoming: (roomId: string) => void;

  // ───────── Bottom drawer slice (P9 T1.2) ─────────
  // Global open/active state + a registry of tab descriptors pushed by
  // the currently-mounted page. Tab *content* is rendered via React
  // portal into the drawer body so React ownership stays with the page
  // (see components/layouts/qorven/bottom-drawer.tsx).
  bottomDrawerOpen: boolean;
  bottomDrawerHeightPx: number;
  bottomDrawerActiveTabId: string | null;
  bottomDrawerTabs: BottomDrawerTab[];
  openBottomDrawer: (tabId?: string) => void;
  closeBottomDrawer: () => void;
  toggleBottomDrawer: () => void;
  setBottomDrawerHeight: (px: number) => void;
  setBottomDrawerActiveTab: (tabId: string) => void;
  registerBottomDrawerTab: (tab: BottomDrawerTab) => void;
  unregisterBottomDrawerTab: (tabId: string) => void;
}

/** Static descriptor for a bottom-drawer tab. The React node for the tab
 *  body is NOT stored — it's rendered via portal from the page itself. */
export interface BottomDrawerTab {
  id: string;
  label: string;
  /** lucide-react icon name, resolved by the drawer. Optional. */
  iconName?: string;
  /** Lower = further left in the tab bar. Pages use 10, 20, 30 so new
   *  tabs can slot between without renumbering. */
  order?: number;
  /** Optional badge count (e.g. "3" for pending items). */
  badge?: number | string;
}

export const useStore = create<Store>((set) => ({
  // Souls
  souls: [],
  soulStates: {},
  setSouls: (souls) => set({ souls: (Array.isArray(souls) ? souls : []).filter(s => !s.agent_key?.startsWith('__')) }),
  updateSoulActivity: (agentId, activity, detail) =>
    set((s) => ({
      soulStates: {
        ...s.soulStates,
        [agentId]: { ...s.soulStates[agentId], activity, lastEvent: detail, tokensToday: s.soulStates[agentId]?.tokensToday ?? 0 },
      },
    })),

  // Live events — keep last 200
  liveEvents: [],
  pushEvent: (event) =>
    set((s) => ({ liveEvents: [event, ...s.liveEvents].slice(0, 200) })),

  // Telemetry — per-session append-only log, FIFO-capped.
  telemetryBySession: {},
  pushTelemetry: (sessionId, event) =>
    set((s) => {
      if (!sessionId) return {};
      const prev = s.telemetryBySession[sessionId] ?? [];
      const next = [...prev, event];
      // Trim from the head once we exceed the cap. The head is the
      // oldest event, which is least interesting for "what's the
      // agent doing RIGHT NOW".
      const trimmed =
        next.length > TELEMETRY_CAP_PER_SESSION
          ? next.slice(next.length - TELEMETRY_CAP_PER_SESSION)
          : next;
      return {
        telemetryBySession: { ...s.telemetryBySession, [sessionId]: trimmed },
      };
    }),
  clearTelemetry: (sessionId) =>
    set((s) => {
      if (!s.telemetryBySession[sessionId]) return {};
      const { [sessionId]: _drop, ...rest } = s.telemetryBySession;
      return { telemetryBySession: rest };
    }),

  // Permission approvals — keyed by request_id.
  approvals: {},
  upsertApproval: (req) =>
    set((s) => {
      // If we've already seen this request_id, preserve any existing
      // resolved state — a late-arriving duplicate permission.requested
      // after a reply (possible on reconnect catch-up) must not un-
      // resolve the card.
      const existing = s.approvals[req.request_id];
      return {
        approvals: {
          ...s.approvals,
          [req.request_id]: existing
            ? { ...existing, request: req }
            : { request: req, createdAt: Date.now(), resolved: null },
        },
      };
    }),
  markApprovalResolved: (requestId, decision, opts) =>
    set((s) => {
      const entry = s.approvals[requestId];
      if (!entry) return {}; // reply arrived before request or for an already-GCd one
      return {
        approvals: {
          ...s.approvals,
          [requestId]: {
            ...entry,
            resolved: { decision, actor: opts?.actor, note: opts?.note },
          },
        },
      };
    }),

  // Sessions
  activeSessions: {},
  setSession: (session) =>
    set((s) => ({ activeSessions: { ...s.activeSessions, [session.id]: session } })),
  removeSession: (id) =>
    set((s) => {
      const { [id]: _, ...rest } = s.activeSessions;
      return { activeSessions: rest };
    }),

  // Daemon agents
  daemonAgents: {},
  daemonTasks: {},
  daemonPlans: {},
  daemonConnected: false,
  setDaemonConnected: (v) => set({ daemonConnected: v }),
  dispatchDaemonEvent: (event: any) => {
    set((s: any) => {
      switch (event.type) {
        case 'agent_snapshot': {
          // Full state hydration on connect/reconnect — replace entire agent map
          const agents: Record<string, any> = {};
          for (const a of (event.data as any[])) agents[a.id] = a;
          return { daemonAgents: agents };
        }
        case 'agent_registered':
          return { daemonAgents: { ...s.daemonAgents, [event.data.id]: event.data } };
        case 'agent_status':
          return { daemonAgents: { ...s.daemonAgents, [event.data.id]: { ...s.daemonAgents[event.data.id], status: event.data.status, current_task_id: event.data.current_task_id } } };
        case 'agent_unregistered': {
          const { [event.data.id]: _, ...rest } = s.daemonAgents;
          return { daemonAgents: rest };
        }
        case 'task_snapshot': {
          // Full state hydration on reconnect — replace entire task map so stale
          // done/failed tasks that were deleted server-side don't persist.
          const tasks: Record<string, any> = {};
          for (const t of (event.data as any[])) tasks[t.id] = t;
          return { daemonTasks: tasks };
        }
        case 'task_created':
          return { daemonTasks: { ...s.daemonTasks, [event.data.id]: event.data } };
        case 'task_assigned':
          return { daemonTasks: { ...s.daemonTasks, [event.data.task_id]: { ...s.daemonTasks[event.data.task_id], owner: event.data.agent_id, status: 'in_progress' } } };
        case 'task_progress':
          return { daemonTasks: { ...s.daemonTasks, [event.data.task_id]: { ...s.daemonTasks[event.data.task_id], percent: event.data.percent } } };
        case 'task_file': {
          const t = s.daemonTasks[event.data.task_id];
          if (!t) return {};
          const files = [...(t.files_changed ?? []), event.data.path];
          return { daemonTasks: { ...s.daemonTasks, [event.data.task_id]: { ...t, files_changed: files } } };
        }
        case 'task_done':
          return { daemonTasks: { ...s.daemonTasks, [event.data.task_id]: { ...s.daemonTasks[event.data.task_id], status: 'done', summary: event.data.summary, files_changed: event.data.files_changed } } };
        case 'task_failed':
          return { daemonTasks: { ...s.daemonTasks, [event.data.task_id]: { ...s.daemonTasks[event.data.task_id], status: 'failed', error: event.data.error } } };
        case 'plan_proposed':
          return { daemonPlans: { ...s.daemonPlans, [event.data.id]: event.data } };
        case 'plan_approved':
          return { daemonPlans: { ...s.daemonPlans, [event.data.plan_id]: { ...s.daemonPlans[event.data.plan_id], status: 'approved' } } };
        case 'plan_rejected':
          return { daemonPlans: { ...s.daemonPlans, [event.data.plan_id]: { ...s.daemonPlans[event.data.plan_id], status: 'rejected' } } };
        default: return {};
      }
    });
  },

  // Streaming
  streamingTokens: {},
  appendToken: (msgId, token) =>
    set((s) => {
      const updated = { ...s.streamingTokens, [msgId]: (s.streamingTokens[msgId] ?? '') + token };
      // Evict oldest entries beyond 20 to prevent unbounded growth from un-cleared streams.
      const keys = Object.keys(updated);
      if (keys.length > 20) {
        const oldest = keys[0]!;
        const { [oldest]: _, ...trimmed } = updated;
        return { streamingTokens: trimmed };
      }
      return { streamingTokens: updated };
    }),
  clearStream: (msgId) =>
    set((s) => {
      const { [msgId]: _, ...rest } = s.streamingTokens;
      return { streamingTokens: rest };
    }),

  // Incoming messages — keep last 50
  incomingMessages: [],
  pushIncomingMessage: (msg) =>
    set((s) => ({ incomingMessages: [...s.incomingMessages, msg].slice(-50) })),
  clearIncomingMessages: (sessionId) =>
    set((s) => ({ incomingMessages: s.incomingMessages.filter((m) => m.sessionId !== sessionId) })),

  // Guaranteed delivery
  wsConnected: false,
  pendingApprovals: 0,
  incrementPendingApprovals: () => set((s) => ({ pendingApprovals: s.pendingApprovals + 1 })),
  clearPendingApprovals: () => set({ pendingApprovals: 0 }),
  setWsConnected: (v) => set({ wsConnected: v }),
  catchUpCounter: 0,
  triggerCatchUp: () => set((s) => ({ catchUpCounter: s.catchUpCounter + 1 })),

  // Resilience
  wsReconnectAttempt: 0,
  wsLastDisconnectAt: undefined,
  setWsReconnecting: (attempt, since) =>
    set({ wsReconnectAttempt: attempt, wsLastDisconnectAt: since }),

  serviceHealth: { database: 'unknown', status: 'unknown' },
  setServiceHealth: (h) => set({ serviceHealth: h }),

  wsDisconnectReason: null,
  setWsDisconnectReason: (r) => set({ wsDisconnectReason: r }),

  // Layout
  activeRail: 'dashboard',
  setActiveRail: (section) => set({ activeRail: section }),
  sidebarCollapsed: false,
  toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
  activeChatId: null,
  activeChatType: null,
  setActiveChat: (id, type) => set({ activeChatId: id, activeChatType: type }),
  workspaceTab: 'chat',
  setWorkspaceTab: (tab) => set({ workspaceTab: tab }),
  contextPanelOpen: false,
  contextPanelContent: null,
  openContextPanel: (type, data) =>
    set({ contextPanelOpen: true, contextPanelContent: { type, data } }),
  closeContextPanel: () =>
    set({ contextPanelOpen: false, contextPanelContent: null }),

  // GitHub page state
  githubSelectedRepo: null,
  setGithubSelectedRepo: (repo) => set({ githubSelectedRepo: repo }),
  githubActiveTab: 'prs',
  setGithubActiveTab: (tab) => set({ githubActiveTab: tab }),

  // GitHub PR approval cards
  prApprovals: {},
  upsertPRApproval: (props) =>
    set((s) => ({
      prApprovals: { ...s.prApprovals, [props.approval_id ?? props.pr_url]: props },
    })),
  removePRApproval: (approvalId) =>
    set((s) => {
      const { [approvalId]: _, ...rest } = s.prApprovals;
      return { prApprovals: rest };
    }),

  // GitHub commit pending cards
  commitPendings: {},
  upsertCommitPending: (props) =>
    set((s) => ({
      commitPendings: { ...s.commitPendings, [props.approval_id ?? props.branch]: props },
    })),
  removeCommitPending: (approvalId) =>
    set((s) => {
      const { [approvalId]: _, ...rest } = s.commitPendings;
      return { commitPendings: rest };
    }),

  // Mail filter + folder
  mailSoulFilter: null,
  setMailSoulFilter: (id) => set({ mailSoulFilter: id }),
  mailFolder: 'inbox',
  setMailFolder: (folder) => set({ mailFolder: folder }),
  mailView: null,
  setMailView: (v) => set({ mailView: v }),

  taskAgentFilter: null,
  setTaskAgentFilter: (id) => set({ taskAgentFilter: id }),

  settingsTab: 'profile',
  setSettingsTab: (tab) => set({ settingsTab: tab }),

  calSoulFilter: null,
  setCalSoulFilter: (id) => set({ calSoulFilter: id }),

  driveSoulFilter: null,
  setDriveSoulFilter: (id) => set({ driveSoulFilter: id }),

  // Code IDE state
  codeProjectName: '',
  setCodeProjectName: (name) => set({ codeProjectName: name }),
  codeTermOpen: false,
  setCodeTermOpen: (v) => set({ codeTermOpen: v }),
  codeChatOpen: false,
  setCodeChatOpen: (v) => set({ codeChatOpen: v }),
  codeGitHubOpen: false,
  setCodeGitHubOpen: (v) => set({ codeGitHubOpen: v }),
  codeProjects: [],
  setCodeProjects: (p) => set({ codeProjects: p }),
  codeActiveProjectId: null,
  setCodeActiveProjectId: (id) => set({ codeActiveProjectId: id }),
  codeTree: [],
  setCodeTree: (t) => set({ codeTree: t }),
  codeProjectPath: '',
  setCodeProjectPath: (p) => set({ codeProjectPath: p }),
  codeSidebarTab: 'explorer',
  setCodeSidebarTab: (t) => set({ codeSidebarTab: t }),

  // Right panel
  rightPanelOpen: false,
  rightPanelTab: null,
  openRightPanel: (tab) => set({ rightPanelOpen: true, rightPanelTab: tab }),
  closeRightPanel: () => set({ rightPanelOpen: false }),

  // Live task monitoring
  liveTasks: {},
  liveTaskAgentId: null,
  setLiveTask: (task) =>
    set((s) => ({ liveTasks: { ...s.liveTasks, [task.id]: task } })),
  clearLiveTasks: () => set({ liveTasks: {} }),
  setLiveTaskAgentId: (id) => set({ liveTaskAgentId: id }),

  // Rooms live state — driven by WS dispatch
  roomTyping: {},
  roomIncomingMessages: {},
  setRoomTyping: (roomId, agentId, typing) =>
    set((s) => {
      const current = new Set(s.roomTyping[roomId] ?? []);
      if (typing) current.add(agentId);
      else current.delete(agentId);
      return {
        roomTyping: { ...s.roomTyping, [roomId]: Array.from(current) },
      };
    }),
  pushRoomMessage: (roomId, msg) =>
    set((s) => {
      const prev = s.roomIncomingMessages[roomId] ?? [];
      // Cap at 100 to bound memory; the page also re-fetches on mount
      // so older messages are available from the server.
      const next = [...prev, msg].slice(-100);
      return {
        roomIncomingMessages: { ...s.roomIncomingMessages, [roomId]: next },
      };
    }),
  updateRoomMessage: (roomId, msgId, patch) =>
    set((s) => {
      const prev = s.roomIncomingMessages[roomId] ?? [];
      const next = prev.map((m: any) => m.id === msgId ? { ...m, ...patch } : m);
      return { roomIncomingMessages: { ...s.roomIncomingMessages, [roomId]: next } };
    }),
  clearRoomIncoming: (roomId) =>
    set((s) => {
      if (!s.roomIncomingMessages[roomId]) return {};
      const { [roomId]: _drop, ...rest } = s.roomIncomingMessages;
      return { roomIncomingMessages: rest };
    }),

  // Bottom drawer — global open state + registry.
  // Default height picked to match VSCode's default panel. Persisted
  // within the session only; reset on reload is fine.
  bottomDrawerOpen: false,
  bottomDrawerHeightPx: 280,
  bottomDrawerActiveTabId: null,
  bottomDrawerTabs: [],
  openBottomDrawer: (tabId) =>
    set((s) => ({
      bottomDrawerOpen: true,
      bottomDrawerActiveTabId:
        tabId ?? s.bottomDrawerActiveTabId ?? s.bottomDrawerTabs[0]?.id ?? null,
    })),
  closeBottomDrawer: () => set({ bottomDrawerOpen: false }),
  toggleBottomDrawer: () => set((s) => ({ bottomDrawerOpen: !s.bottomDrawerOpen })),
  setBottomDrawerHeight: (px) =>
    set({ bottomDrawerHeightPx: Math.max(120, Math.min(800, px)) }),
  setBottomDrawerActiveTab: (tabId) => set({ bottomDrawerActiveTabId: tabId }),
  registerBottomDrawerTab: (tab) =>
    set((s) => {
      // Idempotent upsert — a re-mount (StrictMode, HMR) mustn't double-add.
      const without = s.bottomDrawerTabs.filter((t) => t.id !== tab.id);
      const next = [...without, tab].sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
      return {
        bottomDrawerTabs: next,
        // If nothing's selected yet, select the first-registered tab.
        bottomDrawerActiveTabId: s.bottomDrawerActiveTabId ?? next[0]?.id ?? null,
      };
    }),
  unregisterBottomDrawerTab: (tabId) =>
    set((s) => {
      const next = s.bottomDrawerTabs.filter((t) => t.id !== tabId);
      const activeStillPresent = next.some((t) => t.id === s.bottomDrawerActiveTabId);
      return {
        bottomDrawerTabs: next,
        bottomDrawerActiveTabId: activeStillPresent
          ? s.bottomDrawerActiveTabId
          : next[0]?.id ?? null,
        // If the drawer was open solely because this page registered a tab,
        // and now there are no tabs, close it — avoids an empty drawer.
        bottomDrawerOpen: s.bottomDrawerOpen && next.length > 0,
      };
    }),
}));
