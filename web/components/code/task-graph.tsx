'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * TaskGraph — live DAG canvas for a build session.
 *
 * Node/edge model (v1, single/small-team build):
 *
 *   root "Project" node
 *     └─ one node per agent key (from agentStatus)
 *           └─ leaf nodes for every file the agent produced
 *                 (file_chip / file_created entries, associated by
 *                  proximity: the file follows the most-recent
 *                  tool_start whose tool matches an agent key)
 *   PR node (dangled off the last agent, if buildPrUrl present)
 *
 * Live status: agentStatus drives the agent node border color via
 * CSS vars (--graph-node-working / done / error / idle) — no hex in
 * this file. File nodes use --graph-node-file. The PR node uses
 * --graph-node-pr. Root uses --graph-node-root.
 *
 * Lazy-loaded by build-log.tsx via next/dynamic so the xyflow bundle
 * (~300 kB) is never fetched on non-IDE pages.
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
import type { BuildEntry } from './code-types';

// ─── Types ────────────────────────────────────────────────────────────────────

export interface TaskGraphProps {
  entries: BuildEntry[];
  agentStatus?: Record<string, 'working' | 'done' | 'error'>;
  phase?: string;
  prUrl?: string;
  running: boolean;
}

type AgentNodeKind = 'root' | 'agent' | 'file' | 'pr';

interface AgentNodeData extends Record<string, unknown> {
  kind: AgentNodeKind;
  label: string;
  status?: 'working' | 'done' | 'error' | 'idle';
}

// ─── CSS var helpers (no hex in JSX) ─────────────────────────────────────────

function nodeColor(kind: AgentNodeKind, status?: string): string {
  if (kind === 'root') return 'var(--graph-node-root)';
  if (kind === 'pr')   return 'var(--graph-node-pr)';
  if (kind === 'file') return 'var(--graph-node-file)';
  // agent — driven by status
  if (status === 'working') return 'var(--graph-node-working)';
  if (status === 'done')    return 'var(--graph-node-done)';
  if (status === 'error')   return 'var(--graph-node-error)';
  return 'var(--graph-node-idle)';
}

// ─── Custom node component ────────────────────────────────────────────────────

function AgentNode({ data }: { data: AgentNodeData }) {
  const color = nodeColor(data.kind, data.status);

  return (
    <div
      className={cn(
        'rounded-md border bg-background shadow-sm transition-colors',
        'border-border',
      )}
      style={{ minWidth: 120, maxWidth: 200, borderLeft: `3px solid ${color}` }}
    >
      <Handle type="target" position={Position.Top}
        style={{ background: color, width: 7, height: 7, border: 'none' }} />

      <div className="flex items-center gap-1.5 px-2.5 py-1.5">
        {/* Status dot */}
        <span
          className="h-2 w-2 shrink-0 rounded-full"
          style={{ background: color }}
        />
        <span className={cn(
          'text-2xs font-medium leading-snug',
          data.kind === 'file' ? 'font-mono text-muted-foreground truncate' : 'text-foreground',
        )}>
          {data.label}
        </span>
      </div>

      <Handle type="source" position={Position.Bottom}
        style={{ background: color, width: 7, height: 7, border: 'none' }} />
    </div>
  );
}

const NODE_TYPES = { agent: AgentNode } as const;

// ─── Graph builder ────────────────────────────────────────────────────────────

const COL_W = 220;
const ROW_H =  90;

function buildGraph(
  entries: BuildEntry[],
  agentStatus: Record<string, 'working' | 'done' | 'error'>,
  prUrl?: string,
): { nodes: FlowNode[]; edges: FlowEdge[] } {
  const nodes: FlowNode[] = [];
  const edges: FlowEdge[] = [];

  // ── Root node ──
  nodes.push({
    id: 'root',
    type: 'agent',
    position: { x: 0, y: 0 },
    data: { kind: 'root', label: 'Project' } satisfies AgentNodeData,
  });

  // Collect agent keys — prefer agentStatus (live), fall back to tool_start entries
  const agentKeys: string[] = Object.keys(agentStatus);
  if (agentKeys.length === 0) {
    for (const e of entries) {
      if (e.type === 'tool_start' && e.tool && !agentKeys.includes(e.tool)) {
        agentKeys.push(e.tool);
      }
    }
  }

  // Associate files to agents: as we scan entries in order, the last
  // agent_key we saw from a tool_start entry is the "current agent".
  const agentFiles: Record<string, string[]> = {};
  for (const k of agentKeys) agentFiles[k] = [];
  let currentAgent: string | null = agentKeys[0] ?? null;

  for (const e of entries) {
    if (e.type === 'tool_start' && e.tool && agentKeys.includes(e.tool)) {
      currentAgent = e.tool;
    }
    if ((e.type === 'file_chip' || e.type === 'file_created') && e.path && currentAgent) {
      if (!agentFiles[currentAgent]) agentFiles[currentAgent] = [];
      // Deduplicate paths per agent
      if (!agentFiles[currentAgent]!.includes(e.path)) {
        agentFiles[currentAgent]!.push(e.path);
      }
    }
  }

  // ── Agent nodes + root→agent edges ──
  const agentX = agentKeys.length > 1
    ? (i: number) => (i - (agentKeys.length - 1) / 2) * COL_W
    : () => 0;

  agentKeys.forEach((key, i) => {
    const status = agentStatus[key] ?? 'idle';
    nodes.push({
      id: `agent__${key}`,
      type: 'agent',
      position: { x: agentX(i), y: ROW_H },
      data: { kind: 'agent', label: key, status } satisfies AgentNodeData,
    });
    edges.push({
      id: `root->agent__${key}`,
      source: 'root',
      target: `agent__${key}`,
      type: 'smoothstep',
      style: { stroke: 'var(--graph-edge)', strokeWidth: 1.5 },
    });

    // ── File leaf nodes ──
    const files = agentFiles[key] ?? [];
    files.slice(0, 8).forEach((fp, j) => {
      const fid = `file__${key}__${j}`;
      const label = fp.split('/').pop() ?? fp;
      nodes.push({
        id: fid,
        type: 'agent',
        position: { x: agentX(i) + (j - (files.length - 1) / 2) * 140, y: ROW_H * 2 },
        data: { kind: 'file', label } satisfies AgentNodeData,
      });
      edges.push({
        id: `agent__${key}->${fid}`,
        source: `agent__${key}`,
        target: fid,
        type: 'smoothstep',
        style: { stroke: 'var(--graph-edge)', strokeWidth: 1 },
      });
    });
  });

  // ── PR node ──
  if (prUrl) {
    const lastAgent = agentKeys.length > 0 ? `agent__${agentKeys[agentKeys.length - 1]}` : 'root';
    nodes.push({
      id: 'pr',
      type: 'agent',
      position: { x: agentX(agentKeys.length - 1), y: ROW_H * 3 },
      data: { kind: 'pr', label: 'Pull Request' } satisfies AgentNodeData,
    });
    edges.push({
      id: `${lastAgent}->pr`,
      source: lastAgent,
      target: 'pr',
      type: 'smoothstep',
      style: { stroke: 'var(--graph-node-pr)', strokeWidth: 1.5, strokeDasharray: '4 2' },
    });
  }

  return { nodes, edges };
}

// ─── TaskGraph component ──────────────────────────────────────────────────────

export function TaskGraph({ entries, agentStatus = {}, prUrl, running }: TaskGraphProps) {
  const { nodes, edges } = useMemo(
    () => buildGraph(entries, agentStatus, prUrl),
    // Re-derive when status map or file entries change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [entries, JSON.stringify(agentStatus), prUrl],
  );

  if (nodes.length === 0 && !running) {
    return (
      <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
        No build data yet
      </div>
    );
  }

  return (
    <div className="h-full w-full">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={NODE_TYPES}
        fitView
        fitViewOptions={{ padding: 0.2 }}
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
