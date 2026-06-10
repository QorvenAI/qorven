'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback, useRef } from 'react';
import { request } from '@/lib/api-core';
import { agents } from '@/lib/api';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';
import { PageShell } from '@/components/layouts/page-shell';
import {
  Check, X, Pencil, RefreshCw, Clock, FileText,
  CheckCircle2, XCircle, Inbox, ChevronDown, LayoutList, LayoutGrid,
  MessageCircle, AtSign, Newspaper, Terminal,
} from 'lucide-react';

// ─── Types ──────────────────────────────────────────────────────────────────

interface ContentItem {
  id: string;
  agent_id: string;
  agent_name: string;
  action_type: string;
  content: string;
  platforms: string[];
  channel: string;
  status: string;
  requested_at: string;
  metadata?: Record<string, unknown>;
}

interface ContentStats {
  pending: number;
  approved_today: number;
  rejected_today: number;
  total_30d: number;
}

interface Agent {
  id: string;
  agent_key: string;
  display_name: string;
}

// ─── Constants ──────────────────────────────────────────────────────────────

const platformColors: Record<string, string> = {
  twitter: 'bg-sky-500/20 text-sky-400',
  linkedin: 'bg-blue-600/20 text-blue-400',
  reddit: 'bg-orange-500/20 text-orange-400',
  hackernews: 'bg-amber-500/20 text-amber-400',
  instagram: 'bg-pink-500/20 text-pink-400',
  facebook: 'bg-blue-500/20 text-blue-400',
};

const POLL_INTERVAL = 30_000;

// ─── Helpers ────────────────────────────────────────────────────────────────

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60_000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function agentInitial(name: string): string {
  return (name || '?').charAt(0).toUpperCase();
}

function agentColor(id: string): string {
  const colors = [
    'bg-violet-600', 'bg-sky-600', 'bg-emerald-600', 'bg-amber-600',
    'bg-rose-600', 'bg-indigo-600', 'bg-teal-600', 'bg-fuchsia-600',
  ];
  let hash = 0;
  for (let i = 0; i < id.length; i++) hash = (hash * 31 + id.charCodeAt(i)) | 0;
  return colors[Math.abs(hash) % colors.length] || 'bg-violet-600';
}

// ─── Components ─────────────────────────────────────────────────────────────

function StatCard({ icon: Icon, label, value, loading }: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string | number;
  loading: boolean;
}) {
  return (
    <div className="rounded-xl border border-border bg-card px-4 py-3">
      <div className="flex items-center gap-2 text-muted-foreground mb-1">
        <Icon className="h-4 w-4" />
        <span className="text-xs">{label}</span>
      </div>
      <div className="text-2xl font-semibold text-foreground">
        {loading ? <span className="inline-block h-7 w-12 animate-pulse rounded bg-muted" /> : value}
      </div>
    </div>
  );
}

function PlatformBadge({ platform }: { platform: string }) {
  const color = platformColors[platform.toLowerCase()] || 'bg-muted text-muted-foreground';
  return (
    <span className={cn('inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium', color)}>
      {platform}
    </span>
  );
}

function ContentCard({ item, onApprove, onReject, onSave }: {
  item: ContentItem;
  onApprove: (id: string) => void;
  onReject: (id: string, reason?: string) => void;
  onSave: (id: string, content: string, platforms: string[]) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [editContent, setEditContent] = useState(item.content);
  const [editPlatforms, setEditPlatforms] = useState<string[]>(item.platforms);
  const [expanded, setExpanded] = useState(false);
  const [rejecting, setRejecting] = useState(false);
  const [rejectReason, setRejectReason] = useState('');

  const truncated = item.content.length > 280 && !expanded && !editing;
  const displayContent = truncated ? item.content.slice(0, 280) + '...' : item.content;

  const handleEdit = () => {
    setEditing(true);
    setEditContent(item.content);
    setEditPlatforms([...item.platforms]);
  };

  const handleSave = () => {
    onSave(item.id, editContent, editPlatforms);
    setEditing(false);
  };

  const handleReject = () => {
    if (!rejecting) {
      setRejecting(true);
      return;
    }
    onReject(item.id, rejectReason || undefined);
    setRejecting(false);
    setRejectReason('');
  };

  const togglePlatform = (p: string) => {
    setEditPlatforms((prev) =>
      prev.includes(p) ? prev.filter((x) => x !== p) : [...prev, p]
    );
  };

  return (
    <div className={cn(
      'rounded-xl border border-border bg-card p-4 transition-all',
      item.status === 'approved' && 'opacity-60',
      item.status === 'rejected' && 'opacity-40',
    )}>
      <div className="flex gap-4">
        {/* Left: avatar + meta */}
        <div className="flex flex-col items-center gap-1 shrink-0">
          <div className={cn(
            'h-9 w-9 rounded-full flex items-center justify-center text-sm font-semibold text-white',
            agentColor(item.agent_id),
          )}>
            {agentInitial(item.agent_name)}
          </div>
          <span className="text-xs text-muted-foreground text-center max-w-[64px] truncate">
            {item.agent_name}
          </span>
          <span className="text-2xs text-muted-foreground">
            {timeAgo(item.requested_at)}
          </span>
        </div>

        {/* Center: content */}
        <div className="flex-1 min-w-0">
          {editing ? (
            <div className="space-y-3">
              <textarea
                className="w-full rounded-lg border border-border bg-input px-3 py-2 text-sm text-foreground resize-y min-h-[100px] focus:outline-none focus:ring-1 focus:ring-ring"
                value={editContent}
                onChange={(e) => setEditContent(e.target.value)}
                rows={5}
              />
              <div className="flex flex-wrap gap-2">
                {Object.keys(platformColors).map((p) => (
                  <label key={p} className="flex items-center gap-1.5 text-xs cursor-pointer">
                    <input
                      type="checkbox"
                      checked={editPlatforms.includes(p)}
                      onChange={() => togglePlatform(p)}
                      className="rounded border-border"
                    />
                    <span className="text-muted-foreground">{p}</span>
                  </label>
                ))}
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={handleSave}
                  className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
                >
                  Save
                </button>
                <button
                  onClick={() => setEditing(false)}
                  className="flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs font-medium text-muted-foreground hover:bg-accent"
                >
                  Cancel
                </button>
              </div>
            </div>
          ) : (
            <>
              <p className="text-sm text-foreground whitespace-pre-wrap break-words">
                {displayContent}
              </p>
              {truncated && (
                <button
                  onClick={() => setExpanded(true)}
                  className="mt-1 text-xs text-primary hover:underline"
                >
                  Show more
                </button>
              )}
              {expanded && item.content.length > 280 && (
                <button
                  onClick={() => setExpanded(false)}
                  className="mt-1 text-xs text-primary hover:underline"
                >
                  Show less
                </button>
              )}
              <div className="mt-2 flex flex-wrap gap-1.5">
                {item.platforms.map((p) => (
                  <PlatformBadge key={p} platform={p} />
                ))}
              </div>
            </>
          )}

          {/* Reject reason input */}
          {rejecting && !editing && (
            <div className="mt-3 flex items-center gap-2">
              <input
                type="text"
                placeholder="Reason (optional)"
                value={rejectReason}
                onChange={(e) => setRejectReason(e.target.value)}
                className="flex-1 rounded-lg border border-border bg-input px-3 py-1.5 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleReject();
                  if (e.key === 'Escape') { setRejecting(false); setRejectReason(''); }
                }}
                autoFocus
              />
              <button
                onClick={handleReject}
                className="rounded-lg bg-destructive px-3 py-1.5 text-xs font-medium text-destructive-foreground hover:bg-destructive/90"
              >
                Confirm
              </button>
              <button
                onClick={() => { setRejecting(false); setRejectReason(''); }}
                className="rounded-lg border border-border px-3 py-1.5 text-xs text-muted-foreground hover:bg-accent"
              >
                Cancel
              </button>
            </div>
          )}
        </div>

        {/* Right: action buttons */}
        {item.status === 'pending' && !editing && !rejecting && (
          <div className="flex flex-col gap-1.5 shrink-0">
            <button
              onClick={() => onApprove(item.id)}
              title="Approve & publish"
              className="flex h-8 w-8 items-center justify-center rounded-lg border border-emerald-500/30 text-emerald-400 hover:bg-emerald-500/10 transition-colors"
            >
              <Check className="h-4 w-4" />
            </button>
            <button
              onClick={handleEdit}
              title="Edit"
              className="flex h-8 w-8 items-center justify-center rounded-lg border border-border text-muted-foreground hover:bg-accent transition-colors"
            >
              <Pencil className="h-4 w-4" />
            </button>
            <button
              onClick={handleReject}
              title="Reject"
              className="flex h-8 w-8 items-center justify-center rounded-lg border border-destructive/30 text-destructive hover:bg-destructive/10 transition-colors"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

// ─── Channel Section (Okara-style grouped view) ────────────────────────────

const channelMeta: Record<string, { label: string; icon: typeof MessageCircle; color: string }> = {
  social_post: { label: 'Social Posts', icon: AtSign, color: 'text-sky-400' },
  social: { label: 'Social Posts', icon: AtSign, color: 'text-sky-400' },
  reddit_reply: { label: 'Reddit Opportunities', icon: MessageCircle, color: 'text-orange-400' },
  reddit: { label: 'Reddit Opportunities', icon: MessageCircle, color: 'text-orange-400' },
  article_draft: { label: 'Articles & Blog Posts', icon: Newspaper, color: 'text-emerald-400' },
  article: { label: 'Articles & Blog Posts', icon: Newspaper, color: 'text-emerald-400' },
  seo_fix: { label: 'SEO Recommendations', icon: Terminal, color: 'text-violet-400' },
  hackernews: { label: 'Hacker News', icon: Terminal, color: 'text-amber-400' },
};

function ChannelSection({ channel, items, onApprove, onReject, onSave }: {
  channel: string;
  items: ContentItem[];
  onApprove: (id: string) => void;
  onReject: (id: string, reason?: string) => void;
  onSave: (id: string, content: string, platforms: string[]) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const meta = channelMeta[channel] || { label: channel, icon: FileText, color: 'text-muted-foreground' };
  const Icon = meta.icon;

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between px-4 py-3 hover:bg-accent/30 transition-colors"
      >
        <div className="flex items-center gap-3">
          <Icon className={cn('h-5 w-5', meta.color)} />
          <span className="text-sm font-semibold text-foreground">{meta.label}</span>
          <span className="flex items-center justify-center h-5 min-w-[20px] rounded-full bg-primary/20 text-primary text-xs font-medium px-1.5">
            {items.length}
          </span>
          <span className="text-xs text-muted-foreground">
            {items.length === 1 ? 'item ready' : 'items ready'}
          </span>
        </div>
        <ChevronDown className={cn('h-4 w-4 text-muted-foreground transition-transform', expanded && 'rotate-180')} />
      </button>
      {expanded && (
        <div className="border-t border-border px-3 py-3 space-y-2">
          {items.map((item) => (
            <ContentCard key={item.id} item={item} onApprove={onApprove} onReject={onReject} onSave={onSave} />
          ))}
        </div>
      )}
    </div>
  );
}

// ─── Main Page ──────────────────────────────────────────────────────────────

export default function ContentFeedPage() {
  const [items, setItems] = useState<ContentItem[]>([]);
  const [stats, setStats] = useState<ContentStats>({ pending: 0, approved_today: 0, rejected_today: 0, total_30d: 0 });
  const [agentList, setAgentList] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);

  // Filters + view mode
  const [statusFilter, setStatusFilter] = useState('pending');
  const [channelFilter, setChannelFilter] = useState('');
  const [agentFilter, setAgentFilter] = useState('');
  const [viewMode, setViewMode] = useState<'list' | 'grouped'>('list');

  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchData = useCallback(async () => {
    try {
      const params = new URLSearchParams();
      if (statusFilter && statusFilter !== 'all') params.set('status', statusFilter);
      if (channelFilter) params.set('channel', channelFilter);
      if (agentFilter) params.set('agent_id', agentFilter);
      params.set('limit', '50');

      const [feedItems, feedStats] = await Promise.all([
        request<ContentItem[]>(`/content-feed?${params.toString()}`),
        request<ContentStats>('/content-feed/stats'),
      ]);

      setItems(feedItems || []);
      setStats(feedStats || { pending: 0, approved_today: 0, rejected_today: 0, total_30d: 0 });
    } catch (err) {
      console.error('Failed to fetch content feed:', err);
    } finally {
      setLoading(false);
    }
  }, [statusFilter, channelFilter, agentFilter]);

  useEffect(() => {
    agents.list().then((list) => setAgentList(list as unknown as Agent[])).catch(() => {});
  }, []);

  useEffect(() => {
    setLoading(true);
    fetchData();
  }, [fetchData]);

  // Polling
  useEffect(() => {
    pollRef.current = setInterval(fetchData, POLL_INTERVAL);
    return () => { if (pollRef.current) clearInterval(pollRef.current); };
  }, [fetchData]);

  const handleApprove = async (id: string) => {
    const item = items.find((i) => i.id === id);
    setItems((prev) => prev.filter((i) => i.id !== id));
    try {
      const result = await request<{ status: string; post_id?: string; type?: string; cms?: Record<string, unknown>; reddit?: Record<string, unknown> }>(`/content-feed/${id}/approve`, { method: 'POST' });
      if (result?.type === 'seo_fix') {
        toast.success('SEO recommendation approved');
      } else if (result?.cms) {
        toast.success(`Published to ${(result.cms.platform as string) || 'CMS'}`);
      } else if (result?.reddit) {
        toast.success('Reddit reply posted');
      } else {
        toast.success(`Published to ${item?.platforms.join(', ') || 'platforms'}`);
      }
      setStats((s) => ({ ...s, pending: Math.max(0, s.pending - 1), approved_today: s.approved_today + 1 }));
    } catch (err) {
      // Revert optimistic removal
      if (item) setItems((prev) => [item, ...prev]);
      toast.error('Failed to approve: ' + (err instanceof Error ? err.message : 'Unknown error'));
    }
  };

  const handleReject = async (id: string, reason?: string) => {
    const item = items.find((i) => i.id === id);
    setItems((prev) => prev.filter((i) => i.id !== id));
    try {
      await request<{ status: string }>(`/content-feed/${id}/reject`, {
        method: 'POST',
        body: JSON.stringify({ reason }),
      });
      toast.success('Rejected');
      setStats((s) => ({ ...s, pending: Math.max(0, s.pending - 1), rejected_today: s.rejected_today + 1 }));
    } catch (err) {
      if (item) setItems((prev) => [item, ...prev]);
      toast.error('Failed to reject: ' + (err instanceof Error ? err.message : 'Unknown error'));
    }
  };

  const handleSave = async (id: string, content: string, platforms: string[]) => {
    try {
      const updated = await request<ContentItem>(`/content-feed/${id}`, {
        method: 'PUT',
        body: JSON.stringify({ content, platforms }),
      });
      setItems((prev) => prev.map((i) => (i.id === id ? { ...i, ...updated } : i)));
      toast.success('Content updated');
    } catch (err) {
      toast.error('Failed to save: ' + (err instanceof Error ? err.message : 'Unknown error'));
    }
  };

  return (
    <PageShell
      title="Content Approval"
      description="Review, edit, and approve agent-produced content before publishing"
      actions={
        <button
          onClick={() => { setLoading(true); fetchData(); }}
          disabled={loading}
          className="flex h-9 items-center gap-2 rounded-lg border border-border bg-input px-3 text-sm text-muted-foreground hover:bg-accent disabled:opacity-50"
        >
          <RefreshCw className={cn('h-4 w-4', loading && 'animate-spin')} />
          Refresh
        </button>
      }
      toolbar={
        <>
          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            className="h-9 rounded-lg border border-border bg-input px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
          >
            <option value="pending">Pending</option>
            <option value="approved">Approved</option>
            <option value="rejected">Rejected</option>
            <option value="all">All</option>
          </select>

          <select
            value={channelFilter}
            onChange={(e) => setChannelFilter(e.target.value)}
            className="h-9 rounded-lg border border-border bg-input px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
          >
            <option value="">All channels</option>
            <option value="social_post">Social Posts</option>
            <option value="article_draft">Articles</option>
            <option value="reddit_reply">Reddit</option>
            <option value="seo_fix">SEO Fixes</option>
          </select>

          <select
            value={agentFilter}
            onChange={(e) => setAgentFilter(e.target.value)}
            className="h-9 rounded-lg border border-border bg-input px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
          >
            <option value="">All agents</option>
            {agentList.map((a) => (
              <option key={a.id} value={a.id}>
                {a.display_name || a.agent_key}
              </option>
            ))}
          </select>

          <div className="ml-auto flex items-center gap-1 rounded-lg border border-border p-0.5">
            <button
              onClick={() => setViewMode('list')}
              className={cn('flex h-7 w-7 items-center justify-center rounded-md transition-colors', viewMode === 'list' ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground')}
              title="List view"
            >
              <LayoutList className="h-3.5 w-3.5" />
            </button>
            <button
              onClick={() => setViewMode('grouped')}
              className={cn('flex h-7 w-7 items-center justify-center rounded-md transition-colors', viewMode === 'grouped' ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground')}
              title="Grouped by channel"
            >
              <LayoutGrid className="h-3.5 w-3.5" />
            </button>
          </div>
        </>
      }
      contentClassName="px-6 py-5 space-y-5 sm:px-6"
    >
        {/* Stats row */}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard icon={Clock} label="Pending" value={stats.pending} loading={loading} />
          <StatCard icon={CheckCircle2} label="Approved today" value={stats.approved_today} loading={loading} />
          <StatCard icon={XCircle} label="Rejected today" value={stats.rejected_today} loading={loading} />
          <StatCard icon={FileText} label="Total (30 days)" value={stats.total_30d} loading={loading} />
        </div>

        {/* Feed */}
        {loading && items.length === 0 ? (
          <div className="space-y-3">
            {[1, 2, 3].map((i) => (
              <div key={i} className="h-24 animate-pulse rounded-xl border border-border bg-card" />
            ))}
          </div>
        ) : items.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <Inbox className="h-12 w-12 text-muted-foreground/40 mb-3" />
            <p className="text-sm text-muted-foreground">
              No content awaiting approval. Your agents will produce drafts on their next run.
            </p>
          </div>
        ) : viewMode === 'grouped' ? (
          <div className="space-y-3">
            {Object.entries(
              items.reduce<Record<string, ContentItem[]>>((acc, item) => {
                const ch = item.channel || item.action_type || 'social';
                (acc[ch] ||= []).push(item);
                return acc;
              }, {})
            ).map(([channel, channelItems]) => (
              <ChannelSection
                key={channel}
                channel={channel}
                items={channelItems}
                onApprove={handleApprove}
                onReject={handleReject}
                onSave={handleSave}
              />
            ))}
          </div>
        ) : (
          <div className="space-y-3">
            {items.map((item) => (
              <ContentCard
                key={item.id}
                item={item}
                onApprove={handleApprove}
                onReject={handleReject}
                onSave={handleSave}
              />
            ))}
          </div>
        )}
    </PageShell>
  );
}
