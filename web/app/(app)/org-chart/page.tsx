'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * /org-chart — team hierarchy visualization (T2.10).
 *
 * Backend returns a flat array of agents, each optionally with a
 * manager_id. We build a tree client-side (agents with no manager
 * become roots; Prime is typically the sole root) and render it as
 * indented cards. Orphans — agents whose manager_id doesn't match any
 * known agent — hang off a "Unassigned" group at the bottom.
 */

import { useEffect, useMemo, useState } from 'react';
import {
  Crown, User, Users, Loader2, AlertCircle, ChevronRight, ChevronDown,
} from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import Link from 'next/link';
import { cn } from '@/lib/utils';
import { orgChart, type OrgChartAgent } from '@/lib/api';

interface TreeNode {
  agent: OrgChartAgent;
  children: TreeNode[];
}

export default function OrgChartPage() {
  const [agents, setAgents] = useState<OrgChartAgent[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    orgChart
      .get()
      .then((res) => setAgents(res.agents ?? []))
      .catch((e) => setErr(e instanceof Error ? e.message : 'Failed to load org chart'))
      .finally(() => setLoading(false));
  }, []);

  const { roots, orphans } = useMemo(() => buildTree(agents), [agents]);

  return (
    <div className="mx-auto max-w-4xl space-y-5 p-4 lg:p-6">
      <CanvasHeader title="Org Chart" description="Reporting structure across all Qors. Click a card to open a profile." />

      {loading && (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading…
        </div>
      )}

      {err && (
        <div className="flex items-center gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-xs text-destructive">
          <AlertCircle className="h-4 w-4" />
          <span>{err}</span>
        </div>
      )}

      {!loading && !err && agents.length === 0 && (
        <div className="flex flex-col items-center gap-2 rounded-xl border border-dashed border-border/60 bg-card/40 px-6 py-10 text-center">
          <Users className="h-6 w-6 text-muted-foreground/60" />
          <p className="text-sm">No agents yet.</p>
          <Link href="/qors" className="text-2xs text-primary hover:underline">
            Go to Qors →
          </Link>
        </div>
      )}

      {roots.map((n) => (
        <TreeCard key={n.agent.id} node={n} depth={0} />
      ))}

      {orphans.length > 0 && (
        <section className="mt-6">
          <h2 className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
            Unassigned ({orphans.length})
          </h2>
          <div className="space-y-2">
            {orphans.map((a) => (
              <AgentCard key={a.id} agent={a} />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}

function buildTree(agents: OrgChartAgent[]): { roots: TreeNode[]; orphans: OrgChartAgent[] } {
  const byId = new Map(agents.map((a) => [a.id, { agent: a, children: [] as TreeNode[] }]));
  const roots: TreeNode[] = [];
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

  // Sort: chief first by role hint, then by name.
  const sortFn = (a: TreeNode, b: TreeNode) => {
    const ar = (a.agent.role ?? '').toLowerCase();
    const br = (b.agent.role ?? '').toLowerCase();
    if (ar === 'chief' || ar === 'supervisor') return -1;
    if (br === 'chief' || br === 'supervisor') return 1;
    return (a.agent.display_name ?? '').localeCompare(b.agent.display_name ?? '');
  };
  roots.sort(sortFn);
  for (const n of byId.values()) n.children.sort(sortFn);

  return { roots, orphans };
}

function TreeCard({ node, depth }: { node: TreeNode; depth: number }) {
  const [open, setOpen] = useState(depth < 2);
  const hasChildren = node.children.length > 0;
  return (
    <div>
      <div className="flex items-stretch gap-2" style={{ paddingLeft: `${depth * 20}px` }}>
        {hasChildren ? (
          <button
            onClick={() => setOpen((v) => !v)}
            className="flex w-5 items-center justify-center rounded-sm text-muted-foreground hover:bg-accent hover:text-foreground"
          >
            {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
          </button>
        ) : (
          <span className="w-5" />
        )}
        <div className="flex-1">
          <AgentCard agent={node.agent} highlightSupervisor />
        </div>
      </div>
      {open && hasChildren && (
        <div className="mt-1 space-y-1 border-l border-border/40" style={{ marginLeft: `${depth * 20 + 10}px` }}>
          {node.children.map((c) => (
            <TreeCard key={c.agent.id} node={c} depth={depth + 1} />
          ))}
        </div>
      )}
    </div>
  );
}

function AgentCard({ agent, highlightSupervisor }: { agent: OrgChartAgent; highlightSupervisor?: boolean }) {
  const isChief = highlightSupervisor && (
    agent.role === 'chief' || agent.role === 'supervisor' || agent.agent_key === 'prime'
  );
  const Icon = isChief ? Crown : User;
  return (
    <Link
      href={`/qors/${agent.id}`}
      className={cn(
        'flex items-center gap-2 rounded-lg border px-3 py-2 text-xs transition-colors',
        'border-border bg-card hover:border-border/80 hover:bg-accent/40',
        isChief && 'border-amber-400/40 bg-amber-400/5',
      )}
    >
      <Icon className={cn('h-4 w-4 shrink-0', isChief ? 'text-amber-400' : 'text-muted-foreground')} />
      <span className="min-w-0 flex-1 truncate font-medium">{agent.display_name}</span>
      {agent.role && (
        <span className="shrink-0 rounded-sm bg-muted px-1.5 py-0.5 font-mono text-xs uppercase text-muted-foreground">
          {agent.role}
        </span>
      )}
    </Link>
  );
}
