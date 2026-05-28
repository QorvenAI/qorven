'use client';

import { useEffect, useState } from 'react';
import { request } from '@/lib/api-core';
import { Bot, ChevronDown, ChevronRight, DollarSign, Shield, Users } from 'lucide-react';

interface OrgNode {
  tenant_id: string;
  agent_id: string;
  reports_to: string | null;
  org_level: number;
  org_role: string;
  can_delegate_to: string[];
  max_budget_usd: number;
}

const LEVEL_LABELS: Record<number, string> = {
  1: 'User',
  2: 'C-Suite',
  3: 'Worker',
  4: 'Subagent',
};

const ROLE_COLORS: Record<string, string> = {
  chief: 'bg-purple-500/20 text-purple-400',
  coo: 'bg-blue-500/20 text-blue-400',
  cfo: 'bg-emerald-500/20 text-emerald-400',
  chro: 'bg-amber-500/20 text-amber-400',
  caio: 'bg-cyan-500/20 text-cyan-400',
  cmo: 'bg-pink-500/20 text-pink-400',
  cso: 'bg-red-500/20 text-red-400',
  code: 'bg-violet-500/20 text-violet-400',
  writer: 'bg-orange-500/20 text-orange-400',
  marketer: 'bg-rose-500/20 text-rose-400',
};

function NodeCard({ node, children, depth }: { node: OrgNode; children: OrgNode[]; depth: number }) {
  const [expanded, setExpanded] = useState(depth < 2);
  const hasChildren = children.length > 0;
  const colorClass = ROLE_COLORS[node.org_role] || 'bg-zinc-500/20 text-zinc-400';

  return (
    <div className="flex flex-col" style={{ marginLeft: depth > 0 ? '24px' : '0' }}>
      <div
        className="flex items-center gap-2 px-3 py-2 rounded-lg border border-border/50 hover:border-border bg-card/50 cursor-pointer transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        {hasChildren ? (
          expanded ? <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" /> : <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
        ) : (
          <Bot className="h-3.5 w-3.5 text-muted-foreground" />
        )}
        <span className={`px-1.5 py-0.5 rounded text-2xs font-medium ${colorClass}`}>
          {node.org_role || 'agent'}
        </span>
        <span className="text-xs text-muted-foreground">
          L{node.org_level} · {LEVEL_LABELS[node.org_level] || 'Unknown'}
        </span>
        {node.max_budget_usd > 0 && (
          <span className="ml-auto flex items-center gap-0.5 text-2xs text-emerald-400">
            <DollarSign className="h-3 w-3" />
            {node.max_budget_usd.toFixed(0)}/mo
          </span>
        )}
      </div>
      {expanded && hasChildren && (
        <div className="mt-1 border-l border-border/30 ml-4">
          {children.map(child => (
            <NodeCard key={child.agent_id} node={child} children={[]} depth={depth + 1} />
          ))}
        </div>
      )}
    </div>
  );
}

export function OrgChart() {
  const [nodes, setNodes] = useState<OrgNode[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    request<{ nodes: OrgNode[] }>('/org/hierarchy')
      .then(res => setNodes(res.nodes || []))
      .catch(() => setNodes([]))
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return <div className="flex items-center justify-center py-20 text-muted-foreground text-sm">Loading org chart...</div>;
  }

  if (nodes.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-20 gap-3">
        <Users className="h-10 w-10 text-muted-foreground/50" />
        <p className="text-sm text-muted-foreground">No org hierarchy configured yet</p>
        <p className="text-xs text-muted-foreground/70">
          The org chart populates automatically as agents are assigned roles and reporting lines.
        </p>
      </div>
    );
  }

  const byLevel = nodes.reduce((acc, node) => {
    const level = node.org_level;
    if (!acc[level]) acc[level] = [];
    acc[level].push(node);
    return acc;
  }, {} as Record<number, OrgNode[]>);

  const levels = Object.keys(byLevel).map(Number).sort();

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3 text-xs text-muted-foreground">
        <Shield className="h-4 w-4" />
        <span>{nodes.length} agents across {levels.length} levels</span>
      </div>

      {levels.map(level => (
        <div key={level} className="space-y-1.5">
          <div className="text-2xs font-medium text-muted-foreground uppercase tracking-wider px-1">
            Level {level} — {LEVEL_LABELS[level] || 'Unknown'}
          </div>
          <div className="space-y-1">
            {(byLevel[level] ?? []).map(node => {
              const children = nodes.filter(n => n.reports_to === node.agent_id);
              return <NodeCard key={node.agent_id} node={node} children={children} depth={0} />;
            })}
          </div>
        </div>
      ))}
    </div>
  );
}
