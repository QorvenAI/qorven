'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALc2.

import { brand, buttons } from '@/lib/branding';
import { useEffect, useState, useMemo, useCallback } from 'react';
import { toast } from 'sonner';
import { useRouter } from 'next/navigation';
import { useStore } from '@/store';
import { agents, rooms as roomsApi } from '@/lib/api';
import { cn } from '@/lib/utils';
import { SoulCardSkeleton } from '@/components/skeletons';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { ErrorBoundary } from '@/components/error-boundary';
import { soulGradient } from '@/components/soul-card';
import { SoulPulseRing } from '@/components/soul-pulse-ring';
import { useSoulRun } from '@/hooks/use-soul';
import { useSelectedModels } from '@/hooks/use-selected-models';
import { RoomDetail } from '@/app/(app)/rooms/[id]/client';
import {
  Plus, X, Search, MessageSquare, Settings, Trash2, MoreHorizontal,
  Users, Hash, ChevronDown,
} from 'lucide-react';
import type { Soul } from '@/types';

interface Room {
  id: string;
  name: string;
  display_name?: string;
  member_count?: number;
}

type RightPanel =
  | { type: 'qors' }
  | { type: 'room'; roomId: string };

export default function QorsPage() {
  const [panel, setPanel] = useState<RightPanel>({ type: 'qors' });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [search, setSearch] = useState('');
  const [roleFilter, setRoleFilter] = useState<string>('all');
  const souls = useStore((s) => s.souls);
  const setSouls = useStore((s) => s.setSouls);

  // Hubs (rooms) sidebar state
  const [rooms, setRooms] = useState<Room[]>([]);
  const [roomsLoading, setRoomsLoading] = useState(true);
  const [hubsOpen, setHubsOpen] = useState(true);
  const [newHubName, setNewHubName] = useState('');
  const [showNewHub, setShowNewHub] = useState(false);
  const [creatingHub, setCreatingHub] = useState(false);

  const load = useCallback(() => {
    setLoading(true);
    agents.list()
      .then((data) => { setSouls(data); setLoading(false); })
      .catch((e) => { setError(e.message); setLoading(false); });
  }, [setSouls]);

  const loadRooms = useCallback(() => {
    setRoomsLoading(true);
    roomsApi.list()
      .then((d: any) => {
        setRooms(Array.isArray(d?.rooms) ? d.rooms : Array.isArray(d) ? d : []);
        setRoomsLoading(false);
      })
      .catch(() => setRoomsLoading(false));
  }, []);

  useEffect(() => { load(); loadRooms(); }, [load, loadRooms]);

  const roles = useMemo(() => {
    const r = new Set(souls.map((s) => s.role).filter(Boolean));
    return Array.from(r);
  }, [souls]);

  const filtered = useMemo(() => {
    let list = souls;
    if (search) {
      const q = search.toLowerCase();
      list = list.filter((s) =>
        s.display_name.toLowerCase().includes(q) ||
        s.role?.toLowerCase().includes(q) ||
        s.model?.toLowerCase().includes(q),
      );
    }
    if (roleFilter !== 'all') list = list.filter((s) => s.role === roleFilter);
    return list;
  }, [souls, search, roleFilter]);

  const createHub = async () => {
    if (!newHubName.trim()) return;
    setCreatingHub(true);
    try {
      const created: any = await roomsApi.create({
        name: newHubName.trim().toLowerCase().replace(/\s+/g, '-'),
        display_name: newHubName.trim(),
      });
      setNewHubName('');
      setShowNewHub(false);
      loadRooms();
      if (created?.id) setPanel({ type: 'room', roomId: created.id });
    } catch (e: any) {
      toast.error(e.message ?? 'Failed to create hub');
    } finally {
      setCreatingHub(false);
    }
  };

  return (
    <ErrorBoundary fallbackTitle="Failed to load Qors">
      {/* Full-height two-panel layout */}
      <div
        className="flex overflow-hidden -m-5 lg:-m-6"
        style={{ height: 'calc(100vh - var(--header-height, 44px) - var(--status-bar-height, 0px))' }}
      >
        {/* ── Left sidebar ── */}
        <div className="flex w-(--sidebar-default-width) shrink-0 flex-col border-r border-border bg-card/50 overflow-y-auto">

          {/* Qors section header */}
          <div className="flex items-center justify-between px-3 pt-3 pb-1 shrink-0">
            <button
              onClick={() => setPanel({ type: 'qors' })}
              className={cn(
                'text-xs font-semibold uppercase tracking-wider transition-colors',
                panel.type === 'qors' ? 'text-primary' : 'text-muted-foreground hover:text-foreground',
              )}
            >
              {brand.agentNamePlural}
            </button>
            <button
              onClick={() => setShowCreate(true)}
              title="New Qor"
              className="flex h-5 w-5 items-center justify-center rounded text-muted-foreground hover:text-foreground hover:bg-accent"
            >
              <Plus className="h-3 w-3" />
            </button>
          </div>

          {/* Qors list */}
          <div className="flex flex-col gap-0.5 px-1 pb-2">
            {loading ? (
              <div className="px-3 py-2 text-xs text-muted-foreground">Loading…</div>
            ) : souls.map((soul) => (
              <SidebarSoulRow
                key={soul.id}
                soul={soul}
                active={panel.type === 'qors'}
              />
            ))}
          </div>

          {/* Hubs section header */}
          <div className="flex items-center justify-between px-3 pt-2 pb-1 shrink-0 border-t border-border mt-1">
            <button
              onClick={() => setHubsOpen(v => !v)}
              className="flex items-center gap-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground hover:text-foreground transition-colors"
            >
              <ChevronDown className={cn('h-3 w-3 transition-transform', !hubsOpen && '-rotate-90')} />
              Hubs
            </button>
            <button
              onClick={() => setShowNewHub(v => !v)}
              title="New Hub"
              className="flex h-5 w-5 items-center justify-center rounded text-muted-foreground hover:text-foreground hover:bg-accent"
            >
              <Plus className="h-3 w-3" />
            </button>
          </div>

          {hubsOpen && (
            <div className="flex flex-col gap-0.5 px-1 pb-2">
              {/* New hub inline form */}
              {showNewHub && (
                <div className="mx-1 mb-1 rounded-md border border-border bg-card p-2 space-y-1.5">
                  <input
                    value={newHubName}
                    onChange={(e) => setNewHubName(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && createHub()}
                    placeholder="Hub name"
                    autoFocus
                    className="w-full rounded border border-input bg-transparent px-2 py-1 text-xs placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/40"
                  />
                  <div className="flex gap-1">
                    <button
                      onClick={createHub}
                      disabled={creatingHub || !newHubName.trim()}
                      className="flex-1 rounded bg-primary px-2 py-0.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                    >
                      {creatingHub ? '…' : 'Create'}
                    </button>
                    <button
                      onClick={() => { setShowNewHub(false); setNewHubName(''); }}
                      className="rounded px-1.5 py-0.5 text-xs text-muted-foreground hover:bg-accent"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </div>
                </div>
              )}

              {roomsLoading ? (
                <div className="px-3 py-2 text-xs text-muted-foreground">Loading…</div>
              ) : rooms.length === 0 ? (
                <p className="px-3 py-2 text-xs text-muted-foreground">No hubs yet</p>
              ) : (
                rooms.map((room) => (
                  <button
                    key={room.id}
                    onClick={() => setPanel({ type: 'room', roomId: room.id })}
                    className={cn(
                      'flex w-full items-center gap-1.5 rounded-md px-2.5 py-1.5 text-left text-xs transition-colors',
                      panel.type === 'room' && panel.roomId === room.id
                        ? 'bg-primary/10 text-primary font-medium'
                        : 'text-muted-foreground hover:bg-accent hover:text-foreground',
                    )}
                  >
                    <Hash className="h-3 w-3 shrink-0 opacity-60" />
                    <span className="flex-1 truncate">{room.display_name || room.name}</span>
                    {(room.member_count ?? 0) > 0 && (
                      <span className="flex items-center gap-0.5 text-2xs shrink-0 opacity-50">
                        <Users className="h-2.5 w-2.5" />
                        {room.member_count}
                      </span>
                    )}
                  </button>
                ))
              )}
            </div>
          )}
        </div>

        {/* ── Right panel ── */}
        <div className="flex-1 min-w-0 overflow-y-auto">
          {panel.type === 'room' ? (
            <RoomDetail roomId={panel.roomId} showBack={false} />
          ) : (
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

              {/* Search + role filter */}
              {souls.length > 0 && (
                <div className="flex items-center gap-2">
                  <div className="relative flex-1 max-w-sm">
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

// ─── Sidebar soul row (compact) ───────────────────────────────────────────────
function SidebarSoulRow({ soul, active }: { soul: Soul; active: boolean }) {
  const router = useRouter();
  const { activity } = useSoulRun(soul.id);
  const dotColor = {
    idle: 'bg-emerald-400',
    thinking: 'bg-amber-400',
    running: 'bg-blue-400',
    offline: 'bg-muted-foreground/40',
    error: 'bg-destructive',
  }[activity] ?? 'bg-muted-foreground/40';

  return (
    <button
      onClick={() => router.push(`/qors/${soul.id}`)}
      className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs text-muted-foreground hover:bg-accent hover:text-foreground transition-colors"
    >
      <div className={cn('relative flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-2xs font-semibold text-white', soulGradient(soul.display_name))}>
        {soul.display_name.charAt(0).toUpperCase()}
        <span className={cn('absolute -bottom-0.5 -right-0.5 h-2 w-2 rounded-full border border-card', dotColor)} />
      </div>
      <span className="flex-1 truncate font-medium">{soul.display_name}</span>
    </button>
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

  return (
    <div
      className={cn(
        'group relative rounded-xl border border-border bg-card p-4 transition-all hover:border-primary/30',
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
          <p className="truncate text-sm font-semibold">{soul.display_name}</p>
          <p className="truncate text-xs text-muted-foreground">{soul.title || soul.role || 'Assistant'}</p>
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

// ─── Create Dialog ─────────────────────────────────────────────────────────────
function CreateQorDialog({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const { models } = useSelectedModels();
  const [form, setForm] = useState({
    display_name: '',
    model: '',
    role: 'worker',
    system_prompt: '',
    temperature: 0.5,
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const handleCreate = async () => {
    if (!form.display_name) return;
    setSaving(true);
    setError('');
    try {
      await agents.create({
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
      } as any);
      toast.success(`${form.display_name} created`);
      onCreated();
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Failed to create';
      setError(msg);
      toast.error(msg);
      setSaving(false);
    }
  };

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
