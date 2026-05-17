'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

/**
 * Qor profile operational tabs (P9 T3.1 + T3.2).
 *
 * Six tabs mounted by app/(app)/qors/[id]/page.tsx:
 *
 *   Skills    — what the agent can do + installed skill crystals
 *   Memory    — per-agent vector memory, search-driven
 *   Heartbeat — probe config + on/off + interval
 *   QOROS     — proactive mode start/stop/status
 *   Dreaming  — consolidation schedule + manual trigger
 *   Metrics   — 7-day rolling performance metrics + training export
 *
 * Kept in one file so they share the card/row/section primitives and
 * consistent styling. Each tab is a self-contained functional
 * component that owns its own fetch state.
 */

import { useCallback, useEffect, useState } from 'react';
import Link from 'next/link';
import { toast } from 'sonner';
import {
  Sparkles, Brain, Activity, Moon, BarChart3,
  Search, Loader2, Plus, Minus,
  Play, Pause, RefreshCw, Download, Zap, HeartPulse, Wrench,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { SearchableSelect } from '@/components/searchable-select';
import {
  skills as skillsApi,
  memoryApi, type MemoryRecord,
  heartbeats, type HeartbeatConfig,
  qoros, type QorosStatus,
  agents, type DreamingConfig,
  metricsApi, type MetricsResponse,
  training,
} from '@/lib/api';

// ─── Shared primitives ─────────────────────────────────────────────

function Section({
  icon: Icon,
  title,
  subtitle,
  children,
  actions,
}: {
  icon: typeof Sparkles;
  title: string;
  subtitle?: string;
  actions?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section className="mx-auto max-w-3xl space-y-4">
      <header className="flex items-start gap-3">
        <Icon className="mt-0.5 h-5 w-5 text-primary" />
        <div>
          <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
          {subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}
        </div>
        {actions && <div className="ml-auto flex items-center gap-2">{actions}</div>}
      </header>
      {children}
    </section>
  );
}

function Card({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cn('rounded-xl border border-border bg-card/60', className)}>{children}</div>;
}

function Empty({ icon: Icon, text }: { icon: typeof Sparkles; text: string }) {
  return (
    <Card className="flex flex-col items-center gap-2 px-6 py-10 text-center">
      <Icon className="h-6 w-6 text-muted-foreground/60" />
      <p className="text-sm text-muted-foreground">{text}</p>
    </Card>
  );
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 py-1.5 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <span className="text-right font-mono text-foreground/90">{value}</span>
    </div>
  );
}

// ─── Skills tab ────────────────────────────────────────────────────

export function ProfileSkillsTab({ agentId }: { agentId: string }) {
  const [installed, setInstalled] = useState<any[]>([]);
  const [crystals, setCrystals] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    setLoading(true);
    const safe = async <T,>(p: Promise<T>, fallback: T) => {
      try { return await p; } catch { return fallback; }
    };
    const [byAgent, cryst] = await Promise.all([
      safe(skillsApi.list(), []),
      safe(skillsApi.crystallized(agentId), []),
    ]);
    setInstalled(Array.isArray(byAgent) ? byAgent : []);
    setCrystals(Array.isArray(cryst) ? cryst : []);
    setLoading(false);
  }, [agentId]);

  useEffect(() => { refresh(); }, [refresh]);

  return (
    <Section
      icon={Sparkles}
      title="Skills"
      subtitle="Installed apps & auto-learned patterns"
      actions={
        <button
          onClick={refresh}
          disabled={loading}
          className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-2xs text-muted-foreground hover:bg-accent"
        >
          {loading ? <Loader2 className="h-3 w-3 animate-spin" /> : <RefreshCw className="h-3 w-3" />}
          Refresh
        </button>
      }
    >
      {/* Installed skills */}
      <Card>
        <header className="flex items-center gap-2 border-b border-border/60 px-4 py-2.5">
          <Sparkles className="h-3.5 w-3.5 text-primary" />
          <span className="text-xs font-semibold">Installed Skills</span>
          <span className="ml-1 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">{installed.length}</span>
          <Link href="/apps" className="ml-auto text-2xs text-primary hover:underline">
            Browse marketplace →
          </Link>
        </header>
        {loading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          </div>
        ) : installed.length === 0 ? (
          <div className="px-4 py-8 text-center">
            <Sparkles className="mx-auto mb-2 h-5 w-5 text-muted-foreground/50" />
            <p className="text-xs font-medium text-foreground/70">No skills installed yet</p>
            <p className="mt-1 text-[11px] text-muted-foreground">
              Skills extend what this agent can do — connect services, run automations, fetch data.
            </p>
            <Link
              href="/apps"
              className="mt-3 inline-flex items-center gap-1 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
            >
              <Sparkles className="h-3 w-3" /> Browse Skills
            </Link>
          </div>
        ) : (
          <ul className="divide-y divide-border/60">
            {installed.map((s: any) => (
              <li key={s.id ?? s.slug} className="flex items-start gap-3 px-4 py-3">
                <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-primary/10">
                  <Sparkles className="h-3.5 w-3.5 text-primary" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-xs font-medium">{s.name ?? s.slug}</p>
                  {s.description && (
                    <p className="mt-0.5 text-[11px] text-muted-foreground line-clamp-2">{s.description}</p>
                  )}
                </div>
                {s.enabled === false && (
                  <span className="shrink-0 rounded-sm bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">disabled</span>
                )}
              </li>
            ))}
          </ul>
        )}
      </Card>

      {/* Crystallized learnings */}
      <Card>
        <header className="flex items-center gap-2 border-b border-border/60 px-4 py-2.5">
          <Brain className="h-3.5 w-3.5 text-amber-400" />
          <span className="text-xs font-semibold">Learned Patterns</span>
          <span className="ml-1 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">{crystals.length}</span>
        </header>
        {crystals.length === 0 ? (
          <div className="px-4 py-6 text-center">
            <Brain className="mx-auto mb-2 h-5 w-5 text-muted-foreground/50" />
            <p className="text-xs font-medium text-foreground/70">No patterns learned yet</p>
            <p className="mt-1 text-[11px] text-muted-foreground">
              As this agent completes tasks, it auto-crystallizes repeatable patterns into reusable skills.
              These appear here once enough repetitions are observed.
            </p>
          </div>
        ) : (
          <ul className="divide-y divide-border/60">
            {crystals.map((c: any, i: number) => (
              <li key={c.id ?? i} className="flex items-start gap-3 px-4 py-3">
                <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-amber-400/10">
                  <Brain className="h-3.5 w-3.5 text-amber-400" />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <p className="text-xs font-medium">{c.name ?? c.title ?? 'Learned Pattern'}</p>
                    {c.confidence != null && (
                      <span className="rounded-sm bg-muted px-1.5 py-0.5 text-[10px] font-mono text-muted-foreground">
                        {Math.round((c.confidence as number) * 100)}% confidence
                      </span>
                    )}
                  </div>
                  {c.description && (
                    <p className="mt-0.5 text-[11px] text-muted-foreground">{c.description}</p>
                  )}
                  {c.use_count != null && (
                    <p className="mt-0.5 text-[10px] text-muted-foreground/70">Used {c.use_count} times</p>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </Section>
  );
}

// ─── Memory tab ────────────────────────────────────────────────────

export function ProfileMemoryTab({ agentId, agentName }: { agentId: string; agentName: string }) {
  const [query, setQuery] = useState('');
  const [list, setList] = useState<MemoryRecord[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async (q: string = '') => {
    setLoading(true);
    const res = q.trim()
      ? await memoryApi.search(agentId, q.trim())
      : await memoryApi.list(agentId);
    setList(res ?? []);
    setLoading(false);
  }, [agentId]);

  useEffect(() => { load(); }, [load]);

  return (
    <Section
      icon={Brain}
      title="Memory"
      subtitle={`Per-agent vector memory for ${agentName}`}
    >
      <form
        onSubmit={(e) => { e.preventDefault(); load(query); }}
        className="flex items-center gap-2"
      >
        <div className="relative flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Semantic search — empty for recent"
            className="qr-input pl-8 text-xs" />
        </div>
        <button
          type="submit"
          disabled={loading}
          className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
        >
          {loading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Search className="h-3.5 w-3.5" />}
          Search
        </button>
      </form>

      {loading ? (
        <div className="flex justify-center py-8"><Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /></div>
      ) : list.length === 0 ? (
        <div className="rounded-xl border border-border bg-card/60 px-6 py-10 text-center">
          <Brain className="mx-auto mb-2 h-6 w-6 text-muted-foreground/50" />
          <p className="text-sm font-medium text-foreground/70">No memories yet</p>
          <p className="mt-1 text-xs text-muted-foreground">
            Memories build automatically as this agent runs tasks —<br />
            things it learns about you, preferences it notices, facts it wants to remember.
          </p>
        </div>
      ) : (
        <ul className="space-y-2">
          {list.map((m, idx) => {
            const raw = m as any;
            const memId: string = raw.memory?.id ?? m.id ?? String(idx);
            const memType: string = raw.memory?.memory_type ?? raw.memory_type ?? m.scope ?? '';
            const content: string = raw.memory?.content ?? m.content ?? '';
            const createdAt: string = raw.memory?.created_at ?? m.created_at ?? '';
            const importance: number = raw.memory?.importance ?? 0;
            const typeColors: Record<string, string> = {
              identity:    'bg-violet-500/10 text-violet-400',
              preference:  'bg-sky-500/10 text-sky-400',
              fact:        'bg-emerald-500/10 text-emerald-400',
              decision:    'bg-amber-500/10 text-amber-400',
              goal:        'bg-pink-500/10 text-pink-400',
              observation: 'bg-orange-500/10 text-orange-400',
              todo:        'bg-slate-500/10 text-slate-400',
            };
            const typeClass = typeColors[memType] ?? 'bg-muted text-muted-foreground';
            return (
              <li key={memId} className="rounded-lg border border-border bg-card p-3 text-xs">
                <div className="flex items-center gap-2 text-[11px]">
                  {memType && (
                    <span className={`rounded-sm px-1.5 py-0.5 font-mono capitalize ${typeClass}`}>
                      {memType}
                    </span>
                  )}
                  {m.relevance != null && (
                    <span className="text-muted-foreground">
                      {Math.round((m.relevance as number) * 100)}% match
                    </span>
                  )}
                  {importance > 0 && (
                    <span className="text-muted-foreground/60">
                      importance {importance.toFixed(1)}
                    </span>
                  )}
                  <span className="ml-auto text-muted-foreground">
                    {createdAt ? new Date(createdAt).toLocaleDateString() : ''}
                  </span>
                </div>
                <p className="mt-1.5 whitespace-pre-wrap leading-relaxed">{content || m.content}</p>
              </li>
            );
          })}
        </ul>
      )}
    </Section>
  );
}

// ─── Background tab (Self Check + Self Fix + Memory Refining) ────────

export function ProfileBackgroundTab({ agentId }: { agentId: string }) {
  const [hbCfg, setHbCfg] = useState<HeartbeatConfig | null>(null);
  const [hbLoading, setHbLoading] = useState(true);
  const [hbSaving, setHbSaving] = useState(false);
  const [status, setStatus] = useState<QorosStatus | null>(null);
  const [qBusy, setQBusy] = useState(false);
  const [dream, setDream] = useState<DreamingConfig | null>(null);
  const [dreamSaving, setDreamSaving] = useState(false);
  const [triggering, setTriggering] = useState(false);

  useEffect(() => {
    heartbeats.get(agentId)
      .then(setHbCfg)
      .catch(() => setHbCfg({ enabled: true, interval_hours: 6, mode: 'light', probes: [] }))
      .finally(() => setHbLoading(false));
    qoros.status(agentId)
      .then(setStatus)
      .catch(() => setStatus({ active: false, agent_id: agentId }));
    agents.getDreaming(agentId)
      .then(setDream)
      .catch(() => setDream({ enabled: true, interval_hours: 168, mode: 'consolidate' }));
  }, [agentId]);

  const saveHb = async (patch: Partial<HeartbeatConfig>) => {
    if (!hbCfg) return;
    const next = { ...hbCfg, ...patch };
    setHbCfg(next);
    setHbSaving(true);
    try { await heartbeats.save(agentId, next); }
    catch (e) { toast.error(e instanceof Error ? e.message : 'Save failed'); }
    finally { setHbSaving(false); }
  };

  const saveDream = async (patch: Partial<DreamingConfig>) => {
    if (!dream) return;
    const next = { ...dream, ...patch };
    setDream(next);
    setDreamSaving(true);
    try { await agents.updateDreaming(agentId, next); }
    catch (e) { toast.error(e instanceof Error ? e.message : 'Save failed'); }
    finally { setDreamSaving(false); }
  };

  const toggleQoros = async () => {
    if (!status) return;
    setQBusy(true);
    try {
      if (status.active) await qoros.stop(agentId);
      else await qoros.start(agentId);
      const s = await qoros.status(agentId);
      setStatus(s);
      toast.success(status.active ? 'Self-fix stopped' : 'Self-fix started');
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed'); }
    finally { setQBusy(false); }
  };

  const triggerDream = async () => {
    setTriggering(true);
    try {
      await agents.triggerDream(agentId);
      toast.success('Memory refining triggered — runs asynchronously');
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed'); }
    finally { setTriggering(false); }
  };

  const probeDepthOptions = [
    { value: 'light', label: 'Light — quick status check, minimal tokens' },
    { value: 'standard', label: 'Standard — balanced depth' },
    { value: 'deep', label: 'Deep — full memory + task audit' },
  ];
  const dreamModeOptions = [
    { value: 'consolidate', label: 'Consolidate — merge + deduplicate memories' },
    { value: 'summarize', label: 'Summarize — compress session history' },
    { value: 'deep', label: 'Deep — full consolidation + reflection' },
  ];
  const dreamIntervalOptions = [
    { value: '6', label: 'Every 6 hours' },
    { value: '24', label: 'Daily' },
    { value: '72', label: 'Every 3 days' },
    { value: '168', label: 'Weekly' },
    { value: '720', label: 'Monthly' },
  ];

  return (
    <div className="mx-auto max-w-3xl space-y-8 p-1">

      {/* ── Section 1: Self Check (Heartbeat) ── */}
      <section className="space-y-4">
        <header className="flex items-center gap-2.5">
          <HeartPulse className="h-5 w-5 text-rose-400" />
          <div>
            <h2 className="text-base font-semibold">Self Check</h2>
            <p className="text-xs text-muted-foreground">Periodic health probes — verify memory, tasks and tools are operational</p>
          </div>
          {hbSaving && <Loader2 className="ml-auto h-3.5 w-3.5 animate-spin text-muted-foreground" />}
        </header>
        {hbLoading ? (
          <div className="py-4"><Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /></div>
        ) : hbCfg ? (
          <Card>
            <div className="divide-y divide-border/60">
              <div className="flex items-center justify-between gap-4 px-4 py-3 text-xs">
                <span className="text-muted-foreground">Check interval</span>
                <div className="flex items-center gap-1">
                  <button onClick={() => saveHb({ interval_hours: Math.max(1, (hbCfg.interval_hours ?? 6) - 1) })}
                    className="flex h-6 w-6 items-center justify-center rounded-sm border border-border hover:bg-accent">
                    <Minus className="h-3 w-3" />
                  </button>
                  <span className="min-w-[52px] text-center font-mono">{hbCfg.interval_hours ?? 6}h</span>
                  <button onClick={() => saveHb({ interval_hours: (hbCfg.interval_hours ?? 6) + 1 })}
                    className="flex h-6 w-6 items-center justify-center rounded-sm border border-border hover:bg-accent">
                    <Plus className="h-3 w-3" />
                  </button>
                </div>
              </div>
              <div className="flex items-center justify-between gap-4 px-4 py-3 text-xs">
                <span className="text-muted-foreground">Probe depth</span>
                <div className="w-56">
                  <SearchableSelect
                    value={hbCfg.mode ?? 'light'}
                    onChange={(v) => saveHb({ mode: v })}
                    options={probeDepthOptions}
                  />
                </div>
              </div>
              {hbCfg.probes && hbCfg.probes.length > 0 && (
                <div className="px-4 py-3">
                  <p className="mb-2 text-2xs font-medium uppercase tracking-wider text-muted-foreground">Active probes</p>
                  <div className="flex flex-wrap gap-1.5">
                    {hbCfg.probes.map((p) => (
                      <span key={p} className="rounded-sm bg-muted px-1.5 py-0.5 font-mono text-2xs text-muted-foreground">{p}</span>
                    ))}
                  </div>
                </div>
              )}
              {typeof hbCfg.last_run_at === 'string' && (
                <div className="flex items-center justify-between px-4 py-2 text-2xs text-muted-foreground">
                  <span>Last run</span>
                  <span className="font-mono">{new Date(hbCfg.last_run_at).toLocaleString()}</span>
                </div>
              )}
            </div>
          </Card>
        ) : null}
      </section>

      {/* ── Section 2: Self Fix (QOROS) ── */}
      <section className="space-y-4">
        <header className="flex items-center gap-2.5">
          <Wrench className="h-5 w-5 text-amber-400" />
          <div>
            <h2 className="text-base font-semibold">Self Fix</h2>
            <p className="text-xs text-muted-foreground">Proactive background loop — picks up cron tasks, auto-resolves issues, runs probes</p>
          </div>
        </header>
        <Card className="p-4">
          <div className="flex items-center gap-3">
            <span className={cn(
              'flex h-10 w-10 shrink-0 items-center justify-center rounded-full',
              !status ? 'bg-muted' :
              status.active ? 'bg-emerald-500/15 text-emerald-500' : 'bg-muted-foreground/15 text-muted-foreground',
            )}>
              {!status ? <Loader2 className="h-4 w-4 animate-spin" /> : <Zap className="h-5 w-5" />}
            </span>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium">
                {!status ? 'Checking…' : status.active ? 'Running' : 'Stopped'}
              </p>
              {typeof status?.last_tick_at === 'string' && (
                <p className="text-2xs text-muted-foreground">
                  Last tick: {new Date(status.last_tick_at).toLocaleString()}
                </p>
              )}
            </div>
            <button
              onClick={toggleQoros}
              disabled={qBusy || !status}
              className={cn(
                'inline-flex shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50',
                status?.active
                  ? 'border border-destructive/40 bg-destructive/10 text-destructive hover:bg-destructive/20'
                  : 'bg-primary text-primary-foreground hover:bg-primary/90',
              )}
            >
              {qBusy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> :
               status?.active ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
              {status?.active ? 'Stop' : 'Start'}
            </button>
          </div>
        </Card>
      </section>

      {/* ── Section 3: Memory Refining (Dreaming) ── */}
      <section className="space-y-4">
        <header className="flex items-center gap-2.5">
          <Moon className="h-5 w-5 text-violet-400" />
          <div className="flex-1">
            <h2 className="text-base font-semibold">Memory Refining</h2>
            <p className="text-xs text-muted-foreground">Reviews session history, extracts durable memories, merges duplicates — always scheduled</p>
          </div>
          <button
            onClick={triggerDream}
            disabled={triggering}
            className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs font-medium hover:bg-accent disabled:opacity-50"
          >
            {triggering ? <Loader2 className="h-3 w-3 animate-spin" /> : <RefreshCw className="h-3 w-3" />}
            Run now
          </button>
        </header>
        {dream && (
          <Card>
            <div className="divide-y divide-border/60">
              <div className="flex items-center justify-between gap-4 px-4 py-3 text-xs">
                <span className="text-muted-foreground">Schedule</span>
                <div className="w-44">
                  <SearchableSelect
                    value={String(dream.interval_hours ?? 168)}
                    onChange={(v) => saveDream({ interval_hours: Number(v) })}
                    options={dreamIntervalOptions}
                  />
                </div>
              </div>
              <div className="flex items-center justify-between gap-4 px-4 py-3 text-xs">
                <span className="text-muted-foreground">Mode</span>
                <div className="w-56">
                  <SearchableSelect
                    value={dream.mode ?? 'consolidate'}
                    onChange={(v) => saveDream({ mode: v })}
                    options={dreamModeOptions}
                  />
                </div>
              </div>
              <div className="flex items-center justify-between px-4 py-2 text-2xs text-muted-foreground">
                <span>Last run</span>
                <span className="font-mono">{dream.last_dream_at ? new Date(dream.last_dream_at).toLocaleString() : '—'}</span>
              </div>
              <div className="flex items-center justify-between px-4 py-2 text-2xs text-muted-foreground">
                <span>Next run</span>
                <span className="font-mono">{dream.next_dream_at ? new Date(dream.next_dream_at).toLocaleString() : '—'}</span>
              </div>
              {dreamSaving && (
                <div className="flex items-center gap-1.5 px-4 py-2 text-2xs text-muted-foreground">
                  <Loader2 className="h-3 w-3 animate-spin" /> Saving…
                </div>
              )}
            </div>
          </Card>
        )}
      </section>
    </div>
  );
}

// ProfileDreamingTab kept as re-export alias so any stale imports don't hard-error
export const ProfileDreamingTab = ProfileBackgroundTab;

// ─── Metrics tab ───────────────────────────────────────────────────

export function ProfileMetricsTab({ agentId }: { agentId: string }) {
  const [data, setData] = useState<MetricsResponse | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    metricsApi.get(agentId)
      .then(setData)
      .catch(() => setData({ metrics: {} }))
      .finally(() => setLoading(false));
  }, [agentId]);

  const entries = Object.entries(data?.metrics ?? {});

  return (
    <Section
      icon={BarChart3}
      title="Metrics"
      subtitle="7-day rolling performance"
      actions={
        <a
          href={training.exportUrl(agentId, 'jsonl')}
          download
          className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-2xs text-muted-foreground hover:bg-accent"
          title="Download conversation history as JSONL for fine-tuning"
        >
          <Download className="h-3 w-3" />
          Export training data
        </a>
      }
    >
      {loading ? (
        <div className="flex justify-center py-8"><Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /></div>
      ) : entries.length === 0 ? (
        <div className="rounded-xl border border-border bg-card/60 px-6 py-10 text-center">
          <BarChart3 className="mx-auto mb-2 h-6 w-6 text-muted-foreground/50" />
          <p className="text-sm font-medium text-foreground/70">No metrics yet</p>
          <p className="mt-1 text-xs text-muted-foreground max-w-xs mx-auto">
            Performance metrics — response time, tool usage, success rate — are collected automatically
            as this agent handles tasks. Check back after a few conversations.
          </p>
        </div>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {entries.map(([name, m]) => (
            <Card key={name}>
              <header className="border-b border-border/60 px-3 py-2 text-2xs font-semibold uppercase tracking-wider text-muted-foreground">
                {name}
              </header>
              <div className="p-3">
                <div className="font-mono text-2xl font-semibold">
                  {m.avg != null ? m.avg.toFixed(2) : '—'}
                </div>
                <p className="mt-1 text-2xs text-muted-foreground">
                  {m.count ?? 0} samples
                  {m.last && <> · last {new Date(m.last).toLocaleString()}</>}
                </p>
              </div>
            </Card>
          ))}
        </div>
      )}
    </Section>
  );
}
