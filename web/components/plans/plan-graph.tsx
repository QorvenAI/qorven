'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * PlanGraph — DAG canvas for a plan's nodes and edges.
 *
 * Mirrors the task-graph.tsx pattern (AgentNode, ReactFlow v12,
 * Background + Controls, nodesDraggable=false, fitView).
 *
 * Layout: topological layering — each node gets depth = longest
 * incoming path from any root. Nodes at the same depth are spread
 * left-to-right with even spacing. Cycles are guarded (depth capped
 * at node count).
 *
 * Node colors reuse the existing --graph-node-* CSS vars:
 *   pending/cancelled → --graph-node-idle
 *   running           → --graph-node-working  (+ animate-pulse dot)
 *   done              → --graph-node-done
 *   failed            → --graph-node-error
 *   blocked           → --graph-node-working  (amber, same as "in progress")
 *
 * Lazy-load this via next/dynamic if you want to defer the xyflow bundle
 * on pages that don't always show the graph.
 */

import { useMemo } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  Handle,
  Position,
  type Node as FlowNode,
  type Edge as FlowEdge,
} from '@xyflow/react';
import { cn } from '@/lib/utils';
import { User } from 'lucide-react';
import type { PlanNode, PlanEdge, NodeState, NodeKind } from '@/lib/api';

// ─── State / kind meta ────────────────────────────────────────────────────────

const NODE_STATE: Record<NodeState, { label: string; cls: string; dot: string; color: string }> = {
  pending:   { label: 'Pending',   cls: 'text-muted-foreground', dot: 'bg-muted-foreground/40', color: 'var(--graph-node-idle)' },
  running:   { label: 'Running',   cls: 'text-primary',          dot: 'bg-primary animate-pulse', color: 'var(--graph-node-working)' },
  done:      { label: 'Done',      cls: 'text-emerald-400',      dot: 'bg-emerald-400',           color: 'var(--graph-node-done)' },
  failed:    { label: 'Failed',    cls: 'text-destructive',      dot: 'bg-destructive',           color: 'var(--graph-node-error)' },
  blocked:   { label: 'Blocked',   cls: 'text-amber-400',        dot: 'bg-amber-400',             color: 'var(--graph-node-working)' },
  cancelled: { label: 'Cancelled', cls: 'text-muted-foreground', dot: 'bg-muted-foreground/30',   color: 'var(--graph-node-idle)' },
};

const NODE_KIND_LABEL: Record<NodeKind, string> = {
  planner:        'Planner',
  human_feedback: 'Human Review',
  agent_task:     'Agent Task',
  review:         'Review',
  push:           'Push',
  preview:        'Preview',
};

// ─── Custom node data ─────────────────────────────────────────────────────────

interface PlanNodeData extends Record<string, unknown> {
  title: string;
  kind: NodeKind;
  state: NodeState;
  assignee_soul?: string;
  error?: string;
}

// ─── Custom node component ────────────────────────────────────────────────────

function PlanNodeComponent({ data }: { data: PlanNodeData }) {
  const meta = NODE_STATE[data.state] ?? NODE_STATE.pending;
  const kindLabel = NODE_KIND_LABEL[data.kind] ?? data.kind;

  return (
    <div
      className="rounded-md border border-border bg-background shadow-sm transition-colors"
      style={{ minWidth: 160, maxWidth: 240, borderLeft: `3px solid ${meta.color}` }}
    >
      <Handle
        type="target"
        position={Position.Top}
        style={{ background: meta.color, width: 7, height: 7, border: 'none' }}
      />

      <div className="px-2.5 py-2 space-y-1">
        {/* Title row */}
        <div className="flex items-start gap-1.5">
          <span className={cn('mt-1 h-2 w-2 shrink-0 rounded-full', meta.dot)} />
          <span className="text-2xs font-medium leading-snug text-foreground line-clamp-2">
            {data.title || '(untitled)'}
          </span>
        </div>

        {/* Kind + state badges */}
        <div className="flex flex-wrap items-center gap-1">
          <span className="rounded-sm bg-muted px-1.5 py-0.5 font-mono text-2xs text-muted-foreground">
            {kindLabel}
          </span>
          <span className={cn('text-2xs font-medium', meta.cls)}>{meta.label}</span>
        </div>

        {/* Assignee */}
        {data.assignee_soul && (
          <div className="flex items-center gap-1 text-2xs text-muted-foreground">
            <User className="h-3 w-3" />
            <span className="truncate">{data.assignee_soul}</span>
          </div>
        )}

        {/* Error */}
        {data.error && (
          <p className="text-2xs text-destructive font-mono line-clamp-2">{data.error}</p>
        )}
      </div>

      <Handle
        type="source"
        position={Position.Bottom}
        style={{ background: meta.color, width: 7, height: 7, border: 'none' }}
      />
    </div>
  );
}

const NODE_TYPES = { planNode: PlanNodeComponent } as const;

// ─── Layout ───────────────────────────────────────────────────────────────────

const COL_W = 260;
const ROW_H = 130;

/**
 * Assigns each node a depth (longest path from any root) using
 * a relaxation pass (Bellman-Ford style, capped at node count to
 * guard against cycles).
 */
function computeLayout(
  planNodes: PlanNode[],
  planEdges: PlanEdge[],
): { nodes: FlowNode[]; edges: FlowEdge[] } {
  if (planNodes.length === 0) return { nodes: [], edges: [] };

  const idSet = new Set(planNodes.map((n) => n.id));

  // Build adjacency: incoming edges per node id
  const incomingEdges = new Map<string, PlanEdge[]>();
  for (const n of planNodes) incomingEdges.set(n.id, []);
  for (const e of planEdges) {
    if (idSet.has(e.from_node) && idSet.has(e.to_node)) {
      incomingEdges.get(e.to_node)?.push(e);
    }
  }

  // Roots = nodes with no incoming edges (from known nodes)
  const roots = planNodes.filter((n) => (incomingEdges.get(n.id)?.length ?? 0) === 0);

  // Compute depth: longest path from any root via BFS relaxation
  const depth = new Map<string, number>();
  for (const n of planNodes) depth.set(n.id, 0);

  // Build outgoing adjacency for BFS
  const outgoing = new Map<string, string[]>();
  for (const n of planNodes) outgoing.set(n.id, []);
  for (const e of planEdges) {
    if (idSet.has(e.from_node) && idSet.has(e.to_node)) {
      outgoing.get(e.from_node)?.push(e.to_node);
    }
  }

  // BFS from roots, relaxing depths (cycle-safe: cap at planNodes.length)
  const maxDepth = planNodes.length;
  const queue: string[] = roots.map((r) => r.id);
  while (queue.length > 0) {
    const nodeId = queue.shift()!;
    const d = depth.get(nodeId) ?? 0;
    if (d >= maxDepth) continue; // cycle guard
    for (const childId of outgoing.get(nodeId) ?? []) {
      const existing = depth.get(childId) ?? 0;
      if (d + 1 > existing) {
        depth.set(childId, d + 1);
        queue.push(childId);
      }
    }
  }

  // Group nodes by depth layer
  const layers = new Map<number, PlanNode[]>();
  for (const n of planNodes) {
    const d = depth.get(n.id) ?? 0;
    if (!layers.has(d)) layers.set(d, []);
    layers.get(d)!.push(n);
  }

  // Assign x/y positions
  const flowNodes: FlowNode[] = [];
  for (const [d, layerNodes] of layers) {
    const totalWidth = (layerNodes.length - 1) * COL_W;
    layerNodes.forEach((n, i) => {
      const x = i * COL_W - totalWidth / 2;
      const y = d * ROW_H;
      const meta = NODE_STATE[n.state] ?? NODE_STATE.pending;
      flowNodes.push({
        id: n.id,
        type: 'planNode',
        position: { x, y },
        data: {
          title: n.title,
          kind: n.kind,
          state: n.state,
          assignee_soul: n.assignee_soul,
          error: n.error,
          color: meta.color,
        } satisfies PlanNodeData,
      });
    });
  }

  // Build flow edges
  const SKIP_CONDITIONS = new Set(['', 'always', 'default']);
  const flowEdges: FlowEdge[] = planEdges
    .filter((e) => idSet.has(e.from_node) && idSet.has(e.to_node))
    .map((e) => {
      const targetState = planNodes.find((n) => n.id === e.to_node)?.state;
      const isRunning = targetState === 'running';
      const label = e.condition && !SKIP_CONDITIONS.has(e.condition.toLowerCase())
        ? e.condition
        : undefined;
      return {
        id: `${e.from_node}->${e.to_node}`,
        source: e.from_node,
        target: e.to_node,
        type: 'smoothstep',
        label,
        animated: isRunning,
        labelStyle: { fill: 'var(--foreground)', fontSize: 10 },
        labelBgStyle: { fill: 'var(--background)' },
        style: {
          stroke: 'var(--graph-edge)',
          strokeWidth: 1.5,
        },
      };
    });

  return { nodes: flowNodes, edges: flowEdges };
}

// ─── PlanGraph component ──────────────────────────────────────────────────────

export interface PlanGraphProps {
  nodes: PlanNode[];
  edges: PlanEdge[];
}

export function PlanGraph({ nodes, edges }: PlanGraphProps) {
  const { nodes: flowNodes, edges: flowEdges } = useMemo(
    () => computeLayout(nodes, edges),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [JSON.stringify(nodes), JSON.stringify(edges)],
  );

  return (
    <div className="h-[480px] w-full rounded-xl border border-border overflow-hidden">
      <ReactFlow
        nodes={flowNodes}
        edges={flowEdges}
        nodeTypes={NODE_TYPES}
        fitView
        fitViewOptions={{ padding: 0.25 }}
        proOptions={{ hideAttribution: true }}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
        panOnDrag
        zoomOnScroll
      >
        <Background gap={16} className="opacity-30" />
        <Controls showInteractive={false} className="!border-border !bg-card !shadow-sm" />
      </ReactFlow>
    </div>
  );
}
