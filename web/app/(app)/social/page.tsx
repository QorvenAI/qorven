'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback, useRef } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import {
  Megaphone, Calendar, Clock, CheckCircle2, FileText, Users, Zap,
  Plus, Trash2, Send, Loader2, Check, X,
  ChevronLeft, ChevronRight,
  Image as ImageIcon, Upload, Search, Grid, Video, Copy,
  BarChart2, TrendingUp, Eye, Heart, Share2, MessageCircle,
  CornerDownRight, CheckCheck,
  BookOpen, Edit3, Settings, Pause, Play, Webhook, PlayCircle,
} from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { cn } from '@/lib/utils';
import { social as socialApi } from '@/lib/api';
import { integrationsApi, RelayKeyRecord } from '@/lib/api-integrations';
import { useStore } from '@/store';
import { ErrorBoundary } from '@/components/error-boundary';
import { EmptyState } from '@/components/empty-state';
import { OfficerSetupCard } from '@/components/setup/officer-setup-card';
import { toast } from 'sonner';

// ─── Platform config ──────────────────────────────────────────────────────────

const PLATFORMS = [
  { id: 'twitter',          label: 'X / Twitter',       icon: '𝕏',  color: 'bg-black text-white',                                          maxChars: 280 },
  { id: 'linkedin',         label: 'LinkedIn',           icon: 'in', color: 'bg-blue-700 text-white',                                       maxChars: 3000 },
  { id: 'facebook',         label: 'Facebook',           icon: 'f',  color: 'bg-blue-600 text-white',                                       maxChars: 63206 },
  { id: 'instagram',        label: 'Instagram',          icon: '📸', color: 'bg-gradient-to-br from-purple-600 to-pink-500 text-white',     maxChars: 2200 },
  { id: 'threads',          label: 'Threads',            icon: '@',  color: 'bg-black text-white',                                          maxChars: 500 },
  { id: 'tiktok',           label: 'TikTok',             icon: '♪',  color: 'bg-black text-white',                                          maxChars: 150 },
  { id: 'youtube',          label: 'YouTube',            icon: '▶',  color: 'bg-red-600 text-white',                                        maxChars: 5000 },
  { id: 'bluesky',          label: 'Bluesky',            icon: '🦋', color: 'bg-sky-500 text-white',                                        maxChars: 300 },
  { id: 'mastodon',         label: 'Mastodon',           icon: '🐘', color: 'bg-violet-700 text-white',                                     maxChars: 500 },
  { id: 'pinterest',        label: 'Pinterest',          icon: '📌', color: 'bg-red-700 text-white',                                        maxChars: 500 },
  { id: 'reddit',           label: 'Reddit',             icon: '🤖', color: 'bg-orange-600 text-white',                                     maxChars: 40000 },
  { id: 'discord',          label: 'Discord',            icon: '#',  color: 'bg-indigo-600 text-white',                                     maxChars: 2000 },
  { id: 'slack',            label: 'Slack',              icon: '⚡', color: 'bg-emerald-600 text-white',                                    maxChars: 40000 },
  { id: 'devto',            label: 'Dev.to',             icon: 'D',  color: 'bg-gray-900 text-white',                                       maxChars: 65535 },
  { id: 'medium',           label: 'Medium',             icon: 'M',  color: 'bg-gray-800 text-white',                                       maxChars: 100000 },
  { id: 'wordpress',        label: 'WordPress',          icon: 'W',  color: 'bg-blue-500 text-white',                                       maxChars: 200000 },
  { id: 'googlemybusiness', label: 'Google My Business', icon: 'G',  color: 'bg-blue-600 text-white',                                       maxChars: 1500 },
  { id: 'nostr',            label: 'Nostr',              icon: '⚡', color: 'bg-yellow-500 text-black',                                     maxChars: 2000 },
  { id: 'telegrambot',      label: 'Telegram Bot',       icon: '✈',  color: 'bg-sky-500 text-white',                                        maxChars: 4096 },
];

// Per-platform connection config — drives the connect form fields and instructions
type PlatformAuthField = {
  key: string;          // key into form.extras
  label: string;
  type: 'text' | 'password';
  placeholder: string;
  defaultValue?: string;
  optional?: boolean;
  hint?: string;
};

type PlatformAuth = {
  tokenLabel: string;
  tokenPlaceholder: string;
  tokenHint: string;
  accountIdLabel?: string;
  accountIdPlaceholder?: string;
  showAccountId: boolean;
  docsUrl: string;
  docsLabel: string;
  warning?: string;
  // When set, renders these fields instead of the single token input
  customFields?: PlatformAuthField[];
  // Combines customFields values into the access_token string sent to backend
  buildToken?: (extras: Record<string, string>) => string;
};

const PLATFORM_AUTH: Record<string, PlatformAuth> = {
  twitter: {
    tokenLabel: 'Bearer Token (OAuth 2.0)',
    tokenPlaceholder: 'AAAA…',
    tokenHint: 'From Twitter Developer Portal → Project → Keys & Tokens → Bearer Token',
    showAccountId: true,
    accountIdLabel: 'Twitter User ID',
    accountIdPlaceholder: '123456789',
    docsUrl: 'https://developer.twitter.com/en/portal/projects-and-apps',
    docsLabel: 'Twitter Developer Portal ↗',
  },
  linkedin: {
    tokenLabel: 'Access Token',
    tokenPlaceholder: 'AQV…',
    tokenHint: 'From LinkedIn Developer App → Auth → OAuth 2.0 tokens. Requires r_liteprofile + w_member_social scopes.',
    showAccountId: false,
    docsUrl: 'https://www.linkedin.com/developers/apps',
    docsLabel: 'LinkedIn Developer Apps ↗',
  },
  facebook: {
    tokenLabel: 'Page Access Token',
    tokenPlaceholder: 'EAAn…',
    tokenHint: 'From Meta Graph API Explorer → select your Page → Generate Access Token.',
    showAccountId: true,
    accountIdLabel: 'Facebook Page ID',
    accountIdPlaceholder: '123456789012345',
    docsUrl: 'https://developers.facebook.com/tools/explorer',
    docsLabel: 'Meta Graph Explorer ↗',
  },
  instagram: {
    tokenLabel: 'Instagram Graph API Token',
    tokenPlaceholder: 'EAAn…',
    tokenHint: 'Same token as Facebook Page Access Token — your Instagram Business account must be linked to a Facebook Page. Posts require an image URL.',
    showAccountId: true,
    accountIdLabel: 'Instagram Business Account ID',
    accountIdPlaceholder: '17841400000000000',
    docsUrl: 'https://developers.facebook.com/docs/instagram-api/getting-started',
    docsLabel: 'Instagram Graph API docs ↗',
    warning: 'Instagram only supports image posts — text-only posts will fail.',
  },
  threads: {
    tokenLabel: 'Threads Access Token',
    tokenPlaceholder: 'THR…',
    tokenHint: 'From Meta Developer App with Threads API access. Requires threads_basic + threads_content_publish permissions.',
    showAccountId: false,
    docsUrl: 'https://developers.facebook.com/docs/threads',
    docsLabel: 'Threads API docs ↗',
  },
  tiktok: {
    tokenLabel: 'TikTok Access Token',
    tokenPlaceholder: 'act.…',
    tokenHint: 'From TikTok for Developers → your app → access token. Requires video.upload scope. TikTok only supports video posts.',
    showAccountId: false,
    docsUrl: 'https://developers.tiktok.com/',
    docsLabel: 'TikTok for Developers ↗',
    warning: 'TikTok only supports video posts — text-only posts will fail.',
  },
  youtube: {
    tokenLabel: 'Google OAuth Access Token',
    tokenPlaceholder: 'ya29.…',
    tokenHint: 'From Google Cloud Console → OAuth 2.0 credentials. Requires youtube.upload or youtube.force-ssl scope.',
    showAccountId: false,
    docsUrl: 'https://console.cloud.google.com/apis/credentials',
    docsLabel: 'Google Cloud Console ↗',
  },
  bluesky: {
    tokenLabel: 'App Password',
    tokenPlaceholder: '',
    tokenHint: '',
    showAccountId: false,
    docsUrl: 'https://bsky.app/settings/app-passwords',
    docsLabel: 'Bluesky App Passwords ↗',
    customFields: [
      {
        key: 'bsky_service',
        label: 'Service URL',
        type: 'text',
        placeholder: 'bsky.social',
        defaultValue: 'bsky.social',
        optional: true,
        hint: 'Leave as bsky.social unless you use a custom PDS',
      },
      {
        key: 'bsky_handle',
        label: 'Handle or Email',
        type: 'text',
        placeholder: 'yourhandle.bsky.social',
      },
      {
        key: 'bsky_password',
        label: 'App Password',
        type: 'password',
        placeholder: 'xxxx-xxxx-xxxx-xxxx',
        hint: 'Generate in Bluesky Settings → App Passwords (do not use your main password)',
      },
    ],
    buildToken: (e) => `${e.bsky_handle ?? ''}:${e.bsky_password ?? ''}`,
  },
  mastodon: {
    tokenLabel: 'Access Token',
    tokenPlaceholder: '',
    tokenHint: '',
    showAccountId: false,
    docsUrl: 'https://docs.joinmastodon.org/client/token/',
    docsLabel: 'Mastodon API docs ↗',
    customFields: [
      {
        key: 'masto_instance',
        label: 'Instance URL',
        type: 'text',
        placeholder: 'mastodon.social',
        hint: 'Your Mastodon server, e.g. mastodon.social or fosstodon.org',
      },
      {
        key: 'masto_token',
        label: 'Access Token',
        type: 'password',
        placeholder: 'your-access-token',
        hint: 'From your instance → Settings → Development → New Application → Your token',
      },
    ],
    buildToken: (e) => `${e.masto_instance ?? ''}:${e.masto_token ?? ''}`,
  },
  pinterest: {
    tokenLabel: 'Pinterest Access Token',
    tokenPlaceholder: 'pina_…',
    tokenHint: 'From Pinterest Developer Apps → your app → access token. Requires boards:read + pins:write scopes. Posts require an image URL.',
    showAccountId: false,
    docsUrl: 'https://developers.pinterest.com/apps/',
    docsLabel: 'Pinterest Developer Apps ↗',
    warning: 'Pinterest only supports image pins — text-only posts will fail.',
  },
  reddit: {
    tokenLabel: 'OAuth Access Token',
    tokenPlaceholder: '',
    tokenHint: 'Connect via OAuth to post to Reddit. You\'ll be redirected to Reddit to authorize.',
    showAccountId: false,
    docsUrl: 'https://www.reddit.com/prefs/apps',
    docsLabel: 'Reddit App Preferences ↗',
  },
  discord: {
    tokenLabel: 'Bot Token',
    tokenPlaceholder: '',
    tokenHint: 'Connect via OAuth to get a webhook token for posting to a Discord channel.',
    showAccountId: false,
    docsUrl: 'https://discord.com/developers/applications',
    docsLabel: 'Discord Developer Portal ↗',
  },
  slack: {
    tokenLabel: 'OAuth Bot Token',
    tokenPlaceholder: '',
    tokenHint: 'Connect via OAuth to post to a Slack workspace channel.',
    showAccountId: false,
    docsUrl: 'https://api.slack.com/apps',
    docsLabel: 'Slack API Apps ↗',
  },
  devto: {
    tokenLabel: 'API Key',
    tokenPlaceholder: 'your-devto-api-key',
    tokenHint: 'From dev.to → Settings → Extensions → DEV Community API Keys. Create a new key and paste it here.',
    showAccountId: false,
    docsUrl: 'https://dev.to/settings/extensions',
    docsLabel: 'Dev.to API Keys ↗',
  },
  medium: {
    tokenLabel: 'Integration Token',
    tokenPlaceholder: '',
    tokenHint: 'Connect via OAuth or from Medium → Settings → Security and apps → Integration tokens.',
    showAccountId: false,
    docsUrl: 'https://medium.com/me/settings',
    docsLabel: 'Medium Settings ↗',
  },
  wordpress: {
    tokenLabel: 'Application Password',
    tokenPlaceholder: '',
    tokenHint: '',
    showAccountId: false,
    docsUrl: 'https://developer.wordpress.org/rest-api/using-the-rest-api/authentication/',
    docsLabel: 'WordPress REST API docs ↗',
    customFields: [
      {
        key: 'wp_url',
        label: 'WordPress URL',
        type: 'text',
        placeholder: 'https://yoursite.wordpress.com',
        hint: 'Full URL of your WordPress site',
      },
      {
        key: 'wp_user',
        label: 'Username',
        type: 'text',
        placeholder: 'admin',
      },
      {
        key: 'wp_password',
        label: 'Application Password',
        type: 'password',
        placeholder: 'xxxx xxxx xxxx xxxx xxxx xxxx',
        hint: 'Generate in WordPress Admin → Users → Profile → Application Passwords',
      },
    ],
    buildToken: (e) => `${e.wp_url ?? ''}|${e.wp_user ?? ''}|${e.wp_password ?? ''}`,
  },
  googlemybusiness: {
    tokenLabel: 'OAuth Access Token',
    tokenPlaceholder: '',
    tokenHint: 'Connect via OAuth to post updates to your Google Business Profile.',
    showAccountId: false,
    docsUrl: 'https://developers.google.com/my-business/content/overview',
    docsLabel: 'Google My Business API ↗',
  },
  nostr: {
    tokenLabel: 'Private Key (nsec or hex)',
    tokenPlaceholder: 'nsec1… or 64-char hex',
    tokenHint: 'Your Nostr private key in nsec (bech32) or raw hex format. This is stored encrypted in Qorven — never shared.',
    showAccountId: false,
    docsUrl: 'https://nostr.how/',
    docsLabel: 'Nostr documentation ↗',
    warning: 'Your private key grants full control of your Nostr identity. Use a dedicated key for automation.',
  },
  telegrambot: {
    tokenLabel: 'Bot Token',
    tokenPlaceholder: '',
    tokenHint: '',
    showAccountId: false,
    docsUrl: 'https://core.telegram.org/bots#how-do-i-create-a-bot',
    docsLabel: 'Telegram BotFather docs ↗',
    customFields: [
      {
        key: 'tg_bot_token',
        label: 'Bot Token',
        type: 'password',
        placeholder: '123456789:AABBcc…',
        hint: 'From @BotFather on Telegram → /newbot → token',
      },
      {
        key: 'tg_chat_id',
        label: 'Chat ID',
        type: 'text',
        placeholder: '-100123456789 or @channelname',
        hint: 'Channel/group ID to post to. Use @username for public channels or -100xxxxxxxxx for private ones.',
      },
    ],
    buildToken: (e) => `${e.tg_bot_token ?? ''}:${e.tg_chat_id ?? ''}`,
  },
};

// Platforms that use OAuth 2.0 popup flow via the backend
const OAUTH_PLATFORMS = new Set([
  'twitter', 'linkedin', 'facebook', 'instagram', 'threads', 'tiktok',
  'youtube', 'pinterest', 'reddit', 'discord', 'slack', 'medium', 'googlemybusiness',
]);

const RELAY_PLATFORM_SUPPORT: Record<string, string[]> = {
  outstand: ['twitter', 'instagram', 'facebook', 'tiktok', 'linkedin', 'threads', 'bluesky', 'youtube', 'pinterest', 'googlemybusiness'],
  postforme: ['twitter', 'instagram', 'facebook', 'tiktok', 'linkedin', 'threads', 'bluesky', 'youtube', 'pinterest'],
  buffer: ['twitter', 'instagram', 'facebook', 'tiktok', 'linkedin', 'threads', 'bluesky', 'youtube', 'pinterest', 'googlemybusiness', 'mastodon'],
};

const RELAY_LABELS: Record<string, string> = {
  outstand: 'Outstand',
  postforme: 'PostForMe',
  buffer: 'Buffer',
};

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
  const tab = searchParams.get('tab') ?? 'compose';
  const setTab = (t: string) => router.push(`/social?tab=${t}`);

  const souls = useStore(s => s.souls);
  const [agentFilter, setAgentFilter] = useState('');

  const [cmo, setCmo] = useState<import('@/types').Soul | null>(null);
  const [cmoChecked, setCmoChecked] = useState(false);

  useEffect(() => {
    import('@/lib/api-agents')
      .then(({ agents }) => agents.byRole('cmo'))
      .then((existing) => setCmo(existing))
      .catch(() => setCmo(null))
      .finally(() => setCmoChecked(true));
  }, []);

  // Once the CMO exists, default the agent filter to it so social work attributes to the CMO.
  useEffect(() => {
    if (cmo?.id && !agentFilter) setAgentFilter(cmo.id);
  }, [cmo, agentFilter]);

  const createCmo = async ({ name, model }: { name: string; model: string; providerId: string }) => {
    const { agents } = await import('@/lib/api-agents');
    const coo = (await agents.byRole('coo')) ?? (await agents.byKey('chief'));
    const created = await agents.create({
      agent_key: 'cmo',
      role: 'marketer',
      display_name: name,
      title: 'CMO',
      org_role: 'cmo',
      org_level: 'l2',
      manager_id: coo?.id ?? null,
      ...(model ? { model } : {}),
      system_prompt: `You are ${name}, the CMO. You own marketing and social on the /social page. You plan campaigns and content with the user and delegate execution to specialist writers and social workers. Be brand-aware and concise.`,
    } as Partial<import('@/types').Soul>);
    setCmo(created);
  };

  if (cmoChecked && !cmo) {
    return (
      <ErrorBoundary>
        <div className="h-full">
          <OfficerSetupCard
            role="cmo"
            roleLabel="CMO"
            pageName="social"
            defaultName="Prime Marketer"
            blurb="Your Chief Marketing Officer plans campaigns and content across every platform, delegating execution to specialist writers."
            onCreate={createCmo}
          />
        </div>
      </ErrorBoundary>
    );
  }

  return (
    <ErrorBoundary>
      <div className="flex flex-col h-full min-h-0">
        <CanvasHeader
          title="Social Publishing"
          description="Schedule and publish to 10+ platforms"
          actions={
            <>
              <select
                value={agentFilter}
                onChange={e => setAgentFilter(e.target.value)}
                className="qr-select"
              >
                <option value="">All Agents</option>
                {souls.map(s => <option key={s.id} value={s.id}>{s.display_name}</option>)}
              </select>
              <button
                onClick={() => setTab('compose')}
                className="flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 cursor-pointer"
              >
                <Plus className="h-4 w-4" /> New Post
              </button>
            </>
          }
        />
        <div className="flex-1 overflow-y-auto px-6 pb-6">
          {tab === 'compose'   && <ComposeTab agentId={agentFilter} onScheduled={() => setTab('scheduled')} />}
          {tab === 'calendar'  && <CalendarTab agentId={agentFilter} />}
          {tab === 'scheduled' && <PostsTab agentId={agentFilter} status="scheduled" />}
          {tab === 'published' && <PostsTab agentId={agentFilter} status="published" />}
          {tab === 'drafts'    && <PostsTab agentId={agentFilter} status="draft" />}
          {tab === 'accounts'  && <AccountsTab agentId={agentFilter} />}
          {tab === 'autopost'  && <AutoPostTab agentId={agentFilter} />}
          {tab === 'media'     && <MediaTab agentId={agentFilter} />}
          {tab === 'analytics' && <AnalyticsTab agentId={agentFilter} />}
          {tab === 'sets'      && <SetsTab agentId={agentFilter} />}
          {tab === 'webhooks'  && <WebhooksTab agentId={agentFilter} />}
        </div>
      </div>
    </ErrorBoundary>
  );
}

// ─── Compose Tab ──────────────────────────────────────────────────────────────

function ComposeTab({ agentId, onScheduled }: { agentId: string; onScheduled: () => void }) {
  const souls = useStore(s => s.souls);
  const [content, setContent] = useState('');
  // Per-platform content overrides — key: platform id, value: custom content
  // Empty string means "use the main content"
  const [platformContent, setPlatformContent] = useState<Record<string, string>>({});
  const [selectedPlatforms, setSelectedPlatforms] = useState<string[]>(['twitter']);
  const [scheduleAt, setScheduleAt] = useState('');
  const [mediaUrls, setMediaUrls] = useState('');
  const [saving, setSaving] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const [selectedAgent, setSelectedAgent] = useState(agentId || (souls[0]?.id ?? ''));
  const [showPerPlatform, setShowPerPlatform] = useState(false);
  const textRef = useRef<HTMLTextAreaElement>(null);

  // Load connected integrations for relay account display
  const [composeIntegrations, setComposeIntegrations] = useState<any[]>([]);
  useEffect(() => {
    socialApi.listIntegrations(agentId || undefined)
      .then(d => setComposeIntegrations(Array.isArray(d) ? d : []))
      .catch(() => {});
  }, [agentId]);

  // Build per-platform account summary (only for platforms with multiple accounts or relay accounts)
  const platformAccountHints = useCallback((): Record<string, string> => {
    const hints: Record<string, string> = {};
    const byPlatform: Record<string, any[]> = {};
    for (const integ of composeIntegrations) {
      const pid = integ.platform;
      if (!byPlatform[pid]) byPlatform[pid] = [];
      byPlatform[pid].push(integ);
    }
    for (const [pid, accounts] of Object.entries(byPlatform)) {
      if (accounts.length > 1 || accounts.some((a: any) => a.relay_provider && a.relay_provider !== 'direct')) {
        const labels = accounts.map((a: any) => {
          const name = a.nickname || a.account_name || a.account_id || '';
          const relay = a.relay_provider && a.relay_provider !== 'direct'
            ? ` via ${RELAY_LABELS[a.relay_provider] ?? a.relay_provider}`
            : '';
          return name + relay;
        });
        hints[pid] = labels.join(', ');
      }
    }
    return hints;
  }, [composeIntegrations]);

  // Load content set from sessionStorage if user clicked "Use" in SetsTab
  useEffect(() => {
    const raw = sessionStorage.getItem('social_set_load');
    if (raw) {
      try {
        const set = JSON.parse(raw) as { content: string; platforms: string[] };
        if (set.content) setContent(set.content);
        if (set.platforms?.length) setSelectedPlatforms(set.platforms);
      } catch { /* ignore */ }
      sessionStorage.removeItem('social_set_load');
    }
  }, []);

  const activePlatform = PLATFORMS.find(p => selectedPlatforms[0] === p.id) ?? PLATFORMS[0]!;
  const charsLeft = activePlatform.maxChars - content.length;
  const charColor = charsLeft < 0 ? 'text-destructive' : charsLeft < 20 ? 'text-amber-500' : 'text-muted-foreground';

  const togglePlatform = (id: string) => {
    setSelectedPlatforms(prev => prev.includes(id) ? prev.filter(p => p !== id) : [...prev, id]);
  };

  // Build metadata with per-platform content overrides for the backend
  const buildMetadata = () => {
    const meta: Record<string, string> = {};
    for (const [pid, text] of Object.entries(platformContent)) {
      if (text.trim() && text !== content) {
        meta[`content_${pid}`] = text;
      }
    }
    return Object.keys(meta).length > 0 ? meta : undefined;
  };

  const resetForm = () => {
    setContent(''); setScheduleAt(''); setMediaUrls('');
    setPlatformContent({}); setShowPerPlatform(false);
  };

  const saveDraft = async () => {
    if (!content.trim()) { toast.error('Content required'); return; }
    setSaving(true);
    try {
      await socialApi.createPost({
        content, platforms: selectedPlatforms, status: 'draft',
        agent_id: selectedAgent, media_urls: mediaUrls ? mediaUrls.split('\n').filter(Boolean) : [],
        metadata: buildMetadata(),
      });
      toast.success('Draft saved');
      resetForm();
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
        metadata: buildMetadata(),
      });
      toast.success('Post scheduled ✓');
      resetForm();
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
        metadata: buildMetadata(),
      }) as any;
      const result = await socialApi.publishNow(post.id) as any;
      const ok = result?.results?.filter((r: any) => r.success).length ?? 0;
      const fail = result?.results?.filter((r: any) => !r.success).length ?? 0;
      if (fail === 0) toast.success(`Published to ${ok} platform${ok !== 1 ? 's' : ''} ✓`);
      else toast.error(`${ok} succeeded, ${fail} failed`);
      resetForm();
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
            {(() => { const hints = platformAccountHints(); return PLATFORMS.map(p => (
              <button key={p.id} onClick={() => togglePlatform(p.id)}
                title={hints[p.id] ? `Accounts: ${hints[p.id]}` : undefined}
                className={cn(
                  'flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-all cursor-pointer',
                  selectedPlatforms.includes(p.id)
                    ? 'border-primary bg-primary/10 text-primary'
                    : 'border-border text-muted-foreground hover:border-primary/40 hover:text-foreground',
                )}>
                <span>{p.icon}</span> {p.label}
                {hints[p.id] && selectedPlatforms.includes(p.id) && (
                  <span className="text-[10px] text-muted-foreground font-normal max-w-[120px] truncate">({hints[p.id]})</span>
                )}
                {selectedPlatforms.includes(p.id) && <Check className="h-3 w-3" />}
              </button>
            )); })()}
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

        {/* Per-platform content overrides */}
        {selectedPlatforms.length > 1 && (
          <div className="rounded-xl border border-border overflow-hidden">
            <button
              onClick={() => setShowPerPlatform(v => !v)}
              className="w-full flex items-center justify-between px-4 py-2.5 bg-muted/20 hover:bg-muted/40 transition-colors cursor-pointer text-left"
            >
              <span className="text-xs font-medium text-muted-foreground">
                Per-Platform Content
                {Object.values(platformContent).some(v => v.trim()) && (
                  <span className="ml-2 rounded-full bg-primary/20 text-primary px-1.5 py-0.5 text-xs">
                    {Object.values(platformContent).filter(v => v.trim()).length} customised
                  </span>
                )}
              </span>
              <span className="text-xs text-muted-foreground">{showPerPlatform ? '▲' : '▼'}</span>
            </button>
            {showPerPlatform && (
              <div className="divide-y divide-border/50">
                {selectedPlatforms.map(pid => {
                  const platform = PLATFORMS.find(p => p.id === pid)!;
                  const overrideText = platformContent[pid] ?? '';
                  const displayText = overrideText || content;
                  const chars = displayText.length;
                  const left = platform.maxChars - chars;
                  const leftColor = left < 0 ? 'text-destructive' : left < 20 ? 'text-amber-500' : 'text-muted-foreground';
                  return (
                    <div key={pid} className="p-3 space-y-1.5">
                      <div className="flex items-center gap-2">
                        <span className={cn('h-5 w-5 rounded flex items-center justify-center text-xs font-bold shrink-0', platform.color)}>
                          {platform.icon}
                        </span>
                        <span className="text-xs font-medium">{platform.label}</span>
                        <span className={cn('ml-auto text-xs tabular-nums', leftColor)}>{left} chars</span>
                        {overrideText && (
                          <button
                            onClick={() => setPlatformContent(c => { const n = {...c}; delete n[pid]; return n; })}
                            className="text-xs text-muted-foreground hover:text-destructive cursor-pointer"
                          >Reset</button>
                        )}
                      </div>
                      <textarea
                        value={overrideText || content}
                        onChange={e => setPlatformContent(c => ({ ...c, [pid]: e.target.value }))}
                        rows={3}
                        placeholder={`Custom content for ${platform.label} (leave blank to use main content)`}
                        className={cn(
                          'w-full px-3 py-2 text-sm bg-background border rounded-lg resize-none outline-none placeholder:text-muted-foreground/40',
                          overrideText ? 'border-primary/40' : 'border-border',
                        )}
                      />
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}

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
  const [today] = useState(() => new Date());
  const [current, setCurrent] = useState(() => new Date());
  const [entries, setEntries] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedDay, setSelectedDay] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<'month' | 'week'>('month');

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

  // Build date-to-posts map
  const byDate: Record<string, any[]> = {};
  entries.forEach(e => { byDate[e.date] = e.posts || []; });

  const todayKey = today.toISOString().slice(0, 10);

  // Month stats
  const monthPrefix = `${year}-${String(month + 1).padStart(2, '0')}`;
  const allMonthPosts = entries
    .filter(e => e.date?.startsWith(monthPrefix))
    .flatMap(e => e.posts || []);
  const scheduledCount = allMonthPosts.filter((p: any) => p.status === 'scheduled').length;
  const publishedCount = allMonthPosts.filter((p: any) => p.status === 'published').length;
  const todayPosts = byDate[todayKey] ?? [];

  // Month view cells
  const cells: (number | null)[] = [
    ...Array(firstDay).fill(null),
    ...Array.from({ length: daysInMonth }, (_, i) => i + 1),
  ];
  while (cells.length % 7 !== 0) cells.push(null);

  // Week view: get the week containing today or selected day
  const weekAnchor = selectedDay ? new Date(selectedDay + 'T00:00:00') : today;
  const weekStart = new Date(weekAnchor);
  weekStart.setDate(weekStart.getDate() - weekStart.getDay());
  const weekDays = Array.from({ length: 7 }, (_, i) => {
    const d = new Date(weekStart);
    d.setDate(d.getDate() + i);
    return d;
  });

  const selectedPosts = selectedDay ? (byDate[selectedDay] ?? []) : [];

  const statusDot: Record<string, string> = {
    scheduled: 'bg-blue-400', published: 'bg-emerald-400', draft: 'bg-muted-foreground', failed: 'bg-destructive',
  };

  const goToToday = () => {
    setCurrent(new Date());
    setSelectedDay(todayKey);
  };

  const navigate = (dir: -1 | 1) => {
    if (viewMode === 'month') {
      setCurrent(new Date(year, month + dir, 1));
    } else {
      const next = new Date(weekStart);
      next.setDate(next.getDate() + dir * 7);
      setCurrent(next);
    }
  };

  const publishPost = async (id: string) => {
    try {
      const result = await socialApi.publishNow(id) as any;
      const ok = result?.results?.filter((r: any) => r.success).length ?? 0;
      toast.success(`Published to ${ok} platform${ok !== 1 ? 's' : ''}`);
      load();
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Publish failed'); }
  };

  const deletePost = async (id: string) => {
    if (!confirm('Delete this post?')) return;
    try {
      await socialApi.deletePost(id);
      toast.success('Post deleted');
      load();
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Delete failed'); }
  };

  const formatDateKey = (d: Date) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

  return (
    <div className="space-y-4">
      {/* Stats row */}
      <div className="grid grid-cols-3 gap-3">
        <div className="rounded-xl border border-border bg-card px-4 py-3">
          <div className="flex items-center gap-2">
            <Clock className="h-4 w-4 text-blue-400" />
            <span className="text-xs text-muted-foreground">Scheduled</span>
          </div>
          <p className="text-lg font-semibold mt-1">{scheduledCount}</p>
          <p className="text-xs text-muted-foreground">this month</p>
        </div>
        <div className="rounded-xl border border-border bg-card px-4 py-3">
          <div className="flex items-center gap-2">
            <CheckCircle2 className="h-4 w-4 text-emerald-400" />
            <span className="text-xs text-muted-foreground">Published</span>
          </div>
          <p className="text-lg font-semibold mt-1">{publishedCount}</p>
          <p className="text-xs text-muted-foreground">this month</p>
        </div>
        <div className="rounded-xl border border-border bg-card px-4 py-3">
          <div className="flex items-center gap-2">
            <Calendar className="h-4 w-4 text-primary" />
            <span className="text-xs text-muted-foreground">Today</span>
          </div>
          <p className="text-lg font-semibold mt-1">{todayPosts.length}</p>
          <p className="text-xs text-muted-foreground">post{todayPosts.length !== 1 ? 's' : ''}</p>
        </div>
      </div>

      {/* Navigation bar */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <button onClick={() => navigate(-1)}
            className="h-8 w-8 flex items-center justify-center rounded-lg hover:bg-accent cursor-pointer">
            <ChevronLeft className="h-4 w-4" />
          </button>
          <h2 className="text-base font-semibold min-w-[160px] text-center">
            {viewMode === 'month'
              ? `${MONTHS[month]} ${year}`
              : `${weekDays[0]!.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })} – ${weekDays[6]!.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}`
            }
          </h2>
          <button onClick={() => navigate(1)}
            className="h-8 w-8 flex items-center justify-center rounded-lg hover:bg-accent cursor-pointer">
            <ChevronRight className="h-4 w-4" />
          </button>
          <button onClick={goToToday}
            className="ml-2 h-8 px-3 text-xs font-medium rounded-lg border border-border hover:bg-accent cursor-pointer">
            Today
          </button>
        </div>
        <div className="flex items-center gap-1 rounded-lg border border-border p-0.5">
          <button onClick={() => setViewMode('month')}
            className={cn(
              'h-7 px-3 text-xs font-medium rounded-md cursor-pointer transition-colors',
              viewMode === 'month' ? 'bg-primary text-primary-foreground' : 'hover:bg-accent'
            )}>
            Month
          </button>
          <button onClick={() => setViewMode('week')}
            className={cn(
              'h-7 px-3 text-xs font-medium rounded-md cursor-pointer transition-colors',
              viewMode === 'week' ? 'bg-primary text-primary-foreground' : 'hover:bg-accent'
            )}>
            Week
          </button>
        </div>
      </div>

      {loading && (
        <div className="flex justify-center py-3 gap-2 text-muted-foreground text-sm">
          <Loader2 className="h-4 w-4 animate-spin" /> Loading calendar…
        </div>
      )}

      {/* Month View */}
      {viewMode === 'month' && !loading && (
        <div className="flex gap-5">
          <div className="flex-1 min-w-0">
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
                const scheduledDots = dayPosts.filter((p: any) => p.status === 'scheduled').length;
                const publishedDots = dayPosts.filter((p: any) => p.status === 'published').length;
                const failedDots = dayPosts.filter((p: any) => p.status === 'failed').length;
                return (
                  <div key={i} onClick={() => setSelectedDay(isSelected ? null : key)}
                    className={cn(
                      'bg-background min-h-[80px] p-1.5 cursor-pointer hover:bg-accent/30 transition-colors',
                      isSelected && 'ring-2 ring-primary ring-inset bg-primary/5',
                    )}>
                    <div className={cn(
                      'text-xs font-medium w-6 h-6 flex items-center justify-center rounded-full mb-1',
                      isToday ? 'bg-primary text-primary-foreground' : 'text-muted-foreground',
                    )}>
                      {day}
                    </div>
                    {dayPosts.length > 0 && (
                      <div className="space-y-0.5">
                        {/* Status dots row */}
                        <div className="flex items-center gap-0.5 mb-0.5">
                          {scheduledDots > 0 && <span className="h-2 w-2 rounded-full bg-blue-400" title={`${scheduledDots} scheduled`} />}
                          {publishedDots > 0 && <span className="h-2 w-2 rounded-full bg-emerald-400" title={`${publishedDots} published`} />}
                          {failedDots > 0 && <span className="h-2 w-2 rounded-full bg-destructive" title={`${failedDots} failed`} />}
                          {dayPosts.length > 1 && (
                            <span className="text-xs text-muted-foreground ml-0.5">{dayPosts.length}</span>
                          )}
                        </div>
                        {/* Mini preview of first 2 posts */}
                        {dayPosts.slice(0, 2).map((post: any, pi: number) => (
                          <div key={pi} className={cn(
                            'text-[10px] rounded px-1 py-0.5 truncate',
                            post.status === 'scheduled' ? 'bg-blue-500/10 text-blue-500' :
                            post.status === 'published' ? 'bg-emerald-500/10 text-emerald-500' :
                            post.status === 'failed' ? 'bg-destructive/10 text-destructive' :
                            'bg-muted text-muted-foreground'
                          )}>
                            {post.content?.slice(0, 18) || 'Post'}
                          </div>
                        ))}
                        {dayPosts.length > 2 && (
                          <div className="text-[10px] text-muted-foreground pl-1">+{dayPosts.length - 2}</div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>

          {/* Day detail sidebar */}
          <div className="w-80 shrink-0">
            {selectedDay ? (
              <div className="rounded-xl border border-border bg-card overflow-hidden">
                <div className="px-4 py-3 border-b border-border bg-muted/20">
                  <p className="text-sm font-semibold">
                    {new Date(selectedDay + 'T00:00:00').toLocaleDateString('en-US', { weekday: 'long', month: 'long', day: 'numeric' })}
                  </p>
                  <p className="text-xs text-muted-foreground">{selectedPosts.length} post{selectedPosts.length !== 1 ? 's' : ''}</p>
                </div>
                <div className="divide-y divide-border/50 max-h-[500px] overflow-y-auto">
                  {selectedPosts.length === 0 ? (
                    <p className="text-sm text-muted-foreground px-4 py-6 text-center">No posts this day</p>
                  ) : selectedPosts.map((post: any, i: number) => (
                    <div key={i} className="px-4 py-3 group">
                      <div className="flex items-start gap-2">
                        <span className={cn('text-xs px-1.5 py-0.5 rounded font-medium shrink-0', STATUS_COLORS[post.status] ?? STATUS_COLORS.draft)}>
                          {post.status}
                        </span>
                        <p className="text-xs text-foreground/80 flex-1 line-clamp-2">{post.content?.slice(0, 60) || 'Untitled post'}</p>
                      </div>
                      <div className="flex items-center gap-1 mt-1.5 flex-wrap">
                        {(post.platforms || []).map((p: string) => {
                          const plat = PLATFORMS.find(pl => pl.id === p);
                          return (
                            <span key={p} className={cn('text-[10px] px-1.5 py-0.5 rounded font-medium', plat?.color || 'bg-muted text-muted-foreground')}>
                              {plat?.icon || p}
                            </span>
                          );
                        })}
                        {post.scheduled_at && (
                          <span className="text-[10px] text-muted-foreground ml-auto">
                            {new Date(post.scheduled_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                          </span>
                        )}
                      </div>
                      {/* Quick actions */}
                      {post.id && (post.status === 'scheduled' || post.status === 'draft') && (
                        <div className="flex items-center gap-1 mt-2 opacity-0 group-hover:opacity-100 transition-opacity">
                          <button onClick={() => publishPost(post.id)}
                            className="h-6 px-2 text-[10px] font-medium rounded bg-primary/10 text-primary hover:bg-primary/20 cursor-pointer flex items-center gap-1">
                            <Send className="h-3 w-3" /> Publish
                          </button>
                          <button onClick={() => deletePost(post.id)}
                            className="h-6 px-2 text-[10px] font-medium rounded bg-destructive/10 text-destructive hover:bg-destructive/20 cursor-pointer flex items-center gap-1">
                            <Trash2 className="h-3 w-3" /> Delete
                          </button>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            ) : (
              <div className="rounded-xl border border-dashed border-border p-6 text-center">
                <Calendar className="h-8 w-8 mx-auto mb-2 text-muted-foreground/30" />
                <p className="text-sm text-muted-foreground">Click a day to see posts</p>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Week View */}
      {viewMode === 'week' && !loading && (
        <div className="grid grid-cols-7 gap-2">
          {weekDays.map((wd, i) => {
            const key = formatDateKey(wd);
            const dayPosts = byDate[key] ?? [];
            const isToday = key === todayKey;
            const isSelected = key === selectedDay;
            return (
              <div key={i}
                onClick={() => setSelectedDay(isSelected ? null : key)}
                className={cn(
                  'rounded-xl border bg-card p-2 min-h-[300px] cursor-pointer transition-colors',
                  isSelected ? 'border-primary ring-1 ring-primary' : 'border-border hover:border-primary/50',
                )}>
                <div className={cn(
                  'text-center mb-2 pb-2 border-b border-border',
                )}>
                  <div className="text-xs text-muted-foreground">{DAYS_SHORT[i]}</div>
                  <div className={cn(
                    'text-sm font-semibold w-7 h-7 flex items-center justify-center rounded-full mx-auto mt-0.5',
                    isToday ? 'bg-primary text-primary-foreground' : '',
                  )}>
                    {wd.getDate()}
                  </div>
                </div>
                <div className="space-y-1.5">
                  {dayPosts.length === 0 && (
                    <p className="text-[10px] text-muted-foreground/50 text-center mt-4">No posts</p>
                  )}
                  {dayPosts.map((post: any, pi: number) => (
                    <div key={pi} className={cn(
                      'rounded-lg p-2 text-xs group/card',
                      post.status === 'scheduled' ? 'bg-blue-500/10 border border-blue-500/20' :
                      post.status === 'published' ? 'bg-emerald-500/10 border border-emerald-500/20' :
                      post.status === 'failed' ? 'bg-destructive/10 border border-destructive/20' :
                      'bg-muted/50 border border-border'
                    )}>
                      <div className="flex items-center gap-1 mb-1">
                        <span className={cn('h-1.5 w-1.5 rounded-full shrink-0', statusDot[post.status] || statusDot.draft)} />
                        {post.scheduled_at && (
                          <span className="text-[10px] text-muted-foreground">
                            {new Date(post.scheduled_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                          </span>
                        )}
                      </div>
                      <p className="text-[11px] line-clamp-2 text-foreground/80 leading-relaxed">
                        {post.content?.slice(0, 60) || 'Untitled'}
                      </p>
                      <div className="flex items-center gap-0.5 mt-1">
                        {(post.platforms || []).slice(0, 3).map((p: string) => {
                          const plat = PLATFORMS.find(pl => pl.id === p);
                          return (
                            <span key={p} className={cn('text-[9px] px-1 py-0.5 rounded', plat?.color || 'bg-muted text-muted-foreground')}>
                              {plat?.icon || p}
                            </span>
                          );
                        })}
                      </div>
                      {/* Quick actions on hover */}
                      {post.id && (post.status === 'scheduled' || post.status === 'draft') && (
                        <div className="flex items-center gap-1 mt-1.5 opacity-0 group-hover/card:opacity-100 transition-opacity">
                          <button onClick={(e) => { e.stopPropagation(); publishPost(post.id); }}
                            className="h-5 w-5 flex items-center justify-center rounded bg-primary/10 text-primary hover:bg-primary/20 cursor-pointer"
                            title="Publish now">
                            <Send className="h-2.5 w-2.5" />
                          </button>
                          <button onClick={(e) => { e.stopPropagation(); deletePost(post.id); }}
                            className="h-5 w-5 flex items-center justify-center rounded bg-destructive/10 text-destructive hover:bg-destructive/20 cursor-pointer"
                            title="Delete">
                            <Trash2 className="h-2.5 w-2.5" />
                          </button>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ─── Posts List Tab ───────────────────────────────────────────────────────────

function PostsTab({ agentId, status }: { agentId: string; status: string }) {
  const [posts, setPosts] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [expandedComments, setExpandedComments] = useState<string | null>(null);
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
        const showComments = expandedComments === post.id;
        return (
          <div key={post.id} className="rounded-xl border border-border bg-card overflow-hidden">
            <div className="p-4">
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
                  <button
                    onClick={() => setExpandedComments(showComments ? null : post.id)}
                    className={cn(
                      'h-7 w-7 flex items-center justify-center rounded cursor-pointer transition-colors',
                      showComments
                        ? 'bg-primary/10 text-primary'
                        : 'text-muted-foreground hover:text-primary hover:bg-accent',
                    )}
                    title="Comments"
                  >
                    <MessageCircle className="h-3.5 w-3.5" />
                  </button>
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
            {showComments && (
              <div className="border-t border-border">
                <CommentsPanel postId={post.id} />
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

// ─── Comments Panel ───────────────────────────────────────────────────────────

function CommentsPanel({ postId }: { postId: string }) {
  const [comments, setComments] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [newBody, setNewBody] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [replyTo, setReplyTo] = useState<{ id: string; author: string } | null>(null);

  const load = useCallback(async () => {
    try {
      const data = await socialApi.listComments(postId);
      setComments(data ?? []);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [postId]);

  useEffect(() => { load(); }, [load]);

  async function submit() {
    const body = newBody.trim();
    if (!body) return;
    setSubmitting(true);
    try {
      await socialApi.createComment(postId, {
        body,
        parent_id: replyTo?.id ?? undefined,
      });
      setNewBody('');
      setReplyTo(null);
      await load();
    } catch {
      toast.error('Failed to add comment');
    } finally {
      setSubmitting(false);
    }
  }

  async function del(commentId: string) {
    try {
      await socialApi.deleteComment(postId, commentId);
      setComments(prev => {
        const removed = prev.filter(c => c.id !== commentId);
        return removed.map(c => ({
          ...c,
          replies: (c.replies ?? []).filter((r: any) => r.id !== commentId),
        }));
      });
    } catch {
      toast.error('Failed to delete comment');
    }
  }

  async function toggleResolve(commentId: string, resolved: boolean) {
    try {
      await socialApi.resolveComment(postId, commentId, !resolved);
      setComments(prev =>
        prev.map(c =>
          c.id === commentId ? { ...c, resolved: !resolved } : {
            ...c,
            replies: (c.replies ?? []).map((r: any) =>
              r.id === commentId ? { ...r, resolved: !resolved } : r,
            ),
          },
        ),
      );
    } catch {
      toast.error('Failed to update comment');
    }
  }

  function CommentRow({ c, depth = 0 }: { c: any; depth?: number }) {
    const ts = new Date(c.created_at).toLocaleString([], {
      month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
    });
    return (
      <div className={cn('group', depth > 0 && 'ml-6 border-l border-border pl-3')}>
        <div className="flex items-start gap-2 py-2">
          {depth > 0 && <CornerDownRight className="h-3 w-3 mt-1 shrink-0 text-muted-foreground" />}
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 mb-0.5">
              <span className="text-xs font-medium text-foreground">{c.author_name || 'Team member'}</span>
              <span className="text-xs text-muted-foreground">{ts}</span>
              {c.resolved && (
                <span className="text-xs text-emerald-600 font-medium flex items-center gap-0.5">
                  <CheckCheck className="h-3 w-3" /> resolved
                </span>
              )}
            </div>
            <p className={cn('text-sm text-foreground/80 whitespace-pre-wrap break-words', c.resolved && 'line-through text-muted-foreground')}>
              {c.body}
            </p>
          </div>
          <div className="shrink-0 flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
            {depth === 0 && (
              <button
                onClick={() => setReplyTo(replyTo?.id === c.id ? null : { id: c.id, author: c.author_name })}
                className="h-6 px-1.5 text-xs rounded text-muted-foreground hover:text-primary hover:bg-accent transition-colors"
              >
                reply
              </button>
            )}
            <button
              onClick={() => toggleResolve(c.id, c.resolved)}
              className={cn(
                'h-6 w-6 flex items-center justify-center rounded transition-colors',
                c.resolved
                  ? 'text-emerald-600 hover:text-muted-foreground hover:bg-accent'
                  : 'text-muted-foreground hover:text-emerald-600 hover:bg-accent',
              )}
              title={c.resolved ? 'Unresolve' : 'Mark resolved'}
            >
              <CheckCheck className="h-3 w-3" />
            </button>
            <button
              onClick={() => del(c.id)}
              className="h-6 w-6 flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
              title="Delete"
            >
              <Trash2 className="h-3 w-3" />
            </button>
          </div>
        </div>
        {(c.replies ?? []).map((r: any) => (
          <CommentRow key={r.id} c={r} depth={depth + 1} />
        ))}
      </div>
    );
  }

  return (
    <div className="px-4 py-3 space-y-1">
      {loading ? (
        <div className="flex items-center gap-2 text-xs text-muted-foreground py-2">
          <Loader2 className="h-3 w-3 animate-spin" /> Loading comments…
        </div>
      ) : comments.length === 0 ? (
        <p className="text-xs text-muted-foreground py-2">No comments yet. Add one below.</p>
      ) : (
        <div className="divide-y divide-border/50">
          {comments.map(c => <CommentRow key={c.id} c={c} />)}
        </div>
      )}

      {replyTo && (
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground bg-muted/50 rounded px-2 py-1">
          <CornerDownRight className="h-3 w-3 shrink-0" />
          Replying to <span className="font-medium text-foreground">{replyTo.author}</span>
          <button onClick={() => setReplyTo(null)} className="ml-auto text-muted-foreground hover:text-foreground">
            <X className="h-3 w-3" />
          </button>
        </div>
      )}

      <div className="flex items-end gap-2 pt-1">
        <textarea
          value={newBody}
          onChange={e => setNewBody(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submit(); }}
          placeholder={replyTo ? `Reply to ${replyTo.author}…` : 'Add a comment… (⌘↵ to submit)'}
          rows={2}
          className="flex-1 resize-none rounded-lg border border-border bg-background px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-ring placeholder:text-muted-foreground"
        />
        <button
          onClick={submit}
          disabled={!newBody.trim() || submitting}
          className="h-9 px-3 rounded-lg bg-primary text-primary-foreground text-sm font-medium disabled:opacity-50 hover:bg-primary/90 transition-colors flex items-center gap-1.5"
        >
          {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
          Post
        </button>
      </div>
    </div>
  );
}

// ─── OAuth Connect Button ─────────────────────────────────────────────────────

function OAuthConnectButton({
  platform,
  agentId,
  onSuccess,
}: {
  platform: string;
  agentId: string;
  onSuccess: (accountName: string) => void;
}) {
  const [connecting, setConnecting] = useState(false);
  const platformDef = PLATFORMS.find(p => p.id === platform);
  const auth = PLATFORM_AUTH[platform];

  const connect = () => {
    setConnecting(true);
    const params = new URLSearchParams({ agent_id: agentId });
    const startUrl = `/api/v1/social/oauth/${platform}/start?${params}`;

    // Open OAuth popup
    const popup = window.open(startUrl, `oauth_${platform}`,
      'width=600,height=700,scrollbars=yes,resizable=yes');

    const listener = (e: MessageEvent) => {
      if (e.data?.type === 'social_oauth_success' && e.data.platform === platform) {
        window.removeEventListener('message', listener);
        setConnecting(false);
        onSuccess(e.data.account || platform);
      } else if (e.data?.type === 'social_oauth_error' && e.data.platform === platform) {
        window.removeEventListener('message', listener);
        setConnecting(false);
        toast.error(e.data.error || `Failed to connect ${platform}`);
      }
    };
    window.addEventListener('message', listener);

    // Clean up if popup closed without postMessage
    const poll = setInterval(() => {
      if (popup?.closed) {
        clearInterval(poll);
        window.removeEventListener('message', listener);
        setConnecting(false);
      }
    }, 500);
  };

  return (
    <div className="space-y-3">
      <div className="rounded-lg border border-border bg-muted/20 px-4 py-3 text-xs text-muted-foreground leading-relaxed">
        <p className="font-medium text-foreground mb-1">OAuth 2.0 — Secure Connect</p>
        Click the button below to authorise Qorven on {platformDef?.label ?? platform}.
        You will be redirected to {platformDef?.label ?? platform} and back automatically.
        {auth?.docsUrl && (
          <span> <a href={auth.docsUrl} target="_blank" rel="noopener noreferrer"
            className="text-primary hover:underline">{auth.docsLabel}</a></span>
        )}
      </div>
      <button
        onClick={connect}
        disabled={connecting}
        className={cn(
          'flex w-full items-center justify-center gap-2 rounded-lg border px-4 py-3 text-sm font-medium transition-all cursor-pointer',
          connecting
            ? 'border-border text-muted-foreground opacity-60'
            : 'border-primary/30 bg-primary/5 text-primary hover:bg-primary/10',
        )}
      >
        {connecting ? (
          <><Loader2 className="h-4 w-4 animate-spin" /> Connecting…</>
        ) : (
          <>
            <span className={cn('h-5 w-5 rounded flex items-center justify-center text-xs font-bold', platformDef?.color)}>
              {platformDef?.icon}
            </span>
            Connect with {platformDef?.label ?? platform}
          </>
        )}
      </button>
    </div>
  );
}

// ─── Integration Settings Panel ───────────────────────────────────────────────

const ALL_HOURS = Array.from({ length: 24 }, (_, i) => i);
const DAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

function IntegrationSettingsPanel({
  integration,
  onSaved,
}: {
  integration: any;
  onSaved: (updated: any) => void;
}) {
  const [tab, setTab] = useState<'schedule' | 'rules'>('schedule');
  const [form, setForm] = useState({
    nickname:   integration.nickname   ?? '',
    avatar_url: integration.avatar_url ?? '',
    group_name: integration.group_name ?? '',
    post_hours: (integration.post_hours ?? []) as number[],
    post_days:  (integration.post_days  ?? [0,1,2,3,4,5,6]) as number[],
    paused:     integration.paused ?? false,
  });
  const [saving, setSaving] = useState(false);

  // Content rules state
  const [rules, setRules] = useState({ voice_style: '', content_rules: '', knowledge_context: '', posting_guidelines: '', hashtag_sets: '' });
  const [rulesLoading, setRulesLoading] = useState(false);
  const [rulesSaving, setRulesSaving] = useState(false);
  const [rulesLoaded, setRulesLoaded] = useState(false);

  const loadRules = useCallback(() => {
    if (!integration.agent_id || rulesLoaded) return;
    setRulesLoading(true);
    integrationsApi.getAccountRules(integration.id, integration.agent_id)
      .then(r => {
        setRules({
          voice_style: r.voice_style || '',
          content_rules: r.content_rules || '',
          knowledge_context: r.knowledge_context || '',
          posting_guidelines: r.posting_guidelines || '',
          hashtag_sets: r.hashtag_sets?.default ? r.hashtag_sets.default.join(', ') : '',
        });
        setRulesLoaded(true);
      })
      .catch(() => setRulesLoaded(true))
      .finally(() => setRulesLoading(false));
  }, [integration.id, integration.agent_id, rulesLoaded]);

  useEffect(() => { if (tab === 'rules') loadRules(); }, [tab, loadRules]);

  const toggleHour = (h: number) =>
    setForm(f => ({ ...f, post_hours: f.post_hours.includes(h) ? f.post_hours.filter(x => x !== h) : [...f.post_hours, h].sort((a,b)=>a-b) }));

  const toggleDay = (d: number) =>
    setForm(f => ({ ...f, post_days: f.post_days.includes(d) ? f.post_days.filter(x => x !== d) : [...f.post_days, d].sort((a,b)=>a-b) }));

  async function save() {
    setSaving(true);
    try {
      await socialApi.updateIntegrationSettings(integration.id, form);
      onSaved({ ...integration, ...form });
      toast.success('Settings saved');
    } catch {
      toast.error('Failed to save settings');
    } finally {
      setSaving(false);
    }
  }

  async function saveRules() {
    if (!integration.agent_id) { toast.error('No agent assigned to this account'); return; }
    setRulesSaving(true);
    try {
      const hashtagArr = rules.hashtag_sets.split(',').map(s => s.trim()).filter(Boolean);
      await integrationsApi.setAccountRules(integration.id, {
        agent_id: integration.agent_id,
        integration_id: integration.id,
        voice_style: rules.voice_style,
        content_rules: rules.content_rules,
        knowledge_context: rules.knowledge_context,
        posting_guidelines: rules.posting_guidelines,
        hashtag_sets: hashtagArr.length > 0 ? { default: hashtagArr } : {},
      });
      toast.success('Content rules saved');
    } catch {
      toast.error('Failed to save rules');
    } finally {
      setRulesSaving(false);
    }
  }

  const hasAgent = !!integration.agent_id;

  return (
    <div className="border-t border-border bg-muted/20">
      {/* Tabs */}
      {hasAgent && (
        <div className="flex border-b border-border">
          <button onClick={() => setTab('schedule')} className={cn('px-4 py-2 text-xs font-medium transition-colors cursor-pointer', tab === 'schedule' ? 'text-foreground border-b-2 border-primary' : 'text-muted-foreground hover:text-foreground')}>
            Schedule & Display
          </button>
          <button onClick={() => setTab('rules')} className={cn('px-4 py-2 text-xs font-medium transition-colors cursor-pointer', tab === 'rules' ? 'text-foreground border-b-2 border-primary' : 'text-muted-foreground hover:text-foreground')}>
            Content Rules
          </button>
        </div>
      )}

      {tab === 'schedule' && (
        <div className="px-4 py-4 space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-muted-foreground block mb-1">Display name override</label>
              <input value={form.nickname} onChange={e => setForm(f => ({ ...f, nickname: e.target.value }))}
                placeholder={integration.account_name || 'Channel nickname'}
                className="qr-input" />
            </div>
            <div>
              <label className="text-xs text-muted-foreground block mb-1">Channel group</label>
              <input value={form.group_name} onChange={e => setForm(f => ({ ...f, group_name: e.target.value }))}
                placeholder="e.g. Marketing, Product, Personal"
                className="qr-input" />
            </div>
          </div>
          <div>
            <label className="text-xs text-muted-foreground block mb-1">Avatar URL override</label>
            <input value={form.avatar_url} onChange={e => setForm(f => ({ ...f, avatar_url: e.target.value }))}
              placeholder="https://… (optional)"
              className="qr-input" />
          </div>
          <div>
            <label className="text-xs text-muted-foreground block mb-1.5">
              Allowed posting hours <span className="text-muted-foreground/60">(empty = any hour)</span>
            </label>
            <div className="flex flex-wrap gap-1">
              {ALL_HOURS.map(h => (
                <button key={h} onClick={() => toggleHour(h)}
                  className={cn(
                    'w-8 h-6 text-xs rounded border transition-colors cursor-pointer',
                    form.post_hours.includes(h)
                      ? 'bg-primary text-primary-foreground border-primary'
                      : 'border-border text-muted-foreground hover:border-primary/40',
                  )}>
                  {h}
                </button>
              ))}
            </div>
          </div>
          <div>
            <label className="text-xs text-muted-foreground block mb-1.5">Allowed posting days</label>
            <div className="flex gap-1.5">
              {DAY_LABELS.map((label, d) => (
                <button key={d} onClick={() => toggleDay(d)}
                  className={cn(
                    'flex-1 h-7 text-xs rounded border transition-colors cursor-pointer',
                    form.post_days.includes(d)
                      ? 'bg-primary text-primary-foreground border-primary'
                      : 'border-border text-muted-foreground hover:border-primary/40',
                  )}>
                  {label}
                </button>
              ))}
            </div>
          </div>
          <div className="flex items-center justify-between">
            <button
              onClick={() => setForm(f => ({ ...f, paused: !f.paused }))}
              className={cn(
                'flex items-center gap-1.5 text-sm rounded-lg border px-3 py-1.5 transition-colors cursor-pointer',
                form.paused
                  ? 'border-amber-500/40 text-amber-600 bg-amber-500/10 hover:bg-amber-500/20'
                  : 'border-border text-muted-foreground hover:bg-accent',
              )}
            >
              {form.paused ? <><Play className="h-3.5 w-3.5" /> Resume channel</> : <><Pause className="h-3.5 w-3.5" /> Pause channel</>}
            </button>
            <button onClick={save} disabled={saving}
              className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-1.5 text-sm font-medium hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
              {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}
              Save
            </button>
          </div>
        </div>
      )}

      {tab === 'rules' && (
        <div className="px-4 py-4 space-y-4">
          {rulesLoading ? (
            <div className="flex justify-center py-4"><Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /></div>
          ) : (
            <>
              <div>
                <label className="text-xs text-muted-foreground block mb-1">Voice & tone style</label>
                <input value={rules.voice_style} onChange={e => setRules(r => ({ ...r, voice_style: e.target.value }))}
                  placeholder="Professional and witty, Casual Gen-Z, Formal corporate..."
                  className="qr-input" />
              </div>
              <div>
                <label className="text-xs text-muted-foreground block mb-1">Content rules</label>
                <textarea value={rules.content_rules} onChange={e => setRules(r => ({ ...r, content_rules: e.target.value }))}
                  placeholder="Never post about politics. Always include CTA. Keep under 280 chars for X..."
                  rows={3}
                  className="qr-input resize-none" />
              </div>
              <div>
                <label className="text-xs text-muted-foreground block mb-1">Knowledge context</label>
                <textarea value={rules.knowledge_context} onChange={e => setRules(r => ({ ...r, knowledge_context: e.target.value }))}
                  placeholder="SaaS product for logistics. Target audience: supply chain managers..."
                  rows={2}
                  className="qr-input resize-none" />
              </div>
              <div>
                <label className="text-xs text-muted-foreground block mb-1">Posting guidelines</label>
                <input value={rules.posting_guidelines} onChange={e => setRules(r => ({ ...r, posting_guidelines: e.target.value }))}
                  placeholder="Max 2 posts per day, morning and evening"
                  className="qr-input" />
              </div>
              <div>
                <label className="text-xs text-muted-foreground block mb-1">Hashtag sets <span className="text-muted-foreground/60">(comma-separated)</span></label>
                <input value={rules.hashtag_sets} onChange={e => setRules(r => ({ ...r, hashtag_sets: e.target.value }))}
                  placeholder="#logistics, #ai, #saas, #supplychain"
                  className="qr-input" />
              </div>
              <div className="flex justify-end">
                <button onClick={saveRules} disabled={rulesSaving}
                  className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-1.5 text-sm font-medium hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
                  {rulesSaving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}
                  Save Rules
                </button>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}

// ─── Accounts Tab ─────────────────────────────────────────────────────────────

function AccountsTab({ agentId }: { agentId: string }) {
  const souls = useStore(s => s.souls);
  const [integrations, setIntegrations] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAdd, setShowAdd] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState<string | null>(null);
  const [form, setForm] = useState({ platform: 'twitter', account_name: '', account_id: '', access_token: '', agent_id: agentId || '' });
  const [extras, setExtras] = useState<Record<string, string>>({});

  // OAuth app credentials panel (admin)
  const [showOAuthApps, setShowOAuthApps] = useState(false);
  const [oauthApps, setOauthApps] = useState<{ id: string; name: string; has_creds: boolean; pkce: boolean; redirect_uri: string }[]>([]);
  const [oauthAppsLoading, setOauthAppsLoading] = useState(false);
  const [oauthAppEditing, setOauthAppEditing] = useState<string | null>(null);
  const [oauthAppForm, setOauthAppForm] = useState({ client_id: '', client_secret: '' });
  const [oauthAppSaving, setOauthAppSaving] = useState(false);

  // Relay connect flow
  const [showRelayConnect, setShowRelayConnect] = useState(false);
  const [relayKeys, setRelayKeys] = useState<RelayKeyRecord[]>([]);
  const [relayKeysLoading, setRelayKeysLoading] = useState(false);
  const [relayForm, setRelayForm] = useState({ relay_key_id: '', platform: '', agent_id: agentId || '' });
  const [relayConnecting, setRelayConnecting] = useState(false);

  const loadRelayKeys = useCallback(() => {
    setRelayKeysLoading(true);
    integrationsApi.listRelayKeys()
      .then(keys => { setRelayKeys(keys.filter(k => k.status === 'active')); setRelayKeysLoading(false); })
      .catch(() => { setRelayKeys([]); setRelayKeysLoading(false); });
  }, []);

  const startRelayConnect = async () => {
    if (!relayForm.relay_key_id || !relayForm.platform) {
      toast.error('Select a relay key and platform');
      return;
    }
    setRelayConnecting(true);
    try {
      const res = await socialApi.relayConnect(relayForm.relay_key_id, relayForm.platform, relayForm.agent_id);
      const popup = window.open(res.auth_url, 'relay_connect', 'width=600,height=700,scrollbars=yes,resizable=yes');

      const handleMessage = async (event: MessageEvent) => {
        if (event.data?.type !== 'relay_connect_callback') return;
        window.removeEventListener('message', handleMessage);
        const { session_token, relay_key_id, error } = event.data;
        if (error) {
          toast.error(`Relay authorization failed: ${error}`);
          setRelayConnecting(false);
          return;
        }
        if (!session_token) {
          toast.error('No session token received from relay');
          setRelayConnecting(false);
          return;
        }
        try {
          await socialApi.relayConnectFinalize(relay_key_id || relayForm.relay_key_id, session_token, relayForm.agent_id);
          const updated = await socialApi.listIntegrations(agentId || undefined);
          setIntegrations(Array.isArray(updated) ? updated : []);
          toast.success('Account connected via relay');
          setShowRelayConnect(false);
          setRelayForm({ relay_key_id: '', platform: '', agent_id: agentId || '' });
        } catch (e) {
          toast.error(e instanceof Error ? e.message : 'Failed to finalize connection');
        }
        setRelayConnecting(false);
      };
      window.addEventListener('message', handleMessage);

      // Fallback: if popup closes without postMessage (user cancelled)
      const fallback = setInterval(() => {
        if (popup?.closed) {
          clearInterval(fallback);
          setTimeout(() => {
            window.removeEventListener('message', handleMessage);
            setRelayConnecting(false);
          }, 1500);
        }
      }, 1000);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to start relay connect');
      setRelayConnecting(false);
    }
  };

  const load = useCallback(() => {
    setLoading(true);
    socialApi.listIntegrations(agentId || undefined)
      .then(d => { setIntegrations(Array.isArray(d) ? d : []); setLoading(false); })
      .catch(() => setLoading(false));
  }, [agentId]);

  useEffect(() => { load(); }, [load]);

  const loadOAuthApps = useCallback(() => {
    setOauthAppsLoading(true);
    socialApi.oauthAppsGet()
      .then(d => { setOauthApps(Array.isArray(d) ? d : []); setOauthAppsLoading(false); })
      .catch(() => { setOauthApps([]); setOauthAppsLoading(false); });
  }, []);

  const saveOAuthApp = async (platform: string) => {
    setOauthAppSaving(true);
    try {
      await socialApi.oauthAppSet(platform, oauthAppForm.client_id.trim(), oauthAppForm.client_secret.trim());
      toast.success(`${platform} OAuth app credentials saved`);
      setOauthAppEditing(null);
      setOauthAppForm({ client_id: '', client_secret: '' });
      loadOAuthApps();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to save');
    } finally {
      setOauthAppSaving(false);
    }
  };

  const deleteOAuthApp = async (platform: string) => {
    try {
      await socialApi.oauthAppDelete(platform);
      toast.success('Credentials removed');
      loadOAuthApps();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed');
    }
  };

  const save = async () => {
    const auth = PLATFORM_AUTH[form.platform];
    // Build token from custom fields if the platform defines them
    const token = auth?.buildToken ? auth.buildToken(extras) : form.access_token;
    if (!form.account_name) { toast.error('Account name required'); return; }
    if (!token || token === ':') { toast.error('Credentials required'); return; }
    // Validate all required custom fields are filled
    if (auth?.customFields) {
      for (const f of auth.customFields) {
        if (!f.optional && !extras[f.key]?.trim()) {
          toast.error(`${f.label} is required`);
          return;
        }
      }
    }
    try {
      await socialApi.saveIntegration({ ...form, access_token: token });
      toast.success('Account connected');
      setShowAdd(false);
      setForm({ platform: 'twitter', account_name: '', account_id: '', access_token: '', agent_id: agentId || '' });
      setExtras({});
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
            <button onClick={() => { setShowAdd(false); setExtras({}); }} className="text-muted-foreground hover:text-foreground cursor-pointer"><X className="h-4 w-4" /></button>
          </div>
          {(() => {
            const auth = PLATFORM_AUTH[form.platform];
            return (
          <div className="p-4 space-y-4">
            {/* Row 1: Platform + Agent */}
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="text-xs text-muted-foreground mb-1 block">Platform</label>
                <select value={form.platform} onChange={e => { setForm(f => ({ ...f, platform: e.target.value, account_id: '', access_token: '' })); setExtras({}); }}
                  className="qr-select">
                  {PLATFORMS.map(p => <option key={p.id} value={p.id}>{p.label}</option>)}
                </select>
              </div>
              <div>
                <label className="text-xs text-muted-foreground mb-1 block">Assign to Agent</label>
                <select value={form.agent_id} onChange={e => setForm(f => ({ ...f, agent_id: e.target.value }))}
                  className="qr-select">
                  <option value="">No agent</option>
                  {souls.map(s => <option key={s.id} value={s.id}>{s.display_name}</option>)}
                </select>
              </div>
            </div>

            {/* Per-platform warning */}
            {auth?.warning && (
              <div className="rounded-lg bg-amber-500/10 border border-amber-500/20 px-3 py-2 text-xs text-amber-600 dark:text-amber-400">
                ⚠ {auth.warning}
              </div>
            )}

            {/* Row 2: Account name + optional account ID */}
            <div className={cn('grid gap-3', auth?.showAccountId ? 'grid-cols-2' : 'grid-cols-1')}>
              <div>
                <label className="text-xs text-muted-foreground mb-1 block">Account Name / Handle</label>
                <input value={form.account_name} onChange={e => setForm(f => ({ ...f, account_name: e.target.value }))}
                  placeholder={form.platform === 'bluesky' ? 'yourhandle.bsky.social' : form.platform === 'mastodon' ? '@user@instance.social' : '@handle or display name'}
                  className="qr-input" />
              </div>
              {auth?.showAccountId && (
                <div>
                  <label className="text-xs text-muted-foreground mb-1 block">
                    {auth.accountIdLabel ?? 'Account ID'}
                  </label>
                  <input value={form.account_id} onChange={e => setForm(f => ({ ...f, account_id: e.target.value }))}
                    placeholder={auth.accountIdPlaceholder ?? 'Numeric ID'}
                    className="qr-input" />
                </div>
              )}
            </div>

            {/* Credential fields — OAuth button, custom multi-field, or single token input */}
            {OAUTH_PLATFORMS.has(form.platform) ? (
              <OAuthConnectButton
                platform={form.platform}
                agentId={form.agent_id}
                onSuccess={(accountName) => {
                  toast.success(`${accountName} connected via OAuth`);
                  setShowAdd(false);
                  setExtras({});
                  load();
                }}
              />
            ) : auth?.customFields ? (
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-medium text-muted-foreground uppercase tracking-wide">Credentials</span>
                  {auth.docsUrl && (
                    <a href={auth.docsUrl} target="_blank" rel="noopener noreferrer"
                      className="text-xs text-primary hover:underline">
                      {auth.docsLabel}
                    </a>
                  )}
                </div>
                {auth.customFields.map(field => (
                  <div key={field.key}>
                    <label className="text-xs text-muted-foreground mb-1 block">
                      {field.label}{field.optional && <span className="ml-1 text-muted-foreground/50">(optional)</span>}
                    </label>
                    <input
                      type={field.type}
                      value={extras[field.key] ?? field.defaultValue ?? ''}
                      onChange={e => setExtras(x => ({ ...x, [field.key]: e.target.value }))}
                      placeholder={field.placeholder}
                      className={cn('qr-input', field.type === 'password' && 'font-mono')}
                    />
                    {field.hint && (
                      <p className="mt-1 text-xs text-muted-foreground leading-relaxed">{field.hint}</p>
                    )}
                  </div>
                ))}
              </div>
            ) : (
              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="text-xs text-muted-foreground">
                    {auth?.tokenLabel ?? 'Access Token'}
                  </label>
                  {auth?.docsUrl && (
                    <a href={auth.docsUrl} target="_blank" rel="noopener noreferrer"
                      className="text-xs text-primary hover:underline">
                      {auth.docsLabel}
                    </a>
                  )}
                </div>
                <input type="password" value={form.access_token} onChange={e => setForm(f => ({ ...f, access_token: e.target.value }))}
                  placeholder={auth?.tokenPlaceholder ?? 'Paste token here'}
                  className="qr-input font-mono" />
                {auth?.tokenHint && (
                  <p className="mt-1.5 text-xs text-muted-foreground leading-relaxed">
                    {auth.tokenHint}
                  </p>
                )}
              </div>
            )}

            {!OAUTH_PLATFORMS.has(form.platform) && (
              <div className="flex gap-2 pt-1">
                <button onClick={save}
                  className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 cursor-pointer">
                  <Check className="h-3.5 w-3.5" /> Connect
                </button>
                <button onClick={() => { setShowAdd(false); setExtras({}); }}
                  className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer">
                  Cancel
                </button>
              </div>
            )}
          </div>
            );
          })()}
        </div>
      ) : (
        <div className="flex gap-2">
          <button onClick={() => setShowAdd(true)}
            className="flex items-center gap-1.5 rounded-lg border border-dashed border-border px-4 py-3 text-sm text-muted-foreground hover:text-foreground hover:border-primary/40 hover:bg-accent/30 transition-colors cursor-pointer flex-1">
            <Plus className="h-4 w-4" /> Connect Social Account
          </button>
          <button onClick={() => { setShowRelayConnect(true); loadRelayKeys(); }}
            className="flex items-center gap-1.5 rounded-lg border border-dashed border-emerald-500/40 px-4 py-3 text-sm text-emerald-600 dark:text-emerald-400 hover:border-emerald-500 hover:bg-emerald-500/5 transition-colors cursor-pointer">
            <Zap className="h-4 w-4" /> Connect via Relay
          </button>
        </div>
      )}

      {/* Relay connect form */}
      {showRelayConnect && (
        <div className="rounded-xl border border-emerald-500/30 bg-card overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 border-b border-border bg-emerald-500/5">
            <p className="text-sm font-semibold">Connect via Relay Provider</p>
            <button onClick={() => setShowRelayConnect(false)} className="text-muted-foreground hover:text-foreground cursor-pointer"><X className="h-4 w-4" /></button>
          </div>
          <div className="p-4 space-y-4">
            {relayKeysLoading ? (
              <div className="flex justify-center py-4"><Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /></div>
            ) : relayKeys.length === 0 ? (
              <p className="text-sm text-muted-foreground">No active relay keys. Add one in Settings &rarr; Integrations.</p>
            ) : (
              <>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="text-xs text-muted-foreground mb-1 block">Relay Key</label>
                    <select
                      value={relayForm.relay_key_id}
                      onChange={e => setRelayForm(f => ({ ...f, relay_key_id: e.target.value, platform: '' }))}
                      className="qr-select"
                    >
                      <option value="">Choose relay key...</option>
                      {relayKeys.map(k => (
                        <option key={k.id} value={k.id}>{k.label} ({RELAY_LABELS[k.provider] ?? k.provider})</option>
                      ))}
                    </select>
                  </div>
                  <div>
                    <label className="text-xs text-muted-foreground mb-1 block">Platform</label>
                    <select
                      value={relayForm.platform}
                      onChange={e => setRelayForm(f => ({ ...f, platform: e.target.value }))}
                      className="qr-select"
                      disabled={!relayForm.relay_key_id}
                    >
                      <option value="">Choose platform...</option>
                      {(() => {
                        const key = relayKeys.find(k => k.id === relayForm.relay_key_id);
                        const supported = key ? (RELAY_PLATFORM_SUPPORT[key.provider] ?? []) : [];
                        return supported.map(pid => {
                          const p = PLATFORMS.find(x => x.id === pid);
                          return p ? <option key={pid} value={pid}>{p.label}</option> : null;
                        });
                      })()}
                    </select>
                  </div>
                </div>
                <div>
                  <label className="text-xs text-muted-foreground mb-1 block">Assign to Agent</label>
                  <select value={relayForm.agent_id} onChange={e => setRelayForm(f => ({ ...f, agent_id: e.target.value }))}
                    className="qr-select">
                    <option value="">No agent</option>
                    {souls.map(s => <option key={s.id} value={s.id}>{s.display_name}</option>)}
                  </select>
                </div>
                <div className="flex gap-2 pt-1">
                  <button onClick={startRelayConnect} disabled={relayConnecting || !relayForm.relay_key_id || !relayForm.platform}
                    className="flex items-center gap-1.5 rounded-lg bg-emerald-600 text-white px-4 py-2 text-sm font-medium hover:bg-emerald-700 disabled:opacity-50 cursor-pointer">
                    {relayConnecting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Zap className="h-3.5 w-3.5" />}
                    Connect
                  </button>
                  <button onClick={() => setShowRelayConnect(false)}
                    className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer">
                    Cancel
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
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
            const expiry = i.token_expiry ? new Date(i.token_expiry) : null;
            const now = Date.now();
            const msLeft = expiry ? expiry.getTime() - now : null;
            const isExpired = msLeft !== null && msLeft <= 0;
            const isExpiringSoon = msLeft !== null && msLeft > 0 && msLeft < 7 * 24 * 60 * 60 * 1000;
            const showSettings = settingsOpen === i.id;
            return (
              <div key={i.id} className={cn(
                'rounded-xl border bg-card overflow-hidden',
                isExpired ? 'border-destructive/40' : isExpiringSoon ? 'border-amber-500/40' : 'border-border',
                i.paused && 'opacity-60',
              )}>
                <div className="flex items-center gap-3 px-4 py-3">
                  <span className={cn('h-9 w-9 rounded-lg flex items-center justify-center text-sm font-bold shrink-0', platform?.color ?? 'bg-muted')}>
                    {platform?.icon ?? '?'}
                  </span>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-1.5">
                      <p className="text-sm font-medium">{i.nickname || i.account_name || i.account_id}</p>
                      {i.group_name && (
                        <span className="text-xs bg-muted px-1.5 py-0.5 rounded">{i.group_name}</span>
                      )}
                      {i.relay_provider && i.relay_provider !== 'direct' && (
                        <span className={cn(
                          "text-[10px] px-1.5 py-0.5 rounded font-medium",
                          i.relay_provider === 'outstand' && "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400",
                          i.relay_provider === 'postforme' && "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
                          i.relay_provider === 'buffer' && "bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400",
                        )}>
                          {RELAY_LABELS[i.relay_provider] ?? i.relay_provider}
                        </span>
                      )}
                      {i.paused && <span className="text-xs text-muted-foreground">(paused)</span>}
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {platform?.label ?? i.platform}
                      {soul && ` · ${soul.display_name}`}
                      {expiry && (
                        <span className={cn('ml-1', isExpired ? 'text-destructive' : isExpiringSoon ? 'text-amber-500' : '')}>
                          · {isExpired ? 'expired' : `expires ${expiry.toLocaleDateString()}`}
                        </span>
                      )}
                      {(i.post_hours?.length > 0) && ` · hours: ${i.post_hours.join(',')}`}
                    </p>
                  </div>
                  {isExpired ? (
                    <span className="rounded-full px-2 py-0.5 text-xs font-medium bg-destructive/10 text-destructive">Expired</span>
                  ) : isExpiringSoon ? (
                    <span className="rounded-full px-2 py-0.5 text-xs font-medium bg-amber-500/10 text-amber-500">Expiring soon</span>
                  ) : (
                    <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium',
                      i.paused ? 'bg-muted text-muted-foreground' : i.active ? 'bg-emerald-500/10 text-emerald-500' : 'bg-muted text-muted-foreground')}>
                      {i.paused ? 'Paused' : i.active ? 'Active' : 'Inactive'}
                    </span>
                  )}
                  <button
                    onClick={() => setSettingsOpen(showSettings ? null : i.id)}
                    className={cn(
                      'h-7 w-7 flex items-center justify-center rounded transition-colors cursor-pointer',
                      showSettings ? 'bg-primary/10 text-primary' : 'text-muted-foreground hover:text-foreground hover:bg-accent',
                    )}
                    title="Channel settings"
                  >
                    <Settings className="h-3.5 w-3.5" />
                  </button>
                  <button onClick={() => disconnect(i.id)}
                    className="h-7 w-7 flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 cursor-pointer transition-colors">
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
                {showSettings && (
                  <IntegrationSettingsPanel integration={i} onSaved={updated => {
                    setIntegrations(prev => prev.map(x => x.id === updated.id ? { ...x, ...updated } : x));
                    setSettingsOpen(null);
                  }} />
                )}
              </div>
            );
          })}
        </div>
      )}

      {/* OAuth App Credentials (admin) */}
      <div className="rounded-xl border border-border overflow-hidden">
        <button
          className="w-full flex items-center justify-between px-4 py-3 text-sm font-medium hover:bg-accent/30 transition-colors cursor-pointer"
          onClick={() => {
            setShowOAuthApps(v => {
              if (!v) loadOAuthApps();
              return !v;
            });
          }}
        >
          <span className="flex items-center gap-2">
            <Settings className="h-4 w-4 text-muted-foreground" />
            OAuth App Credentials
            <span className="text-xs font-normal text-muted-foreground">(admin — register your app on each platform)</span>
          </span>
          <ChevronRight className={cn('h-4 w-4 text-muted-foreground transition-transform', showOAuthApps && 'rotate-90')} />
        </button>

        {showOAuthApps && (
          <div className="border-t border-border p-4 space-y-3">
            <p className="text-xs text-muted-foreground">
              For platforms that use OAuth 2.0 (Twitter, LinkedIn, Facebook, etc.), you must create an app in the platform&apos;s developer console and register the callback URL. Paste your client credentials here.
            </p>

            {oauthAppsLoading ? (
              <div className="flex justify-center py-4"><Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /></div>
            ) : (
              <div className="space-y-2">
                {PLATFORMS.filter(p => OAUTH_PLATFORMS.has(p.id)).map(platform => {
                  const app = oauthApps.find(a => a.id === platform.id);
                  const isEditing = oauthAppEditing === platform.id;
                  return (
                    <div key={platform.id} className={cn(
                      'rounded-lg border px-3 py-2.5',
                      app?.has_creds ? 'border-emerald-500/30 bg-emerald-500/5' : 'border-border',
                    )}>
                      <div className="flex items-center gap-2">
                        <span className={cn('h-7 w-7 rounded flex items-center justify-center text-xs font-bold shrink-0', platform.color)}>
                          {platform.icon}
                        </span>
                        <div className="flex-1 min-w-0">
                          <p className="text-sm font-medium">{platform.label}</p>
                          {app?.redirect_uri && (
                            <p className="text-xs text-muted-foreground truncate font-mono">{app.redirect_uri}</p>
                          )}
                        </div>
                        <span className={cn('text-xs rounded-full px-2 py-0.5',
                          app?.has_creds ? 'bg-emerald-500/10 text-emerald-500' : 'bg-muted text-muted-foreground')}>
                          {app?.has_creds ? 'Configured' : 'Not set'}
                        </span>
                        <button
                          onClick={() => {
                            if (isEditing) { setOauthAppEditing(null); return; }
                            setOauthAppEditing(platform.id);
                            setOauthAppForm({ client_id: '', client_secret: '' });
                          }}
                          className="text-xs text-primary hover:underline cursor-pointer"
                        >
                          {isEditing ? 'Cancel' : app?.has_creds ? 'Update' : 'Set'}
                        </button>
                        {app?.has_creds && !isEditing && (
                          <button
                            onClick={() => deleteOAuthApp(platform.id)}
                            className="text-xs text-muted-foreground hover:text-destructive cursor-pointer"
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </button>
                        )}
                      </div>

                      {isEditing && (
                        <div className="mt-3 space-y-2">
                          {app?.redirect_uri && (
                            <div>
                              <p className="text-xs text-muted-foreground mb-1">Callback URL to register:</p>
                              <div className="flex items-center gap-2">
                                <code className="flex-1 font-mono text-xs bg-muted border border-border rounded px-2 py-1 text-foreground/80 break-all">
                                  {app.redirect_uri}
                                </code>
                                <button
                                  onClick={() => { navigator.clipboard.writeText(app.redirect_uri); toast.success('Copied'); }}
                                  className="text-xs text-primary hover:underline cursor-pointer shrink-0"
                                >
                                  Copy
                                </button>
                              </div>
                            </div>
                          )}
                          <div className="grid grid-cols-2 gap-2">
                            <div>
                              <label className="text-xs text-muted-foreground mb-1 block">Client ID</label>
                              <input
                                type="text"
                                value={oauthAppForm.client_id}
                                onChange={e => setOauthAppForm(f => ({ ...f, client_id: e.target.value }))}
                                placeholder="Client ID"
                                className="qr-input text-xs font-mono"
                              />
                            </div>
                            <div>
                              <label className="text-xs text-muted-foreground mb-1 block">Client Secret</label>
                              <input
                                type="password"
                                value={oauthAppForm.client_secret}
                                onChange={e => setOauthAppForm(f => ({ ...f, client_secret: e.target.value }))}
                                placeholder="Client Secret"
                                className="qr-input text-xs font-mono"
                              />
                            </div>
                          </div>
                          <button
                            onClick={() => saveOAuthApp(platform.id)}
                            disabled={oauthAppSaving || !oauthAppForm.client_id.trim()}
                            className="inline-flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-3 py-1.5 text-xs font-medium hover:bg-primary/90 disabled:opacity-40 cursor-pointer"
                          >
                            {oauthAppSaving ? <Loader2 className="h-3 w-3 animate-spin" /> : <Check className="h-3 w-3" />}
                            Save
                          </button>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// ─── Media Library Tab ────────────────────────────────────────────────────────

type MediaAsset = {
  id: string;
  agent_id: string;
  name: string;
  original_name: string;
  mime_type: string;
  size: number;
  width?: number;
  height?: number;
  alt_text: string;
  tags: string[];
  url: string;
  created_at: string;
};

function MediaTab({ agentId }: { agentId: string }) {
  const souls = useStore(s => s.souls);
  const [assets, setAssets] = useState<MediaAsset[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [search, setSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState<'all' | 'image' | 'video'>('all');
  const [offset, setOffset] = useState(0);
  const [selectedAgent, setSelectedAgent] = useState(agentId || (souls[0]?.id ?? ''));
  const [selectedAsset, setSelectedAsset] = useState<MediaAsset | null>(null);
  const [dragging, setDragging] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const limit = 48;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await socialApi.listMedia({
        agentId: selectedAgent || undefined,
        q: search || undefined,
        type: typeFilter === 'all' ? undefined : typeFilter,
        limit,
        offset,
      });
      setAssets(data?.assets ?? []);
      setTotal(data?.total ?? 0);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [selectedAgent, search, typeFilter, offset]);

  useEffect(() => { load(); }, [load]);

  // Reset to page 1 when filters change
  useEffect(() => { setOffset(0); }, [selectedAgent, search, typeFilter]);

  const uploadFiles = async (files: FileList | File[]) => {
    const fileArr = Array.from(files);
    if (fileArr.length === 0) return;
    setUploading(true);
    let uploaded = 0;
    for (const file of fileArr) {
      try {
        await socialApi.uploadMedia(file, selectedAgent || souls[0]?.id || '');
        uploaded++;
      } catch (e) {
        toast.error(`Failed to upload ${file.name}: ${e instanceof Error ? e.message : 'unknown error'}`);
      }
    }
    setUploading(false);
    if (uploaded > 0) {
      toast.success(`Uploaded ${uploaded} file${uploaded !== 1 ? 's' : ''}`);
      load();
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    const files = e.dataTransfer.files;
    if (files.length > 0) uploadFiles(files);
  };

  const deleteAsset = async (id: string) => {
    if (!confirm('Delete this media asset?')) return;
    try {
      await socialApi.deleteMedia(id);
      toast.success('Deleted');
      if (selectedAsset?.id === id) setSelectedAsset(null);
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed');
    }
  };

  const copyUrl = (asset: MediaAsset) => {
    const absoluteUrl = window.location.origin + '/api/v1' + asset.url.replace('/api/v1', '');
    navigator.clipboard.writeText(absoluteUrl);
    toast.success('URL copied to clipboard');
  };

  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes}B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
  };

  const isVideo = (mime: string) => mime.startsWith('video/');

  const pages = Math.ceil(total / limit);
  const currentPage = Math.floor(offset / limit) + 1;

  return (
    <div className="space-y-4">
      {/* Toolbar */}
      <div className="flex flex-wrap gap-2 items-center">
        {/* Agent picker */}
        <select
          value={selectedAgent}
          onChange={e => setSelectedAgent(e.target.value)}
          className="qr-select w-44 shrink-0"
        >
          <option value="">All Agents</option>
          {souls.map(s => <option key={s.id} value={s.id}>{s.display_name}</option>)}
        </select>

        {/* Search */}
        <div className="relative flex-1 min-w-[160px]">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder="Search by name or alt text…"
            className="qr-input pl-8 w-full"
          />
        </div>

        {/* Type filter */}
        <div className="flex rounded-lg border border-border overflow-hidden shrink-0">
          {(['all', 'image', 'video'] as const).map(t => (
            <button
              key={t}
              onClick={() => setTypeFilter(t)}
              className={cn(
                'px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer capitalize',
                typeFilter === t
                  ? 'bg-primary text-primary-foreground'
                  : 'hover:bg-accent text-muted-foreground',
              )}
            >
              {t === 'image' ? <span className="flex items-center gap-1"><ImageIcon className="h-3 w-3" /> Images</span>
               : t === 'video' ? <span className="flex items-center gap-1"><Video className="h-3 w-3" /> Videos</span>
               : <span className="flex items-center gap-1"><Grid className="h-3 w-3" /> All</span>}
            </button>
          ))}
        </div>

        {/* Upload button */}
        <button
          onClick={() => fileInputRef.current?.click()}
          disabled={uploading}
          className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 disabled:opacity-50 cursor-pointer shrink-0"
        >
          {uploading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
          Upload
        </button>
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept="image/*,video/*"
          className="hidden"
          onChange={e => { if (e.target.files) uploadFiles(e.target.files); e.target.value = ''; }}
        />
      </div>

      {/* Drop zone + grid */}
      <div
        onDragOver={e => { e.preventDefault(); setDragging(true); }}
        onDragLeave={() => setDragging(false)}
        onDrop={handleDrop}
        className={cn(
          'relative min-h-[300px] rounded-xl border-2 transition-colors',
          dragging ? 'border-primary bg-primary/5 border-dashed' : 'border-border',
        )}
      >
        {dragging && (
          <div className="absolute inset-0 flex items-center justify-center z-10 pointer-events-none">
            <div className="flex flex-col items-center gap-2 text-primary">
              <Upload className="h-10 w-10" />
              <p className="text-sm font-medium">Drop files to upload</p>
            </div>
          </div>
        )}

        {loading ? (
          <div className="flex justify-center items-center min-h-[300px]">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        ) : assets.length === 0 ? (
          <div className="flex flex-col items-center justify-center min-h-[300px] gap-3 text-muted-foreground">
            <ImageIcon className="h-12 w-12 opacity-20" />
            <p className="text-sm">No media yet — upload images or videos to your library</p>
            <button
              onClick={() => fileInputRef.current?.click()}
              className="flex items-center gap-1.5 rounded-lg border border-dashed border-border px-4 py-2 text-sm hover:border-primary/40 hover:bg-accent/30 cursor-pointer"
            >
              <Upload className="h-4 w-4" /> Upload files
            </button>
          </div>
        ) : (
          <div className="p-3 grid grid-cols-3 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-8 gap-2">
            {assets.map(asset => (
              <button
                key={asset.id}
                onClick={() => setSelectedAsset(selectedAsset?.id === asset.id ? null : asset)}
                className={cn(
                  'group relative rounded-lg overflow-hidden border bg-muted aspect-square cursor-pointer transition-all',
                  selectedAsset?.id === asset.id
                    ? 'border-primary ring-2 ring-primary/30'
                    : 'border-border hover:border-primary/40',
                )}
              >
                {isVideo(asset.mime_type) ? (
                  <div className="w-full h-full flex items-center justify-center bg-muted">
                    <Video className="h-6 w-6 text-muted-foreground" />
                  </div>
                ) : (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    src={asset.url}
                    alt={asset.alt_text || asset.original_name}
                    className="w-full h-full object-cover"
                    loading="lazy"
                  />
                )}
                {/* Hover overlay */}
                <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex flex-col items-end justify-start p-1 gap-1">
                  <button
                    onClick={e => { e.stopPropagation(); copyUrl(asset); }}
                    className="h-6 w-6 rounded flex items-center justify-center bg-white/20 hover:bg-white/40 text-white"
                    title="Copy URL"
                  >
                    <Copy className="h-3 w-3" />
                  </button>
                  <button
                    onClick={e => { e.stopPropagation(); deleteAsset(asset.id); }}
                    className="h-6 w-6 rounded flex items-center justify-center bg-white/20 hover:bg-destructive text-white"
                    title="Delete"
                  >
                    <Trash2 className="h-3 w-3" />
                  </button>
                </div>
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Selected asset detail panel */}
      {selectedAsset && (
        <div className="rounded-xl border border-border bg-card overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 border-b border-border bg-muted/20">
            <p className="text-sm font-semibold truncate flex-1 min-w-0 mr-3">{selectedAsset.original_name}</p>
            <button onClick={() => setSelectedAsset(null)} className="text-muted-foreground hover:text-foreground cursor-pointer shrink-0">
              <X className="h-4 w-4" />
            </button>
          </div>
          <div className="p-4 flex gap-4">
            {/* Preview */}
            <div className="w-40 h-40 rounded-lg overflow-hidden border border-border bg-muted shrink-0 flex items-center justify-center">
              {isVideo(selectedAsset.mime_type) ? (
                <Video className="h-10 w-10 text-muted-foreground" />
              ) : (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={selectedAsset.url}
                  alt={selectedAsset.alt_text || selectedAsset.original_name}
                  className="w-full h-full object-contain"
                />
              )}
            </div>
            {/* Meta */}
            <div className="flex-1 space-y-2 text-sm">
              <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
                <span className="text-muted-foreground">Type</span>
                <span className="font-mono">{selectedAsset.mime_type}</span>
                <span className="text-muted-foreground">Size</span>
                <span>{formatSize(selectedAsset.size)}</span>
                {selectedAsset.width && <>
                  <span className="text-muted-foreground">Dimensions</span>
                  <span>{selectedAsset.width} × {selectedAsset.height}</span>
                </>}
                <span className="text-muted-foreground">Uploaded</span>
                <span>{new Date(selectedAsset.created_at).toLocaleDateString()}</span>
              </div>
              {/* URL row */}
              <div className="flex items-center gap-2 mt-2">
                <code className="flex-1 text-xs bg-muted rounded px-2 py-1.5 font-mono truncate">
                  {window.location.origin + '/api/v1' + selectedAsset.url.replace('/api/v1', '')}
                </code>
                <button
                  onClick={() => copyUrl(selectedAsset)}
                  className="shrink-0 flex items-center gap-1 rounded-lg border border-border px-3 py-1.5 text-xs hover:bg-accent cursor-pointer"
                >
                  <Copy className="h-3 w-3" /> Copy URL
                </button>
              </div>
              <div className="flex gap-2 pt-1">
                <button
                  onClick={() => deleteAsset(selectedAsset.id)}
                  className="flex items-center gap-1.5 rounded-lg border border-destructive/30 text-destructive px-3 py-1.5 text-xs hover:bg-destructive/10 cursor-pointer"
                >
                  <Trash2 className="h-3 w-3" /> Delete
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Pagination */}
      {pages > 1 && (
        <div className="flex items-center justify-between pt-2">
          <p className="text-xs text-muted-foreground">{total} assets total</p>
          <div className="flex items-center gap-2">
            <button
              disabled={currentPage === 1}
              onClick={() => setOffset(Math.max(0, offset - limit))}
              className="h-8 w-8 flex items-center justify-center rounded-lg border border-border hover:bg-accent disabled:opacity-40 cursor-pointer"
            >
              <ChevronLeft className="h-4 w-4" />
            </button>
            <span className="text-xs text-muted-foreground">{currentPage} / {pages}</span>
            <button
              disabled={currentPage >= pages}
              onClick={() => setOffset(offset + limit)}
              className="h-8 w-8 flex items-center justify-center rounded-lg border border-border hover:bg-accent disabled:opacity-40 cursor-pointer"
            >
              <ChevronRight className="h-4 w-4" />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

// ─── Analytics Tab ────────────────────────────────────────────────────────────

function AnalyticsTab({ agentId }: { agentId: string }) {
  const souls = useStore(s => s.souls);
  const [data, setData] = useState<{ by_platform: any[]; top_posts: any[]; days: number } | null>(null);
  const [loading, setLoading] = useState(true);
  const [selectedAgent, setSelectedAgent] = useState(agentId || (souls[0]?.id ?? ''));
  const [expandedPostId, setExpandedPostId] = useState<string | null>(null);
  const [postMetrics, setPostMetrics] = useState<any[] | null>(null);
  const [loadingMetrics, setLoadingMetrics] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const d = await socialApi.analyticsSummary(selectedAgent || undefined);
      setData(d);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [selectedAgent]);

  useEffect(() => { load(); }, [load]);

  const loadPostMetrics = async (postId: string) => {
    if (expandedPostId === postId) { setExpandedPostId(null); setPostMetrics(null); return; }
    setExpandedPostId(postId);
    setLoadingMetrics(true);
    try {
      const m = await socialApi.postMetrics(postId);
      setPostMetrics(Array.isArray(m) ? m : []);
    } catch { setPostMetrics([]); }
    finally { setLoadingMetrics(false); }
  };

  const totalEngagement = (data?.by_platform ?? []).reduce(
    (sum, p) => sum + (p.likes || 0) + (p.shares || 0) + (p.comments || 0), 0,
  );
  const totalImpressions = (data?.by_platform ?? []).reduce((sum, p) => sum + (p.impressions || 0), 0);
  const maxImpressions = Math.max(...(data?.by_platform ?? []).map(p => p.impressions || 0), 1);

  const fmtNum = (n: number) => n >= 1000 ? `${(n / 1000).toFixed(1)}K` : String(n);

  return (
    <div className="space-y-5">
      {/* Agent selector */}
      <div className="flex items-center gap-3">
        <select
          value={selectedAgent}
          onChange={e => setSelectedAgent(e.target.value)}
          className="qr-select w-52"
        >
          <option value="">All Agents</option>
          {souls.map(s => <option key={s.id} value={s.id}>{s.display_name}</option>)}
        </select>
        {data && <span className="text-xs text-muted-foreground">Last {data.days} days</span>}
      </div>

      {loading ? (
        <div className="flex justify-center py-12"><Loader2 className="h-6 w-6 animate-spin text-muted-foreground" /></div>
      ) : !data || data.by_platform.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 gap-3 text-muted-foreground">
          <BarChart2 className="h-12 w-12 opacity-20" />
          <p className="text-sm">No analytics data yet</p>
          <p className="text-xs opacity-60">Publish posts to start seeing engagement metrics</p>
        </div>
      ) : (
        <>
          {/* Summary cards */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            {[
              { label: 'Impressions', value: fmtNum(totalImpressions), icon: Eye, color: 'text-blue-500' },
              { label: 'Engagement', value: fmtNum(totalEngagement), icon: TrendingUp, color: 'text-emerald-500' },
              { label: 'Likes', value: fmtNum((data.by_platform ?? []).reduce((s, p) => s + (p.likes || 0), 0)), icon: Heart, color: 'text-pink-500' },
              { label: 'Shares', value: fmtNum((data.by_platform ?? []).reduce((s, p) => s + (p.shares || 0), 0)), icon: Share2, color: 'text-amber-500' },
            ].map(card => (
              <div key={card.label} className="rounded-xl border border-border bg-card p-4 flex items-center gap-3">
                <div className={cn('h-9 w-9 rounded-lg bg-muted flex items-center justify-center shrink-0', card.color)}>
                  <card.icon className="h-4 w-4" />
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">{card.label}</p>
                  <p className="text-xl font-bold tabular-nums">{card.value}</p>
                </div>
              </div>
            ))}
          </div>

          {/* Per-platform breakdown */}
          <div className="rounded-xl border border-border bg-card overflow-hidden">
            <div className="px-4 py-3 border-b border-border bg-muted/20">
              <p className="text-sm font-semibold">By Platform</p>
            </div>
            <div className="divide-y divide-border/50">
              {data.by_platform.map((p: any) => {
                const platform = PLATFORMS.find(pl => pl.id === p.platform);
                const barWidth = maxImpressions > 0 ? (p.impressions / maxImpressions) * 100 : 0;
                return (
                  <div key={p.platform} className="px-4 py-3">
                    <div className="flex items-center gap-3 mb-2">
                      <span className={cn('h-7 w-7 rounded flex items-center justify-center text-xs font-bold shrink-0', platform?.color ?? 'bg-muted')}>
                        {platform?.icon ?? p.platform[0].toUpperCase()}
                      </span>
                      <span className="text-sm font-medium flex-1">{platform?.label ?? p.platform}</span>
                      <span className="text-xs text-muted-foreground">{p.post_count} post{p.post_count !== 1 ? 's' : ''}</span>
                    </div>
                    {/* Impressions bar */}
                    <div className="relative h-1.5 rounded-full bg-muted overflow-hidden mb-2">
                      <div
                        className="absolute inset-y-0 left-0 rounded-full bg-primary/60 transition-all"
                        style={{ width: `${barWidth}%` }}
                      />
                    </div>
                    <div className="flex gap-4 text-xs text-muted-foreground">
                      <span className="flex items-center gap-1"><Eye className="h-3 w-3" />{fmtNum(p.impressions)}</span>
                      <span className="flex items-center gap-1"><Heart className="h-3 w-3" />{fmtNum(p.likes)}</span>
                      <span className="flex items-center gap-1"><Share2 className="h-3 w-3" />{fmtNum(p.shares)}</span>
                      <span className="flex items-center gap-1"><MessageCircle className="h-3 w-3" />{fmtNum(p.comments)}</span>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>

          {/* Top posts */}
          {data.top_posts && data.top_posts.length > 0 && (
            <div className="rounded-xl border border-border bg-card overflow-hidden">
              <div className="px-4 py-3 border-b border-border bg-muted/20">
                <p className="text-sm font-semibold">Top Posts by Engagement</p>
              </div>
              <div className="divide-y divide-border/50">
                {data.top_posts.map((post: any, i: number) => {
                  const platform = PLATFORMS.find(pl => pl.id === post.platform);
                  const isExpanded = expandedPostId === post.post_id;
                  return (
                    <div key={`${post.post_id}-${i}`}>
                      <div className="px-4 py-3 flex items-start gap-3">
                        <span className="text-xs text-muted-foreground w-4 shrink-0 mt-0.5 tabular-nums">{i + 1}</span>
                        <span className={cn('h-5 w-5 rounded flex items-center justify-center text-xs font-bold shrink-0 mt-0.5', platform?.color ?? 'bg-muted')}>
                          {platform?.icon ?? '?'}
                        </span>
                        <div className="flex-1 min-w-0">
                          <p className="text-sm text-foreground/80 line-clamp-2">{post.content}</p>
                          <div className="flex gap-3 mt-1 text-xs text-muted-foreground">
                            <span>{fmtNum(post.impressions)} views</span>
                            <span>{fmtNum(post.likes)} likes</span>
                            <span>{fmtNum(post.shares)} shares</span>
                          </div>
                        </div>
                        <div className="shrink-0 text-right">
                          <p className="text-sm font-semibold tabular-nums">{fmtNum(post.engagement)}</p>
                          <p className="text-xs text-muted-foreground">engagement</p>
                        </div>
                        <button
                          onClick={() => loadPostMetrics(post.post_id)}
                          title="View engagement history"
                          className={cn('h-6 w-6 flex items-center justify-center rounded text-muted-foreground hover:text-foreground hover:bg-accent cursor-pointer transition-colors shrink-0',
                            isExpanded && 'bg-accent text-foreground')}
                        >
                          <TrendingUp className="h-3.5 w-3.5" />
                        </button>
                      </div>
                      {isExpanded && (
                        <div className="px-4 pb-3 bg-muted/20 border-t border-border/50">
                          {loadingMetrics ? (
                            <div className="flex justify-center py-3"><Loader2 className="h-4 w-4 animate-spin text-muted-foreground" /></div>
                          ) : !postMetrics || postMetrics.length === 0 ? (
                            <p className="text-xs text-muted-foreground py-2">No detailed metrics recorded yet.</p>
                          ) : (
                            <div className="pt-2 space-y-1">
                              <p className="text-xs font-medium text-muted-foreground mb-2">Engagement history</p>
                              {postMetrics.map((m: any, mi: number) => (
                                <div key={mi} className="flex items-center gap-2 text-xs">
                                  <span className="text-muted-foreground w-28 shrink-0">
                                    {m.recorded_at ? new Date(m.recorded_at).toLocaleString() : '—'}
                                  </span>
                                  <span className="flex gap-3 text-foreground/70">
                                    <span>{fmtNum(m.impressions ?? 0)} views</span>
                                    <span>{fmtNum(m.likes ?? 0)} likes</span>
                                    <span>{fmtNum(m.shares ?? 0)} shares</span>
                                    <span>{fmtNum(m.comments ?? 0)} comments</span>
                                  </span>
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ─── AutoPost Tab ─────────────────────────────────────────────────────────────

// ─── Content Sets Tab ─────────────────────────────────────────────────────────

function SetsTab({ agentId }: { agentId: string }) {
  const souls = useStore(s => s.souls);
  const [sets, setSets] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [showNew, setShowNew] = useState(false);
  const [form, setForm] = useState({ name: '', description: '', content: '', platforms: [] as string[], agent_id: agentId || '' });

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await socialApi.listSets(agentId || undefined);
      setSets(data ?? []);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [agentId]);

  useEffect(() => { load(); }, [load]);

  async function createSet() {
    if (!form.name.trim()) return;
    try {
      await socialApi.createSet({
        name: form.name,
        description: form.description,
        content: form.content,
        platforms: form.platforms,
        agent_id: form.agent_id || undefined,
      });
      setShowNew(false);
      setForm({ name: '', description: '', content: '', platforms: [], agent_id: agentId || '' });
      await load();
      toast.success('Content set created');
    } catch {
      toast.error('Failed to create set');
    }
  }

  async function saveEdit(id: string, patch: { name?: string; description?: string; content?: string; platforms?: string[] }) {
    try {
      await socialApi.updateSet(id, patch);
      setEditingId(null);
      await load();
    } catch {
      toast.error('Failed to update set');
    }
  }

  async function del(id: string) {
    try {
      await socialApi.deleteSet(id);
      setSets(prev => prev.filter(s => s.id !== id));
    } catch {
      toast.error('Failed to delete set');
    }
  }

  function loadIntoComposer(set: any) {
    // Store selected set in sessionStorage so ComposeTab can pick it up
    sessionStorage.setItem('social_set_load', JSON.stringify({ content: set.content, platforms: set.platforms }));
    toast.success(`"${set.name}" ready — open Compose to use it`);
  }

  function SetForm({
    initial,
    onSave,
    onCancel,
  }: {
    initial: { name: string; description: string; content: string; platforms: string[] };
    onSave: (v: typeof initial) => void;
    onCancel: () => void;
  }) {
    const [v, setV] = useState(initial);
    const togglePlatform = (pid: string) =>
      setV(f => ({ ...f, platforms: f.platforms.includes(pid) ? f.platforms.filter(p => p !== pid) : [...f.platforms, pid] }));

    return (
      <div className="space-y-3 rounded-xl border border-border bg-card p-4">
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="text-xs text-muted-foreground">Set name *</label>
            <input value={v.name} onChange={e => setV(f => ({ ...f, name: e.target.value }))}
              placeholder="e.g. Product launch template"
              className="mt-1 qr-input" />
          </div>
          <div>
            <label className="text-xs text-muted-foreground">Description</label>
            <input value={v.description} onChange={e => setV(f => ({ ...f, description: e.target.value }))}
              placeholder="What is this set for?"
              className="mt-1 qr-input" />
          </div>
        </div>
        <div>
          <label className="text-xs text-muted-foreground">Content template</label>
          <textarea value={v.content} onChange={e => setV(f => ({ ...f, content: e.target.value }))}
            placeholder="Write your template content here… Use {variable} placeholders if needed."
            rows={5}
            className="mt-1 qr-input font-mono resize-none w-full" />
        </div>
        <div>
          <label className="text-xs text-muted-foreground block mb-1.5">Target platforms</label>
          <div className="flex flex-wrap gap-1.5">
            {PLATFORMS.map(p => (
              <button key={p.id} onClick={() => togglePlatform(p.id)}
                className={cn(
                  'text-xs px-2 py-0.5 rounded-full border transition-colors cursor-pointer',
                  v.platforms.includes(p.id)
                    ? 'bg-primary text-primary-foreground border-primary'
                    : 'border-border text-muted-foreground hover:border-primary/40 hover:text-foreground',
                )}>
                {p.label}
              </button>
            ))}
          </div>
        </div>
        <div className="flex gap-2">
          <button onClick={() => onSave(v)}
            className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 cursor-pointer">
            <Check className="h-3.5 w-3.5" /> Save
          </button>
          <button onClick={onCancel}
            className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer">
            Cancel
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          Reusable content templates — save a draft post as a set, then load it later to fill the composer instantly.
        </p>
        {!showNew && (
          <button onClick={() => setShowNew(true)}
            className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-3 py-1.5 text-sm font-medium hover:bg-primary/90 cursor-pointer shrink-0">
            <Plus className="h-3.5 w-3.5" /> New Set
          </button>
        )}
      </div>

      {showNew && (
        <SetForm
          initial={{ name: '', description: '', content: '', platforms: [] }}
          onSave={v => {
            setForm(f => ({ ...f, ...v }));
            createSet();
          }}
          onCancel={() => setShowNew(false)}
        />
      )}

      {loading ? (
        <div className="flex justify-center py-8"><Loader2 className="h-5 w-5 animate-spin text-muted-foreground" /></div>
      ) : sets.length === 0 ? (
        <EmptyState icon={BookOpen} title="No content sets" description="Create your first template to speed up post creation." />
      ) : (
        <div className="space-y-2">
          {sets.map(s => {
            const soul = souls.find(a => a.id === s.agent_id);
            const isEditing = editingId === s.id;
            return (
              <div key={s.id} className="rounded-xl border border-border bg-card overflow-hidden">
                {isEditing ? (
                  <div className="p-4">
                    <SetForm
                      initial={{ name: s.name, description: s.description, content: s.content, platforms: s.platforms ?? [] }}
                      onSave={v => saveEdit(s.id, v)}
                      onCancel={() => setEditingId(null)}
                    />
                  </div>
                ) : (
                  <div className="p-4">
                    <div className="flex items-start gap-3">
                      <div className="h-9 w-9 rounded-lg bg-primary/10 flex items-center justify-center shrink-0">
                        <BookOpen className="h-4 w-4 text-primary" />
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 mb-0.5">
                          <span className="text-sm font-medium">{s.name}</span>
                          {soul && <span className="text-xs text-muted-foreground">{soul.display_name}</span>}
                          <span className="text-xs text-muted-foreground ml-auto">
                            {new Date(s.created_at).toLocaleDateString()}
                          </span>
                        </div>
                        {s.description && <p className="text-xs text-muted-foreground mb-1">{s.description}</p>}
                        <p className="text-sm text-foreground/70 line-clamp-2 font-mono whitespace-pre-wrap">{s.content || <em className="not-italic text-muted-foreground">(empty)</em>}</p>
                        {(s.platforms ?? []).length > 0 && (
                          <div className="flex flex-wrap gap-1 mt-2">
                            {(s.platforms as string[]).map(pid => (
                              <span key={pid} className="text-xs bg-muted px-1.5 py-0.5 rounded font-mono">{pid}</span>
                            ))}
                          </div>
                        )}
                      </div>
                      <div className="flex items-center gap-1 shrink-0">
                        <button onClick={() => loadIntoComposer(s)}
                          className="h-7 px-2 flex items-center gap-1 rounded text-xs text-muted-foreground hover:text-primary hover:bg-accent transition-colors cursor-pointer"
                          title="Load into composer">
                          <Send className="h-3 w-3" /> Use
                        </button>
                        <button onClick={() => setEditingId(s.id)}
                          className="h-7 w-7 flex items-center justify-center rounded text-muted-foreground hover:text-foreground hover:bg-accent transition-colors cursor-pointer"
                          title="Edit">
                          <Edit3 className="h-3.5 w-3.5" />
                        </button>
                        <button onClick={() => del(s.id)}
                          className="h-7 w-7 flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors cursor-pointer"
                          title="Delete">
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ─── Webhooks Tab ─────────────────────────────────────────────────────────────

const WEBHOOK_EVENTS = [
  { id: 'post.published', label: 'Post published' },
  { id: 'post.failed',    label: 'Post failed' },
  { id: 'post.scheduled', label: 'Post scheduled' },
  { id: 'post.deleted',   label: 'Post deleted' },
];

function WebhooksTab({ agentId }: { agentId: string }) {
  const [webhooks, setWebhooks] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAdd, setShowAdd] = useState(false);
  const [testing, setTesting] = useState<string | null>(null);
  const [form, setForm] = useState({ name: '', url: '', secret: '', events: ['post.published', 'post.failed'] });

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await socialApi.listWebhooks(agentId || undefined);
      setWebhooks(data ?? []);
    } catch { /* ignore */ } finally {
      setLoading(false);
    }
  }, [agentId]);

  useEffect(() => { load(); }, [load]);

  async function create() {
    if (!form.url.trim()) { toast.error('URL required'); return; }
    if (!form.url.startsWith('http')) { toast.error('URL must start with http/https'); return; }
    try {
      await socialApi.createWebhook({ ...form, agent_id: agentId || undefined });
      setShowAdd(false);
      setForm({ name: '', url: '', secret: '', events: ['post.published', 'post.failed'] });
      await load();
      toast.success('Webhook created');
    } catch { toast.error('Failed to create webhook'); }
  }

  async function del(id: string) {
    await socialApi.deleteWebhook(id);
    setWebhooks(prev => prev.filter(w => w.id !== id));
  }

  async function toggle(id: string) {
    await socialApi.toggleWebhook(id);
    setWebhooks(prev => prev.map(w => w.id === id ? { ...w, active: !w.active } : w));
  }

  async function test(id: string) {
    setTesting(id);
    try {
      await socialApi.testWebhook(id);
      toast.success('Test ping delivered');
    } catch { toast.error('Delivery failed — check the URL'); }
    finally { setTesting(null); }
  }

  const toggleEvent = (ev: string) =>
    setForm(f => ({ ...f, events: f.events.includes(ev) ? f.events.filter(e => e !== ev) : [...f.events, ev] }));

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          Receive a POST request when social events happen. Compatible with N8N, Make, Zapier, and any HTTP endpoint.
        </p>
        {!showAdd && (
          <button onClick={() => setShowAdd(true)}
            className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-3 py-1.5 text-sm font-medium hover:bg-primary/90 cursor-pointer shrink-0">
            <Plus className="h-3.5 w-3.5" /> Add Webhook
          </button>
        )}
      </div>

      {showAdd && (
        <div className="rounded-xl border border-border bg-card p-4 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-muted-foreground block mb-1">Name (optional)</label>
              <input value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
                placeholder="e.g. N8N workflow trigger"
                className="qr-input" />
            </div>
            <div>
              <label className="text-xs text-muted-foreground block mb-1">Endpoint URL *</label>
              <input value={form.url} onChange={e => setForm(f => ({ ...f, url: e.target.value }))}
                placeholder="https://n8n.example.com/webhook/…"
                className="qr-input font-mono" />
            </div>
          </div>
          <div>
            <label className="text-xs text-muted-foreground block mb-1">
              Signing secret <span className="text-muted-foreground/60">(optional — adds X-Qorven-Signature header)</span>
            </label>
            <input value={form.secret} onChange={e => setForm(f => ({ ...f, secret: e.target.value }))}
              type="password" placeholder="your-webhook-secret"
              className="qr-input font-mono" />
          </div>
          <div>
            <label className="text-xs text-muted-foreground block mb-1.5">Trigger on</label>
            <div className="flex flex-wrap gap-2">
              {WEBHOOK_EVENTS.map(ev => (
                <button key={ev.id} onClick={() => toggleEvent(ev.id)}
                  className={cn(
                    'text-xs px-2.5 py-1 rounded-full border transition-colors cursor-pointer',
                    form.events.includes(ev.id)
                      ? 'bg-primary text-primary-foreground border-primary'
                      : 'border-border text-muted-foreground hover:border-primary/40 hover:text-foreground',
                  )}>
                  {ev.label}
                </button>
              ))}
            </div>
          </div>
          <div className="flex gap-2">
            <button onClick={create}
              className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 cursor-pointer">
              <Webhook className="h-3.5 w-3.5" /> Create
            </button>
            <button onClick={() => setShowAdd(false)}
              className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer">
              Cancel
            </button>
          </div>
        </div>
      )}

      {loading ? (
        <div className="flex justify-center py-8"><Loader2 className="h-5 w-5 animate-spin text-muted-foreground" /></div>
      ) : webhooks.length === 0 ? (
        <EmptyState icon={Webhook} title="No webhooks" description="Add a webhook to push social events to N8N, Make, Zapier, or any HTTP endpoint." />
      ) : (
        <div className="space-y-2">
          {webhooks.map(wh => (
            <div key={wh.id} className={cn(
              'rounded-xl border bg-card px-4 py-3 flex items-center gap-3',
              wh.active ? 'border-border' : 'border-border opacity-60',
            )}>
              <div className={cn('h-2 w-2 rounded-full shrink-0', wh.active ? 'bg-emerald-400' : 'bg-muted-foreground')} />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium">{wh.name || wh.url}</p>
                {wh.name && <p className="text-xs text-muted-foreground font-mono truncate">{wh.url}</p>}
                <div className="flex flex-wrap gap-1 mt-1">
                  {(wh.events ?? []).map((ev: string) => (
                    <span key={ev} className="text-xs bg-muted px-1.5 py-0.5 rounded">{ev}</span>
                  ))}
                </div>
              </div>
              <button onClick={() => test(wh.id)} disabled={testing === wh.id}
                className="flex items-center gap-1 h-7 px-2 rounded text-xs text-muted-foreground hover:text-primary hover:bg-accent transition-colors cursor-pointer disabled:opacity-50"
                title="Send test ping">
                {testing === wh.id ? <Loader2 className="h-3 w-3 animate-spin" /> : <PlayCircle className="h-3.5 w-3.5" />}
                Test
              </button>
              <button onClick={() => toggle(wh.id)}
                className="h-7 w-7 flex items-center justify-center rounded text-muted-foreground hover:text-foreground hover:bg-accent transition-colors cursor-pointer"
                title={wh.active ? 'Pause' : 'Resume'}>
                {wh.active ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
              </button>
              <button onClick={() => del(wh.id)}
                className="h-7 w-7 flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors cursor-pointer">
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
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

  const toggleRule = async (id: string, active: boolean) => {
    try {
      await socialApi.toggleAutoPost(id, active);
      setAutoposts(prev => prev.map(a => a.id === id ? { ...a, active } : a));
    } catch (e) { toast.error(e instanceof Error ? e.message : 'Failed'); }
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
              <button
                onClick={() => toggleRule(a.id, !a.active)}
                title={a.active ? 'Pause rule' : 'Resume rule'}
                className={cn(
                  'h-7 w-7 flex items-center justify-center rounded cursor-pointer transition-colors',
                  a.active
                    ? 'text-amber-500 hover:bg-amber-500/10'
                    : 'text-emerald-500 hover:bg-emerald-500/10',
                )}>
                {a.active ? <Pause className="h-3.5 w-3.5" /> : <Play className="h-3.5 w-3.5" />}
              </button>
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
