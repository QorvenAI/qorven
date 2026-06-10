'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import { request } from '@/lib/api-core';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';
import {
  BarChart3, TrendingUp, CheckCircle2, Users, Lock,
  RefreshCw, Search, Globe, FileText,
} from 'lucide-react';
import { PageShell } from '@/components/layouts/page-shell';

// --- Types ---

interface OverviewData {
  content_produced_7d: number;
  content_produced_30d: number;
  approved_7d: number;
  rejected_7d: number;
  published_7d: number;
  approval_rate: number;
  posts_by_platform: Record<string, number>;
  posts_by_agent: { agent_id: string; agent_name: string; count: number }[];
}

interface SeoData {
  connected: boolean;
  clicks?: number;
  impressions?: number;
  ctr?: number;
  position?: number;
  connect_url?: string;
}

interface TrafficData {
  connected: boolean;
  sessions_7d?: number;
  users_7d?: number;
  pageviews_7d?: number;
  top_pages?: { path: string; views: number }[];
  connect_url?: string;
}

interface TimelineDay {
  date: string;
  produced: number;
  approved: number;
  published: number;
  rejected: number;
}

type Period = '7d' | '30d';

// --- Page ---

export default function AnalyticsPage() {
  const [period, setPeriod] = useState<Period>('30d');
  const [loading, setLoading] = useState(true);
  const [overview, setOverview] = useState<OverviewData | null>(null);
  const [seo, setSeo] = useState<SeoData | null>(null);
  const [traffic, setTraffic] = useState<TrafficData | null>(null);
  const [timeline, setTimeline] = useState<TimelineDay[]>([]);

  const load = useCallback(() => {
    setLoading(true);
    const days = period === '7d' ? 7 : 30;
    Promise.all([
      request<OverviewData>('/analytics/overview').catch(() => null),
      request<SeoData>('/analytics/seo').catch(() => null),
      request<TrafficData>('/analytics/traffic').catch(() => null),
      request<TimelineDay[]>(`/analytics/timeline?days=${days}`).catch(() => []),
    ]).then(([o, s, t, tl]) => {
      setOverview(o);
      setSeo(s);
      setTraffic(t);
      setTimeline(tl ?? []);
    }).catch(() => {
      toast.error('Failed to load analytics data');
    }).finally(() => {
      setLoading(false);
    });
  }, [period]);

  useEffect(() => { load(); }, [load]);

  const contentProduced = period === '7d' ? (overview?.content_produced_7d ?? 0) : (overview?.content_produced_30d ?? 0);
  const published = overview?.published_7d ?? 0;
  const approvalRate = overview?.approval_rate ?? 0;
  const activeAgents = overview?.posts_by_agent?.length ?? 0;

  return (
    <PageShell
      title="Analytics"
      contentClassName="px-0 py-0 sm:px-0"
      actions={
        <div className="flex items-center gap-2">
          <div className="flex rounded-lg border border-border overflow-hidden text-sm">
            <button
              onClick={() => setPeriod('7d')}
              className={cn(
                'px-3 py-1.5 transition-colors',
                period === '7d' ? 'bg-primary text-primary-foreground' : 'bg-card text-muted-foreground hover:bg-accent'
              )}
            >
              7d
            </button>
            <button
              onClick={() => setPeriod('30d')}
              className={cn(
                'px-3 py-1.5 transition-colors',
                period === '30d' ? 'bg-primary text-primary-foreground' : 'bg-card text-muted-foreground hover:bg-accent'
              )}
            >
              30d
            </button>
          </div>
          <button
            onClick={load}
            disabled={loading}
            className="flex h-9 items-center gap-2 rounded-lg border border-border bg-input px-3 text-sm text-muted-foreground hover:bg-accent disabled:opacity-50"
          >
            <RefreshCw className={cn('h-4 w-4', loading && 'animate-spin')} />
            Refresh
          </button>
        </div>
      }
    >
      <div className="space-y-6 px-6 pb-8 pt-4">
        {/* Row 1: Stat cards */}
        <div className="grid gap-4 grid-cols-1 md:grid-cols-2 lg:grid-cols-4">
          <StatCard
            icon={FileText}
            label="Content Produced"
            value={contentProduced}
            accent="blue"
            loading={loading}
          />
          <StatCard
            icon={CheckCircle2}
            label="Published"
            value={published}
            accent="green"
            loading={loading}
          />
          <StatCard
            icon={TrendingUp}
            label="Approval Rate"
            value={`${approvalRate.toFixed(0)}%`}
            accent="amber"
            loading={loading}
          />
          <StatCard
            icon={Users}
            label="Active Agents"
            value={activeAgents}
            accent="purple"
            loading={loading}
          />
        </div>

        {/* Row 2: Timeline chart */}
        <div className="rounded-xl border border-border bg-card p-5">
          <h2 className="text-sm font-semibold text-foreground mb-4">Content Timeline</h2>
          {loading ? (
            <div className="h-48 animate-pulse rounded bg-muted" />
          ) : timeline.length === 0 ? (
            <div className="h-48 flex items-center justify-center text-sm text-muted-foreground">
              No timeline data available
            </div>
          ) : (
            <TimelineChart data={timeline} />
          )}
        </div>

        {/* Row 3: SEO + Traffic */}
        <div className="grid gap-4 grid-cols-1 md:grid-cols-2">
          <SeoCard data={seo} loading={loading} />
          <TrafficCard data={traffic} loading={loading} />
        </div>

        {/* Row 4: By Platform + By Agent */}
        <div className="grid gap-4 grid-cols-1 md:grid-cols-2">
          <PlatformCard data={overview?.posts_by_platform ?? {}} loading={loading} />
          <AgentCard data={overview?.posts_by_agent ?? []} loading={loading} />
        </div>
      </div>
    </PageShell>
  );
}

// --- Stat Card ---

function StatCard({ icon: Icon, label, value, accent, loading }: {
  icon: typeof FileText;
  label: string;
  value: number | string;
  accent: 'blue' | 'green' | 'amber' | 'purple';
  loading?: boolean;
}) {
  const iconColor = {
    blue: 'text-blue-400',
    green: 'text-emerald-400',
    amber: 'text-amber-400',
    purple: 'text-purple-400',
  }[accent];

  return (
    <div className="rounded-xl border border-border bg-card p-5 relative">
      <Icon className={cn('h-5 w-5 absolute top-4 right-4', iconColor)} />
      {loading ? (
        <div className="h-10 w-24 animate-pulse rounded bg-muted mt-1" />
      ) : (
        <p className="text-3xl font-semibold tabular-nums">{value}</p>
      )}
      <p className="text-xs text-muted-foreground mt-1">{label}</p>
    </div>
  );
}

// --- Timeline Chart (pure SVG) ---

function TimelineChart({ data }: { data: TimelineDay[] }) {
  const [hovered, setHovered] = useState<number | null>(null);

  const maxTotal = Math.max(...data.map(d => d.produced + d.published + d.rejected), 1);
  const chartWidth = 800;
  const chartHeight = 180;
  const barGap = 2;
  const barWidth = Math.max((chartWidth - barGap * data.length) / data.length, 4);

  return (
    <div className="relative">
      <svg
        viewBox={`0 0 ${chartWidth} ${chartHeight + 28}`}
        className="w-full h-auto"
        preserveAspectRatio="xMidYMid meet"
      >
        {data.map((d, i) => {
          const x = i * (barWidth + barGap);
          const producedH = (d.produced / maxTotal) * chartHeight;
          const publishedH = (d.published / maxTotal) * chartHeight;
          const rejectedH = (d.rejected / maxTotal) * chartHeight;
          const totalH = producedH + publishedH + rejectedH;
          const baseY = chartHeight - totalH;

          return (
            <g
              key={d.date}
              onMouseEnter={() => setHovered(i)}
              onMouseLeave={() => setHovered(null)}
              className="cursor-pointer"
            >
              {/* Invisible hit area */}
              <rect x={x} y={0} width={barWidth} height={chartHeight} fill="transparent" />
              {/* Blue: produced */}
              <rect
                x={x}
                y={baseY}
                width={barWidth}
                height={producedH}
                className="fill-blue-500"
                rx={1}
              />
              {/* Green: published */}
              <rect
                x={x}
                y={baseY + producedH}
                width={barWidth}
                height={publishedH}
                className="fill-emerald-500"
                rx={1}
              />
              {/* Red: rejected */}
              <rect
                x={x}
                y={baseY + producedH + publishedH}
                width={barWidth}
                height={rejectedH}
                className="fill-red-500"
                rx={1}
              />
              {/* X-axis label (every 5th) */}
              {i % 5 === 0 && (
                <text
                  x={x + barWidth / 2}
                  y={chartHeight + 16}
                  textAnchor="middle"
                  className="fill-muted-foreground text-2xs"
                >
                  {formatDateLabel(d.date)}
                </text>
              )}
            </g>
          );
        })}
      </svg>

      {/* Tooltip */}
      {hovered !== null && data[hovered] && (
        <div
          className="absolute top-0 pointer-events-none bg-popover border border-border rounded-lg px-3 py-2 text-xs shadow-lg z-10"
          style={{
            left: `${(hovered / data.length) * 100}%`,
            transform: 'translateX(-50%)',
          }}
        >
          <p className="font-medium text-foreground mb-1">{formatDateLabel(data[hovered].date)}</p>
          <div className="space-y-0.5">
            <p><span className="inline-block w-2 h-2 rounded-sm bg-blue-500 mr-1.5" />Produced: {data[hovered].produced}</p>
            <p><span className="inline-block w-2 h-2 rounded-sm bg-emerald-500 mr-1.5" />Published: {data[hovered].published}</p>
            <p><span className="inline-block w-2 h-2 rounded-sm bg-red-500 mr-1.5" />Rejected: {data[hovered].rejected}</p>
          </div>
        </div>
      )}

      {/* Legend */}
      <div className="flex items-center gap-4 mt-2 text-xs text-muted-foreground">
        <span className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-sm bg-blue-500" />Produced</span>
        <span className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-sm bg-emerald-500" />Published</span>
        <span className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-sm bg-red-500" />Rejected</span>
      </div>
    </div>
  );
}

// --- SEO Card ---

function SeoCard({ data, loading }: { data: SeoData | null; loading: boolean }) {
  if (loading) {
    return (
      <div className="rounded-xl border border-border bg-card p-5">
        <h2 className="text-sm font-semibold text-foreground mb-4">SEO Health</h2>
        <div className="h-32 animate-pulse rounded bg-muted" />
      </div>
    );
  }

  if (!data || !data.connected) {
    return (
      <div className="rounded-xl border border-border bg-card p-5">
        <h2 className="text-sm font-semibold text-foreground mb-4">SEO Health</h2>
        <div className="flex flex-col items-center justify-center py-8 text-center">
          <Lock className="h-8 w-8 text-muted-foreground/40 mb-3" />
          <p className="text-sm text-muted-foreground mb-3">
            Connect Google Search Console to see SEO metrics
          </p>
          <a
            href="/settings"
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            <Search className="h-4 w-4" />
            Connect Search Console
          </a>
        </div>
      </div>
    );
  }

  return (
    <div className="rounded-xl border border-border bg-card p-5">
      <h2 className="text-sm font-semibold text-foreground mb-4">SEO Health</h2>
      <div className="grid grid-cols-2 gap-4">
        <MetricItem label="Clicks" value={fmtNum(data.clicks ?? 0)} />
        <MetricItem label="Impressions" value={fmtNum(data.impressions ?? 0)} />
        <MetricItem label="CTR" value={`${((data.ctr ?? 0) * 100).toFixed(1)}%`} />
        <MetricItem label="Avg Position" value={(data.position ?? 0).toFixed(1)} />
      </div>
    </div>
  );
}

// --- Traffic Card ---

function TrafficCard({ data, loading }: { data: TrafficData | null; loading: boolean }) {
  if (loading) {
    return (
      <div className="rounded-xl border border-border bg-card p-5">
        <h2 className="text-sm font-semibold text-foreground mb-4">Traffic</h2>
        <div className="h-32 animate-pulse rounded bg-muted" />
      </div>
    );
  }

  if (!data || !data.connected) {
    return (
      <div className="rounded-xl border border-border bg-card p-5">
        <h2 className="text-sm font-semibold text-foreground mb-4">Traffic</h2>
        <div className="flex flex-col items-center justify-center py-8 text-center">
          <Lock className="h-8 w-8 text-muted-foreground/40 mb-3" />
          <p className="text-sm text-muted-foreground mb-3">
            Connect Google Analytics to see traffic metrics
          </p>
          <a
            href="/settings"
            className="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
          >
            <Globe className="h-4 w-4" />
            Connect Google Analytics
          </a>
        </div>
      </div>
    );
  }

  return (
    <div className="rounded-xl border border-border bg-card p-5">
      <h2 className="text-sm font-semibold text-foreground mb-4">Traffic</h2>
      <div className="grid grid-cols-3 gap-4 mb-4">
        <MetricItem label="Sessions" value={fmtNum(data.sessions_7d ?? 0)} />
        <MetricItem label="Users" value={fmtNum(data.users_7d ?? 0)} />
        <MetricItem label="Pageviews" value={fmtNum(data.pageviews_7d ?? 0)} />
      </div>
      {data.top_pages && data.top_pages.length > 0 && (
        <div className="border-t border-border pt-3">
          <p className="text-xs text-muted-foreground mb-2">Top Pages</p>
          <div className="space-y-1.5">
            {data.top_pages.slice(0, 5).map((page) => (
              <div key={page.path} className="flex items-center justify-between text-xs">
                <span className="text-foreground truncate max-w-[70%]">{page.path}</span>
                <span className="text-muted-foreground tabular-nums">{page.views.toLocaleString()}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// --- Platform Card ---

const PLATFORM_COLORS: Record<string, string> = {
  twitter: 'bg-blue-500',
  linkedin: 'bg-blue-700',
  facebook: 'bg-indigo-500',
  instagram: 'bg-pink-500',
  youtube: 'bg-red-500',
  tiktok: 'bg-slate-700',
  blog: 'bg-emerald-500',
  email: 'bg-amber-500',
};

function PlatformCard({ data, loading }: { data: Record<string, number>; loading: boolean }) {
  const entries = Object.entries(data).sort((a, b) => b[1] - a[1]);
  const maxCount = Math.max(...entries.map(([, v]) => v), 1);

  return (
    <div className="rounded-xl border border-border bg-card p-5">
      <h2 className="text-sm font-semibold text-foreground mb-4">By Platform</h2>
      {loading ? (
        <div className="space-y-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-6 animate-pulse rounded bg-muted" />
          ))}
        </div>
      ) : entries.length === 0 ? (
        <div className="flex items-center justify-center py-8 text-sm text-muted-foreground">
          No platform data yet
        </div>
      ) : (
        <div className="space-y-3">
          {entries.map(([platform, count]) => {
            const barColor = PLATFORM_COLORS[platform.toLowerCase()] ?? 'bg-primary';
            const pct = (count / maxCount) * 100;
            return (
              <div key={platform}>
                <div className="flex items-center justify-between text-xs mb-1">
                  <span className="text-foreground capitalize font-medium">{platform}</span>
                  <span className="text-muted-foreground tabular-nums">{count}</span>
                </div>
                <div className="h-2 rounded-full bg-muted overflow-hidden">
                  <div
                    className={cn('h-full rounded-full transition-all', barColor)}
                    style={{ width: `${pct}%` }}
                  />
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// --- Agent Card ---

function AgentCard({ data, loading }: { data: { agent_id: string; agent_name: string; count: number }[]; loading: boolean }) {
  const sorted = [...data].sort((a, b) => b.count - a.count);

  return (
    <div className="rounded-xl border border-border bg-card p-5">
      <h2 className="text-sm font-semibold text-foreground mb-4">By Agent</h2>
      {loading ? (
        <div className="space-y-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-8 animate-pulse rounded bg-muted" />
          ))}
        </div>
      ) : sorted.length === 0 ? (
        <div className="flex items-center justify-center py-8 text-sm text-muted-foreground">
          No agent data yet
        </div>
      ) : (
        <div className="space-y-2.5">
          {sorted.map((agent) => (
            <div key={agent.agent_id} className="flex items-center gap-3">
              <div className="h-8 w-8 rounded-full bg-primary/10 flex items-center justify-center shrink-0">
                <span className="text-xs font-medium text-primary">
                  {agent.agent_name.slice(0, 2).toUpperCase()}
                </span>
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-foreground truncate">{agent.agent_name}</p>
              </div>
              <span className="text-sm tabular-nums text-muted-foreground font-medium">{agent.count}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// --- Metric Item ---

function MetricItem({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-2xl font-semibold tabular-nums text-foreground">{value}</p>
      <p className="text-xs text-muted-foreground mt-0.5">{label}</p>
    </div>
  );
}

// --- Helpers ---

function fmtNum(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
  return String(n);
}

function formatDateLabel(dateStr: string): string {
  const d = new Date(dateStr);
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}
