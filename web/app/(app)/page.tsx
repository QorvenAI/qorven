'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import dynamic from 'next/dynamic';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import {
  agents, sessions, providers, approvals as approvalsApi,
  outbound, supervisor,
  type ApprovalItem, type OutboundAction, type SupervisorMessage,
} from '@/lib/api';
import { dashboardApi, dashboardLayout, type DashboardStats } from '@/lib/api-dashboard';
import { orgApi, type OrgRosterEntry, type OrgAgentSpend } from '@/lib/api-agents';
import { tickets as ticketsApi } from '@/lib/api-workspace';
import type { Ticket as TicketItem } from '@/types';
import { ErrorBoundary } from '@/components/error-boundary';
import { brand } from '@/lib/branding';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import { cn } from '@/lib/utils';
import {
  AlertCircle, CheckCircle, Circle, Sparkles, Cpu,
  Users, Zap, Plus, Send, Settings, ArrowUpRight,
  ListChecks, GitBranch, ShieldCheck, Check, X, Loader2, RefreshCw,
  Bot, Clock, TrendingUp, TrendingDown, MessageSquare, Activity, Mail,
  Ticket as TicketIcon, DollarSign, Crown, Briefcase, Wrench,
  BarChart3, PieChart, CalendarDays,
} from 'lucide-react';
import { soulGradient } from '@/components/soul-card';
import type { Soul } from '@/types';
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip,
  ResponsiveContainer, BarChart, Bar, Cell,
} from 'recharts';
import { DashboardDataProvider } from '@/contexts/dashboard-data';
import { DashboardToolbar } from '@/components/dashboard/dashboard-toolbar';
import { WidgetPicker } from '@/components/dashboard/widget-picker';
import { WidgetConfigModal } from '@/components/dashboard/widget-config-modal';
import { AIWidgetBuilder } from '@/components/dashboard/ai-widget-builder';
import type { WidgetConfig, WidgetType } from '@/components/dashboard/widget-registry';
import type { Layout, LayoutItem } from 'react-grid-layout';

// Dynamic import for DashboardGrid — avoids SSR issues with react-grid-layout
const DashboardGrid = dynamic(
  () => import('@/components/dashboard/dashboard-grid').then((m) => ({ default: m.DashboardGrid })),
  { ssr: false },
);

// ─── Default layout & widgets ────────────────────────────────────────────────

const DEFAULT_LAYOUT: LayoutItem[] = [
  { i: 'kpi-agents',    x: 0,  y: 0, w: 3, h: 2, minW: 2, minH: 2 },
  { i: 'kpi-spend',     x: 3,  y: 0, w: 3, h: 2, minW: 2, minH: 2 },
  { i: 'kpi-sessions',  x: 6,  y: 0, w: 3, h: 2, minW: 2, minH: 2 },
  { i: 'kpi-approvals', x: 9,  y: 0, w: 3, h: 2, minW: 2, minH: 2 },
  { i: 'chart-spend',   x: 0,  y: 2, w: 8, h: 5, minW: 4, minH: 3 },
  { i: 'top-spenders',  x: 8,  y: 2, w: 4, h: 5, minW: 3, minH: 3 },
  { i: 'fleet',         x: 0,  y: 7, w: 4, h: 6, minW: 3, minH: 4 },
  { i: 'activity',      x: 4,  y: 7, w: 4, h: 6, minW: 3, minH: 4 },
  { i: 'approvals',     x: 8,  y: 7, w: 4, h: 6, minW: 3, minH: 4 },
];

const DEFAULT_WIDGETS: Record<string, WidgetConfig> = {
  'kpi-agents':    { id: 'kpi-agents',    title: 'Active Agents',     type: 'metric',   dataSource: 'agent_status_live',   grid: { w: 3, h: 2 } },
  'kpi-spend':     { id: 'kpi-spend',     title: 'Spend Today',       type: 'metric',   dataSource: 'spend_total_today',   grid: { w: 3, h: 2 }, config: { prefix: '$', showTrend: true } },
  'kpi-sessions':  { id: 'kpi-sessions',  title: 'Sessions Today',    type: 'metric',   dataSource: 'session_count_today', grid: { w: 3, h: 2 } },
  'kpi-approvals': { id: 'kpi-approvals', title: 'Pending Approvals', type: 'metric',   dataSource: 'pending_approvals',   grid: { w: 3, h: 2 } },
  'chart-spend':   { id: 'chart-spend',   title: 'Daily Spend (30 days)', type: 'area', dataSource: 'spend_by_provider_30d', grid: { w: 8, h: 5 }, config: { xKey: 'date', yKey: 'cost_usd' } },
  'top-spenders':  { id: 'top-spenders',  title: 'Top Agents',        type: 'bar',      dataSource: 'spend_by_provider_30d', grid: { w: 4, h: 5 }, config: { xKey: 'name', yKey: 'cost_usd' } },
  'fleet':         { id: 'fleet',         title: 'Agent Fleet',       type: 'agents',   dataSource: 'agent_status_live',   grid: { w: 4, h: 6 } },
  'activity':      { id: 'activity',      title: "Today's Activity",  type: 'activity', dataSource: 'agent_runs_per_hour', grid: { w: 4, h: 6 } },
  'approvals':     { id: 'approvals',     title: 'Needs Review',      type: 'tasks',    dataSource: 'pending_approvals',   grid: { w: 4, h: 6 } },
};

// ─── Page component ───────────────────────────────────────────────────────────

export default function DashboardPage() {
  const router = useRouter();

  // ── Existing data state ────────────────────────────────────────────────────
  const [souls, setSouls] = useState<Soul[]>([]);
  const [providerCount, setProviderCount] = useState(0);
  const [sessionCount, setSessionCount] = useState(0);
  const [recentTickets, setRecentTickets] = useState<TicketItem[]>([]);
  const [pendingApprovals, setPendingApprovals] = useState<ApprovalItem[]>([]);
  const [pendingOutbound, setPendingOutbound] = useState<OutboundAction[]>([]);
  const [auditFeed, setAuditFeed] = useState<SupervisorMessage[]>([]);
  const [gwStats, setGwStats] = useState<DashboardStats | null>(null);
  const [roster, setRoster] = useState<OrgRosterEntry[]>([]);
  const [financeDaily, setFinanceDaily] = useState<{ date: string; cost_usd: number; tokens_in: number; tokens_out: number }[]>([]);
  const [financeSummary, setFinanceSummary] = useState<{ agents: OrgAgentSpend[]; total_month_usd: number } | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Track whether we've ever loaded data — after the first load, background
  // refreshes use setRefreshing(true) instead of setLoading(true) so existing
  // content stays visible while new data arrives silently.
  const hasLoadedOnce = useRef(false);

  // ── Grid / customisation state ─────────────────────────────────────────────
  const [gridLayout, setGridLayout] = useState<LayoutItem[]>([]);
  const [widgetConfigs, setWidgetConfigs] = useState<Record<string, WidgetConfig>>({});
  const [isEditing, setIsEditing] = useState(false);
  const [dashboardId, setDashboardId] = useState('');
  const [dashboardName, setDashboardName] = useState('My Dashboard');
  const [showWidgetPicker, setShowWidgetPicker] = useState(false);
  const [showAIBuilder, setShowAIBuilder] = useState(false);
  const [configWidget, setConfigWidget] = useState<WidgetConfig | null>(null);
  const [layoutDirty, setLayoutDirty] = useState(false);

  // ── Data loading ──────────────────────────────────────────────────────────

  const load = useCallback(() => {
    if (hasLoadedOnce.current) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError(null);
    Promise.all([
      agents.list(),
      providers.list().then((d) => d.length).catch(() => 0),
      sessions.list().then((d) => d as any[]).catch(() => []),
      ticketsApi.list().catch(() => [] as TicketItem[]),
      approvalsApi.list().catch(() => [] as ApprovalItem[]),
      outbound.pending().catch(() => ({ pending: [] as OutboundAction[] })),
      supervisor.auditLog().catch(() => ({ messages: [] as SupervisorMessage[] })),
      dashboardApi.stats().catch(() => null),
      orgApi.roster().catch(() => ({ roster: [] as OrgRosterEntry[] })),
      orgApi.financeDaily(30).catch(() => ({ daily: [] })),
      orgApi.financeSummary().catch(() => ({ agents: [] as OrgAgentSpend[], total_month_usd: 0 })),
    ])
      .then(([a, pc, sess, tix, apps, ob, audit, gs, rosterData, dailyData, summaryData]) => {
        const list = (Array.isArray(a) ? a : []).filter((s: any) => !s.agent_key?.startsWith('__'));
        setSouls(list);
        setProviderCount(pc);
        setSessionCount((sess as any[]).length);
        setRecentTickets(Array.isArray(tix) ? (tix as TicketItem[]).slice(0, 6) : []);
        setPendingApprovals((apps as ApprovalItem[]).filter((x) => (x.state ?? x.status) === 'pending'));
        setPendingOutbound(ob?.pending ?? []);
        const todayStart = new Date(); todayStart.setHours(0, 0, 0, 0);
        const msgs = (audit?.messages ?? [])
          .filter((m) => new Date(m.timestamp) >= todayStart)
          .sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
          .slice(0, 12);
        setAuditFeed(msgs);
        if (gs) setGwStats(gs as DashboardStats);
        setRoster((rosterData as any)?.roster ?? []);
        setFinanceDaily((dailyData as any)?.daily ?? []);
        setFinanceSummary(summaryData as any);
        setLoading(false);
        setRefreshing(false);
        hasLoadedOnce.current = true;
      })
      .catch((e) => { setError(e.message); setLoading(false); setRefreshing(false); });
  }, [router]);

  const decideApproval = async (id: string, decision: 'approve' | 'reject') => {
    const snap = pendingApprovals;
    setPendingApprovals((prev) => prev.filter((x) => x.id !== id));
    try { await approvalsApi.decide(id, decision); }
    catch { setPendingApprovals(snap); }
  };

  const decideOutbound = async (id: string, decision: 'approve' | 'reject') => {
    const snap = pendingOutbound;
    setPendingOutbound((prev) => prev.filter((x) => x.id !== id));
    try {
      if (decision === 'approve') await outbound.approve(id);
      else await outbound.reject(id);
    } catch { setPendingOutbound(snap); }
  };

  useEffect(load, [load]);

  // Live refresh: listen for WebSocket events that indicate dashboard data changed
  useEffect(() => {
    let debounceTimer: ReturnType<typeof setTimeout> | null = null;
    const debouncedLoad = () => {
      if (debounceTimer) clearTimeout(debounceTimer);
      debounceTimer = setTimeout(load, 2000);
    };
    const events = [
      'qorven:budget_warning',
      'qorven:runtime_state_changed',
      'qorven:task_done',
      'qorven:task_blocked',
      'qorven:task_progress',
      'qorven:dashboard_refresh',
    ];
    events.forEach((e) => window.addEventListener(e, debouncedLoad));
    // Periodic light refresh every 30s while the tab is focused
    const interval = setInterval(() => {
      if (document.visibilityState === 'visible') load();
    }, 30_000);
    return () => {
      events.forEach((e) => window.removeEventListener(e, debouncedLoad));
      if (debounceTimer) clearTimeout(debounceTimer);
      clearInterval(interval);
    };
  }, [load]);

  // Load saved layout on mount
  useEffect(() => {
    dashboardLayout.get().then((dl) => {
      if (dl.layout && Array.isArray(dl.layout) && dl.layout.length > 0) {
        setGridLayout(dl.layout as LayoutItem[]);
        setWidgetConfigs(dl.widgets || {});
        setDashboardId(dl.id || '');
        setDashboardName(dl.name || 'My Dashboard');
      } else {
        setGridLayout(DEFAULT_LAYOUT);
        setWidgetConfigs(DEFAULT_WIDGETS);
      }
    }).catch(() => {
      setGridLayout(DEFAULT_LAYOUT);
      setWidgetConfigs(DEFAULT_WIDGETS);
    });
  }, []);

  // ── Layout save ───────────────────────────────────────────────────────────

  const saveLayout = useCallback(() => {
    dashboardLayout.save(dashboardName, gridLayout, widgetConfigs).catch(() => {});
    setLayoutDirty(false);
  }, [dashboardName, gridLayout, widgetConfigs]);

  // ── Widget management ─────────────────────────────────────────────────────

  const handleAddWidget = useCallback((config: WidgetConfig) => {
    const id = config.id || `widget-${Date.now()}`;
    const newConfig = { ...config, id };
    // Place new widget at the bottom
    const maxY = gridLayout.reduce((m, item) => Math.max(m, item.y + item.h), 0);
    const newItem: LayoutItem = {
      i: id,
      x: 0,
      y: maxY,
      w: newConfig.grid.w,
      h: newConfig.grid.h,
      minW: 2,
      minH: 2,
    };
    setGridLayout((prev) => [...prev, newItem]);
    setWidgetConfigs((prev) => ({ ...prev, [id]: newConfig }));
    setLayoutDirty(true);
    setShowWidgetPicker(false);
    setShowAIBuilder(false);
  }, [gridLayout]);

  const handleRemoveWidget = useCallback((id: string) => {
    setGridLayout((prev) => prev.filter((item) => item.i !== id));
    setWidgetConfigs((prev) => {
      const next = { ...prev };
      delete next[id];
      return next;
    });
    setLayoutDirty(true);
  }, []);

  const handleConfigWidget = useCallback((id: string) => {
    const cfg = widgetConfigs[id];
    if (cfg) setConfigWidget(cfg);
  }, [widgetConfigs]);

  const handleSaveWidgetConfig = useCallback((updated: WidgetConfig) => {
    setWidgetConfigs((prev) => ({ ...prev, [updated.id]: updated }));
    setConfigWidget(null);
    setLayoutDirty(true);
  }, []);

  const handleLayoutChange = useCallback((newLayout: Layout) => {
    setGridLayout([...newLayout]);
    setLayoutDirty(true);
  }, []);

  // ── Derived values ────────────────────────────────────────────────────────
  const activeAgents = roster.filter((r) => r.status === 'active' || !r.terminated_at).length || souls.filter((s) => s.status === 'active').length;
  const totalApprovals = pendingApprovals.length + pendingOutbound.length;

  return (
    <ErrorBoundary>
      <DashboardDataProvider>
        <div className="flex flex-col gap-4 pb-8">

          <CanvasHeader
            title="Command Center"
            description="Fleet operations, spend, and activity at a glance"
            actions={
              <>
                <button onClick={load} disabled={loading || refreshing}
                  className="qr-btn-outline qr-btn-sm flex items-center gap-2">
                  <RefreshCw className={cn('h-4 w-4', (loading || refreshing) && 'animate-spin')} />
                  Refresh
                </button>
                <button onClick={() => router.push('/qors')}
                  className="qr-btn-primary qr-btn-sm flex items-center gap-2">
                  <Send className="h-4 w-4" />
                  New Chat
                </button>
              </>
            }
          />

          {error && (
            <div className="flex items-center gap-3 rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3 mx-6">
              <AlertCircle className="h-4 w-4 text-destructive shrink-0" />
              <p className="text-sm text-destructive flex-1">{error}</p>
              <button onClick={load} className="text-sm font-medium text-destructive hover:underline">Retry</button>
            </div>
          )}

          {/* Setup checklist — only while onboarding */}
          {!loading && (
            <SetupChecklist
              agents={souls.length} providers={providerCount} sessions={sessionCount}
            />
          )}

          {/* Dashboard toolbar — edit / add / AI */}
          <div className="px-6">
            <DashboardToolbar
              isEditing={isEditing}
              onToggleEdit={() => setIsEditing((v) => !v)}
              onAddWidget={() => setShowWidgetPicker(true)}
              onAskAI={() => setShowAIBuilder(true)}
              onSave={saveLayout}
              dashboardName={dashboardName}
            />
          </div>

          {/* Main customisable grid */}
          <div className="px-6">
            {gridLayout.length > 0 && (
              <DashboardGrid
                layout={gridLayout}
                widgets={widgetConfigs}
                isEditing={isEditing}
                onLayoutChange={handleLayoutChange}
                onRemoveWidget={handleRemoveWidget}
                onConfigWidget={handleConfigWidget}
                onAddWidget={() => setShowWidgetPicker(true)}
              />
            )}
          </div>

          {/* Quick links */}
          <div className="grid gap-3 grid-cols-2 sm:grid-cols-4 px-6">
            <QuickLink href="/models-hub" icon={Cpu} label="Models Hub" desc="Configure LLM providers" />
            <QuickLink href="/channels" icon={Zap} label="Channels" desc="Connect integrations" />
            <QuickLink href="/code?tab=inbox" icon={ShieldCheck} label="Inbox" desc="Approvals and escalations" />
            <QuickLink href="/settings" icon={Settings} label="Settings" desc="Workspace preferences" />
          </div>

        </div>

        {/* Widget picker sheet */}
        <WidgetPicker
          open={showWidgetPicker}
          onClose={() => setShowWidgetPicker(false)}
          onAdd={handleAddWidget}
        />

        {/* Widget config modal */}
        <WidgetConfigModal
          open={configWidget !== null}
          widget={configWidget}
          onClose={() => setConfigWidget(null)}
          onSave={handleSaveWidgetConfig}
          onRemove={(id) => { handleRemoveWidget(id); setConfigWidget(null); }}
        />

        {/* AI widget builder dialog */}
        <AIWidgetBuilder
          open={showAIBuilder}
          onClose={() => setShowAIBuilder(false)}
          onAdd={handleAddWidget}
        />

      </DashboardDataProvider>
    </ErrorBoundary>
  );
}

// ─── KPI Card ────────────────────────────────────────────────────────────────

const TIER_META = {
  l1: { label: 'Executive', icon: Crown },
  l2: { label: 'Management', icon: Briefcase },
  l3: { label: 'Specialist', icon: Wrench },
} as const;

function KpiCard({ label, value, icon: Icon, trend, alert, href }: {
  label: string;
  value: string | null;
  icon: React.ElementType;
  trend?: { direction: 'up' | 'down'; pct: string };
  alert?: boolean;
  href: string;
}) {
  return (
    <Link href={href} className="group">
      <Card className="transition-colors group-hover:border-primary/30 group-hover:bg-accent/40">
        <CardContent className="p-4">
          <div className="flex items-center gap-3">
            <div className={cn(
              'flex h-9 w-9 items-center justify-center rounded-lg shrink-0',
              alert ? 'bg-amber-500/10 text-amber-500' : 'bg-primary/10 text-primary'
            )}>
              <Icon className="h-4 w-4" />
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-2xs text-muted-foreground">{label}</p>
              {value === null ? (
                <div className="mt-1 h-5 w-16 rounded bg-muted animate-pulse" />
              ) : (
                <div className="flex items-center gap-2 mt-0.5">
                  <p className="font-mono text-lg font-semibold tabular-nums leading-tight">{value}</p>
                  {trend && (
                    <span className={cn(
                      'flex items-center gap-0.5 text-2xs font-medium',
                      trend.direction === 'up' ? 'text-emerald-500' : 'text-destructive'
                    )}>
                      {trend.direction === 'up'
                        ? <TrendingUp className="h-3 w-3" />
                        : <TrendingDown className="h-3 w-3" />}
                      {trend.pct}
                    </span>
                  )}
                </div>
              )}
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

function calcTrend(daily: { cost_usd: number }[]): { direction: 'up' | 'down'; pct: string } | undefined {
  if (daily.length < 7) return undefined;
  const recent = daily.slice(-7).reduce((s, d) => s + d.cost_usd, 0);
  const prev = daily.slice(-14, -7).reduce((s, d) => s + d.cost_usd, 0);
  if (prev === 0) return undefined;
  const change = ((recent - prev) / prev) * 100;
  return { direction: change >= 0 ? 'up' : 'down', pct: `${Math.abs(change).toFixed(0)}%` };
}

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

const outboundLabels: Record<string, string> = {
  email_send: 'Send Email',
  telegram_send: 'Send Telegram Message',
  slack_send: 'Send Slack Message',
  discord_send: 'Send Discord Message',
  whatsapp_send: 'Send WhatsApp Message',
  social_post: 'Post to Social Media',
  webhook_send: 'Fire Webhook',
  sms_send: 'Send SMS',
};

const intentLabels: Record<string, string> = {
  STATUS_REQUEST: 'Status check',
  REVIEW_REQUEST: 'Review requested',
  ACK: 'Task confirmed',
  ESCALATION_NOTICE: 'Escalation raised',
  AUTO_FIX: 'Auto-fix applied',
  HEARTBEAT: 'Agent heartbeat',
};

function friendlyOutboundLabel(actionType: string): string {
  return outboundLabels[actionType] ?? actionType.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

function friendlyIntent(intent: string): string {
  return intentLabels[intent] ?? intent.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

function friendlyAgentId(id: string, souls: Soul[] = []): string {
  if (!id) return '';
  if (id === 'human') return 'You';
  if (id === 'prime' || id === 'supervisor') return 'Supervisor';
  const soul = souls.find(s => s.id === id || s.agent_key === id);
  if (soul) return soul.display_name;
  if (id.length > 12 && id.includes('-')) return 'Agent';
  return id;
}

// ─── Approval Row ────────────────────────────────────────────────────────────

function ApprovalRow({ item: a, onDecide }: { item: ApprovalItem; onDecide: (id: string, d: 'approve' | 'reject') => void | Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const act = async (d: 'approve' | 'reject') => { setBusy(true); await onDecide(a.id, d); setBusy(false); };
  const label = a.kind === 'tool' && a.tool_name
    ? a.tool_name.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
    : a.kind === 'plan' ? 'Plan step approval'
    : a.kind ? a.kind.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
    : 'Approval';
  return (
    <div className="flex items-center gap-2.5 rounded-lg px-2 py-2">
      <div className={cn(
        'flex h-7 w-7 items-center justify-center rounded-md shrink-0',
        a.kind === 'tool' ? 'bg-blue-500/10 text-blue-500' : 'bg-primary/10 text-primary',
      )}>
        {a.kind === 'tool' ? <Cpu className="h-3.5 w-3.5" /> : <GitBranch className="h-3.5 w-3.5" />}
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-xs font-medium text-foreground truncate">{label}</p>
        <p className="text-2xs text-muted-foreground truncate mt-0.5">{a.reason || 'Needs approval'}</p>
      </div>
      <div className="flex items-center gap-1 shrink-0">
        <button onClick={() => act('approve')} disabled={busy}
          className="flex h-6 w-6 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/20 disabled:opacity-50 transition-colors">
          {busy ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />}
        </button>
        <button onClick={() => act('reject')} disabled={busy}
          className="flex h-6 w-6 items-center justify-center rounded-md border border-border text-muted-foreground hover:text-destructive hover:border-destructive/40 disabled:opacity-50 transition-colors">
          <X className="h-3 w-3" />
        </button>
      </div>
    </div>
  );
}

// ─── Outbound Row ────────────────────────────────────────────────────────────

function OutboundRow({ item: ob, onDecide }: {
  item: OutboundAction;
  onDecide: (id: string, d: 'approve' | 'reject') => void | Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const act = async (d: 'approve' | 'reject') => { setBusy(true); await onDecide(ob.id, d); setBusy(false); };
  const label = friendlyOutboundLabel(ob.action_type);
  return (
    <div className="flex items-center gap-2.5 rounded-lg px-2 py-2">
      <div className="flex h-7 w-7 items-center justify-center rounded-md bg-violet-500/10 text-violet-500 shrink-0">
        <Mail className="h-3.5 w-3.5" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-xs font-medium text-foreground truncate">{label}</p>
        <p className="text-2xs text-muted-foreground truncate mt-0.5">Needs approval to send</p>
      </div>
      <div className="flex items-center gap-1 shrink-0">
        <button onClick={() => act('approve')} disabled={busy}
          className="flex h-6 w-6 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/20 disabled:opacity-50 transition-colors">
          {busy ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />}
        </button>
        <button onClick={() => act('reject')} disabled={busy}
          className="flex h-6 w-6 items-center justify-center rounded-md border border-border text-muted-foreground hover:text-destructive hover:border-destructive/40 disabled:opacity-50 transition-colors">
          <X className="h-3 w-3" />
        </button>
      </div>
    </div>
  );
}

// ─── Setup Checklist ─────────────────────────────────────────────────────────

function SetupChecklist({ agents, providers, sessions }: { agents: number; providers: number; sessions: number }) {
  const steps = [
    { label: 'Admin account created', done: true, href: '/settings' },
    { label: 'LLM provider configured', done: providers > 0, href: '/provider-keys' },
    { label: `${brand.agentNamePlural} ready`, done: agents > 0, href: '/qors' },
    { label: 'First conversation', done: sessions > 0, href: '/qors' },
  ];
  const completed = steps.filter((s) => s.done).length;
  if (completed === steps.length) return null;
  return (
    <div className="rounded-xl border border-primary/20 bg-primary/5 p-5 mx-6">
      <div className="flex items-center gap-4 mb-4">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10 shrink-0">
          <TrendingUp className="h-5 w-5 text-primary" />
        </div>
        <div className="flex-1">
          <h3 className="text-sm font-semibold text-foreground">Getting started</h3>
          <p className="text-xs text-muted-foreground mt-0.5">{completed} of {steps.length} steps completed</p>
        </div>
        <div className="flex items-center gap-3">
          <Progress value={(completed / steps.length) * 100} className="h-2 w-32" />
          <span className="text-sm font-semibold text-primary tabular-nums">{Math.round((completed / steps.length) * 100)}%</span>
        </div>
      </div>
      <div className="grid gap-2 grid-cols-2 sm:grid-cols-4">
        {steps.map((step, i) => (
          <Link key={i} href={step.href}
            className={cn('flex items-center gap-2 rounded-lg px-3 py-2 transition-colors', step.done ? 'opacity-50 cursor-default' : 'hover:bg-primary/10')}>
            {step.done
              ? <CheckCircle className="h-4 w-4 text-primary shrink-0" />
              : <Circle className="h-4 w-4 text-muted-foreground shrink-0" />}
            <span className={cn('text-xs', step.done ? 'line-through text-muted-foreground' : 'text-foreground font-medium')}>
              {step.label}
            </span>
          </Link>
        ))}
      </div>
    </div>
  );
}

// ─── Quick Link ──────────────────────────────────────────────────────────────

function QuickLink({ href, icon: Icon, label, desc }: { href: string; icon: React.ElementType; label: string; desc: string }) {
  return (
    <Link href={href}
      className="group flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3.5 hover:border-primary/30 hover:bg-accent transition-colors">
      <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0 group-hover:bg-primary/20 transition-colors">
        <Icon className="h-4 w-4" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-foreground">{label}</p>
        <p className="text-xs text-muted-foreground truncate mt-0.5">{desc}</p>
      </div>
      <ArrowUpRight className="h-4 w-4 text-muted-foreground/40 group-hover:text-primary transition-colors shrink-0" />
    </Link>
  );
}

// ─── Shared Helpers ──────────────────────────────────────────────────────────

function RowSkeleton() {
  return (
    <div className="flex items-center gap-2.5 px-2 py-2">
      <div className="h-6 w-6 rounded-md animate-pulse bg-muted shrink-0" />
      <div className="flex-1 space-y-1.5">
        <div className="h-3 w-28 animate-pulse rounded bg-muted" />
        <div className="h-2.5 w-16 animate-pulse rounded bg-muted" />
      </div>
    </div>
  );
}

function EmptyPanel({ icon, label }: { icon: React.ReactNode; label: string }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-8 text-center">
      <div className="text-muted-foreground/30">{icon}</div>
      <p className="text-sm text-muted-foreground">{label}</p>
    </div>
  );
}
