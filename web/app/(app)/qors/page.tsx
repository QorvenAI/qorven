'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useMemo, useCallback } from 'react';
import { toast } from 'sonner';
import { useRouter } from 'next/navigation';
import { useStore } from '@/store';
import { agents } from '@/lib/api';
import { cn } from '@/lib/utils';
import { SoulCardSkeleton } from '@/components/skeletons';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { ErrorBoundary } from '@/components/error-boundary';
import { soulGradient } from '@/components/soul-card';
import { useSoulRun } from '@/hooks/use-soul';
import { useSelectedModels, type SelectedModel } from '@/hooks/use-selected-models';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { Card, CardContent } from '@/components/ui/card';
import { Avatar, AvatarFallback, AvatarStatus } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip';
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent,
  DropdownMenuItem, DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu';
import {
  Plus, Search as SearchIcon, MessageSquare, Settings, Trash2, MoreHorizontal,
  Cpu, Building2, Code2, Megaphone, ShoppingCart, HeadphonesIcon,
  UserCheck, Shield, BookOpen, DollarSign,
  Crown, Briefcase, Wrench, LayoutGrid, List, Users,
} from 'lucide-react';
import type { Soul } from '@/types';

// ─── Org role metadata ─────────────────────────────────────────────────────────
const ORG_ROLE_META: Record<string, { label: string; Icon: React.ElementType }> = {
  caio:  { label: 'CAIO',  Icon: Cpu },
  coo:   { label: 'COO',   Icon: Building2 },
  cto:   { label: 'CTO',   Icon: Code2 },
  cmo:   { label: 'CMO',   Icon: Megaphone },
  cso:   { label: 'CSO',   Icon: ShoppingCart },
  cco:   { label: 'CCO',   Icon: HeadphonesIcon },
  chro:  { label: 'CHRO',  Icon: UserCheck },
  ciso:  { label: 'CISO',  Icon: Shield },
  cko:   { label: 'CKO',   Icon: BookOpen },
  cfo:   { label: 'CFO',   Icon: DollarSign },
};

const TIER_META = {
  l1: { label: 'Executive',    sublabel: 'Strategic leadership',      Icon: Crown,     accent: 'text-amber-400' },
  l2: { label: 'Management',   sublabel: 'Planning & coordination',   Icon: Briefcase, accent: 'text-primary' },
  l3: { label: 'Specialists',  sublabel: 'Execution & delivery',      Icon: Wrench,    accent: 'text-muted-foreground' },
} as const;

// Activity → AvatarStatus variant mapping
const ACTIVITY_STATUS: Record<string, 'online' | 'busy' | 'away' | 'offline'> = {
  idle:     'online',
  thinking: 'busy',
  running:  'busy',
  error:    'away',
  offline:  'offline',
};

const ACTIVITY_LABEL: Record<string, string> = {
  idle:     'Online',
  thinking: 'Thinking…',
  running:  'Working',
  error:    'Error',
  offline:  'Offline',
};

// ─── OrgRoleBadge ──────────────────────────────────────────────────────────────
function OrgRoleBadge({ orgRole }: { orgRole?: string }) {
  if (!orgRole || orgRole === 'specialist') return null;
  const meta = ORG_ROLE_META[orgRole];
  if (!meta) return null;
  return (
    <Badge variant="secondary" className="h-5 gap-1 px-1.5 text-[10px] font-semibold uppercase tracking-wider">
      <meta.Icon className="size-2.5" />
      {meta.label}
    </Badge>
  );
}

// ─── Model name shortener ──────────────────────────────────────────────────────
function shortModel(model: string): string {
  if (!model) return '';
  if (model.startsWith('claude-')) {
    const m = model.replace(/^claude-/, '').replace(/-\d{8,}$/, '');
    const parts = m.split('-');
    const name = parts[0] ? parts[0].charAt(0).toUpperCase() + parts[0].slice(1) : '';
    const ver  = parts.slice(1).join('.').replace(/\.$/, '');
    return ver ? `${name} ${ver}` : name;
  }
  if (model.startsWith('gpt-')) return model.replace('gpt-', 'GPT-');
  if (model.startsWith('gemini-')) return 'Gemini ' + model.replace('gemini-', '').replace(/-/g, ' ');
  return model.length > 16 ? model.slice(0, 14) + '…' : model;
}

// ─── QorCard ───────────────────────────────────────────────────────────────────
function QorCard({ soul, onDeleted }: { soul: Soul; onDeleted: () => void }) {
  const router = useRouter();
  const { activity, lastEvent, tokensToday } = useSoulRun(soul.id);
  const [deleting, setDeleting] = useState(false);

  const handleDelete = async (e: React.MouseEvent) => {
    e.stopPropagation();
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

  const statusVariant = ACTIVITY_STATUS[activity] ?? 'offline';
  const statusLabel   = ACTIVITY_LABEL[activity]  ?? 'Offline';
  const isActive      = activity === 'thinking' || activity === 'running';

  const roleDesc = soul.org_role && ORG_ROLE_META[soul.org_role]
    ? `${ORG_ROLE_META[soul.org_role]!.label} — ${soul.org_level === 'l1' ? 'Executive' : soul.org_level === 'l2' ? 'Management' : 'Specialist'}`
    : soul.title || soul.role || 'Assistant';

  const modelLabel = shortModel(soul.model);
  const spentUSD   = soul.credit_used_cents ? `$${(soul.credit_used_cents / 100).toFixed(2)}` : null;
  const tokensK    = tokensToday > 0
    ? (tokensToday >= 1000 ? `${(tokensToday / 1000).toFixed(1)}k tok` : `${tokensToday} tok`)
    : null;

  const caps: string[] = [];
  if (soul.web_search_enabled) caps.push('Web');
  if (soul.memory_enabled) caps.push('Memory');
  if (soul.can_delegate) caps.push('Delegate');

  return (
    <Card
      onClick={() => !deleting && router.push(`/qors/${soul.id}`)}
      className={cn(
        'group relative cursor-pointer transition-all duration-150',
        'hover:border-primary/40 hover:shadow-md hover:-translate-y-px',
        isActive && 'ring-1 ring-primary/20',
        deleting && 'opacity-50 pointer-events-none',
      )}
    >
      <CardContent className="p-4">

        {/* Identity row */}
        <div className="flex items-start gap-3">
          <div className="relative shrink-0">
            <Avatar className="size-10">
              <AvatarFallback className={cn('bg-gradient-to-br font-semibold text-white text-sm', soulGradient(soul.display_name))}>
                {soul.display_name.charAt(0).toUpperCase()}
              </AvatarFallback>
            </Avatar>
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="absolute -bottom-0.5 -right-0.5">
                  <AvatarStatus variant={statusVariant} className="size-2.5 border-2 border-card" />
                </span>
              </TooltipTrigger>
              <TooltipContent side="bottom" className="text-xs">{statusLabel}</TooltipContent>
            </Tooltip>
          </div>

          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-1.5 flex-wrap">
              <p className="truncate text-sm font-semibold leading-tight">{soul.display_name}</p>
              <OrgRoleBadge orgRole={soul.org_role} />
            </div>
            <p className="truncate text-xs text-muted-foreground mt-0.5">{roleDesc}</p>
          </div>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                onClick={(e) => e.stopPropagation()}
                className="opacity-0 group-hover:opacity-100 transition-opacity h-7 w-7 p-0 shrink-0"
                aria-label="More options"
              >
                <MoreHorizontal className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-40" onClick={(e) => e.stopPropagation()}>
              <DropdownMenuItem onClick={() => router.push(`/qors/${soul.id}`)}>
                <MessageSquare className="size-3.5 mr-2" /> Chat
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => router.push(`/qors/${soul.id}?tab=settings`)}>
                <Settings className="size-3.5 mr-2" /> Settings
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={handleDelete} className="text-destructive focus:text-destructive focus:bg-destructive/10">
                <Trash2 className="size-3.5 mr-2" /> Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>

        {/* Meta row — model · tokens · spend */}
        {(modelLabel || tokensK || spentUSD) && (
          <div className="mt-2.5 flex items-center gap-2 text-[11px] text-muted-foreground/70">
            {modelLabel && <span className="font-medium">{modelLabel}</span>}
            {tokensK    && <><span className="text-border">·</span><span>{tokensK}</span></>}
            {spentUSD   && <><span className="text-border">·</span><span>{spentUSD}</span></>}
          </div>
        )}

        {/* Last event or capability pills */}
        {lastEvent ? (
          <p className="mt-1 truncate text-[11px] text-muted-foreground/50">{lastEvent}</p>
        ) : caps.length > 0 ? (
          <div className="mt-1.5 flex items-center gap-1 flex-wrap">
            {caps.map((c) => (
              <span key={c} className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground font-medium">{c}</span>
            ))}
          </div>
        ) : null}

        {/* Hover action strip */}
        <div className="mt-3 flex items-center gap-1.5 opacity-0 group-hover:opacity-100 transition-opacity">
          <Button
            variant="outline"
            size="sm"
            className="flex-1 h-7 text-xs"
            onClick={(e) => { e.stopPropagation(); router.push(`/qors/${soul.id}`); }}
          >
            <MessageSquare className="size-3 mr-1.5" />
            Chat
          </Button>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 w-7 p-0"
                onClick={(e) => { e.stopPropagation(); router.push(`/qors/${soul.id}?tab=settings`); }}
              >
                <Settings className="size-3" />
              </Button>
            </TooltipTrigger>
            <TooltipContent className="text-xs">Settings</TooltipContent>
          </Tooltip>
        </div>

      </CardContent>
    </Card>
  );
}

// ─── Tier section ──────────────────────────────────────────────────────────────
function TierSection({
  tier,
  souls,
  onDeleted,
  viewMode,
}: {
  tier: 'l1' | 'l2' | 'l3';
  souls: Soul[];
  onDeleted: () => void;
  viewMode: 'hierarchy' | 'grid';
}) {
  if (souls.length === 0) return null;
  const meta = TIER_META[tier];
  const TierIcon = meta.Icon;

  // All tiers use the same 3-col grid — uniform card size, content fills the space
  const gridCols = 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4';

  return (
    <div className="space-y-3">
      {/* Tier header */}
      <div className="flex items-center gap-2">
        <TierIcon className={cn('size-3.5 shrink-0', meta.accent)} />
        <span className="text-sm font-semibold">{meta.label}</span>
        <span className="text-xs text-muted-foreground">{meta.sublabel}</span>
        <span className="ml-auto text-xs tabular-nums text-muted-foreground/50">{souls.length}</span>
      </div>

      {/* Cards */}
      <div className={cn('grid gap-3', gridCols)}>
        {souls.map((soul) => (
          <QorCard key={soul.id} soul={soul} onDeleted={onDeleted} />
        ))}
      </div>
    </div>
  );
}

// ─── HierarchyView ─────────────────────────────────────────────────────────────
function HierarchyView({ grouped, onDeleted }: {
  grouped: { l1: Soul[]; l2: Soul[]; l3: Soul[] };
  onDeleted: () => void;
}) {
  const tiers = ['l1', 'l2', 'l3'] as const;
  const nextTier: Record<string, 'l2' | 'l3'> = { l1: 'l2', l2: 'l3' };
  return (
    <div className="space-y-8">
      {tiers.map((tier) => (
        grouped[tier].length > 0 && (
          <div key={tier} className="space-y-6">
            <TierSection tier={tier} souls={grouped[tier]} onDeleted={onDeleted} viewMode="hierarchy" />
            {tier !== 'l3' && grouped[nextTier[tier]!]?.length > 0 && (
              <Separator className="opacity-40" />
            )}
          </div>
        )
      ))}
    </div>
  );
}

// ─── Page ──────────────────────────────────────────────────────────────────────
export default function QorsPage() {
  const souls    = useStore((s) => s.souls);
  const setSouls = useStore((s) => s.setSouls);
  const [loading,    setLoading]    = useState(souls.length === 0);
  const [error,      setError]      = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [search,     setSearch]     = useState('');
  const [viewMode,   setViewMode]   = useState<'hierarchy' | 'grid'>('hierarchy');

  const load = useCallback((showSpinner = true) => {
    if (showSpinner) setLoading(true);
    agents.list()
      .then((data) => { setSouls(data); setLoading(false); })
      .catch((e)   => { setError(e.message); setLoading(false); });
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

  const grouped = useMemo(() => ({
    l1: filtered.filter((s) => s.org_level === 'l1'),
    l2: filtered.filter((s) => s.org_level === 'l2'),
    l3: filtered.filter((s) => !s.org_level || s.org_level === 'l3' || s.org_level === 'customer_facing'),
  }), [filtered]);

  return (
    <div className="flex flex-col h-full overflow-hidden">

      {/* ── Header ── */}
      <CanvasHeader
        title="Qors"
        description={`${souls.length} agent${souls.length !== 1 ? 's' : ''}${hasHierarchy ? ' across 3 tiers' : ''}`}
        actions={
          <Button size="sm" onClick={() => setShowCreate(true)}>
            <Plus className="size-4" />
            New Qor
          </Button>
        }
      />

      {/* ── Toolbar ── */}
      <div className="flex items-center gap-3 px-6 pb-4 shrink-0">
        {/* Search */}
        <div className="relative flex-1 max-w-sm">
          <SearchIcon className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground pointer-events-none" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search agents…"
            className="pl-8 h-8 text-sm"
          />
        </div>

        {/* View toggle */}
        <div className="flex items-center rounded-md border border-border overflow-hidden shrink-0">
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                onClick={() => setViewMode('hierarchy')}
                className={cn(
                  'flex items-center justify-center h-8 w-8 transition-colors',
                  viewMode === 'hierarchy'
                    ? 'bg-primary/10 text-primary'
                    : 'text-muted-foreground hover:text-foreground hover:bg-muted',
                )}
              >
                <List className="size-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent className="text-xs">Hierarchy view</TooltipContent>
          </Tooltip>
          <Separator orientation="vertical" className="h-5" />
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                onClick={() => setViewMode('grid')}
                className={cn(
                  'flex items-center justify-center h-8 w-8 transition-colors',
                  viewMode === 'grid'
                    ? 'bg-primary/10 text-primary'
                    : 'text-muted-foreground hover:text-foreground hover:bg-muted',
                )}
              >
                <LayoutGrid className="size-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent className="text-xs">Grid view</TooltipContent>
          </Tooltip>
        </div>
      </div>

      {/* ── Content ── */}
      <div className="flex-1 overflow-y-auto px-6 pb-8">
        <ErrorBoundary>
          {loading ? (
            <div className="grid gap-3 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
              {Array.from({ length: 6 }).map((_, i) => <SoulCardSkeleton key={i} />)}
            </div>
          ) : error ? (
            <EmptyState
              icon={SearchIcon}
              title="Failed to load"
              description={error}
              onAction={() => load()}
              actionLabel="Retry"
            />
          ) : filtered.length === 0 ? (
            <EmptyState
              icon={search ? SearchIcon : Users}
              title={search ? 'No matches' : emptyStates.souls.title}
              description={search ? `No agents match "${search}"` : emptyStates.souls.description}
              onAction={search ? undefined : () => setShowCreate(true)}
              actionLabel={search ? undefined : emptyStates.souls.actionLabel}
            />
          ) : viewMode === 'grid' ? (
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {filtered.map((soul) => (
                <QorCard key={soul.id} soul={soul} onDeleted={load} />
              ))}
            </div>
          ) : (
            <HierarchyView grouped={grouped} onDeleted={load} />
          )}
        </ErrorBoundary>
      </div>

      {/* ── Create dialog ── */}
      {showCreate && <CreateQorDialog onClose={() => setShowCreate(false)} onCreated={() => { setShowCreate(false); load(); }} />}
    </div>
  );
}

// ─── Constants ─────────────────────────────────────────────────────────────────
const ORG_ROLES = [
  { value: '',          label: 'None (general)' },
  { value: 'caio',      label: 'CAIO — Fleet Overseer' },
  { value: 'coo',       label: 'COO — Operations' },
  { value: 'cto',       label: 'CTO — Engineering' },
  { value: 'cmo',       label: 'CMO — Marketing' },
  { value: 'cso',       label: 'CSO — Sales' },
  { value: 'cco',       label: 'CCO — Customer' },
  { value: 'chro',      label: 'CHRO — HR' },
  { value: 'ciso',      label: 'CISO — Security' },
  { value: 'cko',       label: 'CKO — Knowledge' },
  { value: 'cfo',       label: 'CFO — Finance' },
  { value: 'specialist', label: 'Specialist (L3)' },
];

const ORG_LEVEL_FOR_ROLE: Record<string, string> = {
  caio: 'l1', coo: 'l1',
  cto: 'l2', cmo: 'l2', cso: 'l2', cco: 'l2', chro: 'l2', ciso: 'l2', cko: 'l2', cfo: 'l2',
  specialist: 'l3',
};

// ─── Create Dialog ─────────────────────────────────────────────────────────────
function CreateQorDialog({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const { models: selectedModels, loading: modelsLoading } = useSelectedModels();
  const [name,    setName]    = useState('');
  const [role,    setRole]    = useState('');
  const [model,   setModel]   = useState('');
  const [saving,  setSaving]  = useState(false);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onClose]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setSaving(true);
    try {
      const payload: Record<string, string> = { display_name: name.trim() };
      if (model) payload.model = model;
      if (role) {
        payload.org_role  = role;
        payload.org_level = ORG_LEVEL_FOR_ROLE[role] ?? 'l3';
      }
      await agents.create(payload as Parameters<typeof agents.create>[0]);
      toast.success(`${name.trim()} created`);
      onCreated();
    } catch {
      toast.error('Failed to create agent');
      setSaving(false);
    }
  };

  return (
    <>
      <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm" onClick={onClose} />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="create-qor-title"
        className="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border border-border bg-card p-6 shadow-xl"
      >
        <div className="mb-5">
          <h2 id="create-qor-title" className="text-base font-semibold">New Qor</h2>
          <p className="text-sm text-muted-foreground mt-0.5">Add a new AI agent to your workspace.</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <label htmlFor="qor-name" className="text-sm font-medium">Name</label>
            <Input
              id="qor-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. Marketing Lead"
              autoFocus
            />
          </div>

          <div className="space-y-1.5">
            <label htmlFor="qor-role" className="text-sm font-medium">
              Role <span className="text-muted-foreground font-normal">(optional)</span>
            </label>
            <select
              id="qor-role"
              value={role}
              onChange={(e) => setRole(e.target.value)}
              className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-xs transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            >
              {ORG_ROLES.map((r) => (
                <option key={r.value} value={r.value}>{r.label}</option>
              ))}
            </select>
          </div>

          <div className="space-y-1.5">
            <label htmlFor="qor-model" className="text-sm font-medium">
              Model <span className="text-muted-foreground font-normal">(optional)</span>
            </label>
            <select
              id="qor-model"
              value={model}
              onChange={(e) => setModel(e.target.value)}
              disabled={modelsLoading}
              className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-xs transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:opacity-50"
            >
              <option value="">Default model</option>
              {selectedModels.map((m: SelectedModel) => (
                <option key={m.model_id} value={m.model_id}>{m.model_id}</option>
              ))}
            </select>
          </div>

          <Separator />

          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
            <Button type="submit" disabled={!name.trim() || saving}>
              {saving ? 'Creating…' : 'Create Qor'}
            </Button>
          </div>
        </form>
      </div>
    </>
  );
}
