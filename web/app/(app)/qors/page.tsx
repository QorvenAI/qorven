'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALc2.

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
import { Plus, X, Search, MessageSquare, Settings, Trash2, MoreHorizontal,
  Cpu, Building2, Code2, Megaphone, ShoppingCart, HeadphonesIcon,
  UserCheck, Shield, BookOpen, DollarSign, User,
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

function OrgRoleBadge({ orgRole }: { orgRole?: string }) {
  if (!orgRole || orgRole === 'specialist') return null;
  const meta = ORG_ROLE_META[orgRole];
  if (!meta) return null;
  return (
    <span className={`inline-flex items-center gap-0.5 rounded border px-1 py-0.5 text-[10px] font-bold uppercase tracking-wide ${meta.color}`}>
      <meta.Icon className="h-2 w-2" />
      {meta.label}
    </span>
  );
}

export default function QorsPage() {
  const souls = useStore((s) => s.souls);
  const setSouls = useStore((s) => s.setSouls);
  // Skip the skeleton flash if the store already has data from a previous visit.
  const [loading, setLoading] = useState(souls.length === 0);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [search, setSearch] = useState('');
  const [roleFilter, setRoleFilter] = useState<string>('all');
  const [orgFilter, setOrgFilter] = useState<string>('all'); // all | csuite | specialists

  const load = useCallback((showSpinner = true) => {
    if (showSpinner) setLoading(true);
    agents.list()
      .then((data) => { setSouls(data); setLoading(false); })
      .catch((e) => { setError(e.message); setLoading(false); });
  }, [setSouls]);

  useEffect(() => { load(souls.length === 0); }, [load, souls.length]);

  const roles = useMemo(() => {
    const r = new Set(souls.map((s) => s.role).filter(Boolean));
    return Array.from(r);
  }, [souls]);

  const hasCsuite = useMemo(() => souls.some((s) => s.org_role && ORG_ROLE_META[s.org_role]), [souls]);

  const filtered = useMemo(() => {
    let list = souls;
    if (search) {
      const q = search.toLowerCase();
      list = list.filter((s) =>
        s.display_name.toLowerCase().includes(q) ||
        s.role?.toLowerCase().includes(q) ||
        s.org_role?.toLowerCase().includes(q) ||
        s.model?.toLowerCase().includes(q),
      );
    }
    if (roleFilter !== 'all') list = list.filter((s) => s.role === roleFilter);
    if (orgFilter === 'csuite') list = list.filter((s) => s.org_level === 'l1' || s.org_level === 'l2');
    if (orgFilter === 'specialists') list = list.filter((s) => !s.org_level || s.org_level === 'l3');
    return list;
  }, [souls, search, roleFilter, orgFilter]);

  return (
    <ErrorBoundary fallbackTitle="Failed to load Qors">
      <div className="p-5 space-y-5">
        {/* Header */}
        <div className="flex items-start justify-between gap-3 flex-wrap">
          <div>
            <h1 className="text-lg font-semibold">Your {brand.agentNamePlural}</h1>
            <p className="text-sm text-muted-foreground mt-1 max-w-xl">
              Pick someone to chat with — they remember your conversations, can help with tasks, and connect to your apps.
            </p>
          </div>
          <button onClick={() => setShowCreate(true)} className="qr-btn qr-btn-primary qr-btn-lg">
            <Plus className="h-4 w-4" />
            {buttons.newAgent}
          </button>
        </div>

        {/* Search + filter */}
        {souls.length > 0 && (
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative flex-1 max-w-sm min-w-40">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Search by name…"
                className="qr-input pl-9"
              />
              {search && (
                <button onClick={() => setSearch('')} className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
                  <X className="h-3.5 w-3.5" />
                </button>
              )}
            </div>
            {hasCsuite && (
              <div className="flex items-center gap-1">
                {(['all', 'csuite', 'specialists'] as const).map((f) => (
                  <button
                    key={f}
                    onClick={() => setOrgFilter(f)}
                    className={cn(
                      'rounded-full border px-3 py-1 text-xs transition-colors',
                      orgFilter === f
                        ? 'border-primary bg-primary/10 text-primary font-medium'
                        : 'border-border text-muted-foreground hover:text-foreground hover:bg-accent/40',
                    )}
                  >
                    {f === 'all' ? 'All' : f === 'csuite' ? 'C-Suite' : 'Specialists'}
                  </button>
                ))}
              </div>
            )}
            {roles.length > 1 && (
              <details className="relative">
                <summary className="list-none cursor-pointer rounded-lg border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground hover:text-foreground">
                  Filter: {roleFilter === 'all' ? 'All' : roleFilter}
                </summary>
                <div className="absolute right-0 top-full mt-1 w-40 rounded-lg border border-border bg-popover shadow-lg z-10 py-1">
                  {['all', ...roles].map((r) => (
                    <button
                      key={r}
                      onClick={() => setRoleFilter(r)}
                      className={cn(
                        'flex w-full items-center px-3 py-1.5 text-xs hover:bg-accent text-left capitalize',
                        roleFilter === r && 'bg-accent font-medium',
                      )}
                    >
                      {r === 'all' ? 'All roles' : r}
                    </button>
                  ))}
                </div>
              </details>
            )}
          </div>
        )}

        {/* Grid */}
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
            description={`No ${brand.agentNamePlural.toLowerCase()} match "${search}"`}
          />
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {filtered.map((soul) => (
              <QorCard key={soul.id} soul={soul} onDeleted={load} />
            ))}
          </div>
        )}
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

// ─── Qor Card ─────────────────────────────────────────────────────────────────
function QorCard({ soul, onDeleted }: { soul: Soul; onDeleted: () => void }) {
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
    idle: 'text-emerald-400',
    thinking: 'text-amber-400',
    running: 'text-blue-400',
    offline: 'text-muted-foreground',
    error: 'text-destructive',
  }[activity] ?? 'text-muted-foreground';

  const activityLabel = {
    idle: 'Online',
    thinking: 'Thinking...',
    running: 'Running',
    offline: 'Offline',
    error: 'Error',
  }[activity] ?? 'Idle';

  const orgMeta = soul.org_role ? ORG_ROLE_META[soul.org_role] : null;

  return (
    <div
      className={cn(
        'group relative rounded-xl border bg-card p-4 transition-all hover:border-primary/30',
        soul.org_level === 'l1' ? 'border-amber-400/30 hover:border-amber-400/50' :
        soul.org_level === 'l2' ? 'border-blue-400/20 hover:border-blue-400/40' :
        'border-border',
        activity === 'thinking' && 'ring-1 ring-amber-400/30',
        activity === 'running' && 'ring-1 ring-blue-400/30',
        deleting && 'opacity-50 pointer-events-none',
      )}
    >
      <div className="flex items-start gap-3">
        <div className={cn('flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-gradient-to-br text-base font-semibold text-white', soulGradient(soul.display_name))}>
          {soul.display_name.charAt(0).toUpperCase()}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5 flex-wrap">
            <p className="truncate text-sm font-semibold">{soul.display_name}</p>
            <OrgRoleBadge orgRole={soul.org_role} />
          </div>
          <p className="truncate text-xs text-muted-foreground">
            {soul.org_role && ORG_ROLE_META[soul.org_role]
              ? `${ORG_ROLE_META[soul.org_role]!.label} — ${soul.org_level === 'l1' ? 'Executive' : soul.org_level === 'l2' ? 'C-Suite' : 'Specialist'}`
              : soul.title || soul.role || 'Assistant'}
          </p>
          <span className={cn('mt-1 inline-flex items-center gap-1 text-2xs', activityColor)}>
            <span className="inline-block h-1.5 w-1.5 rounded-full bg-current" />
            {activityLabel}
          </span>
        </div>

        <div className="relative">
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

      {lastEvent && (
        <p className="mt-2.5 text-2xs text-muted-foreground truncate">{lastEvent}</p>
      )}

      <div className="mt-3 flex items-center gap-1.5">
        <button
          onClick={() => router.push(`/qors/${soul.id}`)}
          className="flex-1 flex items-center justify-center gap-1.5 rounded-lg bg-primary px-3 py-2 text-xs font-semibold text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          <MessageSquare className="h-3.5 w-3.5" />
          Start chat
        </button>
        <button
          onClick={() => router.push(`/qors/${soul.id}?tab=settings`)}
          className="flex h-8 w-8 items-center justify-center rounded-lg border border-border text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
          title="Settings"
          aria-label="Settings"
        >
          <Settings className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  );
}

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

  // Auto-set org_level when org_role changes
  const handleOrgRoleChange = (orgRole: string) => {
    const level = ORG_LEVEL_FOR_ROLE[orgRole] ?? 'l3';
    const budget = level === 'l1' ? 0 : level === 'l2' ? 50 : 10;
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
        model: form.model || models[0]?.model_id || 'kimi-k2.5',
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
      // Register in org_roster if an org role was set
      if (form.org_role && (created as any)?.id) {
        try {
          await orgApi.hire((created as any).id, form.org_level, form.org_role);
        } catch {
          // non-fatal — agent still created
        }
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
          <h2 className="text-lg font-semibold">Create New {brand.agentName}</h2>
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
              placeholder="e.g. Researcher"
              className="qr-input mt-1"
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-xs font-medium text-muted-foreground">Model</label>
              <select
                value={form.model}
                onChange={(e) => setForm({ ...form, model: e.target.value })}
                className="qr-select mt-1"
              >
                <option value="">Default</option>
                {models.map((m) => (
                  <option key={m.model_id} value={m.model_id}>{m.model_id}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="text-xs font-medium text-muted-foreground">Role</label>
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
              </select>
            </div>
          </div>

          {/* Org Role — optional C-suite designation */}
          <div>
            <label className="text-xs font-medium text-muted-foreground">
              Org Role <span className="text-muted-foreground/50">(optional — place in org hierarchy)</span>
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
          </div>

          {/* Org level + budget — shown only when org_role is set */}
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
                  <option value="l2">L2 — C-Suite</option>
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
              rows={4}
              placeholder="Instructions for this agent..."
              className="qr-textarea mt-1 font-mono text-xs"
            />
          </div>

          <div>
            <label className="text-xs font-medium text-muted-foreground">
              Temperature: {form.temperature.toFixed(2)}
            </label>
            <input
              type="range"
              min="0"
              max="1"
              step="0.05"
              value={form.temperature}
              onChange={(e) => setForm({ ...form, temperature: parseFloat(e.target.value) })}
              className="mt-1 w-full accent-primary"
            />
          </div>

          <button
            onClick={handleCreate}
            disabled={saving || !form.display_name}
            className="qr-btn qr-btn-primary qr-btn-lg w-full"
          >
            {saving ? buttons.creating : buttons.createAgent}
          </button>
        </div>
      </div>
    </div>
  );
}
