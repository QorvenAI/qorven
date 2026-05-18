'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback, useRef } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import {
  Megaphone, Calendar, Clock, CheckCircle2, FileText, Users, Zap,
  Plus, Trash2, Send, Loader2, Twitter, Linkedin, Facebook,
  Instagram, Youtube, Check, AlertCircle, RefreshCw, X,
  ChevronLeft, ChevronRight, ExternalLink,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { social as socialApi } from '@/lib/api';
import { useStore } from '@/store';
import { ErrorBoundary } from '@/components/error-boundary';
import { EmptyState } from '@/components/empty-state';
import { toast } from 'sonner';

// ─── Platform config ──────────────────────────────────────────────────────────

const PLATFORMS = [
  { id: 'twitter', label: 'X / Twitter', icon: '𝕏', color: 'bg-black text-white', maxChars: 280 },
  { id: 'linkedin', label: 'LinkedIn', icon: 'in', color: 'bg-blue-700 text-white', maxChars: 3000 },
  { id: 'facebook', label: 'Facebook', icon: 'f', color: 'bg-blue-600 text-white', maxChars: 63206 },
  { id: 'instagram', label: 'Instagram', icon: '📸', color: 'bg-gradient-to-br from-purple-600 to-pink-500 text-white', maxChars: 2200 },
  { id: 'threads', label: 'Threads', icon: '@', color: 'bg-black text-white', maxChars: 500 },
  { id: 'tiktok', label: 'TikTok', icon: '♪', color: 'bg-black text-white', maxChars: 150 },
  { id: 'youtube', label: 'YouTube', icon: '▶', color: 'bg-red-600 text-white', maxChars: 5000 },
  { id: 'bluesky', label: 'Bluesky', icon: '🦋', color: 'bg-sky-500 text-white', maxChars: 300 },
  { id: 'mastodon', label: 'Mastodon', icon: '🐘', color: 'bg-violet-700 text-white', maxChars: 500 },
  { id: 'pinterest', label: 'Pinterest', icon: '📌', color: 'bg-red-700 text-white', maxChars: 500 },
];

const STATUS_COLORS: Record<string, string> = {
  draft:     'bg-muted text-muted-foreground',
  scheduled: 'bg-blue-500/10 text-blue-500',
  published: 'bg-emerald-500/10 text-emerald-500',
  failed:    'bg-destructive/10 text-destructive',
};

const MONTHS = ['January','February','March','April','May','June','July','August','September','October','November','December'];
const DAYS_SHORT = ['Sun','Mon','Tue','Wed','Thu','Fri','Sat'];

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function SocialPage() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const tab = searchParams.get('tab') || 'compose';
  const setTab = (t: string) => router.push(`/social?tab=${t}`);

  const souls = useStore(s => s.souls);
  const [agentFilter, setAgentFilter] = useState('');

  return (
    <ErrorBoundary>
      <div className="space-y-5">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-lg font-semibold flex items-center gap-2">
              <Megaphone className="h-5 w-5" /> Social Publishing
            </h1>
            <p className="text-sm text-muted-foreground">Schedule and publish to 10+ platforms</p>
          </div>
          <div className="flex items-center gap-2">
            <select
              value={agentFilter}
              onChange={e => setAgentFilter(e.target.value)}
              className="qr-select">
              <option value="">All Agents</option>
              {souls.map(s => <option key={s.id} value={s.id}>{s.display_name}</option>)}
            </select>
            <button
              onClick={() => setTab('compose')}
              className="flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 cursor-pointer"
            >
              <Plus className="h-4 w-4" /> New Post
            </button>
          </div>
        </div>

        {/* Tabs */}
        <div className="flex gap-1 border-b border-border">
          {[
            { id: 'compose', label: 'Compose', icon: Megaphone },
            { id: 'calendar', label: 'Calendar', icon: Calendar },
            { id: 'scheduled', label: 'Scheduled', icon: Clock },
            { id: 'published', label: 'Published', icon: CheckCircle2 },
            { id: 'drafts', label: 'Drafts', icon: FileText },
            { id: 'accounts', label: 'Accounts', icon: Users },
            { id: 'autopost', label: 'AutoPost', icon: Zap },
          ].map(({ id, label, icon: Icon }) => (
            <button key={id} onClick={() => setTab(id)}
              className={cn(
                'flex items-center gap-1.5 px-4 py-2 text-sm font-medium border-b-2 -mb-px cursor-pointer transition-colors',
                tab === id ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'
              )}>
              <Icon className="h-3.5 w-3.5" /> {label}
            </button>
          ))}
        </div>

        {/* Tab Content */}
        {tab === 'compose'   && <ComposeTab agentId={agentFilter} onScheduled={() => setTab('scheduled')} />}
        {tab === 'calendar'  && <CalendarTab agentId={agentFilter} />}
        {tab === 'scheduled' && <PostsTab agentId={agentFilter} status="scheduled" />}
        {tab === 'published' && <PostsTab agentId={agentFilter} status="published" />}
        {tab === 'drafts'    && <PostsTab agentId={agentFilter} status="draft" />}
        {tab === 'accounts'  && <AccountsTab agentId={agentFilter} />}
        {tab === 'autopost'  && <AutoPostTab agentId={agentFilter} />}
      </div>
    </ErrorBoundary>
  );
}

// ─── Compose Tab ──────────────────────────────────────────────────────────────

function ComposeTab({ agentId, onScheduled }: { agentId: string; onScheduled: () => void }) {
  const souls = useStore(s => s.souls);
  const [content, setContent] = useState('');
  const [selectedPlatforms, setSelectedPlatforms] = useState<string[]>(['twitter']);
  const [scheduleAt, setScheduleAt] = useState('');
  const [mediaUrls, setMediaUrls] = useState('');
  const [saving, setSaving] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const [selectedAgent, setSelectedAgent] = useState(agentId || (souls[0]?.id ?? ''));
  const textRef = useRef<HTMLTextAreaElement>(null);

  const activePlatform = PLATFORMS.find(p => selectedPlatforms[0] === p.id) ?? PLATFORMS[0]!;
  const charsLeft = activePlatform.maxChars - content.length;
  const charColor = charsLeft < 0 ? 'text-destructive' : charsLeft < 20 ? 'text-amber-500' : 'text-muted-foreground';

  const togglePlatform = (id: string) => {
    setSelectedPlatforms(prev => prev.includes(id) ? prev.filter(p => p !== id) : [...prev, id]);
  };

  const saveDraft = async () => {
    if (!content.trim()) { toast.error('Content required'); return; }
    setSaving(true);
    try {
      await socialApi.createPost({
        content, platforms: selectedPlatforms, status: 'draft',
        agent_id: selectedAgent, media_urls: mediaUrls ? mediaUrls.split('\n').filter(Boolean) : [],
      });
      toast.success('Draft saved');
      setContent(''); setScheduleAt(''); setMediaUrls('');
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed'); }
    finally { setSaving(false); }
  };

  const schedulePost = async () => {
    if (!content.trim()) { toast.error('Content required'); return; }
    if (!scheduleAt) { toast.error('Schedule time required'); return; }
    setSaving(true);
    try {
      await socialApi.createPost({
        content, platforms: selectedPlatforms, status: 'scheduled',
        scheduled_at: new Date(scheduleAt).toISOString(),
        agent_id: selectedAgent, media_urls: mediaUrls ? mediaUrls.split('\n').filter(Boolean) : [],
      });
      toast.success('Post scheduled ✓');
      setContent(''); setScheduleAt(''); setMediaUrls('');
      onScheduled();
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed'); }
    finally { setSaving(false); }
  };

  const publishNow = async () => {
    if (!content.trim()) { toast.error('Content required'); return; }
    setPublishing(true);
    try {
      const post = await socialApi.createPost({
        content, platforms: selectedPlatforms, status: 'draft',
        agent_id: selectedAgent, media_urls: mediaUrls ? mediaUrls.split('\n').filter(Boolean) : [],
      }) as any;
      const result = await socialApi.publishNow(post.id) as any;
      const ok = result?.results?.filter((r: any) => r.success).length ?? 0;
      const fail = result?.results?.filter((r: any) => !r.success).length ?? 0;
      if (fail === 0) toast.success(`Published to ${ok} platform${ok !== 1 ? 's' : ''} ✓`);
      else toast.error(`${ok} succeeded, ${fail} failed`);
      setContent(''); setScheduleAt(''); setMediaUrls('');
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed'); }
    finally { setPublishing(false); }
  };

  return (
    <div className="grid grid-cols-1 lg:grid-cols-[1fr_320px] gap-5">
      {/* Compose area */}
      <div className="space-y-4">
        {/* Platform selector */}
        <div>
          <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2">Platforms</p>
          <div className="flex flex-wrap gap-2">
            {PLATFORMS.map(p => (
              <button key={p.id} onClick={() => togglePlatform(p.id)}
                className={cn(
                  'flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-all cursor-pointer',
                  selectedPlatforms.includes(p.id)
                    ? 'border-primary bg-primary/10 text-primary'
                    : 'border-border text-muted-foreground hover:border-primary/40 hover:text-foreground',
                )}>
                <span>{p.icon}</span> {p.label}
                {selectedPlatforms.includes(p.id) && <Check className="h-3 w-3" />}
              </button>
            ))}
          </div>
        </div>

        {/* Content */}
        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <div className="flex items-center gap-2 px-4 py-2.5 border-b border-border bg-muted/20">
            <span className="text-xs font-medium text-muted-foreground">Content</span>
            <span className={cn('ml-auto text-xs tabular-nums', charColor)}>
              {charsLeft} chars remaining
            </span>
          </div>
          <textarea
            ref={textRef}
            value={content}
            onChange={e => setContent(e.target.value)}
            placeholder="What's on your mind? Write your post..."
            rows={6}
            className="w-full px-4 py-3 text-sm bg-transparent resize-none outline-none placeholder:text-muted-foreground/40"
          />
        </div>

        {/* Media URLs */}
        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <div className="px-4 py-2.5 border-b border-border bg-muted/20">
            <span className="text-xs font-medium text-muted-foreground">Media URLs (one per line, required for Instagram/Pinterest/TikTok)</span>
          </div>
          <textarea
            value={mediaUrls}
            onChange={e => setMediaUrls(e.target.value)}
            placeholder="https://example.com/image.jpg"
            rows={3}
            className="w-full px-4 py-3 text-sm bg-transparent resize-none outline-none placeholder:text-muted-foreground/40 font-mono"
          />
        </div>

        {/* Agent picker */}
        <div className="flex items-center gap-3">
          <label className="text-xs font-medium text-muted-foreground w-24 shrink-0">Post as agent</label>
          <select value={selectedAgent} onChange={e => setSelectedAgent(e.target.value)}
            className="qr-select flex-1">
            {souls.map(s => <option key={s.id} value={s.id}>{s.display_name}</option>)}
          </select>
        </div>

        {/* Schedule time */}
        <div className="flex items-center gap-3">
          <label className="text-xs font-medium text-muted-foreground w-24 shrink-0">Schedule at</label>
          <input type="datetime-local" value={scheduleAt} onChange={e => setScheduleAt(e.target.value)}
            className="qr-input" />
        </div>

        {/* Actions */}
        <div className="flex gap-2">
          <button onClick={publishNow} disabled={publishing || !content.trim() || selectedPlatforms.length === 0}
            className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
            {publishing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
            Publish Now
          </button>
          <button onClick={schedulePost} disabled={saving || !content.trim() || !scheduleAt}
            className="flex items-center gap-1.5 rounded-lg border border-border px-4 py-2 text-sm font-medium hover:bg-accent cursor-pointer disabled:opacity-50">
            {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Clock className="h-3.5 w-3.5" />}
            Schedule
          </button>
          <button onClick={saveDraft} disabled={saving || !content.trim()}
            className="flex items-center gap-1.5 rounded-lg border border-border px-4 py-2 text-sm text-muted-foreground hover:bg-accent cursor-pointer disabled:opacity-50">
            <FileText className="h-3.5 w-3.5" /> Save Draft
          </button>
        </div>
      </div>

      {/* Preview panel */}
      <div className="space-y-3">
        <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">Preview</p>
        {selectedPlatforms.slice(0, 3).map(platformId => {
          const platform = PLATFORMS.find(p => p.id === platformId);
          if (!platform) return null;
          return (
            <div key={platformId} className="rounded-xl border border-border bg-card p-4">
              <div className="flex items-center gap-2 mb-3">
                <span className={cn('h-6 w-6 rounded flex items-center justify-center text-xs font-bold', platform.color)}>
                  {platform.icon}
                </span>
                <span className="text-xs font-medium">{platform.label}</span>
                <span className="ml-auto text-xs text-muted-foreground">
                  {content.length}/{platform.maxChars}
                </span>
              </div>
              <p className="text-sm whitespace-pre-wrap text-foreground/80 leading-relaxed">
                {content || <span className="text-muted-foreground italic">Post preview will appear here…</span>}
              </p>
            </div>
          );
        })}
        {selectedPlatforms.length === 0 && (
          <div className="rounded-xl border border-dashed border-border p-6 text-center">
            <p className="text-xs text-muted-foreground">Select at least one platform to see preview</p>
          </div>
        )}
      </div>
    </div>
  );
}

// ─── Calendar Tab ─────────────────────────────────────────────────────────────

function CalendarTab({ agentId }: { agentId: string }) {
  const router = useRouter();
  const [today] = useState(() => new Date());
  const [current, setCurrent] = useState(() => new Date());
  const [entries, setEntries] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedDay, setSelectedDay] = useState<string | null>(null);

  const year = current.getFullYear();
  const month = current.getMonth();
  const firstDay = new Date(year, month, 1).getDay();
  const daysInMonth = new Date(year, month + 1, 0).getDate();

  const load = useCallback(() => {
    setLoading(true);
    socialApi.calendar(agentId || undefined)
      .then(d => { setEntries(d?.entries ?? []); setLoading(false); })
      .catch(() => setLoading(false));
  }, [agentId]);

  useEffect(() => { load(); }, [load]);

  const byDate: Record<string, any[]> = {};
  entries.forEach(e => { byDate[e.date] = e.posts || []; });

  const todayKey = today.toISOString().slice(0, 10);
  const cells: (number | null)[] = [
    ...Array(firstDay).fill(null),
    ...Array.from({ length: daysInMonth }, (_, i) => i + 1),
  ];
  while (cells.length % 7 !== 0) cells.push(null);

  const selectedPosts = selectedDay ? (byDate[selectedDay] ?? []) : [];

  const statusDot: Record<string, string> = {
    scheduled: 'bg-blue-400', published: 'bg-emerald-400', draft: 'bg-muted-foreground', failed: 'bg-destructive',
  };

  return (
    <div className="flex gap-5">
      {/* Calendar grid */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center justify-between mb-3">
          <button onClick={() => setCurrent(new Date(year, month - 1, 1))}
            className="h-8 w-8 flex items-center justify-center rounded-lg hover:bg-accent cursor-pointer">
            <ChevronLeft className="h-4 w-4" />
          </button>
          <h2 className="text-base font-semibold">{MONTHS[month]} {year}</h2>
          <button onClick={() => setCurrent(new Date(year, month + 1, 1))}
            className="h-8 w-8 flex items-center justify-center rounded-lg hover:bg-accent cursor-pointer">
            <ChevronRight className="h-4 w-4" />
          </button>
        </div>

        <div className="grid grid-cols-7 mb-1">
          {DAYS_SHORT.map(d => (
            <div key={d} className="text-center text-xs font-medium text-muted-foreground py-1">{d}</div>
          ))}
        </div>

        <div className="grid grid-cols-7 gap-px bg-border rounded-xl overflow-hidden border border-border">
          {cells.map((day, i) => {
            if (!day) return <div key={i} className="bg-background/50 min-h-[80px] p-1" />;
            const key = `${year}-${String(month + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
            const dayPosts = byDate[key] ?? [];
            const isToday = key === todayKey;
            const isSelected = key === selectedDay;
            return (
              <div key={i} onClick={() => setSelectedDay(isSelected ? null : key)}
                className={cn(
                  'bg-background min-h-[80px] p-1.5 cursor-pointer hover:bg-accent/30 transition-colors',
                  isSelected && 'bg-primary/5',
                )}>
                <div className={cn(
                  'text-xs font-medium w-6 h-6 flex items-center justify-center rounded-full mb-1',
                  isToday ? 'bg-primary text-primary-foreground' : 'text-muted-foreground',
                )}>
                  {day}
                </div>
                <div className="space-y-0.5">
                  {dayPosts.slice(0, 3).map((post: any, pi: number) => (
                    <div key={pi} className={cn(
                      'text-xs rounded px-1 py-0.5 truncate flex items-center gap-1',
                      post.status === 'scheduled' ? 'bg-blue-500/10 text-blue-500' :
                      post.status === 'published' ? 'bg-emerald-500/10 text-emerald-500' :
                      'bg-muted text-muted-foreground'
                    )}>
                      <span className={cn('h-1.5 w-1.5 rounded-full shrink-0', statusDot[post.status])} />
                      {post.content?.slice(0, 20) || 'Post'}
                    </div>
                  ))}
                  {dayPosts.length > 3 && (
                    <div className="text-xs text-muted-foreground pl-1">+{dayPosts.length - 3} more</div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
        {loading && (
          <div className="flex justify-center py-3 gap-2 text-muted-foreground text-sm">
            <Loader2 className="h-4 w-4 animate-spin" /> Loading…
          </div>
        )}
      </div>

      {/* Day detail */}
      <div className="w-72 shrink-0">
        {selectedDay ? (
          <div className="rounded-xl border border-border bg-card overflow-hidden">
            <div className="px-4 py-3 border-b border-border bg-muted/20">
              <p className="text-sm font-semibold">
                {new Date(selectedDay + 'T00:00:00').toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric' })}
              </p>
              <p className="text-xs text-muted-foreground">{selectedPosts.length} post{selectedPosts.length !== 1 ? 's' : ''}</p>
            </div>
            <div className="divide-y divide-border/50">
              {selectedPosts.length === 0 ? (
                <p className="text-sm text-muted-foreground px-4 py-6 text-center">No posts this day</p>
              ) : selectedPosts.map((post: any, i: number) => (
                <div key={i} className="px-4 py-3">
                  <div className="flex items-start gap-2">
                    <span className={cn('text-xs px-1.5 py-0.5 rounded font-medium shrink-0', STATUS_COLORS[post.status] ?? STATUS_COLORS.draft)}>
                      {post.status}
                    </span>
                    <p className="text-xs text-foreground/80 truncate flex-1">{post.content}</p>
                  </div>
                  <div className="flex flex-wrap gap-1 mt-1.5">
                    {(post.platforms || []).map((p: string) => (
                      <span key={p} className="text-xs bg-muted px-1.5 py-0.5 rounded">{p}</span>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <div className="rounded-xl border border-dashed border-border p-6 text-center">
            <Calendar className="h-8 w-8 mx-auto mb-2 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">Click a day to see scheduled posts</p>
          </div>
        )}
      </div>
    </div>
  );
}

// ─── Posts List Tab ───────────────────────────────────────────────────────────

function PostsTab({ agentId, status }: { agentId: string; status: string }) {
  const [posts, setPosts] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const souls = useStore(s => s.souls);

  const load = useCallback(() => {
    setLoading(true);
    socialApi.listPosts(agentId || undefined, status)
      .then(d => { setPosts(Array.isArray(d) ? d : []); setLoading(false); })
      .catch(() => setLoading(false));
  }, [agentId, status]);

  useEffect(() => { load(); }, [load]);

  const deletePost = async (id: string) => {
    if (!confirm('Delete this post?')) return;
    await socialApi.deletePost(id);
    toast.success('Deleted');
    setPosts(prev => prev.filter(p => p.id !== id));
  };

  const publishPost = async (id: string) => {
    try {
      const result = await socialApi.publishNow(id) as any;
      const ok = result?.results?.filter((r: any) => r.success).length ?? 0;
      toast.success(`Published to ${ok} platform${ok !== 1 ? 's' : ''}`);
      load();
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed'); }
  };

  if (loading) return (
    <div className="space-y-2">
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="rounded-xl border border-border bg-card p-4 space-y-2">
          <div className="h-4 w-48 animate-pulse rounded bg-muted" />
          <div className="h-3 w-full animate-pulse rounded bg-muted" />
        </div>
      ))}
    </div>
  );

  if (posts.length === 0) return (
    <EmptyState
      icon={status === 'scheduled' ? Clock : status === 'published' ? CheckCircle2 : FileText}
      title={`No ${status} posts`}
      description={`${status === 'draft' ? 'Save a draft' : status === 'scheduled' ? 'Schedule a post' : 'Published posts'} to see them here.`}
    />
  );

  return (
    <div className="space-y-2">
      {posts.map(post => {
        const soul = souls.find(s => s.id === post.agent_id);
        const date = post.scheduled_at || post.published_at || post.created_at;
        return (
          <div key={post.id} className="rounded-xl border border-border bg-card p-4">
            <div className="flex items-start gap-3">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1.5">
                  <span className={cn('text-xs px-1.5 py-0.5 rounded font-medium', STATUS_COLORS[post.status] ?? STATUS_COLORS.draft)}>
                    {post.status}
                  </span>
                  {soul && <span className="text-xs text-muted-foreground">{soul.display_name}</span>}
                  {date && (
                    <span className="text-xs text-muted-foreground ml-auto">
                      {new Date(date).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}
                    </span>
                  )}
                </div>
                <p className="text-sm text-foreground/80 line-clamp-2">{post.content}</p>
                <div className="flex flex-wrap gap-1 mt-2">
                  {(post.platforms || []).map((p: string) => (
                    <span key={p} className="text-xs bg-muted px-1.5 py-0.5 rounded font-mono">{p}</span>
                  ))}
                </div>
              </div>
              <div className="flex items-center gap-1 shrink-0">
                {post.status === 'draft' && (
                  <button onClick={() => publishPost(post.id)}
                    className="h-7 w-7 flex items-center justify-center rounded text-muted-foreground hover:text-primary hover:bg-accent cursor-pointer transition-colors"
                    title="Publish now">
                    <Send className="h-3.5 w-3.5" />
                  </button>
                )}
                <button onClick={() => deletePost(post.id)}
                  className="h-7 w-7 flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 cursor-pointer transition-colors">
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

// ─── Accounts Tab ─────────────────────────────────────────────────────────────

function AccountsTab({ agentId }: { agentId: string }) {
  const souls = useStore(s => s.souls);
  const [integrations, setIntegrations] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAdd, setShowAdd] = useState(false);
  const [form, setForm] = useState({ platform: 'twitter', account_name: '', account_id: '', access_token: '', agent_id: agentId || '' });

  const load = useCallback(() => {
    setLoading(true);
    socialApi.listIntegrations(agentId || undefined)
      .then(d => { setIntegrations(Array.isArray(d) ? d : []); setLoading(false); })
      .catch(() => setLoading(false));
  }, [agentId]);

  useEffect(() => { load(); }, [load]);

  const save = async () => {
    if (!form.account_name || !form.access_token) { toast.error('Account name and access token required'); return; }
    try {
      await socialApi.saveIntegration({ ...form });
      toast.success('Account connected');
      setShowAdd(false);
      setForm({ platform: 'twitter', account_name: '', account_id: '', access_token: '', agent_id: agentId || '' });
      load();
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed'); }
  };

  const disconnect = async (id: string) => {
    if (!confirm('Disconnect this account?')) return;
    await socialApi.deleteIntegration(id);
    toast.success('Disconnected');
    setIntegrations(prev => prev.filter(i => i.id !== id));
  };

  return (
    <div className="space-y-4">
      {/* Add account form */}
      {showAdd ? (
        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 border-b border-border bg-muted/20">
            <p className="text-sm font-semibold">Connect Account</p>
            <button onClick={() => setShowAdd(false)} className="text-muted-foreground hover:text-foreground cursor-pointer"><X className="h-4 w-4" /></button>
          </div>
          <div className="p-4 space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="text-xs text-muted-foreground">Platform</label>
                <select value={form.platform} onChange={e => setForm(f => ({ ...f, platform: e.target.value }))}
                  className="qr-select">
                  {PLATFORMS.map(p => <option key={p.id} value={p.id}>{p.label}</option>)}
                </select>
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Agent</label>
                <select value={form.agent_id} onChange={e => setForm(f => ({ ...f, agent_id: e.target.value }))}
                  className="qr-select">
                  {souls.map(s => <option key={s.id} value={s.id}>{s.display_name}</option>)}
                </select>
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Account Name / Handle</label>
                <input value={form.account_name} onChange={e => setForm(f => ({ ...f, account_name: e.target.value }))}
                  placeholder="@handle or email"
                  className="qr-input" />
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Account ID</label>
                <input value={form.account_id} onChange={e => setForm(f => ({ ...f, account_id: e.target.value }))}
                  placeholder="Numeric user ID"
                  className="qr-input" />
              </div>
            </div>
            <div>
              <label className="text-xs text-muted-foreground">
                Access Token
                {form.platform === 'bluesky' && <span className="ml-1 text-amber-500">(handle:app_password)</span>}
                {form.platform === 'mastodon' && <span className="ml-1 text-amber-500">(instance.url:access_token)</span>}
              </label>
              <input type="password" value={form.access_token} onChange={e => setForm(f => ({ ...f, access_token: e.target.value }))}
                placeholder="Bearer token or credentials"
                className="mt-1 qr-input font-mono" />
            </div>
            <div className="flex gap-2">
              <button onClick={save}
                className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 cursor-pointer">
                <Check className="h-3.5 w-3.5" /> Connect
              </button>
              <button onClick={() => setShowAdd(false)}
                className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer">
                Cancel
              </button>
            </div>
          </div>
        </div>
      ) : (
        <button onClick={() => setShowAdd(true)}
          className="flex items-center gap-1.5 rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted-foreground hover:text-foreground hover:border-primary/40 hover:bg-accent/30 transition-colors cursor-pointer w-full">
          <Plus className="h-4 w-4" /> Connect Social Account
        </button>
      )}

      {/* Account list */}
      {loading ? (
        <div className="flex justify-center py-8"><Loader2 className="h-5 w-5 animate-spin text-muted-foreground" /></div>
      ) : integrations.length === 0 ? (
        <EmptyState icon={Users} title="No accounts connected" description="Connect your social media accounts to start publishing." />
      ) : (
        <div className="space-y-2">
          {integrations.map(i => {
            const platform = PLATFORMS.find(p => p.id === i.platform);
            const soul = souls.find(s => s.id === i.agent_id);
            return (
              <div key={i.id} className="flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3">
                <span className={cn('h-9 w-9 rounded-lg flex items-center justify-center text-sm font-bold shrink-0', platform?.color ?? 'bg-muted')}>
                  {platform?.icon ?? '?'}
                </span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium">{i.account_name || i.account_id}</p>
                  <p className="text-xs text-muted-foreground">
                    {platform?.label ?? i.platform}
                    {soul && ` · ${soul.display_name}`}
                    {i.token_expiry && ` · expires ${new Date(i.token_expiry).toLocaleDateString()}`}
                  </p>
                </div>
                <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium',
                  i.active ? 'bg-emerald-500/10 text-emerald-500' : 'bg-muted text-muted-foreground')}>
                  {i.active ? 'Active' : 'Inactive'}
                </span>
                <button onClick={() => disconnect(i.id)}
                  className="h-7 w-7 flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 cursor-pointer transition-colors">
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ─── AutoPost Tab ─────────────────────────────────────────────────────────────

function AutoPostTab({ agentId }: { agentId: string }) {
  const souls = useStore(s => s.souls);
  const [autoposts, setAutoposts] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAdd, setShowAdd] = useState(false);
  const [form, setForm] = useState({
    name: '', source: 'rss', source_url: '', platforms: 'twitter',
    schedule: '0 9 * * 1', active: true, agent_id: agentId || '',
  });

  const load = useCallback(() => {
    setLoading(true);
    socialApi.listAutoPosts(agentId || undefined)
      .then(d => { setAutoposts(Array.isArray(d) ? d : []); setLoading(false); })
      .catch(() => setLoading(false));
  }, [agentId]);

  useEffect(() => { load(); }, [load]);

  const create = async () => {
    if (!form.name) { toast.error('Name required'); return; }
    try {
      await socialApi.createAutoPost({
        ...form,
        platforms: form.platforms.split(',').map(p => p.trim()),
        agent_id: form.agent_id || souls[0]?.id,
      });
      toast.success('AutoPost rule created');
      setShowAdd(false);
      load();
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed'); }
  };

  const deleteRule = async (id: string) => {
    await socialApi.deleteAutoPost(id);
    toast.success('Rule deleted');
    setAutoposts(prev => prev.filter(a => a.id !== id));
  };

  return (
    <div className="space-y-4">
      <div className="rounded-xl border border-border bg-card p-4 text-sm text-muted-foreground">
        <p className="font-medium text-foreground mb-1">What is AutoPost?</p>
        AutoPost rules automatically create and publish posts from an RSS feed on a cron schedule.
        For example: publish your blog posts to Twitter every Monday at 9am.
      </div>

      {showAdd ? (
        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 border-b border-border bg-muted/20">
            <p className="text-sm font-semibold">New AutoPost Rule</p>
            <button onClick={() => setShowAdd(false)} className="text-muted-foreground hover:text-foreground cursor-pointer"><X className="h-4 w-4" /></button>
          </div>
          <div className="p-4 space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="text-xs text-muted-foreground">Rule Name *</label>
                <input value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
                  placeholder="e.g. Blog to Twitter"
                  className="qr-input" />
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Source</label>
                <select value={form.source} onChange={e => setForm(f => ({ ...f, source: e.target.value }))}
                  className="qr-select">
                  <option value="rss">RSS Feed</option>
                  <option value="webhook">Webhook</option>
                  <option value="manual">Manual</option>
                </select>
              </div>
              <div className="col-span-2">
                <label className="text-xs text-muted-foreground">Source URL</label>
                <input value={form.source_url} onChange={e => setForm(f => ({ ...f, source_url: e.target.value }))}
                  placeholder="https://blog.example.com/rss"
                  className="qr-input" />
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Platforms (comma-separated)</label>
                <input value={form.platforms} onChange={e => setForm(f => ({ ...f, platforms: e.target.value }))}
                  placeholder="twitter, linkedin"
                  className="qr-input" />
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Cron Schedule</label>
                <input value={form.schedule} onChange={e => setForm(f => ({ ...f, schedule: e.target.value }))}
                  placeholder="0 9 * * 1 (Mon 9am)"
                  className="mt-1 qr-input font-mono" />
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Agent</label>
                <select value={form.agent_id} onChange={e => setForm(f => ({ ...f, agent_id: e.target.value }))}
                  className="qr-select">
                  {souls.map(s => <option key={s.id} value={s.id}>{s.display_name}</option>)}
                </select>
              </div>
            </div>
            <div className="flex gap-2">
              <button onClick={create}
                className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 cursor-pointer">
                <Zap className="h-3.5 w-3.5" /> Create Rule
              </button>
              <button onClick={() => setShowAdd(false)}
                className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer">
                Cancel
              </button>
            </div>
          </div>
        </div>
      ) : (
        <button onClick={() => setShowAdd(true)}
          className="flex items-center gap-1.5 rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted-foreground hover:text-foreground hover:border-primary/40 hover:bg-accent/30 transition-colors cursor-pointer w-full">
          <Plus className="h-4 w-4" /> New AutoPost Rule
        </button>
      )}

      {loading ? (
        <div className="flex justify-center py-8"><Loader2 className="h-5 w-5 animate-spin text-muted-foreground" /></div>
      ) : autoposts.length === 0 ? (
        <EmptyState icon={Zap} title="No autopost rules" description="Create a rule to automatically publish content from RSS or webhooks." />
      ) : (
        <div className="space-y-2">
          {autoposts.map(a => (
            <div key={a.id} className="flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3">
              <div className={cn('h-2 w-2 rounded-full shrink-0', a.active ? 'bg-emerald-400' : 'bg-muted-foreground')} />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium">{a.name}</p>
                <p className="text-xs text-muted-foreground truncate">
                  {a.source} · <code className="font-mono">{a.schedule}</code> · {(a.platforms || []).join(', ')}
                </p>
              </div>
              <button onClick={() => deleteRule(a.id)}
                className="h-7 w-7 flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 cursor-pointer transition-colors">
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
