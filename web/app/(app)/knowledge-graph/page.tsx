'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

/**
 * /knowledge-graph — KG visualization (T2.2).
 *
 * Renders the full knowledge graph on a pan-zoom canvas with a force-
 * directed layout. Sidebar exposes:
 *   • Stats (entities, relationships, components, avg/max degree)
 *   • Clusters (nodes grouped by type)
 *   • God nodes (top-connected hubs)
 *   • Selected-node detail — neighbors + relevance
 *   • Analysis on demand — pagerank + betweenness + surprising
 *     connections + suggested questions (expensive; loads only when
 *     the user clicks "Insights")
 *
 * Backend data model (lib/api.ts): nodes carry color + size (so the
 * UI doesn't re-derive), edges carry weight/thickness/color (so weak
 * links visually fade). We preserve those hints everywhere possible.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import {
  Share2, Search, Crown, Sparkles, Loader2, AlertCircle,
  Focus, ChevronDown, FileText, Info,
} from 'lucide-react';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { PageSkeleton } from '@/components/page-skeleton';
import { cn } from '@/lib/utils';
import {
  graph,
  type GraphData,
  type GraphNode,
  type GraphEdge,
  type GodNode,
  type GraphAnalysis,
} from '@/lib/api';

// Physics-node: carries live x/y/vx/vy alongside the server payload.
type PhysicsNode = GraphNode & { _x: number; _y: number; _vx: number; _vy: number; _r: number };

// Fallback color palette when the backend doesn't ship `color` per
// node. Should never fire on a healthy KG — the Go handler always
// colors nodes — but guards against legacy data.
const TYPE_FALLBACK: Record<string, string> = {
  person: '#3b82f6',
  organization: '#ef4444',
  product: '#f59e0b',
  concept: '#8b5cf6',
  technology: '#06b6d4',
  event: '#f97316',
  location: '#10b981',
  source: '#6b7280',
  synthesis: '#a855f7',
  comparison: '#ec4899',
};
const colorFor = (n: GraphNode) => n.color ?? TYPE_FALLBACK[(n.type ?? '').toLowerCase()] ?? '#64748b';
const radiusFor = (n: GraphNode) => {
  if (n.size && n.size > 0) return Math.max(5, Math.min(18, n.size));
  const lc = n.link_count ?? 0;
  return 6 + Math.min(10, Math.sqrt(lc) * 1.5);
};

function useSize(ref: React.RefObject<HTMLDivElement | null>) {
  const [size, setSize] = useState({ w: 800, h: 500 });
  useEffect(() => {
    if (!ref.current) return;
    const ro = new ResizeObserver(([e]) => { if (e) setSize({ w: e.contentRect.width, h: e.contentRect.height }); });
    ro.observe(ref.current);
    return () => ro.disconnect();
  }, [ref]);
  return size;
}

export default function KnowledgeGraphPage() {
  const router = useRouter();
  const [data, setData] = useState<GraphData | null>(null);
  const [godNodes, setGodNodes] = useState<GodNode[]>([]);
  const [clusters, setClusters] = useState<Record<string, number>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState<PhysicsNode | null>(null);

  // Analysis is lazy — users opt in.
  const [analysis, setAnalysis] = useState<GraphAnalysis | null>(null);
  const [analysisLoading, setAnalysisLoading] = useState(false);

  const canvasRef = useRef<HTMLCanvasElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const nodesRef = useRef<PhysicsNode[]>([]);
  const { w, h } = useSize(containerRef);

  // Size the canvas bitmap to device pixels so HiDPI displays render
  // crisp lines and text. CSS size stays at logical w × h (set inline
  // on the <canvas>); the bitmap is w·dpr × h·dpr. draw() compensates
  // with setTransform(dpr, …).
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const dpr = window.devicePixelRatio || 1;
    canvas.width = Math.round(w * dpr);
    canvas.height = Math.round(h * dpr);
  }, [w, h]);

  const transform = useRef({ tx: 0, ty: 0, scale: 1 });
  const drag = useRef({ active: false, lastX: 0, lastY: 0 });

  useEffect(() => {
    Promise.all([graph.all(), graph.godNodes(), graph.clusters()])
      .then(([g, gods, clus]) => {
        setData(g);
        setGodNodes(Array.isArray(gods) ? gods : []);
        setClusters(clus ?? {});
      })
      .catch((e) => setError(e instanceof Error ? e.message : 'Failed to load graph'))
      .finally(() => setLoading(false));
  }, []);

  const nodes = data?.nodes ?? [];
  const edges = data?.edges ?? [];

  // Initialize physics positions when nodes change (or canvas size does
  // a large jump). Preserve existing positions for nodes we've already
  // laid out — prevents the whole graph from respawning on resize.
  useEffect(() => {
    if (!nodes.length) return;
    const cx = w / 2, cy = h / 2;
    const byId = new Map(nodesRef.current.map((n) => [n.id, n]));
    nodesRef.current = nodes.map((n, i) => {
      const existing = byId.get(n.id);
      if (existing) {
        return { ...existing, ...n, _r: radiusFor(n) };
      }
      // Backend may ship x/y hints; if present, use them. Otherwise seed
      // on a circle so the force sim has room to breathe.
      const angle = (i / nodes.length) * 2 * Math.PI;
      const radius = Math.min(w, h) * 0.32;
      return {
        ...n,
        _x: n.x ?? cx + Math.cos(angle) * radius,
        _y: n.y ?? cy + Math.sin(angle) * radius,
        _vx: 0,
        _vy: 0,
        _r: radiusFor(n),
      };
    });
  }, [nodes, w, h]);

  // Adjacency map for attraction. Built once per edge array change.
  const adjacency = useMemo(() => {
    const map = new Map<string, string[]>();
    for (const e of edges) {
      if (!map.has(e.source)) map.set(e.source, []);
      if (!map.has(e.target)) map.set(e.target, []);
      map.get(e.source)!.push(e.target);
      map.get(e.target)!.push(e.source);
    }
    return map;
  }, [edges]);

  // Draw — pan/zoom applied as a matrix transform on the 2D context.
  // HiDPI: canvas bitmap is sized w*dpr × h*dpr (see the w/h effect below);
  // we pre-multiply by dpr so a logical 1px is always a device pixel,
  // eliminating the "blurry graph" on Retina / high-scale displays.
  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const dpr = typeof window !== 'undefined' ? window.devicePixelRatio || 1 : 1;
    const { tx, ty, scale } = transform.current;
    const q = search.trim().toLowerCase();

    // Use setTransform so we don't accumulate translate/scale calls across
    // frames; clear the full bitmap (in device pixels), then restore the
    // dpr baseline and layer the pan/zoom on top.
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.save();
    ctx.translate(tx, ty);
    ctx.scale(scale, scale);

    // Edges first so nodes sit on top.
    const ns = nodesRef.current;
    for (const e of edges) {
      const a = ns.find((n) => n.id === e.source);
      const b = ns.find((n) => n.id === e.target);
      if (!a || !b) continue;
      ctx.lineWidth = Math.max(0.6, (e.thickness ?? e.weight ?? 0.5) * 1.8);
      ctx.strokeStyle = e.color ?? `rgba(255,255,255,${0.06 + (e.weight ?? 0) * 0.12})`;
      ctx.beginPath();
      ctx.moveTo(a._x, a._y);
      ctx.lineTo(b._x, b._y);
      ctx.stroke();
    }

    // Nodes + labels.
    const showAllLabels = ns.length <= 40;
    for (const n of ns) {
      const match = q && n.label.toLowerCase().includes(q);
      const isSel = selected?.id === n.id;
      const baseR = n._r;
      const r = isSel ? baseR + 4 : match ? baseR + 2 : baseR;
      const c = colorFor(n);

      if (isSel) {
        ctx.beginPath();
        ctx.arc(n._x, n._y, r + 6, 0, Math.PI * 2);
        ctx.fillStyle = c + '33';
        ctx.fill();
      }

      ctx.beginPath();
      ctx.arc(n._x, n._y, r, 0, Math.PI * 2);
      ctx.fillStyle = c;
      ctx.fill();

      if (match || isSel || showAllLabels || (n.link_count ?? 0) >= 5) {
        // Use the page font (Inter, loaded via next/font) rather than a
        // hardcoded stack; kept weight at 500/600 to stay legible at any
        // zoom on HiDPI — thin weights render blurry at sub-pixel sizes.
        ctx.font = `${isSel ? '600' : '500'} 12px var(--font-inter), ui-sans-serif, system-ui`;
        ctx.fillStyle = match || isSel ? '#e4e4e7' : '#a1a1aa';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'top';
        ctx.fillText(n.label, n._x, n._y + r + 6);
      }
    }

    ctx.restore();
  }, [edges, search, selected]);

  // Physics loop. Runs ~300 ticks then settles. Restarts on nodes/edges
  // change because topology shifts require a re-layout.
  useEffect(() => {
    if (!nodes.length) return;
    let raf = 0;
    let steps = 0;

    const tick = () => {
      const ns = nodesRef.current;
      const cx = w / 2, cy = h / 2;
      const alpha = Math.max(0.02, 1 - steps / 280);

      for (let i = 0; i < ns.length; i++) {
        const a = ns[i]!;
        // gravity toward center
        a._vx += (cx - a._x) * 0.0012 * alpha;
        a._vy += (cy - a._y) * 0.0012 * alpha;
        // pairwise repulsion — O(n²); fine up to ~400 nodes, which is
        // past what the KG handler returns in practice.
        for (let j = i + 1; j < ns.length; j++) {
          const b = ns[j]!;
          let dx = a._x - b._x, dy = a._y - b._y;
          const d2 = dx * dx + dy * dy + 1;
          const force = 2800 / d2;
          const d = Math.sqrt(d2);
          const fx = (dx / d) * force * 0.01;
          const fy = (dy / d) * force * 0.01;
          a._vx += fx; a._vy += fy;
          b._vx -= fx; b._vy -= fy;
        }
        // edge spring
        const neighbors = adjacency.get(a.id) ?? [];
        for (const nid of neighbors) {
          const b = ns.find((n) => n.id === nid);
          if (!b) continue;
          const dx = b._x - a._x, dy = b._y - a._y;
          const d = Math.sqrt(dx * dx + dy * dy) || 1;
          const spring = (d - 80) * 0.022 * alpha;
          a._vx += (dx / d) * spring;
          a._vy += (dy / d) * spring;
        }
        // damping + integration
        a._vx *= 0.84;
        a._vy *= 0.84;
        a._x += a._vx;
        a._y += a._vy;
      }

      steps++;
      draw();
      if (steps < 320) raf = requestAnimationFrame(tick);
    };

    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nodes.length, edges.length, w, h]);

  // Redraw on search/selected change without a full physics step.
  useEffect(() => { draw(); }, [draw]);

  // Mouse interaction
  const toWorld = (px: number, py: number) => {
    const { tx, ty, scale } = transform.current;
    return { x: (px - tx) / scale, y: (py - ty) / scale };
  };

  const onMouseDown = (e: React.MouseEvent) => {
    drag.current = { active: true, lastX: e.clientX, lastY: e.clientY };
  };
  const onMouseMove = (e: React.MouseEvent) => {
    if (!drag.current.active) return;
    transform.current.tx += e.clientX - drag.current.lastX;
    transform.current.ty += e.clientY - drag.current.lastY;
    drag.current.lastX = e.clientX;
    drag.current.lastY = e.clientY;
    draw();
  };
  const onMouseUp = () => { drag.current.active = false; };
  const onWheel = (e: React.WheelEvent) => {
    // Note: listener isn't passive because we need preventDefault. React
    // attaches it that way via onWheel automatically when preventDefault
    // is called, matching the old non-passive behavior.
    const rect = canvasRef.current!.getBoundingClientRect();
    const mx = e.clientX - rect.left, my = e.clientY - rect.top;
    const factor = e.deltaY < 0 ? 1.1 : 0.9;
    const { tx, ty, scale } = transform.current;
    transform.current.scale = Math.min(5, Math.max(0.1, scale * factor));
    transform.current.tx = mx - (mx - tx) * (transform.current.scale / scale);
    transform.current.ty = my - (my - ty) * (transform.current.scale / scale);
    draw();
  };

  const onClick = (e: React.MouseEvent) => {
    const rect = canvasRef.current!.getBoundingClientRect();
    const { x, y } = toWorld(e.clientX - rect.left, e.clientY - rect.top);
    const hit = nodesRef.current.find((n) => {
      const dx = n._x - x, dy = n._y - y;
      return Math.sqrt(dx * dx + dy * dy) <= n._r + 3;
    });
    setSelected(hit ?? null);
  };

  // Focus camera on a specific node (used by god-node list clicks).
  const focusNode = (id: string) => {
    const node = nodesRef.current.find((n) => n.id === id);
    if (!node) return;
    setSelected(node);
    // Move camera so the node is center-ish with a zoom-in.
    const { scale } = transform.current;
    const targetScale = Math.max(1.4, scale);
    transform.current.scale = targetScale;
    transform.current.tx = w / 2 - node._x * targetScale;
    transform.current.ty = h / 2 - node._y * targetScale;
    draw();
  };

  const loadAnalysis = async () => {
    if (analysis || analysisLoading) return;
    setAnalysisLoading(true);
    try {
      const a = await graph.analysis();
      setAnalysis(a);
    } catch {
      // swallow — analysis is opt-in so failing silently is fine; the
      // "Insights" section will just stay empty with the button visible
    } finally {
      setAnalysisLoading(false);
    }
  };

  if (loading) return <PageSkeleton cols={3} rows={2} />;

  if (error) {
    return (
      <div className="p-6">
        <div className="flex items-center gap-2 rounded-xl border border-destructive/40 bg-destructive/5 p-4 text-sm text-destructive">
          <AlertCircle className="h-4 w-4" />
          <span>{error}</span>
        </div>
      </div>
    );
  }

  if (!data || nodes.length === 0) {
    return (
      <div className="p-6">
        <EmptyState {...emptyStates.knowledgeGraph} />
      </div>
    );
  }

  const nodeDegree = (id: string) => adjacency.get(id)?.length ?? 0;
  const selectedNeighbors: PhysicsNode[] = selected
    ? (adjacency.get(selected.id) ?? [])
        .map((nid) => nodesRef.current.find((n) => n.id === nid))
        .filter((n): n is PhysicsNode => !!n)
    : [];

  return (
    <div className="flex h-full min-h-0 flex-col gap-3 full-bleed p-4 lg:p-6">
      {/* Header + stats */}
      <div className="flex flex-wrap items-center gap-3">
        <Share2 className="h-6 w-6 text-primary" />
        <h1 className="text-lg font-semibold">Knowledge Graph</h1>
        <StatChip label="entities" value={data.stats.total_nodes} />
        <StatChip label="edges" value={data.stats.total_edges} />
        <StatChip label="components" value={data.stats.components} />
        <StatChip label="avg °" value={data.stats.avg_degree.toFixed(1)} />
        <StatChip label="max °" value={data.stats.max_degree} />
      </div>

      {/* Search + tools */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative flex-1 min-w-[240px] max-w-md">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Highlight nodes by label…"
            className="qr-input text-xs pl-8 py-1.5 h-auto"
          />
        </div>
        <button
          onClick={() => { transform.current = { tx: 0, ty: 0, scale: 1 }; draw(); }}
          className="flex items-center gap-1 rounded-md border border-border px-2.5 py-1.5 text-2xs text-muted-foreground hover:bg-accent"
        >
          <Focus className="h-3 w-3" />
          Reset view
        </button>
        <button
          onClick={() => router.push('/memories')}
          className="flex items-center gap-1 rounded-md border border-border px-2.5 py-1.5 text-2xs text-muted-foreground hover:bg-accent"
        >
          <FileText className="h-3 w-3" />
          Compile wiki
        </button>
      </div>

      {/* Main grid: graph + sidebar */}
      <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 lg:grid-cols-[1fr_320px]">
        {/* Canvas */}
        <div
          ref={containerRef}
          className="relative min-h-[520px] overflow-hidden rounded-xl border border-border bg-background"
        >
          <canvas
            ref={canvasRef}
            className="h-full w-full cursor-grab active:cursor-grabbing"
            style={{ width: w, height: h }}
            onMouseDown={onMouseDown}
            onMouseMove={onMouseMove}
            onMouseUp={onMouseUp}
            onMouseLeave={onMouseUp}
            onWheel={onWheel}
            onClick={onClick}
          />
          {/* Type legend — pulled from backend-reported node-type counts */}
          <div className="absolute bottom-3 left-3 flex flex-wrap gap-1.5 text-xs">
            {Object.entries(data.stats.nodes_by_type).map(([type, count]) => (
              <span
                key={type}
                className="flex items-center gap-1 rounded-full border border-border bg-background/90 px-2 py-0.5 text-foreground/70"
              >
                <span
                  className="inline-block h-2 w-2 rounded-full"
                  style={{ background: TYPE_FALLBACK[type.toLowerCase()] ?? '#64748b' }}
                />
                {type}
                <span className="text-muted-foreground/70">· {count}</span>
              </span>
            ))}
          </div>
          {/* Pan/zoom hint */}
          <div className="absolute bottom-3 right-3 rounded-full bg-background/70 px-2 py-0.5 text-xs text-muted-foreground">
            drag to pan · scroll to zoom · click node to inspect
          </div>
        </div>

        {/* Sidebar */}
        <aside className="flex min-h-0 flex-col gap-3 overflow-y-auto">
          {selected ? (
            <NodeDetail
              node={selected}
              degree={nodeDegree(selected.id)}
              neighbors={selectedNeighbors}
              onPick={(id) => focusNode(id)}
              onDismiss={() => setSelected(null)}
            />
          ) : (
            <>
              <GodNodesCard godNodes={godNodes} onPick={focusNode} />
              <ClustersCard clusters={clusters} />
              <InsightsCard
                analysis={analysis}
                loading={analysisLoading}
                onLoad={loadAnalysis}
                onPickEdge={(sourceId) => focusNode(sourceId)}
              />
            </>
          )}
        </aside>
      </div>
    </div>
  );
}

// ───────────────────────────────────────────────────────────────────
// Sidebar cards
// ───────────────────────────────────────────────────────────────────

function StatChip({ label, value }: { label: string; value: number | string }) {
  return (
    <span className="rounded-md border border-border bg-muted/30 px-2 py-0.5 text-2xs text-muted-foreground">
      <span className="font-mono font-medium text-foreground">{value}</span> {label}
    </span>
  );
}

function GodNodesCard({ godNodes, onPick }: { godNodes: GodNode[]; onPick: (id: string) => void }) {
  if (!godNodes.length) return null;
  return (
    <section className="rounded-xl border border-border bg-card">
      <header className="flex items-center gap-2 border-b border-border/60 px-3 py-2">
        <Crown className="h-3.5 w-3.5 text-amber-400" />
        <h2 className="text-xs font-semibold tracking-wider">GOD NODES</h2>
        <span className="ml-auto text-2xs text-muted-foreground">Top {godNodes.length}</span>
      </header>
      <ul className="divide-y divide-border/60">
        {godNodes.map((g) => (
          <li key={g.id}>
            <button
              onClick={() => onPick(g.id)}
              className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs hover:bg-accent/40"
            >
              <span
                className="inline-block h-2 w-2 shrink-0 rounded-full"
                style={{ background: TYPE_FALLBACK[(g.type ?? '').toLowerCase()] ?? '#64748b' }}
              />
              <span className="min-w-0 flex-1 truncate">{g.name}</span>
              <span className="shrink-0 font-mono text-2xs text-muted-foreground">° {g.degree}</span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}

function ClustersCard({ clusters }: { clusters: Record<string, number> }) {
  const entries = Object.entries(clusters).sort((a, b) => b[1] - a[1]);
  if (!entries.length) return null;
  const total = entries.reduce((s, [, v]) => s + v, 0);
  return (
    <section className="rounded-xl border border-border bg-card">
      <header className="flex items-center gap-2 border-b border-border/60 px-3 py-2">
        <Sparkles className="h-3.5 w-3.5 text-primary" />
        <h2 className="text-xs font-semibold tracking-wider">CLUSTERS</h2>
      </header>
      <div className="space-y-1.5 p-3">
        {entries.map(([label, count]) => {
          const pct = total > 0 ? (count / total) * 100 : 0;
          return (
            <div key={label} className="text-2xs">
              <div className="flex items-baseline justify-between">
                <span className="truncate font-medium">{label}</span>
                <span className="font-mono text-muted-foreground">{count}</span>
              </div>
              <div className="mt-0.5 h-1 overflow-hidden rounded-full bg-muted">
                <div
                  className="h-full rounded-full"
                  style={{ width: `${pct}%`, background: TYPE_FALLBACK[label.toLowerCase()] ?? '#8b5cf6' }}
                />
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function InsightsCard({
  analysis,
  loading,
  onLoad,
  onPickEdge,
}: {
  analysis: GraphAnalysis | null;
  loading: boolean;
  onLoad: () => void;
  onPickEdge: (sourceId: string) => void;
}) {
  const [open, setOpen] = useState(false);

  if (!analysis) {
    return (
      <section className="rounded-xl border border-dashed border-border bg-card/40 p-3">
        <div className="flex items-center gap-2">
          <Info className="h-3.5 w-3.5 text-muted-foreground" />
          <p className="text-xs font-medium">Insights</p>
          <span className="ml-auto text-2xs text-muted-foreground">pagerank · clusters · surprises</span>
        </div>
        <p className="mt-1.5 text-2xs text-muted-foreground">
          Full centrality + clustering is expensive on big graphs; loads on demand.
        </p>
        <button
          onClick={onLoad}
          disabled={loading}
          className="mt-2 flex w-full items-center justify-center gap-1.5 rounded-md border border-primary/40 bg-primary/10 px-2 py-1.5 text-2xs font-medium text-primary hover:bg-primary/20 disabled:opacity-60"
        >
          {loading ? <Loader2 className="h-3 w-3 animate-spin" /> : null}
          {loading ? 'Computing…' : 'Compute insights'}
        </button>
      </section>
    );
  }

  const surprises = analysis.surprising_connections ?? [];
  const questions = analysis.suggested_questions ?? [];

  return (
    <section className="rounded-xl border border-border bg-card">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 border-b border-border/60 px-3 py-2 text-left hover:bg-accent/40"
      >
        <Sparkles className="h-3.5 w-3.5 text-primary" />
        <h2 className="flex-1 text-xs font-semibold tracking-wider">INSIGHTS</h2>
        <ChevronDown className={cn('h-3.5 w-3.5 text-muted-foreground transition-transform', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="space-y-3 p-3 text-2xs">
          <div className="grid grid-cols-3 gap-2">
            <MetricTile label="entities" value={analysis.total_entities} />
            <MetricTile label="edges" value={analysis.total_relationships} />
            <MetricTile label="clusters" value={analysis.total_clusters} />
          </div>

          {surprises.length > 0 && (
            <div>
              <h3 className="font-semibold uppercase tracking-wider text-muted-foreground">Surprising links</h3>
              <ul className="mt-1 space-y-1">
                {surprises.slice(0, 5).map((s) => (
                  <li
                    key={`${s.source_id}-${s.target_id}`}
                    className="group rounded-md border border-border/80 p-2 hover:border-primary/50 cursor-pointer"
                    onClick={() => onPickEdge(s.source_id)}
                  >
                    <div className="flex items-center gap-1 font-medium">
                      <span className="truncate">{s.source_name}</span>
                      <span className="text-muted-foreground">→</span>
                      <span className="truncate">{s.target_name}</span>
                      <span className="ml-auto shrink-0 rounded bg-amber-400/15 px-1 text-xs font-mono text-amber-400">
                        {s.surprise_score.toFixed(1)}
                      </span>
                    </div>
                    <p className="mt-0.5 italic text-muted-foreground">{s.reason}</p>
                  </li>
                ))}
              </ul>
            </div>
          )}

          {questions.length > 0 && (
            <div>
              <h3 className="font-semibold uppercase tracking-wider text-muted-foreground">Suggested questions</h3>
              <ul className="mt-1 space-y-1">
                {questions.slice(0, 6).map((q) => (
                  <li key={q} className="rounded-md border border-border/60 px-2 py-1 text-muted-foreground">
                    {q}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function MetricTile({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-md border border-border/60 bg-muted/20 p-2 text-center">
      <div className="font-mono text-sm font-semibold text-foreground">{value}</div>
      <div className="text-xs text-muted-foreground">{label}</div>
    </div>
  );
}

function NodeDetail({
  node,
  degree,
  neighbors,
  onPick,
  onDismiss,
}: {
  node: PhysicsNode;
  degree: number;
  neighbors: PhysicsNode[];
  onPick: (id: string) => void;
  onDismiss: () => void;
}) {
  return (
    <section className="rounded-xl border border-border bg-card">
      <header className="flex items-start gap-2 border-b border-border/60 px-3 py-2">
        <span className="mt-1 inline-block h-2.5 w-2.5 shrink-0 rounded-full" style={{ background: colorFor(node) }} />
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-sm font-semibold">{node.label}</h2>
          <p className="mt-0.5 font-mono text-2xs text-muted-foreground">
            {node.type ?? 'unknown'} · degree {degree}
          </p>
        </div>
        <button onClick={onDismiss} className="text-2xs text-muted-foreground hover:text-foreground">
          close
        </button>
      </header>

      {node.description && (
        <div className="border-b border-border/60 px-3 py-2 text-xs leading-relaxed text-muted-foreground">
          {node.description}
        </div>
      )}

      <div className="p-3">
        <h3 className="mb-1.5 text-2xs font-semibold uppercase tracking-wider text-muted-foreground">
          Neighbors ({neighbors.length})
        </h3>
        {neighbors.length === 0 ? (
          <p className="text-2xs text-muted-foreground">No outgoing links.</p>
        ) : (
          <ul className="space-y-0.5">
            {neighbors.slice(0, 20).map((n) => (
              <li key={n.id}>
                <button
                  onClick={() => onPick(n.id)}
                  className="flex w-full items-center gap-2 rounded-md px-1.5 py-1 text-left text-2xs hover:bg-accent/40"
                >
                  <span className="inline-block h-1.5 w-1.5 shrink-0 rounded-full" style={{ background: colorFor(n) }} />
                  <span className="min-w-0 flex-1 truncate">{n.label}</span>
                  <span className="shrink-0 font-mono text-muted-foreground">{n.type ?? ''}</span>
                </button>
              </li>
            ))}
            {neighbors.length > 20 && (
              <li className="px-1.5 pt-1 text-2xs italic text-muted-foreground">
                +{neighbors.length - 20} more neighbors
              </li>
            )}
          </ul>
        )}
      </div>
    </section>
  );
}
