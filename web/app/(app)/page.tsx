'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import {
  agents, sessions, providers, approvals as approvalsApi,
  outbound, supervisor,
  type ApprovalItem, type OutboundAction, type SupervisorMessage,
} from '@/lib/api';
import { dashboardApi, type PinnedTile } from '@/lib/api-dashboard';
import { tickets as ticketsApi } from '@/lib/api-workspace';
import type { Ticket as TicketItem } from '@/types';
import { ErrorBoundary } from '@/components/error-boundary';
import { brand } from '@/lib/branding';
import {
  AlertCircle, CheckCircle, Circle, Sparkles, Cpu,
  Users, Zap, Plus, Send, Settings, ArrowUpRight,
  ListChecks, GitBranch, ShieldCheck, Check, X, Loader2, RefreshCw,
  Bot, Clock, TrendingUp, MessageSquare, Activity, Mail, Ticket as TicketIcon,
} from 'lucide-react';
import { soulGradient } from '@/components/soul-card';
import type { Soul } from '@/types';
import Link from 'next/link';
import { cn } from '@/lib/utils';

export default function DashboardPage() {
  const router = useRouter();
  const [souls, setSouls] = useState<Soul[]>([]);
  const [providerCount, setProviderCount] = useState(0);
  const [sessionCount, setSessionCount] = useState(0);
  const [recentTickets, setRecentTickets] = useState<TicketItem[]>([]);
  const [pendingApprovals, setPendingApprovals] = useState<ApprovalItem[]>([]);
  const [pendingOutbound, setPendingOutbound] = useState<OutboundAction[]>([]);
  const [auditFeed, setAuditFeed] = useState<SupervisorMessage[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = () => {
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
    ])
      .then(([a, pc, sess, tix, apps, ob, audit]) => {
        const list = (Array.isArray(a) ? a : []).filter((s: any) => !s.agent_key?.startsWith('__'));
        setSouls(list);
        setProviderCount(pc);
        setSessionCount((sess as any[]).length);
        setRecentTickets(Array.isArray(tix) ? (tix as TicketItem[]).slice(0, 6) : []);
        setPendingApprovals((apps as ApprovalItem[]).filter((x) => (x.state ?? x.status) === 'pending'));
        setPendingOutbound(ob?.pending ?? []);
        // Show today's supervisor messages, newest first, capped at 15.
        const todayStart = new Date(); todayStart.setHours(0, 0, 0, 0);
        const msgs = (audit?.messages ?? [])
          .filter((m) => new Date(m.timestamp) >= todayStart)
          .sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())
          .slice(0, 15);
        setAuditFeed(msgs);
        setLoading(false);
        if (pc === 0) router.replace('/setup');
      })
      .catch((e) => { setError(e.message); setLoading(false); });
  };

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

  useEffect(load, []);

  const active = souls.filter((s) => s.status === 'active').length;
  const inProgressTickets = recentTickets.filter((t: TicketItem) => t.status === 'in_progress').length;

  return (
    <ErrorBoundary>
      <div className="flex flex-col gap-6 pb-8">

        {/* Header */}
        <div className="flex items-center justify-between gap-4 flex-wrap">
          <div>
            <h1 className="text-xl font-semibold text-foreground">{brand.platformName}</h1>
            <p className="text-sm text-muted-foreground mt-0.5">
              {brand.supervisorName}&apos;s command center — what needs you, what&apos;s running, what happened today
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button onClick={load} disabled={loading}
              className="flex h-9 items-center gap-2 rounded-lg border border-border bg-input px-3 text-sm text-muted-foreground hover:bg-accent transition-colors disabled:opacity-50">
              <RefreshCw className={cn('h-4 w-4', loading && 'animate-spin')} />
              Refresh
            </button>
            <button onClick={() => router.push('/qors')}
              className="flex h-9 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors">
              <Send className="h-4 w-4" />
              New Chat
            </button>
          </div>
        </div>

        {error && (
          <div className="flex items-center gap-3 rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3">
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

        {/* ── Pinned tiles row ── */}
        <PinnedTilesRow />

        {/* ── 3-col layout ── */}
        <div className="grid gap-6 lg:grid-cols-3">

          {/* LEFT — Approval inbox */}
          <div className="flex flex-col gap-6">

            {/* Plan / tool approvals */}
            <Panel
              icon={<ShieldCheck className="h-4 w-4 text-amber-500" />}
              title="Needs your review"
              count={loading ? undefined : pendingApprovals.length + pendingOutbound.length}
              countColor="amber"
              action={<Link href="/approvals" className="text-sm text-primary hover:underline flex items-center gap-1">All <ArrowUpRight className="h-3.5 w-3.5" /></Link>}
              accent="amber"
            >
              {loading ? (
                Array.from({ length: 3 }).map((_, i) => <RowSkeleton key={i} />)
              ) : pendingApprovals.length === 0 && pendingOutbound.length === 0 ? (
                <EmptyPanel icon={<CheckCircle className="h-5 w-5 text-emerald-400" />} label="Nothing pending — all clear" />
              ) : (
                <>
                  {pendingApprovals.slice(0, 4).map((a) => (
                    <ApprovalRow key={a.id} item={a} onDecide={decideApproval} />
                  ))}
                  {pendingOutbound.slice(0, 3).map((ob) => (
                    <OutboundRow key={ob.id} item={ob} onDecide={decideOutbound} />
                  ))}
                </>
              )}
            </Panel>

          </div>

          {/* MIDDLE — Active Hubs + active Qors */}
          <div className="flex flex-col gap-6">

            {/* Recent Tickets */}
            <Panel
              icon={<TicketIcon className="h-4 w-4 text-violet-500" />}
              title="Recent Tickets"
              count={loading ? undefined : inProgressTickets}
              countColor="blue"
              action={<Link href="/code?tab=tickets" className="text-sm text-primary hover:underline flex items-center gap-1">All <ArrowUpRight className="h-3.5 w-3.5" /></Link>}
            >
              {loading ? (
                Array.from({ length: 4 }).map((_, i) => <RowSkeleton key={i} />)
              ) : recentTickets.length === 0 ? (
                <EmptyPanel
                  icon={<TicketIcon className="h-5 w-5" />}
                  label="No tickets yet"
                  action={<Link href="/code?tab=tickets" className="text-xs text-primary hover:underline flex items-center gap-1"><Plus className="h-3 w-3" />Create ticket</Link>}
                />
              ) : (
                recentTickets.map((t) => (
                  <Link key={t.id} href="/code?tab=tickets"
                    className="flex items-center gap-3 rounded-lg px-3 py-2.5 hover:bg-accent transition-colors">
                    <div className={cn('flex h-8 w-8 items-center justify-center rounded-lg shrink-0',
                      t.status === 'in_progress' ? 'bg-blue-500/10 text-blue-500' :
                      t.status === 'done'        ? 'bg-emerald-500/10 text-emerald-500' :
                      t.status === 'blocked'     ? 'bg-destructive/10 text-destructive' :
                                                   'bg-muted text-muted-foreground')}>
                      <TicketIcon className="h-4 w-4" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-foreground truncate">{t.title}</p>
                      <p className="text-xs text-muted-foreground mt-0.5 capitalize">{t.status.replace('_', ' ')} · {t.priority}</p>
                    </div>
                  </Link>
                ))
              )}
            </Panel>

            {/* Active Qors */}
            <Panel
              icon={<Sparkles className="h-4 w-4 text-primary" />}
              title={`Active ${brand.agentNamePlural}`}
              count={loading ? undefined : active}
              countColor="emerald"
              action={<Link href="/qors" className="text-sm text-primary hover:underline flex items-center gap-1">All <ArrowUpRight className="h-3.5 w-3.5" /></Link>}
            >
              {loading ? (
                Array.from({ length: 4 }).map((_, i) => <RowSkeleton key={i} />)
              ) : souls.length === 0 ? (
                <EmptyPanel
                  icon={<Bot className="h-5 w-5" />}
                  label={`No ${brand.agentNamePlural.toLowerCase()} yet`}
                  action={<Link href="/qors" className="text-xs text-primary hover:underline flex items-center gap-1"><Plus className="h-3 w-3" />Create one</Link>}
                />
              ) : (
                souls.filter((s) => s.status === 'active').slice(0, 5).concat(
                  souls.filter((s) => s.status !== 'active').slice(0, Math.max(0, 5 - souls.filter((s) => s.status === 'active').length))
                ).map((s) => (
                  <Link key={s.id} href={`/qors/${s.id}`}
                    className="flex items-center gap-3 rounded-lg px-3 py-2.5 hover:bg-accent transition-colors">
                    <div className={cn(
                      'flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold text-white shrink-0 bg-gradient-to-br',
                      soulGradient(s.display_name),
                    )}>
                      {s.display_name?.[0]?.toUpperCase() ?? '?'}
                    </div>
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-foreground truncate">{s.display_name}</p>
                      <p className="text-xs text-muted-foreground truncate mt-0.5">{s.role || s.model || 'Agent'}</p>
                    </div>
                    <span className={cn(
                      'h-2 w-2 rounded-full shrink-0',
                      s.status === 'active' ? 'bg-emerald-400' : 'bg-muted-foreground/30',
                    )} />
                  </Link>
                ))
              )}
            </Panel>

          </div>

          {/* RIGHT — Today's activity feed */}
          <Panel
            icon={<Activity className="h-4 w-4 text-primary" />}
            title="Today's activity"
            action={<Link href="/supervisor" className="text-sm text-primary hover:underline flex items-center gap-1">Supervisor <ArrowUpRight className="h-3.5 w-3.5" /></Link>}
          >
            {loading ? (
              Array.from({ length: 6 }).map((_, i) => <RowSkeleton key={i} />)
            ) : auditFeed.length === 0 ? (
              <EmptyPanel icon={<MessageSquare className="h-5 w-5" />} label="No activity yet today" />
            ) : (
              auditFeed.map((m) => (
                <div key={m.id} className="flex items-start gap-3 px-3 py-2.5">
                  <div className="flex h-7 w-7 items-center justify-center rounded-md bg-primary/10 text-primary shrink-0 mt-0.5">
                    <Zap className="h-3.5 w-3.5" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="text-xs font-medium text-foreground truncate">
                      <span className="text-muted-foreground">{m.from}</span>
                      {' → '}
                      <span className="text-muted-foreground">{m.to}</span>
                    </p>
                    <p className="text-xs text-foreground/80 truncate mt-0.5">{m.intent}</p>
                    <p className="text-2xs text-muted-foreground mt-0.5">
                      {new Date(m.timestamp).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })}
                    </p>
                  </div>
                </div>
              ))
            )}
          </Panel>

        </div>

        {/* Quick links */}
        <div className="grid gap-3 grid-cols-2 sm:grid-cols-4">
          <QuickLink href="/models-hub"      icon={Cpu}        label="Models Hub"   desc="Configure LLM providers" />
          <QuickLink href="/channels"      icon={Zap}        label="Channels"     desc="Connect integrations" />
          <QuickLink href="/code?tab=inbox" icon={ShieldCheck} label="Inbox"      desc="Approvals and escalations" />
          <QuickLink href="/settings"      icon={Settings}   label="Settings"     desc="Workspace preferences" />
        </div>

      </div>
    </ErrorBoundary>
  );
}

// ─── Outbound row ─────────────────────────────────────────────────────────────

function OutboundRow({ item: ob, onDecide }: {
  item: OutboundAction;
  onDecide: (id: string, d: 'approve' | 'reject') => void | Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const act = async (d: 'approve' | 'reject') => { setBusy(true); await onDecide(ob.id, d); setBusy(false); };
  return (
    <div className="flex items-center gap-3 px-3 py-2.5">
      <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-violet-500/10 text-violet-500 shrink-0">
        <Mail className="h-4 w-4" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-foreground truncate">{ob.action_type}</p>
        <p className="text-xs text-muted-foreground truncate mt-0.5">Outbound · {ob.agent_id?.slice(0, 8)}</p>
      </div>
      <div className="flex items-center gap-1.5 shrink-0">
        <button onClick={() => act('approve')} disabled={busy}
          className="flex h-7 w-7 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/20 disabled:opacity-50 transition-colors">
          {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}
        </button>
        <button onClick={() => act('reject')} disabled={busy}
          className="flex h-7 w-7 items-center justify-center rounded-md border border-border text-muted-foreground hover:text-destructive hover:border-destructive/40 disabled:opacity-50 transition-colors">
          <X className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  );
}

// ─── Approval row ─────────────────────────────────────────────────────────────

function ApprovalRow({ item: a, onDecide }: { item: ApprovalItem; onDecide: (id: string, d: 'approve' | 'reject') => void | Promise<void> }) {
  const [busy, setBusy] = useState(false);
  const act = async (d: 'approve' | 'reject') => { setBusy(true); await onDecide(a.id, d); setBusy(false); };
  const label = a.kind === 'tool' && a.tool_name ? a.tool_name
    : a.kind === 'plan' ? (a.node_id ? `Plan ${String(a.node_id).slice(0, 8)}` : 'Plan step')
    : a.kind || 'Approval';
  return (
    <div className="flex items-center gap-3 px-3 py-2.5">
      <div className={cn(
        'flex h-8 w-8 items-center justify-center rounded-lg shrink-0',
        a.kind === 'tool' ? 'bg-blue-500/10 text-blue-500' : 'bg-primary/10 text-primary',
      )}>
        {a.kind === 'tool' ? <Cpu className="h-4 w-4" /> : <GitBranch className="h-4 w-4" />}
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-foreground truncate">{label}</p>
        <p className="text-xs text-muted-foreground truncate mt-0.5">{a.reason || a.requested_by || (a.kind === 'tool' ? 'Tool approval required' : 'Plan approval required')}</p>
      </div>
      <div className="flex items-center gap-1.5 shrink-0">
        <button onClick={() => act('approve')} disabled={busy}
          className="flex h-7 w-7 items-center justify-center rounded-md bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/20 disabled:opacity-50 transition-colors">
          {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}
        </button>
        <button onClick={() => act('reject')} disabled={busy}
          className="flex h-7 w-7 items-center justify-center rounded-md border border-border text-muted-foreground hover:text-destructive hover:border-destructive/40 disabled:opacity-50 transition-colors">
          <X className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  );
}

// ─── Panel wrapper ────────────────────────────────────────────────────────────

function Panel({
  icon, title, count, countColor = 'muted', action, accent, children,
}: {
  icon: React.ReactNode; title: string; count?: number;
  countColor?: 'amber' | 'blue' | 'emerald' | 'muted';
  action?: React.ReactNode; accent?: 'amber' | 'blue'; children: React.ReactNode;
}) {
  const accentBorder = accent === 'amber' ? 'border-amber-500/20' : accent === 'blue' ? 'border-blue-500/20' : 'border-border';
  const accentBg    = accent === 'amber' ? 'bg-amber-500/5' : accent === 'blue' ? 'bg-blue-500/5' : 'bg-card';
  const accentDiv   = accent === 'amber' ? 'divide-amber-500/10' : accent === 'blue' ? 'divide-blue-500/10' : 'divide-border';
  const badge       = countColor === 'amber'   ? 'bg-amber-500/10 text-amber-600'
                    : countColor === 'blue'    ? 'bg-blue-500/10 text-blue-600'
                    : countColor === 'emerald' ? 'bg-emerald-500/10 text-emerald-600'
                    : 'bg-muted text-muted-foreground';
  return (
    <div className={cn('rounded-xl border flex flex-col', accentBorder, accentBg)}>
      <div className="flex items-center justify-between px-5 py-4 border-b border-border/60 shrink-0">
        <div className="flex items-center gap-2.5">
          {icon}
          <span className="text-sm font-semibold text-foreground">{title}</span>
          {count !== undefined && (
            <span className={cn('inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium', badge)}>
              {count}
            </span>
          )}
        </div>
        {action && <div>{action}</div>}
      </div>
      <div className={cn('divide-y flex-1', accentDiv)}>
        {children}
      </div>
    </div>
  );
}

// ─── Setup checklist ──────────────────────────────────────────────────────────

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
    <div className="rounded-xl border border-primary/20 bg-primary/5 p-5">
      <div className="flex items-center gap-4 mb-4">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-primary/10 shrink-0">
          <TrendingUp className="h-5 w-5 text-primary" />
        </div>
        <div className="flex-1">
          <h3 className="text-sm font-semibold text-foreground">Getting started</h3>
          <p className="text-xs text-muted-foreground mt-0.5">{completed} of {steps.length} steps completed</p>
        </div>
        <div className="flex items-center gap-3">
          <div className="h-2 w-32 rounded-full bg-muted overflow-hidden">
            <div className="h-full rounded-full bg-primary transition-all duration-500" style={{ width: `${(completed / steps.length) * 100}%` }} />
          </div>
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

// ─── Quick links ──────────────────────────────────────────────────────────────

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

// ─── Pinned Dashboard Tiles ───────────────────────────────────────────────────

function PinnedTilesRow() {
  const [tiles, setTiles] = useState<PinnedTile[]>([]);

  const refresh = useCallback(() => {
    dashboardApi.tiles().then(setTiles).catch(() => {});
  }, []);

  // Initial load — runs once on mount.
  useEffect(() => { refresh(); }, [refresh]);

  // Interval — set once after tiles are loaded; uses shortest refresh_interval_sec.
  // Separated from the initial load effect to avoid a double-fetch on first render.
  useEffect(() => {
    if (tiles.length === 0) return;
    const minInterval = Math.min(...tiles.map((t) => (t.refresh_interval_sec > 0 ? t.refresh_interval_sec : 300))) * 1000;
    const id = setInterval(refresh, minInterval);
    return () => clearInterval(id);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refresh, tiles.length === 0]);

  if (tiles.length === 0) return null;

  return (
    <div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {tiles.map((tile) => (
        <TileCard
          key={tile.id}
          tile={tile}
          onUnpin={(id) => setTiles((prev) => prev.filter((t) => t.id !== id))}
        />
      ))}
    </div>
  );
}

function TileCard({ tile, onUnpin }: { tile: PinnedTile; onUnpin: (id: string) => void }) {
  const [unpinBusy, setUnpinBusy] = useState(false);

  const handleUnpin = async () => {
    setUnpinBusy(true);
    try {
      await dashboardApi.unpin(tile.id);
      onUnpin(tile.id);
    } catch {
      setUnpinBusy(false);
    }
  };

  return (
    <div className="relative bg-card border border-border rounded-xl p-4 flex flex-col gap-2 min-h-[100px]">
      {/* Unpin button */}
      <button
        onClick={handleUnpin}
        disabled={unpinBusy}
        className="absolute top-2 right-2 flex h-6 w-6 items-center justify-center rounded-md text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors disabled:opacity-50"
        title="Unpin tile"
      >
        {unpinBusy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <X className="h-3.5 w-3.5" />}
      </button>

      {tile.label && (
        <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide pr-6">{tile.label}</p>
      )}

      <TileContent tile={tile} />
    </div>
  );
}

function TileContent({ tile }: { tile: PinnedTile }) {
  const data = tile.data;

  if (!data) {
    return <p className="text-xs text-muted-foreground">No data yet</p>;
  }

  switch (tile.widget_type) {
    case 'stat-card': {
      // Show the first numeric or string value as a big stat
      const entries = Object.entries(data);
      if (entries.length === 0) return <p className="text-xs text-muted-foreground">No data</p>;
      const firstEntry = entries[0];
      if (!firstEntry) return <p className="text-xs text-muted-foreground">No data</p>;
      const [key, val] = firstEntry;
      return (
        <div className="flex flex-col gap-1">
          <p className="text-2xl font-bold text-foreground leading-none">
            {typeof val === 'number' || typeof val === 'string' ? String(val) : JSON.stringify(val)}
          </p>
          <p className="text-xs text-muted-foreground">{key}</p>
        </div>
      );
    }

    case 'data-table': {
      // Render up to 10 rows; if data is an object with an array value use that
      let rows: Record<string, unknown>[] = [];
      if (Array.isArray(data)) {
        rows = (data as unknown[]).slice(0, 10) as Record<string, unknown>[];
      } else {
        const firstArray = Object.values(data).find(Array.isArray);
        if (firstArray) {
          rows = (firstArray as unknown[]).slice(0, 10) as Record<string, unknown>[];
        }
      }
      if (rows.length === 0) return <pre className="text-xs overflow-auto">{JSON.stringify(data, null, 2)}</pre>;
      const firstRow = rows[0] ?? {};
      const cols = Object.keys(firstRow);
      return (
        <div className="overflow-auto">
          <table className="text-xs w-full border-collapse">
            <thead>
              <tr>
                {cols.map((c) => (
                  <th key={c} className="text-left text-muted-foreground font-medium pb-1 pr-3 border-b border-border">{c}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row, i) => (
                <tr key={i} className="border-b border-border/50 last:border-0">
                  {cols.map((c) => (
                    <td key={c} className="py-1 pr-3 text-foreground truncate max-w-[8rem]">
                      {String(row[c] ?? '')}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      );
    }

    case 'feed':
    case 'list': {
      // Bullet list of string items
      let items: string[] = [];
      if (Array.isArray(data)) {
        items = (data as unknown[]).slice(0, 10).map(String);
      } else {
        const firstArray = Object.values(data).find(Array.isArray);
        if (firstArray) {
          items = (firstArray as unknown[]).slice(0, 10).map(String);
        } else {
          items = Object.entries(data).map(([k, v]) => `${k}: ${String(v)}`).slice(0, 10);
        }
      }
      return (
        <ul className="space-y-1">
          {items.map((item, i) => (
            <li key={i} className="flex items-start gap-2 text-xs text-foreground">
              <span className="mt-1 h-1.5 w-1.5 rounded-full bg-primary shrink-0" />
              <span className="truncate">{item}</span>
            </li>
          ))}
        </ul>
      );
    }

    case 'chart':
      return (
        <p className="text-xs text-muted-foreground">Chart: open connector for full view</p>
      );

    default:
      return <pre className="text-xs overflow-auto">{JSON.stringify(data, null, 2)}</pre>;
  }
}

// ─── Skeletons & helpers ──────────────────────────────────────────────────────

function RowSkeleton() {
  return (
    <div className="flex items-center gap-3 px-3 py-2.5">
      <div className="h-8 w-8 rounded-lg animate-pulse bg-muted shrink-0" />
      <div className="flex-1 space-y-1.5">
        <div className="h-3 w-32 animate-pulse rounded bg-muted" />
        <div className="h-2.5 w-20 animate-pulse rounded bg-muted" />
      </div>
    </div>
  );
}

function EmptyPanel({ icon, label, action }: { icon: React.ReactNode; label: string; action?: React.ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 px-4 py-8 text-center">
      <div className="text-muted-foreground/30">{icon}</div>
      <p className="text-sm text-muted-foreground">{label}</p>
      {action && <div className="mt-1">{action}</div>}
    </div>
  );
}
