'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import {
  agents, sessions, providers, approvals as approvalsApi,
  outbound, supervisor,
  type ApprovalItem, type OutboundAction, type SupervisorMessage,
} from '@/lib/api';
import { dashboardApi, type PinnedTile, type DashboardStats } from '@/lib/api-dashboard';
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

export default function DashboardPage() {
  const router = useRouter();
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
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
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
        if (pc === 0) router.replace('/setup');
      })
      .catch((e) => { setError(e.message); setLoading(false); });
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

  const activeAgents = roster.filter((r) => r.status === 'active' || !r.terminated_at).length || souls.filter((s) => s.status === 'active').length;
  const totalApprovals = pendingApprovals.length + pendingOutbound.length;

  return (
    <ErrorBoundary>
      <div className="flex flex-col gap-6 pb-8">

        <CanvasHeader
          title="Command Center"
          description="Fleet operations, spend, and activity at a glance"
          actions={
            <>
              <button onClick={load} disabled={loading}
                className="qr-btn-outline qr-btn-sm flex items-center gap-2">
                <RefreshCw className={cn('h-4 w-4', loading && 'animate-spin')} />
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

        {/* ── KPI Strip ── */}
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 px-6">
          <KpiCard
            label="Spend this month"
            value={loading ? null : `$${(financeSummary?.total_month_usd ?? gwStats?.cost_this_month_usd ?? 0).toFixed(2)}`}
            icon={DollarSign}
            trend={financeSummary && financeDaily.length > 1 ? calcTrend(financeDaily) : undefined}
            href="/usage"
          />
          <KpiCard
            label="Active agents"
            value={loading ? null : String(activeAgents)}
            icon={Users}
            href="/qors"
          />
          <KpiCard
            label="Sessions today"
            value={loading ? null : String(auditFeed.length)}
            icon={Activity}
            href="/audit"
          />
          <KpiCard
            label="Pending approvals"
            value={loading ? null : String(totalApprovals)}
            icon={ShieldCheck}
            alert={totalApprovals > 0}
            href="/approvals"
          />
        </div>

        {/* ── Spend Chart + Top Spenders ── */}
        <div className="grid gap-6 lg:grid-cols-3 px-6">
          <Card className="lg:col-span-2">
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-sm">
                <BarChart3 className="h-4 w-4 text-muted-foreground" />
                Daily Spend (30 days)
              </CardTitle>
            </CardHeader>
            <CardContent className="pt-0">
              {loading ? (
                <div className="h-[200px] flex items-center justify-center">
                  <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                </div>
              ) : financeDaily.length === 0 ? (
                <div className="h-[200px] flex items-center justify-center text-sm text-muted-foreground">
                  No spend data yet
                </div>
              ) : (
                <ResponsiveContainer width="100%" height={200}>
                  <AreaChart data={financeDaily}>
                    <defs>
                      <linearGradient id="spendGradient" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor="var(--chart-1)" stopOpacity={0.3} />
                        <stop offset="95%" stopColor="var(--chart-1)" stopOpacity={0} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                    <XAxis
                      dataKey="date"
                      tick={{ fontSize: 11 }}
                      tickFormatter={(v) => v.slice(5)}
                      stroke="var(--muted-foreground)"
                    />
                    <YAxis
                      tick={{ fontSize: 11 }}
                      tickFormatter={(v) => `$${v}`}
                      stroke="var(--muted-foreground)"
                      width={50}
                    />
                    <Tooltip
                      contentStyle={{
                        background: 'var(--card)',
                        border: '1px solid var(--border)',
                        borderRadius: 8,
                        fontSize: 12,
                      }}
                      formatter={(value: number) => [`$${value.toFixed(4)}`, 'Cost']}
                      labelFormatter={(l) => l}
                    />
                    <Area
                      type="monotone"
                      dataKey="cost_usd"
                      stroke="var(--chart-1)"
                      fill="url(#spendGradient)"
                      strokeWidth={2}
                    />
                  </AreaChart>
                </ResponsiveContainer>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-sm">
                <PieChart className="h-4 w-4 text-muted-foreground" />
                Top Spenders
              </CardTitle>
            </CardHeader>
            <CardContent className="pt-0">
              {loading ? (
                <div className="space-y-3">
                  {Array.from({ length: 4 }).map((_, i) => (
                    <div key={i} className="h-8 rounded bg-muted animate-pulse" />
                  ))}
                </div>
              ) : !financeSummary || financeSummary.agents.length === 0 ? (
                <p className="text-sm text-muted-foreground py-4 text-center">No spend data</p>
              ) : (
                <div className="space-y-3">
                  {financeSummary.agents.slice(0, 5).map((agent) => {
                    const pct = financeSummary.total_month_usd > 0
                      ? (agent.month_cost_usd / financeSummary.total_month_usd) * 100
                      : 0;
                    return (
                      <div key={agent.agent_id} className="space-y-1.5">
                        <div className="flex items-center justify-between">
                          <span className="text-xs font-medium text-foreground truncate">
                            {agent.display_name || agent.org_role || 'Unknown'}
                          </span>
                          <span className="text-xs font-mono text-muted-foreground tabular-nums">
                            ${agent.month_cost_usd.toFixed(2)}
                          </span>
                        </div>
                        <Progress
                          value={pct}
                          className="h-1.5"
                          indicatorClassName={cn(
                            pct > 80 ? 'bg-destructive' :
                            pct > 50 ? 'bg-amber-500' :
                            'bg-primary'
                          )}
                        />
                      </div>
                    );
                  })}
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        {/* ── Fleet + Activity + Approvals ── */}
        <div className="grid gap-6 lg:grid-cols-3 px-6">

          {/* Fleet Status */}
          <Card className="lg:col-span-1">
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-sm">
                <Users className="h-4 w-4 text-muted-foreground" />
                Fleet
              </CardTitle>
              <Link href="/qors" className="text-xs text-primary hover:underline flex items-center gap-1">
                All <ArrowUpRight className="h-3 w-3" />
              </Link>
            </CardHeader>
            <CardContent className="pt-0">
              {loading ? (
                <div className="space-y-2">
                  {Array.from({ length: 4 }).map((_, i) => <RowSkeleton key={i} />)}
                </div>
              ) : roster.length === 0 ? (
                <EmptyPanel icon={<Bot className="h-5 w-5" />} label="No agents hired yet" />
              ) : (
                <div className="space-y-4">
                  {(['l1', 'l2', 'l3'] as const).map((tier) => {
                    const tierAgents = roster.filter((r) => r.org_level === tier);
                    if (tierAgents.length === 0) return null;
                    const meta = TIER_META[tier];
                    return (
                      <div key={tier} className="space-y-1.5">
                        <div className="flex items-center gap-2">
                          <meta.icon className="h-3 w-3 text-muted-foreground" />
                          <span className="text-2xs font-medium text-muted-foreground uppercase tracking-wide">
                            {meta.label}
                          </span>
                          <Badge variant="secondary" size="xs" appearance="light">
                            {tierAgents.length}
                          </Badge>
                        </div>
                        <div className="space-y-0.5">
                          {tierAgents.slice(0, 4).map((agent) => (
                            <Link
                              key={agent.id}
                              href={`/qors/${agent.id}`}
                              className="flex items-center gap-2.5 rounded-lg px-2 py-1.5 hover:bg-accent transition-colors"
                            >
                              <span className={cn(
                                'h-2 w-2 rounded-full shrink-0',
                                agent.status === 'active' ? 'bg-soul-idle' :
                                agent.terminated_at ? 'bg-soul-offline' :
                                'bg-soul-running'
                              )} />
                              <span className="text-xs font-medium text-foreground truncate flex-1">
                                {agent.display_name}
                              </span>
                              {agent.org_role && (
                                <span className="text-2xs text-muted-foreground uppercase">
                                  {agent.org_role}
                                </span>
                              )}
                            </Link>
                          ))}
                          {tierAgents.length > 4 && (
                            <p className="text-2xs text-muted-foreground pl-2">
                              +{tierAgents.length - 4} more
                            </p>
                          )}
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </CardContent>
          </Card>

          {/* Activity Feed */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-sm">
                <Activity className="h-4 w-4 text-muted-foreground" />
                Today&apos;s Activity
              </CardTitle>
              <Link href="/audit" className="text-xs text-primary hover:underline flex items-center gap-1">
                Work Log <ArrowUpRight className="h-3 w-3" />
              </Link>
            </CardHeader>
            <CardContent className="pt-0">
              {loading ? (
                <div className="space-y-2">
                  {Array.from({ length: 5 }).map((_, i) => <RowSkeleton key={i} />)}
                </div>
              ) : auditFeed.length === 0 ? (
                <EmptyPanel icon={<MessageSquare className="h-5 w-5" />} label="No activity yet today" />
              ) : (
                <div className="space-y-0.5">
                  {auditFeed.map((m) => {
                    const intentLabel = friendlyIntent(m.intent);
                    const from = friendlyAgentId(m.from, souls);
                    const isEscalation = m.intent === 'ESCALATION_NOTICE';
                    const isAck = m.intent === 'ACK';
                    return (
                      <div key={m.id} className="flex items-start gap-2.5 rounded-lg px-2 py-2 hover:bg-accent/50 transition-colors">
                        <div className={cn(
                          'flex h-6 w-6 items-center justify-center rounded-md shrink-0 mt-0.5',
                          isEscalation ? 'bg-amber-500/10 text-amber-500' :
                          isAck ? 'bg-emerald-500/10 text-emerald-500' :
                          'bg-primary/10 text-primary'
                        )}>
                          {isEscalation ? <AlertCircle className="h-3 w-3" /> :
                           isAck ? <CheckCircle className="h-3 w-3" /> :
                           <Zap className="h-3 w-3" />}
                        </div>
                        <div className="min-w-0 flex-1">
                          <p className="text-xs font-medium text-foreground truncate">{intentLabel}</p>
                          <p className="text-2xs text-muted-foreground truncate mt-0.5">
                            {from} &middot; {new Date(m.timestamp).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })}
                          </p>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </CardContent>
          </Card>

          {/* Approvals */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-sm">
                <ShieldCheck className="h-4 w-4 text-muted-foreground" />
                Needs Review
                {totalApprovals > 0 && (
                  <Badge variant="warning" appearance="light" size="xs">{totalApprovals}</Badge>
                )}
              </CardTitle>
              <Link href="/approvals" className="text-xs text-primary hover:underline flex items-center gap-1">
                All <ArrowUpRight className="h-3 w-3" />
              </Link>
            </CardHeader>
            <CardContent className="pt-0">
              {loading ? (
                <div className="space-y-2">
                  {Array.from({ length: 3 }).map((_, i) => <RowSkeleton key={i} />)}
                </div>
              ) : totalApprovals === 0 ? (
                <EmptyPanel icon={<CheckCircle className="h-5 w-5 text-emerald-400" />} label="All clear" />
              ) : (
                <div className="space-y-0.5">
                  {pendingApprovals.slice(0, 4).map((a) => (
                    <ApprovalRow key={a.id} item={a} onDecide={decideApproval} />
                  ))}
                  {pendingOutbound.slice(0, 3).map((ob) => (
                    <OutboundRow key={ob.id} item={ob} onDecide={decideOutbound} />
                  ))}
                </div>
              )}
            </CardContent>
          </Card>

        </div>

        {/* ── Quick links ── */}
        <div className="grid gap-3 grid-cols-2 sm:grid-cols-4 px-6">
          <QuickLink href="/models-hub" icon={Cpu} label="Models Hub" desc="Configure LLM providers" />
          <QuickLink href="/channels" icon={Zap} label="Channels" desc="Connect integrations" />
          <QuickLink href="/code?tab=inbox" icon={ShieldCheck} label="Inbox" desc="Approvals and escalations" />
          <QuickLink href="/settings" icon={Settings} label="Settings" desc="Workspace preferences" />
        </div>

      </div>
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
