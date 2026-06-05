'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import {
  Minus, Plus, Maximize2, RefreshCw, Users,
  Cpu, Building2, Code2, Megaphone, ShoppingCart, HeadphonesIcon,
  UserCheck, Shield, BookOpen, DollarSign,
} from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { orgApi, type OrgChartAgent } from '@/lib/api-agents';
import { useStore } from '@/store';
import { soulGradient } from '@/components/soul-card';

// ── Layout constants ────────────────────────────────────────────────────────────
const CARD_W   = 210;
const CARD_H   = 96;
const GAP_X    = 28;
const GAP_Y    = 72;
const PADDING  = 64;
const MIN_ZOOM = 0.15;
const MAX_ZOOM = 2;

// ── Role metadata ───────────────────────────────────────────────────────────────
const ROLE_META: Record<string, { label: string; color: string; Icon: React.ElementType }> = {
  caio:  { label: 'CAIO',  color: '#a78bfa', Icon: Cpu },
  coo:   { label: 'COO',   color: '#f59e0b', Icon: Building2 },
  cto:   { label: 'CTO',   color: '#60a5fa', Icon: Code2 },
  cmo:   { label: 'CMO',   color: '#f472b6', Icon: Megaphone },
  cso:   { label: 'CSO',   color: '#34d399', Icon: ShoppingCart },
  cco:   { label: 'CCO',   color: '#22d3ee', Icon: HeadphonesIcon },
  chro:  { label: 'CHRO',  color: '#fb923c', Icon: UserCheck },
  ciso:  { label: 'CISO',  color: '#f87171', Icon: Shield },
  cko:   { label: 'CKO',   color: '#2dd4bf', Icon: BookOpen },
  cfo:   { label: 'CFO',   color: '#a3e635', Icon: DollarSign },
};

const STATUS_COLOR: Record<string, string> = {
  idle:      '#4ade80',
  thinking:  '#f59e0b',
  running:   '#22d3ee',
  error:     '#f87171',
  offline:   '#71717a',
};

// ── Tree types ──────────────────────────────────────────────────────────────────
interface TreeNode {
  agent: OrgChartAgent;
  children: TreeNode[];
  x: number;
  y: number;
}

interface Point { x: number; y: number }

// ── Layout algorithm (same approach as Paperclip) ───────────────────────────────
function subtreeWidth(node: TreeNode): number {
  if (node.children.length === 0) return CARD_W;
  const childW = node.children.reduce((s, c) => s + subtreeWidth(c), 0);
  const gaps = (node.children.length - 1) * GAP_X;
  return Math.max(CARD_W, childW + gaps);
}

function assignPositions(node: TreeNode, x: number, y: number): void {
  const total = subtreeWidth(node);
  node.x = x + (total - CARD_W) / 2;
  node.y = y;
  if (node.children.length > 0) {
    const childTotal = node.children.reduce((s, c) => s + subtreeWidth(c), 0);
    const gaps = (node.children.length - 1) * GAP_X;
    let cx = x + (total - childTotal - gaps) / 2;
    for (const child of node.children) {
      const cw = subtreeWidth(child);
      assignPositions(child, cx, y + CARD_H + GAP_Y);
      cx += cw + GAP_X;
    }
  }
}

function layoutForest(roots: TreeNode[]): void {
  let x = PADDING;
  for (const root of roots) {
    assignPositions(root, x, PADDING);
    x += subtreeWidth(root) + GAP_X;
  }
}

function flattenNodes(nodes: TreeNode[]): TreeNode[] {
  const result: TreeNode[] = [];
  const walk = (n: TreeNode) => { result.push(n); n.children.forEach(walk); };
  nodes.forEach(walk);
  return result;
}

function collectEdges(nodes: TreeNode[]): Array<{ parent: TreeNode; child: TreeNode }> {
  const edges: Array<{ parent: TreeNode; child: TreeNode }> = [];
  const walk = (n: TreeNode) => {
    for (const c of n.children) { edges.push({ parent: n, child: c }); walk(c); }
  };
  nodes.forEach(walk);
  return edges;
}

function buildForest(agents: OrgChartAgent[]): TreeNode[] {
  const byId = new Map<string, TreeNode>(
    agents.map(a => [a.id, { agent: a, children: [], x: 0, y: 0 }])
  );
  const roots: TreeNode[] = [];
  for (const a of agents) {
    const node = byId.get(a.id)!;
    if (!a.manager_id || !byId.has(a.manager_id)) {
      roots.push(node);
    } else {
      byId.get(a.manager_id)!.children.push(node);
    }
  }
  // Sort roots/children by org_level then display_name
  const levelOrder: Record<string, number> = { l1: 0, l2: 1, l3: 2, customer_facing: 3 };
  const sortNodes = (nodes: TreeNode[]) => {
    nodes.sort((a, b) =>
      (levelOrder[a.agent.org_level ?? 'l3'] ?? 9) - (levelOrder[b.agent.org_level ?? 'l3'] ?? 9) ||
      (a.agent.display_name ?? '').localeCompare(b.agent.display_name ?? '')
    );
    nodes.forEach(n => sortNodes(n.children));
  };
  sortNodes(roots);
  return roots;
}

function clamp(v: number, min: number, max: number) { return Math.min(Math.max(v, min), max); }

// ── Avatar gradient initials ───────────────────────────────────────────────────
function initials(name: string): string {
  return name.split(/\s+/).slice(0, 2).map(w => w[0]?.toUpperCase() ?? '').join('');
}

// ── Short model name ───────────────────────────────────────────────────────────
function shortModel(model?: string): string {
  if (!model) return '';
  if (model.startsWith('claude-')) {
    const m = model.replace(/^claude-/, '').replace(/-\d{8,}$/, '');
    const parts = m.split('-');
    const name = parts[0] ? parts[0].charAt(0).toUpperCase() + parts[0].slice(1) : '';
    const ver  = parts.slice(1).join('.');
    return ver ? `${name} ${ver}` : name;
  }
  if (model.startsWith('gpt-')) return model.replace('gpt-', 'GPT-');
  if (model.startsWith('gemini-')) return 'Gemini ' + model.replace('gemini-', '').split('-')[0];
  return model.length > 14 ? model.slice(0, 12) + '…' : model;
}

// ── Main page ─────────────────────────────────────────────────────────────────
export default function OrgChartPage() {
  const router  = useRouter();
  const souls   = useStore(s => s.souls);
  const soulStates = useStore(s => s.soulStates);

  const [agents,   setAgents]   = useState<OrgChartAgent[]>([]);
  const [loading,  setLoading]  = useState(true);
  const [pan,      setPan]      = useState<Point>({ x: 0, y: 0 });
  const [zoom,     setZoom]     = useState(1);
  const [dragging, setDragging] = useState(false);

  const containerRef = useRef<HTMLDivElement>(null);
  const dragStart    = useRef({ mx: 0, my: 0, px: 0, py: 0 });
  const hasInited    = useRef(false);
  const suppressRef  = useRef(false);

  const load = useCallback(() => {
    setLoading(true);
    orgApi.chart()
      .then(r => setAgents(Array.isArray((r as { agents?: OrgChartAgent[] })?.agents)
        ? ((r as { agents: OrgChartAgent[] }).agents)
        : Array.isArray(r) ? (r as OrgChartAgent[]) : []))
      .catch(() => setAgents(souls as unknown as OrgChartAgent[]))
      .finally(() => setLoading(false));
  }, [souls]);

  useEffect(() => { load(); }, [load]);

  // Merge live status from store
  const mergedAgents = useMemo<OrgChartAgent[]>(() =>
    agents.map(a => ({
      ...a,
      status: (soulStates[a.id]?.activity as string) ?? a.status ?? 'offline',
      model: (souls.find(s => s.id === a.id) as { model?: string } | undefined)?.model ?? (a as { model?: string }).model,
    })),
  [agents, souls, soulStates]);

  // Build layout
  const forest   = useMemo(() => buildForest(mergedAgents), [mergedAgents]);
  const allNodes = useMemo(() => { layoutForest(forest); return flattenNodes(forest); }, [forest]);
  const edges    = useMemo(() => collectEdges(forest), [forest]);

  // Bounds
  const bounds = useMemo(() => {
    if (allNodes.length === 0) return { width: 800, height: 500 };
    let mx = 0, my = 0;
    for (const n of allNodes) { mx = Math.max(mx, n.x + CARD_W); my = Math.max(my, n.y + CARD_H); }
    return { width: mx + PADDING, height: my + PADDING };
  }, [allNodes]);

  // Fit on first load
  useEffect(() => {
    if (hasInited.current || allNodes.length === 0 || !containerRef.current) return;
    hasInited.current = true;
    fitToScreen();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allNodes, bounds]);

  const fitToScreen = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;
    const cw = el.clientWidth, ch = el.clientHeight;
    const z  = clamp(Math.min((cw - 40) / bounds.width, (ch - 40) / bounds.height), MIN_ZOOM, 1);
    setZoom(z);
    setPan({ x: (cw - bounds.width * z) / 2, y: (ch - bounds.height * z) / 2 });
  }, [bounds]);

  // Mouse pan
  const onMouseDown = useCallback((e: React.MouseEvent) => {
    if ((e.target as HTMLElement).closest('[data-card]')) return;
    setDragging(true);
    dragStart.current = { mx: e.clientX, my: e.clientY, px: pan.x, py: pan.y };
  }, [pan]);

  const onMouseMove = useCallback((e: React.MouseEvent) => {
    if (!dragging) return;
    setPan({
      x: dragStart.current.px + e.clientX - dragStart.current.mx,
      y: dragStart.current.py + e.clientY - dragStart.current.my,
    });
  }, [dragging]);

  const onMouseUp = useCallback(() => setDragging(false), []);

  // Scroll zoom — toward mouse position
  const onWheel = useCallback((e: React.WheelEvent) => {
    e.preventDefault();
    const el = containerRef.current;
    if (!el) return;
    const rect  = el.getBoundingClientRect();
    const mx    = e.clientX - rect.left;
    const my    = e.clientY - rect.top;
    const factor = e.deltaY < 0 ? 1.12 : 0.88;
    const nz    = clamp(zoom * factor, MIN_ZOOM, MAX_ZOOM);
    const scale = nz / zoom;
    setPan({ x: mx - scale * (mx - pan.x), y: my - scale * (my - pan.y) });
    setZoom(nz);
  }, [zoom, pan]);

  const zoomToCenter = useCallback((factor: number) => {
    const el = containerRef.current;
    if (!el) return;
    const cx = el.clientWidth / 2, cy = el.clientHeight / 2;
    const nz = clamp(zoom * factor, MIN_ZOOM, MAX_ZOOM);
    const sc = nz / zoom;
    setPan({ x: cx - sc * (cx - pan.x), y: cy - sc * (cy - pan.y) });
    setZoom(nz);
  }, [zoom, pan]);

  const handleCardClick = useCallback((id: string) => {
    if (suppressRef.current) return;
    router.push(`/qors/${id}`);
  }, [router]);

  if (loading) {
    return (
      <div className="flex flex-col px-6 py-8 gap-3 text-muted-foreground text-sm">
        <RefreshCw className="h-4 w-4 animate-spin" />
        Loading org chart…
      </div>
    );
  }

  if (allNodes.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-20 gap-3 text-muted-foreground">
        <Users className="h-10 w-10 opacity-30" />
        <p className="text-sm">No agents yet. Create your first Qor to populate the org chart.</p>
      </div>
    );
  }

  return (
    <div className="full-bleed flex flex-col" style={{ height: 'calc(100dvh - var(--header-height, 56px) - var(--status-bar-height, 24px) - var(--agent-pill-height, 56px))' }}>
      <CanvasHeader
        title="Org Chart"
        description={`${allNodes.length} agents across ${new Set(mergedAgents.map(a => a.org_level)).size} tiers`}
        actions={
          <Button variant="outline" size="sm" onClick={load} className="gap-1.5">
            <RefreshCw className="h-3.5 w-3.5" />
            Refresh
          </Button>
        }
      />

      {/* Canvas */}
      <div
        ref={containerRef}
        className="relative flex-1 min-h-0 overflow-hidden bg-muted/10 border border-border rounded-xl mx-6 mb-4"
        style={{ cursor: dragging ? 'grabbing' : 'grab', touchAction: 'none', userSelect: 'none' }}
        onMouseDown={onMouseDown}
        onMouseMove={onMouseMove}
        onMouseUp={onMouseUp}
        onMouseLeave={onMouseUp}
        onWheel={onWheel}
      >
        {/* Zoom controls */}
        <div className="absolute top-3 right-3 z-10 flex flex-col gap-1.5">
          {[
            { icon: Plus,      title: 'Zoom in',        onClick: () => zoomToCenter(1.2) },
            { icon: Minus,     title: 'Zoom out',       onClick: () => zoomToCenter(0.8) },
            { icon: Maximize2, title: 'Fit to screen',  onClick: fitToScreen },
          ].map(({ icon: Icon, title, onClick }) => (
            <button
              key={title}
              title={title}
              onClick={onClick}
              className="flex h-7 w-7 items-center justify-center rounded border border-border bg-background/90 text-muted-foreground hover:text-foreground hover:bg-accent transition-colors backdrop-blur-sm"
            >
              <Icon className="h-3.5 w-3.5" />
            </button>
          ))}
          <div className="mt-1 text-center text-[10px] text-muted-foreground/50 font-mono">
            {Math.round(zoom * 100)}%
          </div>
        </div>

        {/* SVG edges */}
        <svg className="absolute inset-0 pointer-events-none overflow-visible" style={{ width: '100%', height: '100%' }}>
          <g transform={`translate(${pan.x},${pan.y}) scale(${zoom})`}>
            {edges.map(({ parent, child }) => {
              const x1 = parent.x + CARD_W / 2;
              const y1 = parent.y + CARD_H;
              const x2 = child.x  + CARD_W / 2;
              const y2 = child.y;
              const my = (y1 + y2) / 2;
              return (
                <path
                  key={`${parent.agent.id}-${child.agent.id}`}
                  d={`M${x1},${y1} L${x1},${my} L${x2},${my} L${x2},${y2}`}
                  fill="none"
                  stroke="var(--border)"
                  strokeWidth={1.5}
                  strokeLinecap="round"
                />
              );
            })}
          </g>
        </svg>

        {/* Cards */}
        <div
          className="absolute inset-0"
          style={{ transform: `translate(${pan.x}px,${pan.y}px) scale(${zoom})`, transformOrigin: '0 0' }}
        >
          {allNodes.map((node) => {
            const a        = node.agent;
            const roleMeta = ROLE_META[a.org_role ?? ''];
            const status   = (soulStates[a.id]?.activity as string) ?? a.status ?? 'offline';
            const dotColor = STATUS_COLOR[status] ?? STATUS_COLOR.offline;
            const lastEvt  = soulStates[a.id]?.lastEvent;
            const gradCls  = soulGradient(a.display_name);
            const model    = shortModel((souls.find(s => s.id === a.id) as { model?: string } | undefined)?.model);
            const isActive = status === 'thinking' || status === 'running';

            return (
              <div
                key={a.id}
                data-card
                className={cn(
                  'absolute bg-card border border-border rounded-xl shadow-sm cursor-pointer',
                  'hover:border-primary/40 hover:shadow-md transition-[border-color,box-shadow] duration-150',
                  isActive && 'border-primary/30 shadow-[0_0_0_1px_rgba(82,113,255,0.2)]',
                )}
                style={{ left: node.x, top: node.y, width: CARD_W, minHeight: CARD_H }}
                onClick={() => handleCardClick(a.id)}
              >
                <div className="flex items-start gap-3 px-4 py-3.5">
                  {/* Avatar + status dot */}
                  <div className="relative shrink-0 mt-0.5">
                    <div className={cn(
                      'h-9 w-9 rounded-full flex items-center justify-center text-white text-sm font-bold bg-gradient-to-br',
                      gradCls,
                    )}>
                      {initials(a.display_name)}
                    </div>
                    <span
                      className="absolute -bottom-0.5 -right-0.5 h-3 w-3 rounded-full border-2 border-card"
                      style={{ backgroundColor: dotColor }}
                      title={status}
                    />
                  </div>

                  {/* Info */}
                  <div className="min-w-0 flex-1">
                    {/* Name + role badge */}
                    <div className="flex items-center gap-1.5 flex-wrap">
                      <span className="text-sm font-semibold text-foreground leading-tight truncate max-w-[120px]">
                        {a.display_name}
                      </span>
                      {roleMeta && (
                        <span
                          className="inline-flex items-center gap-0.5 px-1 py-0.5 rounded text-[10px] font-bold border border-current/20"
                          style={{ color: roleMeta.color, background: roleMeta.color + '18' }}
                        >
                          {roleMeta.label}
                        </span>
                      )}
                    </div>

                    {/* Title/role */}
                    <p className="text-[11px] text-muted-foreground mt-0.5 truncate leading-tight">
                      {a.title ?? a.role ?? (a.org_level === 'l1' ? 'Executive' : a.org_level === 'l2' ? 'Management' : 'Specialist')}
                    </p>

                    {/* Model name */}
                    {model && (
                      <p className="text-[10px] text-muted-foreground/60 font-mono mt-1 truncate leading-tight">
                        {model}
                      </p>
                    )}

                    {/* Last event */}
                    {lastEvt && (
                      <p className="text-[10px] text-muted-foreground/50 mt-0.5 truncate leading-tight">
                        {lastEvt}
                      </p>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
