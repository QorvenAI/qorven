'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Link from 'next/link';
import { Tree, TreeNode } from 'react-organizational-chart';
import {
  Crown, User, Users, Loader2, AlertCircle,
  DollarSign, BarChart3, GitBranch, UserCheck, TrendingUp, Shield,
  Briefcase, BookOpen, Code2, Megaphone, ShoppingCart, HeadphonesIcon,
  Building2, Cpu, RefreshCw, ZoomIn, ZoomOut, Maximize2,
} from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { orgApi, type OrgChartAgent, type OrgAgentSpend } from '@/lib/api-agents';

// ─── Role + level meta ────────────────────────────────────────────────────────

const ROLE_META: Record<string, { label: string; color: string; ring: string; Icon: React.ElementType }> = {
  caio:  { label: 'CAIO',  color: 'bg-violet-500/15 text-violet-500',   ring: 'ring-violet-500/30', Icon: Cpu },
  coo:   { label: 'COO',   color: 'bg-amber-500/15 text-amber-500',     ring: 'ring-amber-500/30',  Icon: Building2 },
  cto:   { label: 'CTO',   color: 'bg-blue-500/15 text-blue-500',       ring: 'ring-blue-500/30',   Icon: Code2 },
  cmo:   { label: 'CMO',   color: 'bg-pink-500/15 text-pink-500',       ring: 'ring-pink-500/30',   Icon: Megaphone },
  cso:   { label: 'CSO',   color: 'bg-emerald-500/15 text-emerald-500', ring: 'ring-emerald-500/30', Icon: ShoppingCart },
  cco:   { label: 'CCO',   color: 'bg-cyan-500/15 text-cyan-500',       ring: 'ring-cyan-500/30',   Icon: HeadphonesIcon },
  chro:  { label: 'CHRO',  color: 'bg-orange-500/15 text-orange-500',   ring: 'ring-orange-500/30', Icon: UserCheck },
  ciso:  { label: 'CISO',  color: 'bg-red-500/15 text-red-500',         ring: 'ring-red-500/30',    Icon: Shield },
  cko:   { label: 'CKO',   color: 'bg-teal-500/15 text-teal-500',       ring: 'ring-teal-500/30',   Icon: BookOpen },
  cfo:   { label: 'CFO',   color: 'bg-lime-500/15 text-lime-600',       ring: 'ring-lime-500/30',   Icon: DollarSign },
};

const LEVEL_LABEL: Record<string, string> = {
  l1: 'L1 Executive',
  l2: 'L2 C-Suite',
  l3: 'L3 Specialist',
  customer_facing: 'Customer-Facing',
};

function roleInitials(name: string): string {
  return name.split(/\s+/).slice(0, 2).map((w) => w[0]?.toUpperCase() ?? '').join('');
}

function fmtUSD(n: number): string {
  if (n === 0) return '$0';
  if (n < 0.01) return '<$0.01';
  return `$${n.toFixed(2)}`;
}

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(0)}k`;
  return String(n);
}

// ─── Tree data types ──────────────────────────────────────────────────────────

interface OrgNode {
  agent: OrgChartAgent;
  children: OrgNode[];
}

function buildTree(agents: OrgChartAgent[]): { roots: OrgNode[]; orphans: OrgChartAgent[] } {
  const byId = new Map(agents.map((a) => [a.id, { agent: a, children: [] as OrgNode[] }]));
  const roots: OrgNode[] = [];
  const orphans: OrgChartAgent[] = [];

  for (const a of agents) {
    const node = byId.get(a.id)!;
    if (!a.manager_id) {
      roots.push(node);
    } else {
      const parent = byId.get(a.manager_id);
      if (parent) parent.children.push(node);
      else orphans.push(a);
    }
  }

  const lvlOrder = (l?: string) => ({ l1: 0, l2: 1, l3: 2, customer_facing: 3 }[l ?? 'l3'] ?? 2);
  const sortNodes = (ns: OrgNode[]) =>
    ns.sort((a, b) => {
      const ld = lvlOrder(a.agent.org_level) - lvlOrder(b.agent.org_level);
      if (ld !== 0) return ld;
      return (a.agent.display_name ?? '').localeCompare(b.agent.display_name ?? '');
    });

  sortNodes(roots);
  for (const n of byId.values()) sortNodes(n.children);
  return { roots, orphans };
}

// ─── Sub-components ───────────────────────────────────────────────────────────

function AgentAvatar({ agent, size = 'md' }: { agent: OrgChartAgent; size?: 'sm' | 'md' | 'lg' }) {
  const meta = ROLE_META[agent.org_role ?? ''];
  const Icon = meta?.Icon ?? User;
  const sizeClass = size === 'sm' ? 'size-7' : size === 'lg' ? 'size-12' : 'size-9';
  const iconSize = size === 'sm' ? 'h-3.5 w-3.5' : size === 'lg' ? 'h-6 w-6' : 'h-4 w-4';
  return (
    <Avatar className={sizeClass}>
      {agent.avatar ? <AvatarImage src={agent.avatar} alt={agent.display_name} /> : null}
      <AvatarFallback className={cn('rounded-full text-xs font-semibold', meta?.color ?? 'bg-muted text-muted-foreground')}>
        {agent.display_name ? roleInitials(agent.display_name) : <Icon className={iconSize} />}
      </AvatarFallback>
    </Avatar>
  );
}

function RoleBadge({ orgRole }: { orgRole?: string }) {
  if (!orgRole) return null;
  const meta = ROLE_META[orgRole];
  return (
    <span className={cn('inline-flex items-center gap-0.5 rounded border px-1 py-0.5 text-[10px] font-bold uppercase tracking-wide border-current/20', meta?.color ?? 'bg-muted text-muted-foreground')}>
      {meta ? <meta.Icon className="h-2 w-2" /> : null}
      {meta?.label ?? orgRole.toUpperCase()}
    </span>
  );
}

function LevelBadge({ level }: { level?: string }) {
  const color =
    level === 'l1' ? 'bg-amber-500/15 text-amber-600 border-amber-500/20' :
    level === 'l2' ? 'bg-blue-500/15 text-blue-600 border-blue-500/20' :
    level === 'customer_facing' ? 'bg-cyan-500/15 text-cyan-600 border-cyan-500/20' :
    'bg-muted/60 text-muted-foreground border-border';
  return (
    <span className={cn('inline-flex items-center rounded border px-1 py-0.5 text-[10px] font-medium uppercase tracking-wide', color)}>
      {LEVEL_LABEL[level ?? 'l3'] ?? 'L3'}
    </span>
  );
}

function StatusDot({ status }: { status?: string }) {
  return (
    <span className={cn('inline-block h-1.5 w-1.5 rounded-full',
      status === 'active' ? 'bg-emerald-500' :
      status === 'suspended' ? 'bg-amber-400' : 'bg-muted-foreground/30'
    )} />
  );
}

// ─── Interactive node card for the org chart ──────────────────────────────────

function OrgNodeCard({ agent }: { agent: OrgChartAgent }) {
  const meta = ROLE_META[agent.org_role ?? ''];
  const isL1 = agent.org_level === 'l1';
  const isCustomer = agent.org_level === 'customer_facing';

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <div
            className={cn(
              'inline-flex w-44 flex-col gap-1.5 rounded-xl border px-3 py-2.5 shadow-sm cursor-default select-none transition-shadow hover:shadow-md',
              isL1
                ? 'border-amber-400/40 bg-amber-400/5 hover:bg-amber-400/8'
                : isCustomer
                ? 'border-cyan-400/40 bg-cyan-400/5 hover:bg-cyan-400/8'
                : meta
                ? `border-current/10 ${meta.color} bg-opacity-5 hover:bg-opacity-10`
                : 'border-border bg-card hover:bg-accent/30',
            )}
          >
            <div className="flex items-center gap-2">
              <AgentAvatar agent={agent} size={isL1 ? 'md' : 'sm'} />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1">
                  <StatusDot status={agent.status} />
                  <Link
                    href={`/qors/${agent.id}`}
                    className="truncate text-xs font-semibold hover:underline"
                    onClick={(e) => e.stopPropagation()}
                  >
                    {agent.display_name}
                  </Link>
                </div>
                {agent.title ? (
                  <p className="truncate text-[10px] text-muted-foreground">{agent.title}</p>
                ) : null}
              </div>
            </div>
            <div className="flex items-center gap-1 flex-wrap">
              <RoleBadge orgRole={agent.org_role} />
              <LevelBadge level={agent.org_level} />
            </div>
          </div>
        </TooltipTrigger>
        <TooltipContent side="bottom" className="text-xs">
          <div className="space-y-0.5">
            <p className="font-semibold">{agent.display_name}</p>
            {agent.title ? <p className="text-muted-foreground">{agent.title}</p> : null}
            {agent.monthly_budget_usd ? (
              <p className="text-muted-foreground">Budget: {fmtUSD(agent.monthly_budget_usd as number)}/mo</p>
            ) : null}
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

// CEO root card
function CEOCard() {
  return (
    <div className="inline-flex w-44 flex-col gap-1.5 rounded-xl border border-amber-400/50 bg-gradient-to-b from-amber-400/10 to-amber-400/5 px-3 py-2.5 shadow-sm">
      <div className="flex items-center gap-2">
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-amber-400/20 text-amber-500 ring-2 ring-amber-400/30">
          <Crown className="h-4 w-4" />
        </div>
        <div>
          <p className="text-xs font-bold">CEO — You</p>
          <p className="text-[10px] text-muted-foreground">Owner · Full access</p>
        </div>
      </div>
      <Badge variant="outline" className="w-fit border-amber-400/40 bg-amber-400/10 text-amber-600 text-[10px] font-bold uppercase px-1 py-0.5">
        L0 Owner
      </Badge>
    </div>
  );
}

// ─── Recursive tree renderer using the library ────────────────────────────────

function OrgTreeNode({ node }: { node: OrgNode }) {
  return (
    <TreeNode label={<OrgNodeCard agent={node.agent} />}>
      {node.children.map((child) => (
        <OrgTreeNode key={child.agent.id} node={child} />
      ))}
    </TreeNode>
  );
}

// ─── Org chart tab with zoom + pan ────────────────────────────────────────────

function OrgChartTab({ agents }: { agents: OrgChartAgent[] }) {
  const { roots, orphans } = useMemo(() => buildTree(agents), [agents]);
  const [zoom, setZoom] = useState(1);
  const containerRef = useRef<HTMLDivElement>(null);

  const clampZoom = (v: number) => Math.min(2, Math.max(0.3, v));
  const zoomIn = () => setZoom((z) => clampZoom(z + 0.15));
  const zoomOut = () => setZoom((z) => clampZoom(z - 0.15));
  const resetZoom = () => setZoom(1);

  // Wheel-to-zoom
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      if (e.ctrlKey || e.metaKey) {
        e.preventDefault();
        setZoom((z) => clampZoom(z - e.deltaY * 0.001));
      }
    };
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => el.removeEventListener('wheel', onWheel);
  }, []);

  return (
    <div className="space-y-3">
      {/* Zoom controls */}
      <div className="flex items-center justify-end gap-1">
        <span className="mr-1 text-xs text-muted-foreground">{Math.round(zoom * 100)}%</span>
        <button onClick={zoomOut} className="flex h-7 w-7 items-center justify-center rounded-lg border border-border bg-card hover:bg-accent/40 text-muted-foreground hover:text-foreground">
          <ZoomOut className="h-3.5 w-3.5" />
        </button>
        <button onClick={zoomIn} className="flex h-7 w-7 items-center justify-center rounded-lg border border-border bg-card hover:bg-accent/40 text-muted-foreground hover:text-foreground">
          <ZoomIn className="h-3.5 w-3.5" />
        </button>
        <button onClick={resetZoom} className="flex h-7 w-7 items-center justify-center rounded-lg border border-border bg-card hover:bg-accent/40 text-muted-foreground hover:text-foreground">
          <Maximize2 className="h-3.5 w-3.5" />
        </button>
        <span className="ml-2 text-[10px] text-muted-foreground/60 hidden sm:inline">Ctrl+scroll to zoom</span>
      </div>

      {/* Chart area — scrollable, zoomable */}
      <div
        ref={containerRef}
        className="overflow-auto rounded-xl border border-border bg-[hsl(var(--background))] p-6"
        style={{ maxHeight: '70vh', minHeight: 300 }}
      >
        <div
          style={{
            transform: `scale(${zoom})`,
            transformOrigin: 'top center',
            transition: 'transform 0.15s ease',
            paddingBottom: zoom < 1 ? `${(1 - zoom) * 100}%` : undefined,
          }}
        >
          <Tree
            label={<CEOCard />}
            lineHeight="20px"
            lineWidth="1.5px"
            lineColor="hsl(var(--border))"
            lineBorderRadius="6px"
            nodePadding="8px"
          >
            {roots.map((n) => (
              <OrgTreeNode key={n.agent.id} node={n} />
            ))}

            {orphans.map((a) => (
              <TreeNode key={a.id} label={<OrgNodeCard agent={a} />} />
            ))}
          </Tree>
        </div>
      </div>

      {/* Orphan notice */}
      {orphans.length > 0 ? (
        <p className="text-[11px] text-muted-foreground">
          {orphans.length} agent{orphans.length !== 1 ? 's' : ''} shown without a parent (manager not in the roster).
        </p>
      ) : null}
    </div>
  );
}

// ─── Roster tab ───────────────────────────────────────────────────────────────

function RosterTab({ agents }: { agents: OrgChartAgent[] }) {
  const sorted = useMemo(
    () =>
      [...agents].sort((a, b) => {
        const la = { l1: 0, l2: 1, l3: 2, customer_facing: 3 }[a.org_level ?? 'l3'] ?? 2;
        const lb = { l1: 0, l2: 1, l3: 2, customer_facing: 3 }[b.org_level ?? 'l3'] ?? 2;
        if (la !== lb) return la - lb;
        return (a.display_name ?? '').localeCompare(b.display_name ?? '');
      }),
    [agents],
  );

  return (
    <div className="overflow-x-auto rounded-xl border border-border">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b border-border bg-muted/40 text-left text-muted-foreground">
            <th className="px-3 py-2.5 font-medium">Agent</th>
            <th className="px-3 py-2.5 font-medium">Role</th>
            <th className="px-3 py-2.5 font-medium">Level</th>
            <th className="px-3 py-2.5 font-medium">Status</th>
            <th className="px-3 py-2.5 font-medium text-right">Budget/mo</th>
            <th className="px-3 py-2.5 font-medium text-right">Hired</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border/60">
          {sorted.map((a) => (
            <tr key={a.id} className="group hover:bg-accent/30 transition-colors">
              <td className="px-3 py-2.5">
                <div className="flex items-center gap-2">
                  <AgentAvatar agent={a} size="sm" />
                  <div>
                    <Link href={`/qors/${a.id}`} className="font-medium hover:underline">
                      {a.display_name}
                    </Link>
                    {a.title ? <p className="text-muted-foreground">{a.title}</p> : null}
                  </div>
                </div>
              </td>
              <td className="px-3 py-2.5"><RoleBadge orgRole={a.org_role} /></td>
              <td className="px-3 py-2.5"><LevelBadge level={a.org_level} /></td>
              <td className="px-3 py-2.5">
                <div className="flex items-center gap-1.5">
                  <StatusDot status={a.status} />
                  <span className="capitalize text-muted-foreground">{a.status ?? 'active'}</span>
                </div>
              </td>
              <td className="px-3 py-2.5 text-right text-muted-foreground">
                {a.monthly_budget_usd ? fmtUSD(a.monthly_budget_usd as number) : '—'}
              </td>
              <td className="px-3 py-2.5 text-right text-muted-foreground">
                {a.hired_at ? new Date(a.hired_at).toLocaleDateString() : '—'}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {sorted.length === 0 ? (
        <div className="py-10 text-center text-sm text-muted-foreground">No agents yet.</div>
      ) : null}
    </div>
  );
}

// ─── Finance tab ─────────────────────────────────────────────────────────────

function FinanceTab() {
  const [data, setData] = useState<{ agents: OrgAgentSpend[]; total_month_usd: number } | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    orgApi.financeSummary()
      .then(setData)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <div className="flex items-center gap-2 py-10 justify-center text-xs text-muted-foreground">
        <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading spend data…
      </div>
    );
  }

  const agentRows = data?.agents ?? [];
  const total = data?.total_month_usd ?? 0;

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        <div className="rounded-xl border border-border bg-card p-4">
          <p className="text-xs text-muted-foreground">This Month Total</p>
          <p className="mt-1 text-2xl font-bold">{fmtUSD(total)}</p>
        </div>
        <div className="rounded-xl border border-border bg-card p-4">
          <p className="text-xs text-muted-foreground">Active Agents</p>
          <p className="mt-1 text-2xl font-bold">{agentRows.length}</p>
        </div>
        <div className="rounded-xl border border-border bg-card p-4 col-span-2 sm:col-span-1">
          <p className="text-xs text-muted-foreground">Avg / Agent</p>
          <p className="mt-1 text-2xl font-bold">
            {agentRows.length ? fmtUSD(total / agentRows.length) : '$0'}
          </p>
        </div>
      </div>

      <div className="overflow-x-auto rounded-xl border border-border">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-border bg-muted/40 text-left text-muted-foreground">
              <th className="px-3 py-2.5 font-medium">Agent</th>
              <th className="px-3 py-2.5 font-medium">Role</th>
              <th className="px-3 py-2.5 font-medium text-right">Tokens In</th>
              <th className="px-3 py-2.5 font-medium text-right">Tokens Out</th>
              <th className="px-3 py-2.5 font-medium text-right">Month Cost</th>
              <th className="px-3 py-2.5 font-medium w-32">Share</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border/60">
            {agentRows.map((a) => {
              const pct = total > 0 ? (a.month_cost_usd / total) * 100 : 0;
              return (
                <tr key={a.agent_id} className="hover:bg-accent/30 transition-colors">
                  <td className="px-3 py-2.5 font-medium">{a.display_name || a.agent_id.slice(0, 8)}</td>
                  <td className="px-3 py-2.5"><RoleBadge orgRole={a.org_role} /></td>
                  <td className="px-3 py-2.5 text-right text-muted-foreground">{fmtTokens(a.tokens_in)}</td>
                  <td className="px-3 py-2.5 text-right text-muted-foreground">{fmtTokens(a.tokens_out)}</td>
                  <td className="px-3 py-2.5 text-right font-semibold">{fmtUSD(a.month_cost_usd)}</td>
                  <td className="px-3 py-2.5">
                    <div className="flex items-center gap-2">
                      <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                        <div
                          className="h-full rounded-full bg-primary/60"
                          style={{ width: `${Math.min(100, pct)}%` }}
                        />
                      </div>
                      <span className="w-9 text-right text-muted-foreground">{pct.toFixed(0)}%</span>
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        {agentRows.length === 0 ? (
          <div className="py-10 text-center text-sm text-muted-foreground">No spend data yet.</div>
        ) : null}
      </div>
    </div>
  );
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function OrgChartPage() {
  const [agents, setAgents] = useState<OrgChartAgent[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setErr(null);
    orgApi.chart()
      .then((res) => setAgents(res.agents ?? []))
      .catch((e) => setErr(e instanceof Error ? e.message : 'Failed to load'))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  return (
    <div className="mx-auto max-w-5xl space-y-5 p-4 lg:p-6">
      <CanvasHeader
        title="Org Chart"
        description="Your AI organisation — executives, department heads, and specialists."
        actions={
          <button
            onClick={load}
            disabled={loading}
            className="flex items-center gap-1.5 rounded-lg border border-border bg-card px-3 py-1.5 text-xs hover:bg-accent/40 disabled:opacity-50"
          >
            <RefreshCw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
            Refresh
          </button>
        }
      />

      {err ? (
        <div className="flex items-center gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
          <AlertCircle className="h-4 w-4 shrink-0" />
          <span>{err}</span>
        </div>
      ) : null}

      {loading && agents.length === 0 ? (
        <div className="flex items-center gap-2 py-10 justify-center text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading org chart…
        </div>
      ) : null}

      {!loading && agents.length === 0 && !err ? (
        <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-border/60 bg-card/40 px-6 py-12 text-center">
          <Users className="h-7 w-7 text-muted-foreground/50" />
          <p className="text-sm font-medium">No agents yet</p>
          <p className="text-xs text-muted-foreground">Create your first agent to start building the org.</p>
          <Link href="/qors" className="mt-1 text-xs text-primary hover:underline">Go to Agents →</Link>
        </div>
      ) : null}

      {agents.length > 0 ? (
        <Tabs defaultValue="chart">
          <TabsList variant="line" className="w-full justify-start gap-1">
            <TabsTrigger value="chart" className="flex items-center gap-1.5 text-xs">
              <GitBranch className="h-3.5 w-3.5" /> Org Chart
            </TabsTrigger>
            <TabsTrigger value="roster" className="flex items-center gap-1.5 text-xs">
              <Users className="h-3.5 w-3.5" /> Roster
            </TabsTrigger>
            <TabsTrigger value="finance" className="flex items-center gap-1.5 text-xs">
              <BarChart3 className="h-3.5 w-3.5" /> Finance
            </TabsTrigger>
          </TabsList>

          <TabsContent value="chart" className="mt-4">
            <OrgChartTab agents={agents} />
          </TabsContent>

          <TabsContent value="roster" className="mt-4">
            <RosterTab agents={agents} />
          </TabsContent>

          <TabsContent value="finance" className="mt-4">
            <FinanceTab />
          </TabsContent>
        </Tabs>
      ) : null}
    </div>
  );
}
