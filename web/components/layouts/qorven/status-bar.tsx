'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

/**
 * StatusBar — 24px bar pinned to the bottom of the viewport (P9 T1.3).
 *
 * Single source of truth for "is the platform alive and what is it
 * running?". Replaces a fifth panel / extra rail icons for Terminal
 * + Models; those now appear here as live chips.
 *
 * Layout (left → right):
 *   • "Qorven" brand link  — links to qorven.ai
 *   • Version chip         — clicks open a changelog lightbox
 *   • spacer
 *   • Disconnect dot       — only visible when WS is offline
 *
 * Intentionally does NOT include breadcrumbs, titles, or page chrome —
 * that's the header's job.
 */

import Link from 'next/link';
import { useEffect, useRef, useState } from 'react';
import { usePathname } from 'next/navigation';
import { useStore } from '@/store';
import { X, ExternalLink, MemoryStick, HardDrive, Bot, ArrowUpCircle, Loader2, CheckCircle2, TrendingUp, Clock } from 'lucide-react';

// ── Live clock ────────────────────────────────────────────────────────────────
function LiveClock() {
  const [time, setTime] = useState('');
  useEffect(() => {
    const fmt = () => new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
    setTime(fmt());
    const id = setInterval(() => setTime(fmt()), 1000);
    return () => clearInterval(id);
  }, []);
  if (!time) return null;
  return (
    <span className="flex items-center gap-1 px-1.5 h-full font-mono text-muted-foreground/70 tabular-nums text-2xs select-none">
      <Clock className="h-3 w-3 shrink-0 opacity-60" />
      {time}
    </span>
  );
}

interface UpdateInfo {
  current: string;
  latest: string;
  up_to_date: boolean;
  release_url: string;
}

type UpdateState = 'idle' | 'checking' | 'available' | 'up_to_date' | 'installing' | 'restarting' | 'error';

interface AgentSpend {
  id: string;
  name: string;
  cost_usd: number;
  tokens_in: number;
  tokens_out: number;
}

interface StatsBar {
  mem_used_gb: number;
  mem_total_gb: number;
  disk_used_gb: number;
  disk_total_gb: number;
  uptime_sec: number;
  db_ok: boolean;
  cost_month_usd: number;
  tokens_in_today: number;
  tokens_out_today: number;
  active_qors: number;
  goroutines: number;
  top_agents: AgentSpend[];
}

// Version seen on the first successful response — any change triggers a reload.
let _loadedVersion: string | null = null;

function useStatsBar() {
  const [stats, setStats] = useState<StatsBar | null>(null);
  useEffect(() => {
    const fetch_ = () => {
      const token = typeof window !== 'undefined'
        ? (localStorage.getItem('qorven_token') || process.env.NEXT_PUBLIC_API_TOKEN || '')
        : '';
      fetch('/api/v1/stats/bar', { headers: token ? { Authorization: `Bearer ${token}` } : {} })
        .then(r => r.ok ? r.json() : null)
        .then((d: StatsBar | null) => d && setStats(d))
        .catch(() => {});
    };
    fetch_();
    const t = setInterval(fetch_, 10_000);
    return () => clearInterval(t);
  }, []);
  return stats;
}

export function StatusBar() {
  const pathname = usePathname();
  const wsConnected = useStore((s) => s.wsConnected);
  // Don't show the disconnect dot during the initial connection window.
  const [wsGracePeriod, setWsGracePeriod] = useState(true);
  useEffect(() => {
    const t = setTimeout(() => setWsGracePeriod(false), 3000);
    return () => clearTimeout(t);
  }, []);
  const [version, setVersion] = useState<string>('');
  const [changelogOpen, setChangelogOpen] = useState(false);
  const [changelogMd, setChangelogMd] = useState<string>('');
  const modalRef = useRef<HTMLDivElement>(null);
  const stats = useStatsBar();
  const [updateState, setUpdateState] = useState<UpdateState>('idle');
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null);
  const [displaySec, setDisplaySec] = useState(0);

  useEffect(() => {
    if (stats?.uptime_sec == null) return;
    setDisplaySec(stats.uptime_sec);
  }, [stats?.uptime_sec]);

  useEffect(() => {
    const t = setInterval(() => setDisplaySec(s => s + 1), 1000);
    return () => clearInterval(t);
  }, []);

  useEffect(() => {
    fetch('/api/health/detailed')
      .then((r) => r.ok ? r.json() : null)
      .then((d: { version?: string } | null) => {
        if (d?.version) {
          // Strip any leading "v" from the backend string — we add it ourselves.
          setVersion(d.version.replace(/^v/, ''));
        }
      })
      .catch(() => { /* leave empty */ });
  }, []);

  // Pages that paint their own bottom bar (e.g. /terminal's tmux footer)
  // set data-qorven-no-status-bar on the main canvas.
  const [hide, setHide] = useState(false);
  useEffect(() => {
    if (typeof document === 'undefined') return;
    const flagged = document.querySelector('[data-qorven-no-status-bar]');
    setHide(!!flagged);
  }, [pathname]);

  // Close modal on Escape
  useEffect(() => {
    if (!changelogOpen) return;
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') setChangelogOpen(false); };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [changelogOpen]);

  const openChangelog = async () => {
    setChangelogOpen(true);
    // Load changelog markdown
    if (!changelogMd) {
      try {
        const r = await fetch('/api/v1/changelog');
        const d = await r.json();
        setChangelogMd(d.changelog ?? '');
      } catch {
        setChangelogMd('Failed to load changelog.');
      }
    }
    // Check for updates (once per modal open, skip if already checked)
    if (updateState === 'idle') {
      setUpdateState('checking');
      try {
        const token = typeof window !== 'undefined' ? localStorage.getItem('qorven_token') : '';
        const r = await fetch('/api/v1/admin/update/check', {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        });
        if (r.ok) {
          const d: UpdateInfo = await r.json();
          setUpdateInfo(d);
          setUpdateState(d.up_to_date ? 'up_to_date' : 'available');
        } else {
          setUpdateState('idle');
        }
      } catch {
        setUpdateState('idle');
      }
    }
  };

  const triggerUpdate = async () => {
    setUpdateState('installing');
    try {
      const token = typeof window !== 'undefined' ? localStorage.getItem('qorven_token') : '';
      const r = await fetch('/api/v1/admin/update/install', {
        method: 'POST',
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (r.ok) {
        setUpdateState('restarting');
        // Poll /health until the new version comes up
        const target = updateInfo?.latest;
        const poll = setInterval(async () => {
          try {
            const hr = await fetch('/api/health/detailed');
            const hd = await hr.json();
            const newVer = (hd.version ?? '').replace(/^v/, '');
            if (target && newVer === target) {
              clearInterval(poll);
              window.location.reload();
            }
          } catch { /* server still restarting */ }
        }, 2000);
      } else {
        setUpdateState('error');
      }
    } catch {
      setUpdateState('error');
    }
  };

  // When an update is available show the new version's changelog section.
  // Otherwise show the currently installed version's section.
  const displayVersion = (updateState === 'available' && updateInfo?.latest) ? updateInfo.latest : version;

  const extractSection = (md: string, ver: string) => {
    if (!md || !ver) return md;
    const lines = md.split('\n');
    let inSection = false;
    const out: string[] = [];
    for (const line of lines) {
      if (line.startsWith('## ')) {
        if (inSection) break;
        if (line.includes(ver)) { inSection = true; out.push(line); }
      } else if (inSection) {
        out.push(line);
      }
    }
    return out.join('\n').trim() || '';
  };

  const currentSection = (() => {
    if (!changelogMd) return '';
    const section = extractSection(changelogMd, displayVersion);
    return section || changelogMd;
  })();

  if (hide) return null;

  return (
    <>
      <div
        className="qorven-status-bar fixed bottom-0 z-30 h-6 flex items-center gap-2 border-t border-border bg-muted px-2 text-2xs text-muted-foreground select-none"
        style={{ left: 'var(--nav-width)', right: 0 }}
      >
        {/* Version chip — opens changelog + update check */}
        {version ? (
          <button
            onClick={openChangelog}
            title={updateState === 'available' ? `Update available: v${updateInfo?.latest}` : 'View changelog & check for updates'}
            className="relative flex items-center gap-1 px-1.5 h-full font-mono text-muted-foreground hover:text-foreground transition-colors tabular-nums rounded-sm hover:bg-accent cursor-pointer shrink-0"
          >
            v{version}
          </button>
        ) : null}

        <div className="flex-1" />
        {/* Clock — centered */}
        <LiveClock />
        <div className="flex-1" />

        {/* Right side — system + cost stats */}
        <div className="flex items-center gap-0.5 ml-auto">
          {stats && (
            <>
              {/* DB health dot */}
              <span
                title={stats.db_ok ? 'Database connected' : 'Database disconnected'}
                className={`h-1.5 w-1.5 rounded-full mx-1.5 ${stats.db_ok ? 'bg-emerald-500' : 'bg-destructive'}`}
              />

              {/* Uptime */}
              <StatusChip title={`Uptime: ${fmtUptime(displaySec)} · ${stats.goroutines} goroutines`}>
                {fmtUptime(displaySec)}
              </StatusChip>

              <StatusDivider />

              {/* Memory */}
              <StatusChip title={`RAM used: ${stats.mem_used_gb.toFixed(2)} GB · Available: ${(stats.mem_total_gb - stats.mem_used_gb).toFixed(2)} GB · Total: ${stats.mem_total_gb.toFixed(1)} GB`}>
                <MemoryStick className="h-3 w-3 shrink-0" strokeWidth={2.5} /><span>{stats.mem_used_gb.toFixed(1)}/{stats.mem_total_gb.toFixed(0)}GB</span>
              </StatusChip>

              <StatusDivider />

              {/* Disk */}
              <StatusChip title={`Disk used: ${stats.disk_used_gb.toFixed(2)} GB · Free: ${(stats.disk_total_gb - stats.disk_used_gb).toFixed(2)} GB · Total: ${stats.disk_total_gb.toFixed(1)} GB`}>
                <HardDrive className="h-3 w-3 shrink-0" strokeWidth={2.5} /><span>{stats.disk_used_gb.toFixed(0)}/{stats.disk_total_gb.toFixed(0)}GB</span>
              </StatusChip>

              <StatusDivider />

              {/* Tokens today */}
              <StatusChip title={`Tokens today — Prompt (↑): ${stats.tokens_in_today.toLocaleString()} · Completion (↓): ${stats.tokens_out_today.toLocaleString()} · Total: ${(stats.tokens_in_today + stats.tokens_out_today).toLocaleString()}`}>
                <span className="text-blue-400/70">↑</span>{fmtK(stats.tokens_in_today)}&nbsp;<span className="text-emerald-400/70">↓</span>{fmtK(stats.tokens_out_today)}
              </StatusChip>

              <StatusDivider />

              {/* Cost this month — hoverable with per-agent breakdown */}
              <CostChip cost={stats.cost_month_usd} topAgents={stats.top_agents ?? []} />

              <StatusDivider />

              {/* Active Qors */}
              <ActiveQorsChip count={stats.active_qors} />
            </>
          )}

          {/* Disconnect dot — only after grace period so initial connect doesn't flash */}
          {!wsConnected && !wsGracePeriod && (
            <span title="Disconnected — reconnecting" className="relative flex h-1.5 w-1.5 mx-1.5">
              <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-destructive/70" />
            </span>
          )}
        </div>
      </div>

      {/* Changelog lightbox */}
      {changelogOpen && (
        <div
          className="fixed inset-0 z-50 flex items-end justify-start"
          style={{ paddingLeft: 'calc(var(--rail-width) + 8px)', paddingBottom: '32px' }}
          onClick={(e) => { if (e.target === e.currentTarget) setChangelogOpen(false); }}
        >
          <div
            ref={modalRef}
            role="dialog"
            aria-modal="true"
            aria-label="Changelog"
            className="relative bg-popover border border-border rounded-xl shadow-xl flex flex-col overflow-hidden"
            style={{ width: '420px', maxHeight: '480px' }}
          >
            {/* Header */}
            <div className="flex items-center justify-between px-4 py-2.5 border-b border-border shrink-0">
              <span className="text-xs font-semibold text-foreground">
                {updateState === 'available' && updateInfo?.latest
                  ? `What's new in v${updateInfo.latest}`
                  : version ? `What's new in v${version}` : "What's new"}
              </span>
              <div className="flex items-center gap-2">
                <Link
                  href="https://qorven.ai/changelog"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-1 text-2xs text-muted-foreground hover:text-foreground transition-colors"
                >
                  Full changelog
                  <ExternalLink className="h-3 w-3" strokeWidth={2.5} />
                </Link>
                <button
                  onClick={() => setChangelogOpen(false)}
                  className="flex items-center justify-center h-5 w-5 rounded text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
                >
                  <X className="h-3.5 w-3.5" strokeWidth={2.5} />
                </button>
              </div>
            </div>

            {/* Update banner */}
            <div className="shrink-0 px-4 pt-3 pb-1">
              {updateState === 'checking' && (
                <div className="flex items-center gap-2 text-2xs text-muted-foreground">
                  <Loader2 className="h-3 w-3 animate-spin shrink-0" strokeWidth={2.5} />
                  Checking for updates…
                </div>
              )}
              {updateState === 'up_to_date' && (
                <div className="flex items-center gap-2 text-2xs text-emerald-500">
                  <CheckCircle2 className="h-3 w-3 shrink-0" strokeWidth={2.5} />
                  You're on the latest version.
                </div>
              )}
              {updateState === 'available' && updateInfo && (
                <div className="flex items-center justify-between gap-3 rounded-lg border border-emerald-500/30 bg-emerald-500/5 px-3 py-2">
                  <div className="flex items-center gap-2 min-w-0">
                    <ArrowUpCircle className="h-3.5 w-3.5 text-emerald-500 shrink-0" strokeWidth={2.5} />
                    <span className="text-2xs text-foreground font-medium">
                      v{updateInfo.latest} available
                    </span>
                    <span className="text-2xs text-muted-foreground truncate">
                      — currently v{updateInfo.current || version}
                    </span>
                  </div>
                  <button
                    onClick={triggerUpdate}
                    className="shrink-0 rounded-md bg-emerald-500 px-2.5 py-1 text-2xs font-semibold text-white hover:bg-emerald-600 transition-colors"
                  >
                    Update now
                  </button>
                </div>
              )}
              {updateState === 'installing' && (
                <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/40 px-3 py-2">
                  <Loader2 className="h-3.5 w-3.5 animate-spin text-primary shrink-0" strokeWidth={2.5} />
                  <span className="text-2xs text-foreground font-medium">Downloading update…</span>
                </div>
              )}
              {updateState === 'restarting' && (
                <div className="flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2">
                  <Loader2 className="h-3.5 w-3.5 animate-spin text-amber-500 shrink-0" strokeWidth={2.5} />
                  <span className="text-2xs text-amber-600 dark:text-amber-400 font-medium">Restarting — page will reload automatically…</span>
                </div>
              )}
              {updateState === 'error' && (
                <div className="flex items-center gap-2 text-2xs text-destructive">
                  Update failed. Check server logs.
                </div>
              )}
            </div>

            {/* Body — rendered as plain text / simple markdown */}
            <div className="overflow-y-auto flex-1 px-4 py-3">
              {currentSection ? (
                <ChangelogBody markdown={currentSection} />
              ) : (
                <span className="text-xs text-muted-foreground">Loading…</span>
              )}
            </div>
          </div>
        </div>
      )}
    </>
  );
}

function CostChip({ cost, topAgents }: { cost: number; topAgents: AgentSpend[] }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen(o => !o)}
        className="px-1.5 h-6 flex items-center gap-0.5 font-mono text-muted-foreground hover:text-foreground hover:bg-accent transition-colors rounded-sm cursor-pointer tabular-nums"
        title={`Total spend this month: $${cost.toFixed(6)}`}
      >
        <TrendingUp className="h-3 w-3 mr-0.5 shrink-0" strokeWidth={2.5} />
        ${cost.toFixed(4)}
      </button>
      {open && (
        <div className="absolute bottom-full right-0 mb-1.5 w-56 rounded-xl border border-border bg-popover shadow-xl z-50 overflow-hidden">
          <div className="flex items-center justify-between px-3 py-2 border-b border-border">
            <span className="text-xs font-semibold text-foreground">Monthly spend</span>
            <Link
              href="/settings?section=usage"
              onClick={() => setOpen(false)}
              className="text-2xs text-primary hover:text-primary/80 transition-colors"
            >
              Usage →
            </Link>
          </div>
          {topAgents.length === 0 ? (
            <p className="px-3 py-2 text-2xs text-muted-foreground">No usage recorded yet.</p>
          ) : (
            <div className="py-1">
              {topAgents.map((a) => (
                <div key={a.id} className="flex items-center justify-between px-3 py-1.5 hover:bg-accent transition-colors">
                  <span className="text-2xs text-foreground truncate max-w-[120px]">{a.name}</span>
                  <span className="text-2xs font-mono text-muted-foreground shrink-0">${a.cost_usd.toFixed(4)}</span>
                </div>
              ))}
            </div>
          )}
          <div className="px-3 py-2 border-t border-border flex items-center justify-between">
            <span className="text-2xs text-muted-foreground">Total</span>
            <span className="text-2xs font-mono font-semibold text-foreground">${cost.toFixed(4)}</span>
          </div>
        </div>
      )}
    </div>
  );
}

function StatusChip({ children, title }: { children: React.ReactNode; title?: string }) {
  return (
    <span
      title={title}
      className="px-1.5 h-full flex items-center gap-1 font-mono text-muted-foreground hover:text-foreground hover:bg-accent transition-colors rounded-sm cursor-default tabular-nums"
    >
      {children}
    </span>
  );
}

function StatusDivider() {
  return <span className="h-3 w-px bg-border mx-0.5" />;
}

function fmtUptime(sec: number): string {
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const hh = String(h).padStart(2, '0');
  const mm = String(m).padStart(2, '0');
  const ss = String(s).padStart(2, '0');
  if (d > 0) return `${d}d ${hh}:${mm}:${ss}`;
  return `${hh}:${mm}:${ss}`;
}

function fmtK(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

function ActiveQorsChip({ count }: { count: number }) {
  const soulStates = useStore((s) => s.soulStates);
  const thinking = Object.values(soulStates).filter(s => s.activity === 'thinking').length;
  const running  = Object.values(soulStates).filter(s => s.activity === 'running').length;
  const active   = thinking + running;

  let dotClass = 'bg-muted-foreground/40';
  if (running > 0)  dotClass = 'bg-emerald-500';
  else if (thinking > 0) dotClass = 'bg-amber-400';

  const tooltip = [
    `Active Qors (DB): ${count}`,
    thinking > 0 ? `Thinking: ${thinking}` : '',
    running  > 0 ? `Running: ${running}`   : '',
    active === 0 ? 'All Qors idle' : '',
  ].filter(Boolean).join(' · ');

  return (
    <span
      title={tooltip}
      className="px-1.5 h-full flex items-center gap-1 font-mono text-muted-foreground hover:text-foreground hover:bg-accent transition-colors rounded-sm cursor-default tabular-nums"
    >
      <span className={`h-1.5 w-1.5 rounded-full shrink-0 ${dotClass}`} />
      <Bot className="h-3 w-3 shrink-0" strokeWidth={2.5} />
      {active > 0 ? active : count}
    </span>
  );
}

/** Lightweight markdown renderer for changelog content. */
function ChangelogBody({ markdown }: { markdown: string }) {
  const lines = markdown.split('\n');
  const nodes: React.ReactNode[] = [];
  let key = 0;

  for (const raw of lines) {
    const line = raw.trimEnd();
    if (line.startsWith('## ')) {
      nodes.push(
        <h2 key={key++} className="text-sm font-semibold text-foreground mt-1 mb-2">
          {line.slice(3)}
        </h2>
      );
    } else if (line.startsWith('### ')) {
      nodes.push(
        <h3 key={key++} className="text-xs font-semibold text-foreground/80 mt-3 mb-1 uppercase tracking-wide">
          {line.slice(4)}
        </h3>
      );
    } else if (line.startsWith('- **')) {
      // Bold title + description pattern: "- **Title** — description"
      const inner = line.slice(2);
      const m = inner.match(/^\*\*(.+?)\*\*(.*)$/);
      if (m) {
        nodes.push(
          <div key={key++} className="flex gap-1.5 text-xs mb-1 leading-relaxed">
            <span className="text-muted-foreground/50 shrink-0 mt-0.5">•</span>
            <span>
              <strong className="font-semibold text-foreground">{m[1]}</strong>
              <span className="text-muted-foreground">{m[2]}</span>
            </span>
          </div>
        );
      } else {
        nodes.push(
          <div key={key++} className="flex gap-1.5 text-xs mb-1 text-muted-foreground leading-relaxed">
            <span className="shrink-0 mt-0.5">•</span><span>{inner}</span>
          </div>
        );
      }
    } else if (line.startsWith('- ')) {
      nodes.push(
        <div key={key++} className="flex gap-1.5 text-xs mb-1 text-muted-foreground leading-relaxed">
          <span className="shrink-0 mt-0.5">•</span><span>{line.slice(2)}</span>
        </div>
      );
    } else if (line === '') {
      nodes.push(<div key={key++} className="h-1" />);
    } else {
      nodes.push(
        <p key={key++} className="text-xs text-muted-foreground mb-1 leading-relaxed">{line}</p>
      );
    }
  }

  return <>{nodes}</>;
}
