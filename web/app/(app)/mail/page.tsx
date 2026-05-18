'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import {
  Mail, Send, RefreshCw, ArrowLeft, Loader2, Star, Reply,
  MoreHorizontal, Search, Plus,
  AlertCircle, Check, Shield, ShieldAlert, X,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { mail as mailApi } from '@/lib/api';
import { useStore } from '@/store';
import { ErrorBoundary } from '@/components/error-boundary';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { toast } from 'sonner';

// ─── Types ────────────────────────────────────────────────────────────────────

type MailMsg = {
  id: string; from: string; to: string[]; subject: string;
  body: string; body_text?: string; status: string;
  created_at: string; received_at?: string;
  read: boolean; starred: boolean; agent_id?: string;
  thread_id?: string; direction?: string;
  // Security fields from buildOutlookContext
  auth_status?: string; // 'verified' | 'known' | 'unknown' | 'fail'
  is_verified_thread?: boolean;
};


// ─── Page ─────────────────────────────────────────────────────────────────────

export default function MailPage() {
  const folder = useStore((s) => s.mailFolder);
  const agentFilter = useStore((s) => s.mailSoulFilter);
  const setFolder = useStore((s) => s.setMailFolder);
  const souls = useStore((s) => s.souls);

  const [messages, setMessages] = useState<MailMsg[]>([]);
  const [selected, setSelected] = useState<MailMsg | null>(null);
  const [composing, setComposing] = useState(false);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');

  const load = useCallback(() => {
    setLoading(true);
    mailApi.folder(folder)
      .then(d => {
        let msgs: MailMsg[] = Array.isArray(d) ? d : [];
        if (agentFilter) msgs = msgs.filter(m => m.agent_id === agentFilter);
        setMessages(msgs);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, [folder, agentFilter]);

  useEffect(() => { load(); setSelected(null); }, [load]);

  const filtered = search
    ? messages.filter(m =>
        m.subject?.toLowerCase().includes(search.toLowerCase()) ||
        m.from?.toLowerCase().includes(search.toLowerCase()) ||
        m.body?.toLowerCase().includes(search.toLowerCase())
      )
    : messages;

  const unread = messages.filter(m => !m.read).length;

  return (
    <ErrorBoundary fallbackTitle="Failed to load mail">
      {/* Full-bleed 2-pane layout: message list + view.
          The shared MailSidebar in sidebar.tsx already renders the folder list — no second sidebar here. */}
      <div className="full-bleed flex h-[calc(100vh-var(--header-height))] overflow-hidden bg-muted/20">

        {/* Pane 1 — Message list */}
        <div className="w-72 xl:w-80 shrink-0 flex flex-col border-r border-border bg-background">
          {/* List header: search + compose + refresh */}
          <div className="flex items-center gap-2 px-3 py-2.5 border-b border-border shrink-0">
            <div className="relative flex-1">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
              <input
                value={search}
                onChange={e => setSearch(e.target.value)}
                placeholder="Search…"
                className="qr-input pl-8 text-xs"
              />
              {search && (
                <button onClick={() => setSearch('')} className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
                  <X className="h-3 w-3" />
                </button>
              )}
            </div>
            <button
              onClick={() => { setComposing(true); setSelected(null); }}
              className="h-7 w-7 flex items-center justify-center rounded-md bg-primary text-primary-foreground hover:bg-primary/90 cursor-pointer shrink-0"
              title="Compose"
            >
              <Plus className="h-3.5 w-3.5" />
            </button>
            <button onClick={load} className="h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground hover:bg-accent cursor-pointer shrink-0" title="Refresh">
              <RefreshCw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
            </button>
          </div>

          <div className="px-3 py-1.5 flex items-center justify-between border-b border-border/50">
            <span className="text-xs font-medium capitalize text-foreground">{folder}</span>
            <span className="text-xs text-muted-foreground">{agentFilter ? souls.find(s => s.id === agentFilter)?.display_name ?? 'Agent' : 'All Agents'}</span>
          </div>

          {/* Message rows */}
          <div className="flex-1 overflow-y-auto">
            {loading ? (
              Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="px-3 py-3 border-b border-border/40 space-y-1.5">
                  <div className="h-3 w-32 animate-pulse rounded bg-muted" />
                  <div className="h-2.5 w-48 animate-pulse rounded bg-muted" />
                  <div className="h-2 w-20 animate-pulse rounded bg-muted" />
                </div>
              ))
            ) : filtered.length === 0 ? (
              <EmptyState {...emptyStates.mail} description={`No messages in ${folder}`} className="py-10" />
            ) : filtered.map(m => (
              <MessageRow
                key={m.id}
                msg={m}
                selected={selected?.id === m.id}
                souls={souls}
                onClick={() => { setSelected(m); setComposing(false); }}
              />
            ))}
          </div>
        </div>

        {/* Pane 2 — Message view or compose */}
        <div className="flex-1 flex flex-col min-w-0 bg-background">
          {composing ? (
            <ComposePane
              souls={souls}
              onClose={() => setComposing(false)}
              onSent={() => { setComposing(false); load(); }}
            />
          ) : selected ? (
            <MessageView
              msg={selected}
              souls={souls}
              onClose={() => setSelected(null)}
              onReply={(to, subj) => setComposing(true)}
              onStarToggle={() => {
                setMessages(prev => prev.map(m => m.id === selected.id ? { ...m, starred: !m.starred } : m));
                setSelected(s => s ? { ...s, starred: !s.starred } : null);
              }}
            />
          ) : (
            <div className="flex-1 flex items-center justify-center">
              <EmptyState
                icon={Mail}
                title="Select a message"
                description="Choose a message from the list to read it, or compose a new one."
              />
            </div>
          )}
        </div>
      </div>
    </ErrorBoundary>
  );
}

// ─── Message Row ──────────────────────────────────────────────────────────────

function MessageRow({ msg, selected, souls, onClick }: {
  msg: MailMsg; selected: boolean; souls: any[]; onClick: () => void;
}) {
  const soul = souls.find(s => s.id === msg.agent_id);
  const date = msg.received_at || msg.created_at;
  const dateStr = date ? formatDate(date) : '';
  const preview = (msg.body_text || msg.body || '').replace(/\n/g, ' ').slice(0, 80);

  return (
    <button
      onClick={onClick}
      className={cn(
        'w-full text-left px-3 py-3 border-b border-border/40 cursor-pointer transition-colors group',
        selected ? 'bg-primary/5 border-l-2 border-l-primary' : 'hover:bg-accent/40',
        !msg.read && 'bg-primary/[0.02]',
      )}
    >
      <div className="flex items-start gap-2">
        {/* Avatar */}
        <div className={cn(
          'flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold mt-0.5',
          msg.direction === 'outbound' ? 'bg-emerald-500/10 text-emerald-500' : 'bg-primary/10 text-primary'
        )}>
          {(msg.from || 'U').charAt(0).toUpperCase()}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between gap-1">
            <span className={cn('text-sm truncate', !msg.read ? 'font-semibold' : 'font-medium')}>
              {msg.from || 'Unknown'}
            </span>
            <span className="text-xs text-muted-foreground shrink-0">{dateStr}</span>
          </div>
          <p className={cn('text-xs truncate mt-0.5', !msg.read ? 'text-foreground/90 font-medium' : 'text-muted-foreground')}>
            {msg.subject || '(no subject)'}
          </p>
          <p className="text-xs text-muted-foreground truncate mt-0.5">{preview}</p>
          {soul && <p className="text-xs text-muted-foreground mt-0.5 truncate">via {soul.display_name}</p>}
        </div>
      </div>
      {/* Unread dot */}
      {!msg.read && (
        <div className="flex justify-end mt-1">
          <span className="h-2 w-2 rounded-full bg-primary inline-block" />
        </div>
      )}
    </button>
  );
}

// ─── Message View ─────────────────────────────────────────────────────────────

function MessageView({ msg, souls, onClose, onReply, onStarToggle }: {
  msg: MailMsg; souls: any[];
  onClose: () => void;
  onReply: (to: string, subject: string) => void;
  onStarToggle: () => void;
}) {
  const [showReply, setShowReply] = useState(false);
  const [replyBody, setReplyBody] = useState('');
  const [sending, setSending] = useState(false);
  const soul = souls.find(s => s.id === msg.agent_id);
  const date = msg.received_at || msg.created_at;

  // Parse security info from the structured body
  const isVerified = !!(msg.body?.includes('✅ DKIM verified') || msg.is_verified_thread);
  const isFailed = !!(msg.body?.includes('🔴 DKIM FAILED'));
  const isKnown = !!(msg.body?.includes('📬 Known sender'));

  // Extract the new message content (strip our security wrapper for display)
  const displayBody = parseDisplayBody(msg.body || msg.body_text || '');

  const sendReply = async () => {
    if (!replyBody.trim()) return;
    setSending(true);
    try {
      await mailApi.send({ to: [msg.from], subject: `Re: ${msg.subject || ''}`, body: replyBody });
      toast.success('Reply sent');
      setShowReply(false);
      setReplyBody('');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to send');
    } finally { setSending(false); }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Top toolbar */}
      <div className="flex items-center gap-2 px-4 py-2.5 border-b border-border shrink-0">
        <button onClick={onClose} className="h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent cursor-pointer">
          <ArrowLeft className="h-4 w-4" />
        </button>
        <div className="flex-1" />
        <button onClick={onStarToggle}
          className={cn('h-7 w-7 flex items-center justify-center rounded-md cursor-pointer transition-colors',
            msg.starred ? 'text-amber-400' : 'text-muted-foreground hover:text-amber-400 hover:bg-accent')}>
          <Star className={cn('h-4 w-4', msg.starred && 'fill-current')} />
        </button>
        <button onClick={() => setShowReply(v => !v)}
          className="flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs font-medium hover:bg-accent cursor-pointer transition-colors">
          <Reply className="h-3.5 w-3.5" /> Reply
        </button>
        <button className="h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent cursor-pointer">
          <MoreHorizontal className="h-4 w-4" />
        </button>
      </div>

      {/* Message content */}
      <div className="flex-1 overflow-y-auto">
        <div className="px-6 py-5 max-w-4xl">
          {/* Subject */}
          <h2 className="text-lg font-semibold leading-tight mb-4">
            {msg.subject || '(no subject)'}
          </h2>

          {/* Security badge */}
          <SecurityBadge isVerified={isVerified} isFailed={isFailed} isKnown={isKnown} />

          {/* Sender info card */}
          <div className="flex items-start gap-3 rounded-xl border border-border bg-card px-4 py-3 mb-5">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary font-semibold">
              {(msg.from || 'U').charAt(0).toUpperCase()}
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-sm font-semibold">{msg.from}</span>
                {soul && <span className="text-xs text-muted-foreground">via {soul.display_name}</span>}
              </div>
              <p className="text-xs text-muted-foreground mt-0.5">
                To: {Array.isArray(msg.to) ? msg.to.join(', ') : msg.to}
              </p>
              <p className="text-xs text-muted-foreground mt-0.5">{date ? new Date(date).toLocaleString() : ''}</p>
            </div>
            {msg.direction === 'outbound' && (
              <span className="rounded-full bg-emerald-500/10 text-emerald-500 px-2 py-0.5 text-xs font-medium shrink-0">Sent</span>
            )}
          </div>

          {/* Message body */}
          <div className="rounded-xl border border-border bg-card p-5">
            <div className="text-sm leading-relaxed whitespace-pre-wrap font-sans">{displayBody}</div>
          </div>
        </div>

        {/* Reply compose box */}
        {showReply && (
          <div className="px-6 pb-6 max-w-4xl">
            <div className="rounded-xl border border-border bg-card overflow-hidden">
              <div className="flex items-center gap-2 px-4 py-2.5 border-b border-border bg-muted/20">
                <Reply className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="text-xs text-muted-foreground">Reply to {msg.from}</span>
              </div>
              <textarea
                value={replyBody}
                onChange={e => setReplyBody(e.target.value)}
                placeholder="Write your reply…"
                rows={6}
                autoFocus
                className="w-full px-4 py-3 text-sm bg-transparent resize-none outline-none"
              />
              <div className="flex items-center justify-between px-4 py-2.5 border-t border-border bg-muted/20">
                <button onClick={() => setShowReply(false)}
                  className="text-xs text-muted-foreground hover:text-foreground cursor-pointer">
                  Cancel
                </button>
                <button onClick={sendReply} disabled={sending || !replyBody.trim()}
                  className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-1.5 text-xs font-medium hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
                  {sending ? <Loader2 className="h-3 w-3 animate-spin" /> : <Send className="h-3 w-3" />}
                  Send Reply
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// ─── Security Badge ───────────────────────────────────────────────────────────

function SecurityBadge({ isVerified, isFailed, isKnown }: {
  isVerified: boolean; isFailed: boolean; isKnown: boolean;
}) {
  if (isFailed) return (
    <div className="flex items-center gap-2 rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 mb-4">
      <ShieldAlert className="h-4 w-4 text-destructive shrink-0" />
      <p className="text-xs text-destructive">DKIM verification failed — sender domain could not be confirmed. Treat with caution.</p>
    </div>
  );
  if (isVerified) return (
    <div className="flex items-center gap-2 rounded-lg border border-emerald-500/30 bg-emerald-500/5 px-3 py-2 mb-4">
      <Shield className="h-4 w-4 text-emerald-500 shrink-0" />
      <p className="text-xs text-emerald-600 dark:text-emerald-400">DKIM verified by mail provider — sender identity confirmed.</p>
    </div>
  );
  if (isKnown) return (
    <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2 mb-4">
      <Check className="h-4 w-4 text-muted-foreground shrink-0" />
      <p className="text-xs text-muted-foreground">Known sender — prior correspondence exists in your mailbox.</p>
    </div>
  );
  return (
    <div className="flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 px-3 py-2 mb-4">
      <AlertCircle className="h-4 w-4 text-amber-500 shrink-0" />
      <p className="text-xs text-amber-600 dark:text-amber-400">Unknown sender — no prior correspondence found. Verify before acting on any requests.</p>
    </div>
  );
}

// ─── Compose Pane ─────────────────────────────────────────────────────────────

function ComposePane({ souls, onClose, onSent }: {
  souls: any[]; onClose: () => void; onSent: () => void;
}) {
  const [to, setTo] = useState('');
  const [subject, setSubject] = useState('');
  const [body, setBody] = useState('');
  const [sending, setSending] = useState(false);

  const send = async () => {
    if (!to.trim() || !body.trim()) { toast.error('To and body are required'); return; }
    setSending(true);
    try {
      await mailApi.send({ to: [to], subject, body });
      toast.success('Message sent');
      onSent();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to send');
    } finally { setSending(false); }
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-2.5 border-b border-border shrink-0">
        <button onClick={onClose} className="h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent cursor-pointer">
          <X className="h-4 w-4" />
        </button>
        <span className="text-sm font-semibold">New Message</span>
      </div>

      {/* Fields */}
      <div className="flex-1 flex flex-col">
        <div className="border-b border-border px-4 py-2.5 flex items-center gap-2">
          <span className="text-xs text-muted-foreground w-12 shrink-0">To</span>
          <input value={to} onChange={e => setTo(e.target.value)}
            placeholder="recipient@example.com"
            className="flex-1 bg-transparent text-sm outline-none" />
        </div>
        <div className="border-b border-border px-4 py-2.5 flex items-center gap-2">
          <span className="text-xs text-muted-foreground w-12 shrink-0">Subject</span>
          <input value={subject} onChange={e => setSubject(e.target.value)}
            placeholder="Subject"
            className="flex-1 bg-transparent text-sm outline-none" />
        </div>
        <textarea
          value={body}
          onChange={e => setBody(e.target.value)}
          placeholder="Write your message…"
          className="flex-1 px-4 py-3 text-sm bg-transparent resize-none outline-none"
        />
      </div>

      {/* Footer */}
      <div className="flex items-center gap-2 px-4 py-3 border-t border-border bg-muted/20 shrink-0">
        <button onClick={send} disabled={sending || !to.trim() || !body.trim()}
          className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
          {sending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
          Send
        </button>
        <button onClick={onClose}
          className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer transition-colors">
          Discard
        </button>
      </div>
    </div>
  );
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function formatDate(dateStr: string): string {
  const d = new Date(dateStr);
  const now = new Date();
  const diffDays = Math.floor((now.getTime() - d.getTime()) / (1000 * 60 * 60 * 24));
  if (diffDays === 0) return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  if (diffDays === 1) return 'Yesterday';
  if (diffDays < 7) return d.toLocaleDateString([], { weekday: 'short' });
  return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
}

function parseDisplayBody(body: string): string {
  // If body contains our security wrapper, extract just the new message content
  const newMsgMatch = body.match(/## New Message\s*\n\n([\s\S]*?)(?:\n\n---\n|$)/);
  if (newMsgMatch) return (newMsgMatch[1] ?? '').trim();
  // If it contains the box header, strip it
  if (body.includes('╔══ INBOUND EMAIL')) {
    const emailBodyMatch = body.match(/--- Email Body ---\n([\s\S]*?)\n--- End Email Body ---/);
    if (emailBodyMatch) return (emailBodyMatch[1] ?? '').trim();
  }
  return body;
}

