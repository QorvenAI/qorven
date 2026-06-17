'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import {
  Minus, Plus, Maximize2, RefreshCw, Users,
  Cpu, Building2, Code2, Megaphone, ShoppingCart, HeadphonesIcon,
  UserCheck, Shield, BookOpen, DollarSign, UserPlus, UserX, Network,
} from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { orgApi, type OrgChartAgent } from '@/lib/api-agents';
import { useStore } from '@/store';
import { soulGradient } from '@/components/soul-card';
import { agentDepartment } from '@/components/agents/agent-card-meta';

// ── Layout constants ────────────────────────────────────────────────────────────
const CARD_W   = 224;
const CARD_H   = 76;
const GAP_X    = 28;
const GAP_Y    = 72;
const PADDING  = 64;
const MIN_ZOOM = 0.15;
const MAX_ZOOM = 2;

// ── Role metadata ───────────────────────────────────────────────────────────────
// Colors are design tokens (defined in css/config.qorven.css) referenced via
// var(), so the chart stays on the token system rather than hardcoding hex.
const ROLE_META: Record<string, { label: string; color: string; Icon: React.ElementType }> = {
  caio:  { label: 'CAIO',  color: 'var(--org-role-caio)', Icon: Cpu },
  coo:   { label: 'COO',   color: 'var(--org-role-coo)',  Icon: Building2 },
  cto:   { label: 'CTO',   color: 'var(--org-role-cto)',  Icon: Code2 },
  cmo:   { label: 'CMO',   color: 'var(--org-role-cmo)',  Icon: Megaphone },
  cso:   { label: 'CSO',   color: 'var(--org-role-cso)',  Icon: ShoppingCart },
  cco:   { label: 'CCO',   color: 'var(--org-role-cco)',  Icon: HeadphonesIcon },
  chro:  { label: 'CHRO',  color: 'var(--org-role-chro)', Icon: UserCheck },
  ciso:  { label: 'CISO',  color: 'var(--org-role-ciso)', Icon: Shield },
  cko:   { label: 'CKO',   color: 'var(--org-role-cko)',  Icon: BookOpen },
  cfo:   { label: 'CFO',   color: 'var(--org-role-cfo)',  Icon: DollarSign },
};

const C_SUITE_ROLES = new Set(['caio','coo','cto','cmo','cso','cco','chro','ciso','cko','cfo']);

// Synthetic CEO node ID — represents the human admin user at the top.
const CEO_ID = '__ceo__';

const STATUS_COLOR: Record<string, string> = {
  idle:      'var(--org-status-idle)',
  thinking:  'var(--org-status-thinking)',
  running:   'var(--org-status-running)',
  error:     'var(--org-status-error)',
  offline:   'var(--org-status-offline)',
};

// Available org levels and roles for the hire form
const ORG_LEVELS = [
  { value: 'l1', label: 'L1 — Prime / CROO' },
  { value: 'l2', label: 'L2 — C-Suite' },
  { value: 'l3', label: 'L3 — Manager / Specialist' },
  { value: 'customer_facing', label: 'Customer-Facing' },
];

const ORG_ROLES = [
  { value: 'coo',       label: 'COO' },
  { value: 'cto',       label: 'CTO' },
  { value: 'cmo',       label: 'CMO' },
  { value: 'cfo',       label: 'CFO' },
  { value: 'cso',       label: 'CSO' },
  { value: 'cco',       label: 'CCO' },
  { value: 'chro',      label: 'CHRO' },
  { value: 'ciso',      label: 'CISO' },
  { value: 'cko',       label: 'CKO' },
  { value: 'caio',      label: 'CAIO' },
  { value: 'manager',   label: 'Manager' },
  { value: 'worker',    label: 'Worker' },
  { value: 'specialist', label: 'Specialist' },
];

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

function buildForest(agents: OrgChartAgent[], userName = 'You'): TreeNode[] {
  // Filter out test/system agents — keep only real named agents that are active or have a role
  const real = agents.filter(a =>
    a.display_name && a.display_name.trim() !== '' &&
    !a.agent_key?.startsWith('concurrent-') &&
    !a.agent_key?.startsWith('xss-test-') &&
    !a.agent_key?.startsWith('sess-iso-') &&
    !a.agent_key?.startsWith('adversarial-') &&
    !a.agent_key?.startsWith('loop-test-') &&
    !a.agent_key?.startsWith('tester-') &&
    !a.agent_key?.includes('201231') &&
    !a.agent_key?.includes('201331') &&
    !a.agent_key?.includes('201332')
  );

  const byId = new Map<string, TreeNode>(
    real.map(a => [a.id, { agent: a, children: [], x: 0, y: 0 }])
  );

  // The human user is the CEO at the very top — represented by a synthetic node,
  // never a real agent. Every top-level agent (Prime and anyone without a
  // resolvable manager) hangs under it, so the chart always reads You → Prime → … .
  const makeCEONode = (): TreeNode => ({
    agent: {
      id: CEO_ID,
      display_name: userName,
      org_role: 'ceo',
      org_level: 'l0',
      title: 'Chief Executive Officer',
      status: 'idle',
    },
    children: [],
    x: 0, y: 0,
  });

  // If manager_id links exist in data, use them directly — but still root the
  // whole forest under the synthetic CEO (You), rather than leaving bare roots.
  const hasManagerLinks = real.some(a => a.manager_id && byId.has(a.manager_id));

  if (hasManagerLinks) {
    const ceo = makeCEONode();
    for (const a of real) {
      const node = byId.get(a.id)!;
      if (!a.manager_id || !byId.has(a.manager_id)) ceo.children.push(node);
      else byId.get(a.manager_id)!.children.push(node);
    }
    const sortChildrenML = (n: TreeNode) => {
      n.children.sort((x, y) => (x.agent.display_name ?? '').localeCompare(y.agent.display_name ?? ''));
      n.children.forEach(sortChildrenML);
    };
    sortChildrenML(ceo);
    return [ceo];
  }

  // Infer hierarchy from org_role and agent_key:
  // L0: CEO (synthetic — the human admin)
  // L1: Prime/CROO — agent_key='chief' or org_level='l1' with chief role
  // L2: C-Suite — org_role in C_SUITE_ROLES
  // L3: Workers — everyone else

  const ceoNode: TreeNode = {
    agent: {
      id: CEO_ID,
      display_name: userName,
      org_role: 'ceo',
      org_level: 'l0',
      title: 'Chief Executive Officer',
      status: 'idle',
    },
    children: [],
    x: 0, y: 0,
  };

  // Find Prime/CROO — agent_key='chief' is canonical, also accept explicit prime role
  const prime = real.find(a =>
    a.agent_key === 'chief' || a.org_role === 'croo' || a.org_role === 'prime'
  );

  // C-Suite agents — have a named C-level role, and are not the Prime
  const cSuite = real.filter(a =>
    a !== prime && C_SUITE_ROLES.has(a.org_role ?? '')
  );

  // Workers — everyone else (no named C-suite role, not Prime)
  const workers = real.filter(a =>
    a !== prime && !C_SUITE_ROLES.has(a.org_role ?? '')
  );

  // Attach prime to CEO
  const primeNode = prime ? byId.get(prime.id)! : null;
  if (primeNode) ceoNode.children.push(primeNode);

  // Attach C-suite under prime (or directly under CEO if no prime)
  const cSuiteParent = primeNode ?? ceoNode;
  for (const cs of cSuite) {
    cSuiteParent.children.push(byId.get(cs.id)!);
  }

  // Attach workers: if their manager_id points to a C-suite agent use that,
  // otherwise group under prime, otherwise under CEO
  for (const w of workers) {
    const explicitParent = w.manager_id ? byId.get(w.manager_id) : undefined;
    const parent = explicitParent ?? primeNode ?? ceoNode;
    parent.children.push(byId.get(w.id)!);
  }

  // Sort children by display_name
  const sortChildren = (n: TreeNode) => {
    n.children.sort((a, b) => (a.agent.display_name ?? '').localeCompare(b.agent.display_name ?? ''));
    n.children.forEach(sortChildren);
  };
  sortChildren(ceoNode);

  return [ceoNode];
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

// ── Decode role from JWT (same approach as sidebar.tsx) ────────────────────────
function getLocalUserRole(): string {
  if (typeof window === 'undefined') return 'user';
  try {
    const stored = localStorage.getItem('qorven_user');
    if (stored) {
      const parsed = JSON.parse(stored) as { role?: string };
      return parsed.role ?? 'user';
    }
    const token = localStorage.getItem('qorven_token');
    if (token) {
      const payload = JSON.parse(atob(token.split('.')[1]!)) as { role?: string };
      return payload.role ?? 'user';
    }
  } catch {}
  return 'user';
}

// ── ConfirmDialog ─────────────────────────────────────────────────────────────
function ConfirmDialog({
  title, body, confirmLabel = 'Delete', onCancel, onConfirm, loading = false,
}: {
  title: string; body: string; confirmLabel?: string;
  onCancel: () => void; onConfirm: () => void; loading?: boolean;
}) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape' && !loading) onCancel(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [onCancel, loading]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => { if (!loading) onCancel(); }}>
      <div role="alertdialog" className="w-full max-w-sm rounded-xl border border-border bg-card p-6 shadow-xl space-y-4" onClick={e => e.stopPropagation()}>
        <h2 className="text-base font-semibold">{title}</h2>
        <p className="text-sm text-muted-foreground">{body}</p>
        <div className="flex justify-end gap-2 pt-2">
          <button
            className="inline-flex items-center justify-center rounded-lg border border-border bg-transparent px-4 py-2 text-sm font-medium text-foreground hover:bg-accent transition-colors disabled:opacity-50"
            onClick={onCancel} disabled={loading}
          >
            Cancel
          </button>
          <button
            className="inline-flex items-center justify-center rounded-lg bg-destructive px-4 py-2 text-sm font-medium text-destructive-foreground hover:bg-destructive/90 transition-colors disabled:opacity-50"
            onClick={onConfirm} disabled={loading}
          >
            {loading ? <RefreshCw className="h-3.5 w-3.5 animate-spin mr-1.5" /> : null}
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── HireModal ─────────────────────────────────────────────────────────────────
function HireModal({
  agents, onClose, onSuccess,
}: {
  agents: OrgChartAgent[];
  onClose: () => void;
  onSuccess: () => void;
}) {
  const [agentId,  setAgentId]  = useState('');
  const [orgLevel, setOrgLevel] = useState('l3');
  const [orgRole,  setOrgRole]  = useState('worker');
  const [error,    setError]    = useState('');
  const [loading,  setLoading]  = useState(false);

  // Only real (non-CEO) agents can be hired
  const hirable = agents.filter(a => a.id !== CEO_ID);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape' && !loading) onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [onClose, loading]);

  async function handleSubmit() {
    if (!agentId) { setError('Select an agent.'); return; }
    setError('');
    setLoading(true);
    try {
      await orgApi.hire(agentId, orgLevel, orgRole);
      onSuccess();
      onClose();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Hire failed';
      setError(msg.toLowerCase().includes('403') || msg.toLowerCase().includes('admin')
        ? 'Admin role required to hire agents.'
        : msg);
    } finally {
      setLoading(false);
    }
  }

  const selectCls = 'w-full rounded-lg border border-border bg-background px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => { if (!loading) onClose(); }}>
      <div role="dialog" aria-modal="true" className="w-full max-w-sm rounded-xl border border-border bg-card p-6 shadow-xl space-y-4" onClick={e => e.stopPropagation()}>
        <h2 className="text-base font-semibold flex items-center gap-2">
          <UserPlus className="h-4 w-4 text-primary" />
          Hire Agent
        </h2>

        <div className="space-y-3">
          <div>
            <label className="block text-xs font-medium mb-1 text-muted-foreground">Agent</label>
            <select value={agentId} onChange={e => setAgentId(e.target.value)} className={selectCls} disabled={loading}>
              <option value="">Select an agent…</option>
              {hirable.map(a => (
                <option key={a.id} value={a.id}>{a.display_name}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs font-medium mb-1 text-muted-foreground">Org Level</label>
            <select value={orgLevel} onChange={e => setOrgLevel(e.target.value)} className={selectCls} disabled={loading}>
              {ORG_LEVELS.map(l => (
                <option key={l.value} value={l.value}>{l.label}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs font-medium mb-1 text-muted-foreground">Org Role</label>
            <select value={orgRole} onChange={e => setOrgRole(e.target.value)} className={selectCls} disabled={loading}>
              {ORG_ROLES.map(r => (
                <option key={r.value} value={r.value}>{r.label}</option>
              ))}
            </select>
          </div>
        </div>

        {error && <p className="text-xs text-destructive">{error}</p>}

        <div className="flex justify-end gap-2 pt-2">
          <button
            className="inline-flex items-center justify-center rounded-lg border border-border bg-transparent px-4 py-2 text-sm font-medium text-foreground hover:bg-accent transition-colors disabled:opacity-50"
            onClick={onClose} disabled={loading}
          >
            Cancel
          </button>
          <button
            className="inline-flex items-center justify-center rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
            onClick={handleSubmit} disabled={loading || !agentId}
          >
            {loading ? <RefreshCw className="h-3.5 w-3.5 animate-spin mr-1.5" /> : null}
            Hire
          </button>
        </div>
      </div>
    </div>
  );
}

// ── ReassignModal ─────────────────────────────────────────────────────────────
function ReassignModal({
  agent, agents, onClose, onSuccess,
}: {
  agent: OrgChartAgent;
  agents: OrgChartAgent[];
  onClose: () => void;
  onSuccess: () => void;
}) {
  const [managerId, setManagerId] = useState<string>(agent.manager_id ?? '');
  const [error,     setError]     = useState('');
  const [loading,   setLoading]   = useState(false);

  // Exclude the agent itself and the synthetic CEO from the manager list
  const candidates = agents.filter(a => a.id !== agent.id && a.id !== CEO_ID);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape' && !loading) onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [onClose, loading]);

  async function handleSubmit() {
    setError('');
    setLoading(true);
    try {
      await orgApi.reassignManager(agent.id, managerId || null);
      onSuccess();
      onClose();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Reassign failed';
      if (msg.toLowerCase().includes('403') || msg.toLowerCase().includes('admin')) {
        setError('Admin role required to reassign managers.');
      } else if (msg.toLowerCase().includes('cycle') || msg.toLowerCase().includes('reporting cycle')) {
        setError('Cannot reassign: this would create a reporting cycle.');
      } else {
        setError(msg);
      }
    } finally {
      setLoading(false);
    }
  }

  const selectCls = 'w-full rounded-lg border border-border bg-background px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => { if (!loading) onClose(); }}>
      <div role="dialog" aria-modal="true" className="w-full max-w-sm rounded-xl border border-border bg-card p-6 shadow-xl space-y-4" onClick={e => e.stopPropagation()}>
        <h2 className="text-base font-semibold flex items-center gap-2">
          <Network className="h-4 w-4 text-primary" />
          Reassign Manager
        </h2>
        <p className="text-sm text-muted-foreground">
          Change who <strong>{agent.display_name}</strong> reports to.
        </p>

        <div>
          <label className="block text-xs font-medium mb-1 text-muted-foreground">New Manager</label>
          <select value={managerId} onChange={e => setManagerId(e.target.value)} className={selectCls} disabled={loading}>
            <option value="">(None — top-level)</option>
            {candidates.map(a => (
              <option key={a.id} value={a.id}>{a.display_name}</option>
            ))}
          </select>
        </div>

        {error && <p className="text-xs text-destructive">{error}</p>}

        <div className="flex justify-end gap-2 pt-2">
          <button
            className="inline-flex items-center justify-center rounded-lg border border-border bg-transparent px-4 py-2 text-sm font-medium text-foreground hover:bg-accent transition-colors disabled:opacity-50"
            onClick={onClose} disabled={loading}
          >
            Cancel
          </button>
          <button
            className="inline-flex items-center justify-center rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
            onClick={handleSubmit} disabled={loading}
          >
            {loading ? <RefreshCw className="h-3.5 w-3.5 animate-spin mr-1.5" /> : null}
            Reassign
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Main page ─────────────────────────────────────────────────────────────────
export default function OrgChartPage() {
  const router  = useRouter();
  const souls   = useStore(s => s.souls);
  const soulStates = useStore(s => s.soulStates);
  const userName = useStore(s => (s as { user?: { display_name?: string; username?: string } }).user?.display_name
    || (s as { user?: { username?: string } }).user?.username
    || 'You');

  const [agents,   setAgents]   = useState<OrgChartAgent[]>([]);
  const [loading,  setLoading]  = useState(true);
  const [pan,      setPan]      = useState<Point>({ x: 0, y: 0 });
  const [zoom,     setZoom]     = useState(1);
  const [dragging, setDragging] = useState(false);

  // Admin gating — decoded from JWT on the client (same pattern as sidebar.tsx)
  const [isAdmin, setIsAdmin] = useState(false);
  useEffect(() => { setIsAdmin(getLocalUserRole() === 'admin'); }, []);

  // Modal state
  const [hireOpen,      setHireOpen]      = useState(false);
  const [terminateAgent, setTerminateAgent] = useState<OrgChartAgent | null>(null);
  const [reassignAgent,  setReassignAgent]  = useState<OrgChartAgent | null>(null);
  const [terminateLoading, setTerminateLoading] = useState(false);
  const [actionError,   setActionError]   = useState('');

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
  const forest   = useMemo(() => buildForest(mergedAgents, userName), [mergedAgents, userName]);
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

  async function handleTerminateConfirm() {
    if (!terminateAgent) return;
    setTerminateLoading(true);
    setActionError('');
    try {
      await orgApi.terminate(terminateAgent.id, 'Terminated via org chart');
      setTerminateAgent(null);
      load();
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Terminate failed';
      setActionError(msg.toLowerCase().includes('403') || msg.toLowerCase().includes('admin')
        ? 'Admin role required to terminate agents.'
        : msg);
      setTerminateAgent(null);
    } finally {
      setTerminateLoading(false);
    }
  }

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
          <div className="flex items-center gap-2">
            {actionError && (
              <span className="text-xs text-destructive px-2">{actionError}</span>
            )}
            {isAdmin && (
              <Button variant="outline" size="sm" onClick={() => { setActionError(''); setHireOpen(true); }} className="gap-1.5">
                <UserPlus className="h-3.5 w-3.5" />
                Hire
              </Button>
            )}
            <Button variant="outline" size="sm" onClick={load} className="gap-1.5">
              <RefreshCw className="h-3.5 w-3.5" />
              Refresh
            </Button>
          </div>
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
          <div className="mt-1 text-center text-2xs text-muted-foreground/50 font-mono">
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
            const isCEO    = a.id === CEO_ID;
            const roleMeta = ROLE_META[a.org_role ?? ''];
            const status    = isCEO ? 'idle' : ((soulStates[a.id]?.activity as string) ?? a.status ?? 'offline');
            // dotColor and lastEvt are kept for future tooltip/status indicators
            void (STATUS_COLOR[status] ?? STATUS_COLOR.offline);
            void soulStates[a.id]?.lastEvent;
            const gradCls  = isCEO ? 'from-amber-400 to-yellow-500' : soulGradient(a.display_name);
            const model    = shortModel((souls.find(s => s.id === a.id) as { model?: string } | undefined)?.model);
            const isActive = status === 'thinking' || status === 'running';
            const department = isCEO ? '' : agentDepartment({ org_role: a.org_role, role: (a as { role?: string }).role ?? '' });

            return (
              <div
                key={a.id}
                data-card
                className={cn(
                  'absolute bg-card border rounded-xl shadow-sm transition-[border-color,box-shadow] duration-150 group',
                  isCEO
                    ? 'border-amber-500/40 shadow-[0_0_0_1px_rgba(245,158,11,0.15)] cursor-default'
                    : 'border-border cursor-pointer hover:border-primary/40 hover:shadow-md',
                  isActive && 'border-primary/30 shadow-[0_0_0_1px_rgba(82,113,255,0.2)]',
                )}
                style={{ left: node.x, top: node.y, width: CARD_W, minHeight: CARD_H }}
                onClick={() => !isCEO && handleCardClick(a.id)}
              >
                <div className="flex items-center gap-3 px-4 py-3.5 h-full">
                  {/* Avatar — image if set, else gradient initials. Pulse ring only while working, no status dot. */}
                  <div className={cn(
                    'h-10 w-10 shrink-0 rounded-full overflow-hidden flex items-center justify-center text-white text-sm font-bold bg-gradient-to-br',
                    gradCls,
                    isActive && 'ring-2 ring-primary/60 ring-offset-2 ring-offset-card animate-pulse',
                  )}>
                    {(a as { avatar?: string }).avatar
                      ? <img src={(a as { avatar?: string }).avatar} alt={a.display_name} className="h-full w-full object-cover" />
                      : initials(a.display_name)}
                  </div>

                  {/* Info */}
                  <div className="min-w-0 flex-1">
                    {/* Name + designation badge */}
                    <div className="flex items-center gap-1.5">
                      <span className="text-sm font-semibold text-foreground leading-tight truncate max-w-[110px]">
                        {a.display_name}
                      </span>
                      {isCEO ? (
                        <span className="shrink-0 inline-flex items-center px-1.5 py-0.5 rounded text-2xs font-bold border border-amber-500/30 text-amber-400 bg-amber-500/10">
                          CEO
                        </span>
                      ) : (a.agent_key === 'chief' || a.org_role === 'coo') ? (
                        <span className="shrink-0 inline-flex items-center px-1.5 py-0.5 rounded text-2xs font-bold border border-violet-500/30 text-violet-400 bg-violet-500/10">
                          {roleMeta?.label ?? 'COO'}
                        </span>
                      ) : roleMeta ? (
                        <span
                          className="shrink-0 inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-2xs font-bold border border-current/20"
                          style={{ color: roleMeta.color, background: roleMeta.color + '18' }}
                        >
                          {roleMeta.label}
                        </span>
                      ) : null}
                    </div>

                    {/* model · department subline */}
                    <div className="flex items-center gap-1.5 mt-1 text-2xs text-muted-foreground/70 leading-tight">
                      {model && <span className="truncate">{model}</span>}
                      {model && department && <span className="text-border">·</span>}
                      {department && <span className="truncate">{department}</span>}
                      {isCEO && !model && <span>You</span>}
                    </div>
                  </div>
                </div>

                {/* Admin action buttons — visible on hover for non-CEO nodes */}
                {isAdmin && !isCEO && (
                  <div
                    className="absolute -bottom-7 left-1/2 -translate-x-1/2 flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity duration-150"
                    onClick={e => e.stopPropagation()}
                  >
                    <button
                      title="Reassign manager"
                      className="flex h-6 w-6 items-center justify-center rounded-full border border-border bg-background/95 text-muted-foreground hover:text-foreground hover:bg-accent transition-colors shadow-sm"
                      onClick={() => { suppressRef.current = true; setActionError(''); setReassignAgent(a); setTimeout(() => { suppressRef.current = false; }, 200); }}
                    >
                      <Network className="h-3 w-3" />
                    </button>
                    <button
                      title="Terminate"
                      className="flex h-6 w-6 items-center justify-center rounded-full border border-destructive/30 bg-background/95 text-destructive/60 hover:text-destructive hover:bg-destructive/10 transition-colors shadow-sm"
                      onClick={() => { suppressRef.current = true; setActionError(''); setTerminateAgent(a); setTimeout(() => { suppressRef.current = false; }, 200); }}
                    >
                      <UserX className="h-3 w-3" />
                    </button>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>

      {/* Modals */}
      {hireOpen && (
        <HireModal
          agents={mergedAgents}
          onClose={() => setHireOpen(false)}
          onSuccess={load}
        />
      )}

      {terminateAgent && (
        <ConfirmDialog
          title={`Terminate ${terminateAgent.display_name}?`}
          body="This agent will be marked as terminated and removed from the active roster. This cannot be undone."
          confirmLabel="Terminate"
          onCancel={() => setTerminateAgent(null)}
          onConfirm={handleTerminateConfirm}
          loading={terminateLoading}
        />
      )}

      {reassignAgent && (
        <ReassignModal
          agent={reassignAgent}
          agents={mergedAgents}
          onClose={() => setReassignAgent(null)}
          onSuccess={load}
        />
      )}

    </div>
  );
}
