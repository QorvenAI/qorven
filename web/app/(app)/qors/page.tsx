'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { brand, buttons } from '@/lib/branding';
import { useEffect, useState, useMemo, useCallback } from 'react';
import { toast } from 'sonner';
import { useRouter } from 'next/navigation';
import { useStore } from '@/store';
import { agents } from '@/lib/api';
import { orgApi } from '@/lib/api-agents';
import { cn } from '@/lib/utils';
import { SoulCardSkeleton } from '@/components/skeletons';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { ErrorBoundary } from '@/components/error-boundary';
import { soulGradient } from '@/components/soul-card';
import { useSoulRun } from '@/hooks/use-soul';
import { useSelectedModels } from '@/hooks/use-selected-models';
import {
  Plus, X, Search, MessageSquare, Settings, Trash2, MoreHorizontal,
  Cpu, Building2, Code2, Megaphone, ShoppingCart, HeadphonesIcon,
  UserCheck, Shield, BookOpen, DollarSign, User, ChevronDown,
  Crown, Briefcase, Wrench, Activity,
} from 'lucide-react';
import type { Soul } from '@/types';

// ─── Org role display helpers ─────────────────────────────────────────────────
const ORG_ROLE_META: Record<string, { label: string; color: string; Icon: React.ElementType }> = {
  caio:  { label: 'CAIO',  color: 'bg-violet-500/15 text-violet-500 border-violet-500/20', Icon: Cpu },
  coo:   { label: 'COO',   color: 'bg-amber-500/15 text-amber-500 border-amber-500/20',   Icon: Building2 },
  cto:   { label: 'CTO',   color: 'bg-blue-500/15 text-blue-500 border-blue-500/20',       Icon: Code2 },
  cmo:   { label: 'CMO',   color: 'bg-pink-500/15 text-pink-500 border-pink-500/20',       Icon: Megaphone },
  cso:   { label: 'CSO',   color: 'bg-emerald-500/15 text-emerald-500 border-emerald-500/20', Icon: ShoppingCart },
  cco:   { label: 'CCO',   color: 'bg-cyan-500/15 text-cyan-500 border-cyan-500/20',       Icon: HeadphonesIcon },
  chro:  { label: 'CHRO',  color: 'bg-orange-500/15 text-orange-500 border-orange-500/20', Icon: UserCheck },
  ciso:  { label: 'CISO',  color: 'bg-red-500/15 text-red-500 border-red-500/20',          Icon: Shield },
  cko:   { label: 'CKO',   color: 'bg-teal-500/15 text-teal-500 border-teal-500/20',       Icon: BookOpen },
  cfo:   { label: 'CFO',   color: 'bg-lime-500/15 text-lime-600 border-lime-500/20',       Icon: DollarSign },
};

const TIER_META: Record<string, { label: string; sublabel: string; Icon: React.ElementType; border: string; accent: string }> = {
  l1: { label: 'Executive', sublabel: 'Strategic leadership', Icon: Crown,     border: 'border-amber-500/30', accent: 'text-amber-500' },
  l2: { label: 'Management', sublabel: 'Planning & coordination', Icon: Briefcase, border: 'border-blue-500/20', accent: 'text-blue-500' },
  l3: { label: 'Specialists', sublabel: 'Execution & delivery', Icon: Wrench,    border: 'border-border', accent: 'text-muted-foreground' },
};

function OrgRoleBadge({ orgRole }: { orgRole?: string }) {
  if (!orgRole || orgRole === 'specialist') return null;
  const meta = ORG_ROLE_META[orgRole];
  if (!meta) return null;
  return (
    <span className={`inline-flex items-center gap-0.5 rounded border px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wide ${meta.color}`}>
      <meta.Icon className="h-2.5 w-2.5" />
      {meta.label}
    </span>
  );
}

export default function QorsPage() {
  const souls = useStore((s) => s.souls);
  const setSouls = useStore((s) => s.setSouls);
  const [loading, setLoading] = useState(souls.length === 0);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [search, setSearch] = useState('');
  const [viewMode, setViewMode] = useState<'hierarchy' | 'grid'>('hierarchy');

  const load = useCallback((showSpinner = true) => {
    if (showSpinner) setLoading(true);
    agents.list()
      .then((data) => { setSouls(data); setLoading(false); })
      .catch((e) => { setError(e.message); setLoading(false); });
  }, [setSouls]);

  useEffect(() => { load(souls.length === 0); }, [load, souls.length]);

  const hasHierarchy = useMemo(() =>
    souls.some((s) => s.org_level === 'l1' || s.org_level === 'l2'),
  [souls]);

  const filtered = useMemo(() => {
    if (!search) return souls;
    const q = search.toLowerCase();
    return souls.filter((s) =>
      s.display_name.toLowerCase().includes(q) ||
      s.role?.toLowerCase().includes(q) ||
      s.org_role?.toLowerCase().includes(q) ||
      s.model?.toLowerCase().includes(q),
    );
  }, [souls, search]);

  // Group by org level for hierarchy view
  const grouped = useMemo(() => {
    const l1 = filtered.filter((s) => s.org_level === 'l1');
    const l2 = filtered.filter((s) => s.org_level === 'l2');
    const l3 = filtered.filter((s) => !s.org_level || s.org_level === 'l3' || s.org_level === 'customer_facing');
    return { l1, l2, l3 };
  }, [filtered]);

  return (
    <ErrorBoundary fallbackTitle="Failed to load Qors">
      <div className="flex flex-col h-full overflow-hidden">
        {/* Header */}
        <div className="p-5 pb-0 space-y-4 shrink-0">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h1 className="text-lg font-semibold">Organization</h1>
              <p className="text-sm text-muted-foreground mt-0.5">
                {souls.length} agent{souls.length !== 1 ? 's' : ''} across {hasHierarchy ? '3 tiers' : 'your workspace'}
              </p>
            </div>
            <button onClick={() => setShowCreate(true)} className="qr-btn qr-btn-primary qr-btn-lg">
              <Plus className="h-4 w-4" />
              {buttons.newAgent}
            </button>
          </div>

          {/* Search + view toggle */}
          {souls.length > 0 && (
            <div className="flex items-center gap-2">
              <div className="relative flex-1 max-w-sm min-w-40">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <input
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Search agents…"
                  className="qr-input pl-9"
                />
                {search && (
                  <button onClick={() => setSearch('')} className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
                    <X className="h-3.5 w-3.5" />
                  </button>
                )}
              </div>
              {hasHierarchy && (
                <div className="flex rounded-lg border border-border overflow-hidden">
                  <button
                    onClick={() => setViewMode('hierarchy')}
                    className={cn('px-3 py-1.5 text-xs transition-colors', viewMode === 'hierarchy' ? 'bg-primary/10 text-primary font-medium' : 'text-muted-foreground hover:text-foreground')}
                  >
                    Hierarchy
                  </button>
                  <button
                    onClick={() => setViewMode('grid')}
                    className={cn('px-3 py-1.5 text-xs transition-colors', viewMode === 'grid' ? 'bg-primary/10 text-primary font-medium' : 'text-muted-foreground hover:text-foreground')}
                  >
                    Grid
                  </button>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-5 pt-4">
          {loading ? (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {Array.from({ length: 6 }).map((_, i) => <SoulCardSkeleton key={i} />)}
            </div>
          ) : error ? (
            <EmptyState
              icon={emptyStates.souls.icon}
              title="Failed to load"
              description={error}
              actionLabel="Retry"
              onAction={load}
            />
          ) : souls.length === 0 ? (
            <EmptyState
              {...emptyStates.souls}
              onAction={() => setShowCreate(true)}
            />
          ) : filtered.length === 0 ? (
            <EmptyState
              icon={emptyStates.souls.icon}
              title="No matches"
              description={`No agents match "${search}"`}
            />
          ) : viewMode === 'hierarchy' && hasHierarchy ? (
            <HierarchyView grouped={grouped} onDeleted={load} />
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {filtered.map((soul) => (
                <QorCard key={soul.id} soul={soul} onDeleted={load} />
              ))}
            </div>
          )}
        </div>
      </div>

      {showCreate && (
        <CreateQorDialog
          onClose={() => setShowCreate(false)}
          onCreated={() => { setShowCreate(false); load(); }}
        />
      )}
    </ErrorBoundary>
  );
}

// ─── Hierarchy View ──────────────────────────────────────────────────────────
function HierarchyView({ grouped, onDeleted }: { grouped: { l1: Soul[]; l2: Soul[]; l3: Soul[] }; onDeleted: () => void }) {
  return (
    <div className="space-y-6">
      {(['l1', 'l2', 'l3'] as const).map((tier) => {
        const agents = grouped[tier];
        if (agents.length === 0) return null;
        const meta = TIER_META[tier]!;
        const TierIcon = meta.Icon;
        return (
          <div key={tier}>
            {/* Tier header */}
            <div className="flex items-center gap-2 mb-3">
              <TierIcon className={cn('h-4 w-4', meta.accent)} />
              <h2 className="text-sm font-semibold">{meta.label}</h2>
              <span className="text-2xs text-muted-foreground">{meta.sublabel}</span>
              <span className="ml-auto text-2xs text-muted-foreground/50">{agents.length}</span>
            </div>

            {/* Cards */}
            <div className={cn(
              'grid gap-3',
              tier === 'l1' ? 'grid-cols-1 sm:grid-cols-2' :
              tier === 'l2' ? 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3' :
              'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4',
            )}>
              {agents.map((soul) => (
                <QorCard key={soul.id} soul={soul} onDeleted={onDeleted} compact={tier === 'l3'} />
              ))}
            </div>

            {/* Divider between tiers */}
            {tier !== 'l3' && (
              <div className="mt-5 flex items-center gap-2 text-muted-foreground/20">
                <div className="flex-1 h-px bg-current" />
                <ChevronDown className="h-3 w-3" />
                <div className="flex-1 h-px bg-current" />
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

// ─── Qor Card ─────────────────────────────────────────────────────────────────
function QorCard({ soul, onDeleted, compact }: { soul: Soul; onDeleted: () => void; compact?: boolean }) {
  const router = useRouter();
  const { activity, lastEvent } = useSoulRun(soul.id);
  const [menuOpen, setMenuOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const handleDelete = async (e: React.MouseEvent) => {
    e.stopPropagation();
    e.preventDefault();
    if (!confirm(`Delete ${soul.display_name}? This cannot be undone.`)) return;
    setDeleting(true);
    try {
      await agents.delete(soul.id);
      toast.success(`${soul.display_name} deleted`);
      onDeleted();
    } catch {
      toast.error('Failed to delete agent');
      setDeleting(false);
    }
  };

  const activityColor = {
    idle: 'bg-emerald-400',
    thinking: 'bg-amber-400',
    running: 'bg-blue-400',
    offline: 'bg-muted-foreground/40',
    error: 'bg-destructive',
  }[activity] ?? 'bg-muted-foreground/40';

  const activityLabel = {
    idle: 'Online',
    thinking: 'Thinking…',
    running: 'Running',
    offline: 'Offline',
    error: 'Error',
  }[activity] ?? 'Idle';

  const tierBorder = soul.org_level === 'l1' ? 'border-amber-500/30' :
                     soul.org_level === 'l2' ? 'border-blue-400/20' : 'border-border';

  return (
    <div
      onClick={() => router.push(`/qors/${soul.id}`)}
      className={cn(
        'group relative rounded-xl border bg-card transition-all cursor-pointer hover:shadow-md hover:border-primary/30',
        tierBorder,
        compact ? 'p-3' : 'p-4',
        activity === 'thinking' && 'ring-1 ring-amber-400/30',
        activity === 'running' && 'ring-1 ring-blue-400/30',
        deleting && 'opacity-50 pointer-events-none',
      )}
    >
      <div className="flex items-start gap-3">
        {/* Avatar */}
        <div className="relative">
          <div className={cn(
            'flex items-center justify-center rounded-full bg-gradient-to-br font-semibold text-white',
            soulGradient(soul.display_name),
            compact ? 'h-9 w-9 text-xs' : 'h-11 w-11 text-sm',
          )}>
            {soul.display_name.charAt(0).toUpperCase()}
          </div>
          <span className={cn('absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full border-2 border-card', activityColor)} title={activityLabel} />
        </div>

        {/* Info */}
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5 flex-wrap">
            <p className={cn('truncate font-semibold', compact ? 'text-xs' : 'text-sm')}>{soul.display_name}</p>
            <OrgRoleBadge orgRole={soul.org_role} />
          </div>
          <p className={cn('truncate text-muted-foreground', compact ? 'text-2xs' : 'text-xs')}>
            {soul.org_role && ORG_ROLE_META[soul.org_role]
              ? `${ORG_ROLE_META[soul.org_role]!.label} — ${soul.org_level === 'l1' ? 'Executive' : soul.org_level === 'l2' ? 'Manager' : 'Specialist'}`
              : soul.title || soul.role || 'Assistant'}
          </p>
          {!compact && lastEvent && (
            <p className="mt-1 text-2xs text-muted-foreground/60 truncate">{lastEvent}</p>
          )}
        </div>

        {/* Menu */}
        <div className="relative shrink-0">
          <button
            onClick={(e) => { e.stopPropagation(); setMenuOpen(!menuOpen); }}
            className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground opacity-0 group-hover:opacity-100 hover:bg-muted transition-all"
            aria-label="More options"
          >
            <MoreHorizontal className="h-4 w-4" />
          </button>
          {menuOpen && (
            <>
              <div className="fixed inset-0 z-40" onClick={(e) => { e.stopPropagation(); setMenuOpen(false); }} />
              <div className="absolute right-0 top-8 z-50 w-40 rounded-lg border border-border bg-popover p-1 shadow-lg">
                <button
                  onClick={(e) => { e.stopPropagation(); setMenuOpen(false); router.push(`/qors/${soul.id}`); }}
                  className="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-sm hover:bg-accent transition-colors"
                >
                  <MessageSquare className="h-3.5 w-3.5" /> Chat
                </button>
                <button
                  onClick={(e) => { e.stopPropagation(); setMenuOpen(false); router.push(`/qors/${soul.id}?tab=settings`); }}
                  className="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-sm hover:bg-accent transition-colors"
                >
                  <Settings className="h-3.5 w-3.5" /> Settings
                </button>
                <div className="my-1 h-px bg-border" />
                <button
                  onClick={handleDelete}
                  className="flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-sm text-destructive hover:bg-destructive/10 transition-colors"
                >
                  <Trash2 className="h-3.5 w-3.5" /> Delete
                </button>
              </div>
            </>
          )}
        </div>
      </div>

      {/* Quick action footer */}
      {!compact && (
        <div className="mt-3 flex items-center gap-1.5">
          <button
            onClick={(e) => { e.stopPropagation(); router.push(`/qors/${soul.id}`); }}
            className="flex-1 flex items-center justify-center gap-1.5 rounded-lg bg-primary/10 px-3 py-1.5 text-xs font-medium text-primary hover:bg-primary/20 transition-colors"
          >
            <MessageSquare className="h-3 w-3" />
            Chat
          </button>
          <button
            onClick={(e) => { e.stopPropagation(); router.push(`/qors/${soul.id}?tab=inbox`); }}
            className="flex items-center justify-center gap-1 rounded-lg border border-border px-2.5 py-1.5 text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
          >
            <Activity className="h-3 w-3" />
            Work
          </button>
          <button
            onClick={(e) => { e.stopPropagation(); router.push(`/qors/${soul.id}?tab=settings`); }}
            className="flex h-7 w-7 items-center justify-center rounded-lg border border-border text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
            title="Settings"
          >
            <Settings className="h-3 w-3" />
          </button>
        </div>
      )}
    </div>
  );
}

// ─── Constants ────────────────────────────────────────────────────────────────
const ORG_ROLES = [
  { value: '', label: 'None (general)' },
  { value: 'caio',     label: 'CAIO — Fleet Overseer' },
  { value: 'coo',      label: 'COO — Operations' },
  { value: 'cto',      label: 'CTO — Engineering' },
  { value: 'cmo',      label: 'CMO — Marketing' },
  { value: 'cso',      label: 'CSO — Sales' },
  { value: 'cco',      label: 'CCO — Customer' },
  { value: 'chro',     label: 'CHRO — HR' },
  { value: 'ciso',     label: 'CISO — Security' },
  { value: 'cko',      label: 'CKO — Knowledge' },
  { value: 'cfo',      label: 'CFO — Finance' },
  { value: 'specialist', label: 'Specialist (L3)' },
];

const ORG_LEVEL_FOR_ROLE: Record<string, string> = {
  caio: 'l1', coo: 'l1',
  cto: 'l2', cmo: 'l2', cso: 'l2', cco: 'l2', chro: 'l2', ciso: 'l2', cko: 'l2', cfo: 'l2',
  specialist: 'l3',
};

// ─── Create Dialog ─────────────────────────────────────────────────────────────
function CreateQorDialog({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const { models } = useSelectedModels();
  const [form, setForm] = useState({
    display_name: '',
    model: '',
    role: 'worker',
    system_prompt: '',
    temperature: 0.5,
    org_role: '',
    org_level: 'l3',
    monthly_budget_usd: 10,
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const handleOrgRoleChange = (orgRole: string) => {
    const level = ORG_LEVEL_FOR_ROLE[orgRole] ?? 'l3';
    const budget = level === 'l1' ? 100 : level === 'l2' ? 50 : 10;
    setForm((f) => ({ ...f, org_role: orgRole, org_level: level, monthly_budget_usd: budget }));
  };

  const handleCreate = async () => {
    if (!form.display_name) return;
    setSaving(true);
    setError('');
    try {
      const created = await agents.create({
        display_name: form.display_name,
        agent_key: form.display_name.toLowerCase().replace(/[^a-z0-9]/g, '-'),
        model: form.model || models[0]?.model_id || 'auto',
        role: form.role,
        system_prompt: form.system_prompt,
        temperature: form.temperature,
        memory_enabled: true,
        tool_profile: 'full',
        max_tool_iterations: 20,
        context_window: 128000,
        org_role: form.org_role || undefined,
        org_level: form.org_level,
      } as any);
      if (form.org_role && (created as any)?.id) {
        try {
          await orgApi.hire((created as any).id, form.org_level, form.org_role);
        } catch { /* non-fatal */ }
      }
      toast.success(`${form.display_name} created`);
      onCreated();
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to create';
      setError(msg);
      toast.error(msg);
      setSaving(false);
    }
  };

  const hasOrgRole = !!form.org_role;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div className="w-full max-w-lg rounded-xl border border-border bg-card p-6 shadow-xl" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-5">
          <h2 className="text-lg font-semibold">Hire New Agent</h2>
          <button onClick={onClose} className="text-muted-foreground hover:text-foreground">
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="space-y-4">
          {error && (
            <div className="rounded-lg bg-destructive/10 border border-destructive/20 px-3 py-2 text-sm text-destructive">
              {error}
            </div>
          )}

          <div>
            <label className="text-xs font-medium text-muted-foreground">Name *</label>
            <input
              value={form.display_name}
              onChange={(e) => setForm({ ...form, display_name: e.target.value })}
              autoFocus
              placeholder="e.g. Marcus Reid"
              className="qr-input mt-1"
            />
          </div>

          {/* Org Role — determines placement in hierarchy */}
          <div>
            <label className="text-xs font-medium text-muted-foreground">
              Position
            </label>
            <select
              value={form.org_role}
              onChange={(e) => handleOrgRoleChange(e.target.value)}
              className="qr-select mt-1"
            >
              {ORG_ROLES.map((r) => (
                <option key={r.value} value={r.value}>{r.label}</option>
              ))}
            </select>
            {hasOrgRole && (
              <p className="mt-1 text-2xs text-muted-foreground">
                Tier: <span className="font-medium">{form.org_level === 'l1' ? 'Executive (Opus)' : form.org_level === 'l2' ? 'Management (Sonnet)' : 'Specialist (Haiku)'}</span>
                {' · '}Budget: <span className="font-medium">${form.monthly_budget_usd}/mo</span>
              </p>
            )}
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-xs font-medium text-muted-foreground">Model</label>
              <select
                value={form.model}
                onChange={(e) => setForm({ ...form, model: e.target.value })}
                className="qr-select mt-1"
              >
                <option value="">Auto (tier-based)</option>
                {models.map((m) => (
                  <option key={m.model_id} value={m.model_id}>{m.model_id}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="text-xs font-medium text-muted-foreground">Archetype</label>
              <select
                value={form.role}
                onChange={(e) => setForm({ ...form, role: e.target.value })}
                className="qr-select mt-1"
              >
                <option value="supervisor">Supervisor</option>
                <option value="worker">Worker</option>
                <option value="researcher">Researcher</option>
                <option value="developer">Developer</option>
                <option value="writer">Writer</option>
                <option value="analyst">Analyst</option>
              </select>
            </div>
          </div>

          {/* Budget override — only shown for org roles */}
          {hasOrgRole && (
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="text-xs font-medium text-muted-foreground">Org Level</label>
                <select
                  value={form.org_level}
                  onChange={(e) => setForm({ ...form, org_level: e.target.value })}
                  className="qr-select mt-1"
                >
                  <option value="l1">L1 — Executive</option>
                  <option value="l2">L2 — Management</option>
                  <option value="l3">L3 — Specialist</option>
                  <option value="customer_facing">Customer-Facing</option>
                </select>
              </div>
              <div>
                <label className="text-xs font-medium text-muted-foreground">Monthly Budget (USD)</label>
                <input
                  type="number"
                  min={0}
                  step={5}
                  value={form.monthly_budget_usd}
                  onChange={(e) => setForm({ ...form, monthly_budget_usd: parseFloat(e.target.value) || 0 })}
                  className="qr-input mt-1"
                  placeholder="0 = unlimited"
                />
              </div>
            </div>
          )}

          <div>
            <label className="text-xs font-medium text-muted-foreground">System Prompt</label>
            <textarea
              value={form.system_prompt}
              onChange={(e) => setForm({ ...form, system_prompt: e.target.value })}
              rows={3}
              placeholder="Custom instructions (optional — role-based defaults apply)…"
              className="qr-textarea mt-1 font-mono text-xs"
            />
          </div>

          <button
            onClick={handleCreate}
            disabled={saving || !form.display_name}
            className="qr-btn qr-btn-primary qr-btn-lg w-full"
          >
            {saving ? 'Creating…' : 'Hire Agent'}
          </button>
        </div>
      </div>
    </div>
  );
}
