'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * /skills — Installed tab + Marketplace tab.
 *
 * Installed: shows skills from GET /skills (filesystem + DB combined).
 *   Each row has a Delete button. Uninstall (agent-scoped) is available
 *   from the per-Qor workspace; here we delete the global skill definition.
 *
 * Marketplace: browse + install from GET /marketplace/skills.
 *
 * Crystallized: skills the agent has distilled from conversation — shown
 *   under Installed with a Promote button (private → shared → marketplace).
 */

import { useCallback, useEffect, useState } from 'react';
import { toast } from 'sonner';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { skills as skillsApi, agents } from '@/lib/api';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { ErrorBoundary } from '@/components/error-boundary';
import { TableRowSkeleton } from '@/components/skeletons';
import { cn } from '@/lib/utils';
import {
  Search, Star, Download, Sparkles, ArrowUpRight,
  Trash2, Loader2, Package, Store, CheckCircle2,
  Pin, PinOff, PackageMinus,
} from 'lucide-react';
import type { Skill } from '@/types';

const MARKET_CATEGORIES = ['All', 'Research', 'DevOps', 'Writing', 'Communication', 'Data', 'Custom'];

type InstalledSkill = {
  id?: string;
  name: string;
  slug: string;
  description?: string;
  source?: string;
  path?: string;
  pinned?: boolean;
};

type CrystallizedSkill = {
  id: string;
  name: string;
  slug: string;
  reuse_count: number;
  promote_ready: boolean;
  scope: string;
};

export default function SkillsPage() {
  const [tab, setTab] = useState<'installed' | 'marketplace'>('installed');
  const [chiefId, setChiefId] = useState('');

  // Installed
  const [installed, setInstalled] = useState<InstalledSkill[]>([]);
  const [installedLoading, setInstalledLoading] = useState(true);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [uninstallingSlug, setUninstallingSlug] = useState<string | null>(null);
  const [pinningId, setPinningId] = useState<string | null>(null);

  // Crystallized
  const [crystallized, setCrystallized] = useState<CrystallizedSkill[]>([]);
  const [promotingId, setPromotingId] = useState<string | null>(null);

  // Marketplace
  const [marketplace, setMarketplace] = useState<Skill[]>([]);
  const [marketLoading, setMarketLoading] = useState(false);
  const [marketLoaded, setMarketLoaded] = useState(false);
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('All');
  const [installingSlug, setInstallingSlug] = useState<string | null>(null);
  const [installedSlugs, setInstalledSlugs] = useState<Set<string>>(new Set());

  const loadInstalled = useCallback(async () => {
    setInstalledLoading(true);
    try {
      const data = await skillsApi.list();
      const list = Array.isArray(data) ? data as unknown as InstalledSkill[]
        : (data as any).skills ?? [];
      setInstalled(list);
      setInstalledSlugs(new Set(list.map((s: InstalledSkill) => s.slug)));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to load skills');
    }
    setInstalledLoading(false);
  }, []);

  const loadMarketplace = useCallback(async () => {
    if (marketLoaded) return;
    setMarketLoading(true);
    try {
      const data = await skillsApi.marketplace();
      setMarketplace(Array.isArray(data) ? data : []);
      setMarketLoaded(true);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to load marketplace');
    }
    setMarketLoading(false);
  }, [marketLoaded]);

  useEffect(() => { loadInstalled(); }, [loadInstalled]);

  useEffect(() => {
    agents.chief()
      .then((agent) => {
        if (!agent?.id) return Promise.resolve(null);
        setChiefId(agent.id);
        return skillsApi.crystallized(agent.id);
      })
      .then((data) => {
        if (!data) return;
        const list = Array.isArray(data) ? data : (data as any).skills ?? [];
        setCrystallized(list as CrystallizedSkill[]);
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    if (tab === 'marketplace') loadMarketplace();
  }, [tab, loadMarketplace]);

  const handleDelete = async (skill: InstalledSkill) => {
    const key = skill.id ?? skill.slug;
    setDeletingId(key);
    try {
      if (skill.id) await skillsApi.delete(skill.id);
      setInstalled(prev => prev.filter(s => (s.id ?? s.slug) !== key));
      setInstalledSlugs(prev => { const n = new Set(prev); n.delete(skill.slug); return n; });
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to delete skill');
    }
    setDeletingId(null);
  };

  const handleInstall = async (slug: string) => {
    setInstallingSlug(slug);
    try {
      await skillsApi.install(slug, chiefId);
      setInstalledSlugs(prev => new Set([...prev, slug]));
      toast.success(`${slug} installed`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to install skill');
    }
    setInstallingSlug(null);
  };

  const handleUninstall = async (skill: InstalledSkill) => {
    if (!skill.slug) return;
    setUninstallingSlug(skill.slug);
    try {
      await skillsApi.uninstall(skill.slug, '');
      setInstalled(prev => prev.filter(s => s.slug !== skill.slug));
      setInstalledSlugs(prev => { const n = new Set(prev); n.delete(skill.slug); return n; });
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to uninstall skill');
    }
    setUninstallingSlug(null);
  };

  const handleTogglePin = async (skill: InstalledSkill) => {
    if (!skill.id) return;
    setPinningId(skill.id);
    try {
      const next = !skill.pinned;
      await skillsApi.pin(skill.id, next);
      setInstalled(prev => prev.map(s => s.id === skill.id ? { ...s, pinned: next } : s));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to update pin');
    }
    setPinningId(null);
  };

  const handlePromote = async (cs: CrystallizedSkill) => {
    const nextScope: 'shared' | 'marketplace' = cs.scope === 'private' ? 'shared' : 'marketplace';
    setPromotingId(cs.id);
    try {
      await skillsApi.promote(cs.id, nextScope);
      setCrystallized(prev => prev.map(s => s.id === cs.id ? { ...s, scope: nextScope } : s));
      toast.success(`Promoted to ${nextScope}`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to promote skill');
    }
    setPromotingId(null);
  };

  const filteredMarket = marketplace.filter((s) => {
    if (search && !s.name.toLowerCase().includes(search.toLowerCase())
      && !s.description.toLowerCase().includes(search.toLowerCase())) return false;
    if (category !== 'All' && s.category !== category.toLowerCase()) return false;
    return true;
  });

  return (
    <ErrorBoundary fallbackTitle="Failed to load skills">
      <div className="space-y-5">
        <CanvasHeader title="Skills" description="Extend your Qors with installable capabilities." />

        {/* Tab bar */}
        <div className="flex gap-1 border-b border-border">
          {([
            { id: 'installed',   icon: Package, label: 'Installed' },
            { id: 'marketplace', icon: Store,   label: 'Marketplace' },
          ] as const).map(({ id, icon: Icon, label }) => (
            <button
              key={id}
              onClick={() => setTab(id)}
              className={cn(
                'flex items-center gap-1.5 px-3 py-2 text-sm font-medium border-b-2 -mb-px transition-colors',
                tab === id
                  ? 'border-primary text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground',
              )}
            >
              <Icon className="h-4 w-4" />
              {label}
              {id === 'installed' && !installedLoading && (
                <span className="rounded-full bg-muted px-1.5 py-0.5 text-xs font-mono">
                  {installed.length}
                </span>
              )}
            </button>
          ))}
        </div>

        {/* ── Installed tab ── */}
        {tab === 'installed' && (
          <div className="space-y-4">
            {installedLoading ? (
              <div className="space-y-1">{Array.from({ length: 4 }).map((_, i) => <TableRowSkeleton key={i} cols={3} />)}</div>
            ) : installed.length === 0 ? (
              <EmptyState {...emptyStates.skills} actionLabel="Browse Marketplace" onAction={() => setTab('marketplace')} />
            ) : (
              <div className="rounded-xl border border-border overflow-hidden">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-border bg-muted/30 text-2xs uppercase tracking-wider text-muted-foreground">
                      <th className="px-4 py-2.5 text-left font-medium">Skill</th>
                      <th className="px-4 py-2.5 text-left font-medium">Source</th>
                      <th className="px-4 py-2.5 text-left font-medium hidden sm:table-cell">Slug</th>
                      <th className="px-4 py-2.5 text-right font-medium">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {installed.map((skill) => {
                      const key = skill.id ?? skill.slug;
                      return (
                        <tr key={key} className="border-b border-border/60 last:border-0 hover:bg-accent/30 transition-colors">
                          <td className="px-4 py-3">
                            <p className="font-medium">{skill.name}</p>
                            {skill.description && (
                              <p className="mt-0.5 text-2xs text-muted-foreground line-clamp-1">{skill.description}</p>
                            )}
                          </td>
                          <td className="px-4 py-3">
                            <span className="rounded-sm bg-muted px-1.5 py-0.5 font-mono text-2xs text-muted-foreground uppercase">
                              {skill.source ?? 'filesystem'}
                            </span>
                          </td>
                          <td className="px-4 py-3 font-mono text-2xs text-muted-foreground hidden sm:table-cell">{skill.slug}</td>
                          <td className="px-4 py-3 text-right">
                            <div className="inline-flex items-center gap-2">
                              {/* Pin toggle — only for DB-stored skills */}
                              {skill.id && (
                                <button
                                  onClick={() => handleTogglePin(skill)}
                                  disabled={pinningId === skill.id}
                                  title={skill.pinned ? 'Unpin skill' : 'Pin skill (prevent modification)'}
                                  className={cn(
                                    'inline-flex items-center gap-1 rounded-md border px-2 py-1 text-xs transition-colors disabled:opacity-50',
                                    skill.pinned
                                      ? 'border-amber-500/40 bg-amber-500/10 text-amber-600 hover:bg-amber-500/20'
                                      : 'border-border text-muted-foreground hover:bg-accent',
                                  )}
                                >
                                  {pinningId === skill.id
                                    ? <Loader2 className="h-3 w-3 animate-spin" />
                                    : skill.pinned ? <Pin className="h-3 w-3" /> : <PinOff className="h-3 w-3" />}
                                  {skill.pinned ? 'Pinned' : 'Pin'}
                                </button>
                              )}

                              {/* Uninstall for marketplace, Delete for DB-custom, Built-in for filesystem */}
                              {skill.source === 'marketplace' ? (
                                <button
                                  onClick={() => handleUninstall(skill)}
                                  disabled={uninstallingSlug === skill.slug || !!skill.pinned}
                                  title={skill.pinned ? 'Unpin before uninstalling' : 'Uninstall skill'}
                                  className="inline-flex items-center gap-1.5 rounded-md border border-destructive/40 bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive hover:bg-destructive/20 disabled:opacity-50"
                                >
                                  {uninstallingSlug === skill.slug
                                    ? <Loader2 className="h-3 w-3 animate-spin" />
                                    : <PackageMinus className="h-3 w-3" />}
                                  Uninstall
                                </button>
                              ) : skill.id ? (
                                <button
                                  onClick={() => handleDelete(skill)}
                                  disabled={deletingId === key || !!skill.pinned}
                                  title={skill.pinned ? 'Unpin before deleting' : 'Delete skill'}
                                  className="inline-flex items-center gap-1.5 rounded-md border border-destructive/40 bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive hover:bg-destructive/20 disabled:opacity-50"
                                >
                                  {deletingId === key
                                    ? <Loader2 className="h-3 w-3 animate-spin" />
                                    : <Trash2 className="h-3 w-3" />}
                                  Delete
                                </button>
                              ) : (
                                <span className="text-2xs text-muted-foreground">Built-in</span>
                              )}
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}

            {/* Crystallized skills */}
            {crystallized.length > 0 && (
              <div>
                <h2 className="mb-3 flex items-center gap-2 text-sm font-semibold">
                  <Sparkles className="h-4 w-4 text-primary" />
                  Crystallized Skills
                  <span className="text-2xs font-normal text-muted-foreground">— distilled from conversations</span>
                </h2>
                <div className="space-y-2">
                  {crystallized.map((cs) => (
                    <div key={cs.id} className="flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3">
                      <Sparkles className="h-4 w-4 shrink-0 text-primary" />
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-medium truncate">{cs.name}</p>
                        <p className="text-2xs text-muted-foreground">
                          Reused {cs.reuse_count}× · Scope: <span className="font-mono">{cs.scope}</span>
                        </p>
                      </div>
                      {cs.promote_ready && cs.scope !== 'marketplace' ? (
                        <button
                          onClick={() => handlePromote(cs)}
                          disabled={promotingId === cs.id}
                          className="inline-flex items-center gap-1.5 rounded-lg border border-primary px-3 py-1 text-xs font-medium text-primary hover:bg-primary/10 disabled:opacity-50"
                        >
                          {promotingId === cs.id
                            ? <Loader2 className="h-3 w-3 animate-spin" />
                            : <ArrowUpRight className="h-3 w-3" />}
                          Promote to {cs.scope === 'private' ? 'shared' : 'marketplace'}
                        </button>
                      ) : cs.scope === 'marketplace' ? (
                        <span className="flex items-center gap-1 text-2xs text-emerald-500">
                          <CheckCircle2 className="h-3.5 w-3.5" /> Published
                        </span>
                      ) : (
                        <span className="text-2xs text-muted-foreground">
                          {3 - cs.reuse_count} more uses to promote
                        </span>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}

        {/* ── Marketplace tab ── */}
        {tab === 'marketplace' && (
          <div className="space-y-4">
            {/* Search + filters */}
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
              <div className="flex flex-1 items-center gap-2 rounded-lg border border-border bg-input px-3 py-2">
                <Search className="h-4 w-4 text-muted-foreground" />
                <input
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Search skills..."
                  className="flex-1 bg-transparent text-sm outline-none"
                />
              </div>
              <div className="flex gap-1 overflow-x-auto">
                {MARKET_CATEGORIES.map((cat) => (
                  <button
                    key={cat}
                    onClick={() => setCategory(cat)}
                    className={cn(
                      'whitespace-nowrap rounded-full px-3 py-1 text-2sm font-medium transition-colors',
                      category === cat ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:text-foreground',
                    )}
                  >
                    {cat}
                  </button>
                ))}
              </div>
            </div>

            {marketLoading ? (
              <div className="space-y-1">{Array.from({ length: 6 }).map((_, i) => <TableRowSkeleton key={i} cols={4} />)}</div>
            ) : filteredMarket.length === 0 ? (
              <EmptyState {...emptyStates.skills} />
            ) : (
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {filteredMarket.map((skill) => {
                  const alreadyInstalled = installedSlugs.has(skill.slug);
                  return (
                    <div key={skill.id} className="rounded-xl border border-border bg-card p-4 space-y-2">
                      <div className="flex items-start justify-between">
                        <div>
                          <p className="text-sm font-medium">{skill.name}</p>
                          <p className="text-2xs text-muted-foreground">{skill.author}</p>
                        </div>
                        <span className="rounded-full bg-muted px-2 py-0.5 text-2xs">{skill.category}</span>
                      </div>
                      <p className="text-2sm text-muted-foreground line-clamp-2">{skill.description}</p>
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-3 text-2xs text-muted-foreground">
                          <span className="inline-flex items-center gap-1"><Star className="h-3 w-3" />{skill.rating?.toFixed(1) ?? '—'}</span>
                          <span className="inline-flex items-center gap-1"><Download className="h-3 w-3" />{skill.install_count ?? 0}</span>
                        </div>
                        {alreadyInstalled ? (
                          <span className="inline-flex items-center gap-1 text-2xs text-emerald-500">
                            <CheckCircle2 className="h-3.5 w-3.5" /> Installed
                          </span>
                        ) : (
                          <button
                            onClick={() => handleInstall(skill.slug)}
                            disabled={installingSlug === skill.slug}
                            className="inline-flex items-center gap-1 rounded-lg bg-primary px-3 py-1 text-2xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                          >
                            {installingSlug === skill.slug ? 'Installing…' : 'Install'}
                          </button>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </div>
    </ErrorBoundary>
  );
}
