'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback, useRef } from 'react';
import {
  Mail, Send, RefreshCw, ArrowLeft, Loader2, Star, Reply,
  MoreHorizontal, Search, Plus, Settings,
  AlertCircle, Check, Shield, ShieldAlert, X,
  Paperclip, ChevronDown, ChevronUp, FileText,
  Archive, Trash2, ReplyAll, Forward, BookOpen,
  CheckSquare, Square, MailOpen, MailCheck,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { mail as mailApi, MailIdentity } from '@/lib/api';
import { RichTextEditor } from '@/components/mail/rich-text-editor';
import { HtmlBody } from '@/components/mail/html-body';
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem,
  DropdownMenuSeparator, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useStore } from '@/store';
import { ErrorBoundary } from '@/components/error-boundary';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { toast } from 'sonner';

// ─── Types ────────────────────────────────────────────────────────────────────

type MailMsg = {
  id: string; from: string; to: string[]; subject: string;
  body: string; body_text?: string; body_html?: string; status: string;
  created_at: string; received_at?: string;
  read: boolean; starred: boolean; agent_id?: string;
  thread_id?: string; direction?: string;
  cc?: string[];
  importance?: 'low' | 'normal' | 'high' | string;
  attachments?: Array<{ id?: string; name: string; content_type?: string; size?: number }>;
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
  const [showAccounts, setShowAccounts] = useState(false);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [searchResults, setSearchResults] = useState<MailMsg[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [bulkActing, setBulkActing] = useState(false);
  const searchDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Folders that are client-side filtered (no dedicated endpoint)
  const CLIENT_FILTERED_FOLDERS = ['starred', 'important'];

  const load = useCallback(() => {
    setLoading(true);
    setSelectedIds(new Set());
    // starred and important are client-side: fetch inbox and filter
    const fetchFolder = CLIENT_FILTERED_FOLDERS.includes(folder) ? 'inbox' : folder;
    mailApi.folder(fetchFolder, agentFilter ?? undefined)
      .then(d => {
        let msgs: MailMsg[] = Array.isArray(d) ? d : [];
        if (agentFilter) msgs = msgs.filter(m => m.agent_id === agentFilter);
        if (folder === 'starred') msgs = msgs.filter(m => m.starred);
        if (folder === 'important') msgs = msgs.filter(m => m.importance === 'high');
        setMessages(msgs);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, [folder, agentFilter]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => { load(); setSelected(null); setSearch(''); setSearchResults(null); }, [load]);

  // Server-side search with 300ms debounce
  useEffect(() => {
    if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current);
    if (!search.trim()) { setSearchResults(null); setSearching(false); return; }
    searchDebounceRef.current = setTimeout(async () => {
      setSearching(true);
      try {
        const results = await mailApi.search(search, agentFilter ?? undefined);
        const raw: any[] = Array.isArray(results) ? results : [];
        // Normalize API MailMessage fields to local MailMsg shape
        const msgs: MailMsg[] = raw.map((m: any) => ({
          ...m,
          from: m.from_address ?? m.from ?? '',
          to: m.to_addresses ?? m.to ?? [],
          cc: m.cc_addresses ?? m.cc ?? [],
          body: m.body_text ?? m.body ?? '',
          read: m.is_read ?? m.read ?? false,
          starred: m.is_starred ?? m.starred ?? false,
        }));
        setSearchResults(msgs);
      } catch {
        setSearchResults([]);
      } finally {
        setSearching(false);
      }
    }, 300);
    return () => { if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current); };
  }, [search, agentFilter]);

  const displayMessages = searchResults ?? messages;
  const unread = messages.filter(m => !m.read).length;

  // Multi-select helpers
  const toggleSelectId = (id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };
  const selectAll = () => setSelectedIds(new Set(displayMessages.map(m => m.id)));
  const clearSelection = () => setSelectedIds(new Set());

  const handleBulkAction = async (action: 'read' | 'star' | 'move' | 'delete', value?: unknown) => {
    if (selectedIds.size === 0) return;
    setBulkActing(true);
    try {
      await mailApi.bulk(Array.from(selectedIds), action, value);
      const label = action === 'delete' ? 'Deleted' : action === 'move' ? `Moved to ${value}` : action === 'read' ? (value ? 'Marked read' : 'Marked unread') : (value ? 'Starred' : 'Unstarred');
      toast.success(label);
      clearSelection();
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Action failed');
    } finally {
      setBulkActing(false);
    }
  };

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
              {(search || searching) && (
                <button onClick={() => { setSearch(''); setSearchResults(null); }} className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
                  {searching ? <Loader2 className="h-3 w-3 animate-spin" /> : <X className="h-3 w-3" />}
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
            <button
              onClick={() => { setShowAccounts(true); setSelected(null); setComposing(false); }}
              className={cn('h-7 w-7 flex items-center justify-center rounded-md cursor-pointer shrink-0', showAccounts ? 'text-primary bg-primary/10' : 'text-muted-foreground hover:bg-accent')}
              title="Email Accounts"
            >
              <Settings className="h-3.5 w-3.5" />
            </button>
          </div>

          {/* Folder + agent + unread info row */}
          <div className="px-3 py-1.5 flex items-center justify-between border-b border-border/50">
            <div className="flex items-center gap-1.5">
              <span className="text-xs font-medium capitalize text-foreground">
                {searchResults !== null ? `Results for "${search}"` : folder}
              </span>
              {searchResults !== null && (
                <span className="text-xs text-muted-foreground">({searchResults.length})</span>
              )}
              {searchResults === null && unread > 0 && (
                <span className="rounded-full bg-primary/15 text-primary text-2xs font-semibold px-1.5 py-0.5 leading-none">{unread}</span>
              )}
            </div>
            <span className="text-xs text-muted-foreground">{agentFilter ? souls.find(s => s.id === agentFilter)?.display_name ?? 'Agent' : 'All Agents'}</span>
          </div>

          {/* Bulk action bar — appears when messages are selected */}
          {selectedIds.size > 0 && (
            <div className="flex items-center gap-1 px-2 py-1.5 border-b border-border bg-primary/5 shrink-0 flex-wrap">
              <span className="text-xs text-primary font-medium mr-1">{selectedIds.size} selected</span>
              <button
                onClick={() => handleBulkAction('read', true)}
                disabled={bulkActing}
                title="Mark read"
                className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground transition-colors disabled:opacity-50"
              >
                <MailOpen className="h-3.5 w-3.5" />
              </button>
              <button
                onClick={() => handleBulkAction('read', false)}
                disabled={bulkActing}
                title="Mark unread"
                className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground transition-colors disabled:opacity-50"
              >
                <MailCheck className="h-3.5 w-3.5" />
              </button>
              <button
                onClick={() => handleBulkAction('star', true)}
                disabled={bulkActing}
                title="Star"
                className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground transition-colors disabled:opacity-50"
              >
                <Star className="h-3.5 w-3.5" />
              </button>
              <button
                onClick={() => handleBulkAction('move', 'archive')}
                disabled={bulkActing}
                title="Archive"
                className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent hover:text-foreground transition-colors disabled:opacity-50"
              >
                <Archive className="h-3.5 w-3.5" />
              </button>
              <button
                onClick={() => handleBulkAction('delete')}
                disabled={bulkActing}
                title="Delete"
                className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-destructive hover:bg-destructive/10 transition-colors disabled:opacity-50"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
              {bulkActing && <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground ml-1" />}
              <div className="flex-1" />
              <button onClick={clearSelection} title="Clear selection" className="h-5 w-5 flex items-center justify-center rounded text-muted-foreground hover:text-foreground hover:bg-accent">
                <X className="h-3 w-3" />
              </button>
            </div>
          )}

          {/* Select-all row */}
          {displayMessages.length > 0 && selectedIds.size === 0 && (
            <div className="flex items-center gap-2 px-3 py-1 border-b border-border/30 shrink-0">
              <button
                onClick={selectAll}
                className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors"
                title="Select all"
              >
                <Square className="h-3.5 w-3.5" />
                <span>Select all</span>
              </button>
            </div>
          )}

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
            ) : displayMessages.length === 0 ? (
              <EmptyState {...emptyStates.mail} description={searchResults !== null ? `No results for "${search}"` : `No messages in ${folder}`} className="py-10" />
            ) : displayMessages.map(m => (
              <MessageRow
                key={m.id}
                msg={m}
                selected={selected?.id === m.id}
                checked={selectedIds.has(m.id)}
                souls={souls}
                onClick={() => { setSelected(m); setComposing(false); }}
                onCheck={(e) => { e.stopPropagation(); toggleSelectId(m.id); }}
                onStarToggle={(e) => {
                  e.stopPropagation();
                  mailApi.setStar(m.id, !m.starred)
                    .then(() => setMessages(prev => prev.map(x => x.id === m.id ? { ...x, starred: !m.starred } : x)))
                    .catch(() => toast.error('Failed to update star'));
                }}
                onQuickArchive={(e) => {
                  e.stopPropagation();
                  mailApi.archive(m.id).then(() => { toast.success('Archived'); load(); }).catch(() => toast.error('Failed'));
                }}
                onQuickTrash={(e) => {
                  e.stopPropagation();
                  mailApi.trash(m.id).then(() => { toast.success('Deleted'); load(); }).catch(() => toast.error('Failed'));
                }}
                onQuickRead={(e) => {
                  e.stopPropagation();
                  mailApi.setRead(m.id, !m.read)
                    .then(() => { setMessages(prev => prev.map(x => x.id === m.id ? { ...x, read: !m.read } : x)); })
                    .catch(() => toast.error('Failed'));
                }}
              />
            ))}
          </div>
        </div>

        {/* Pane 2 — Message view or compose or accounts */}
        <div className="flex-1 flex flex-col min-w-0 bg-background">
          {showAccounts ? (
            <EmailAccountsPanel onClose={() => setShowAccounts(false)} />
          ) : composing ? (
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
              onReply={(prefill) => { setComposing(true); }}
              onStarToggle={() => {
                setMessages(prev => prev.map(m => m.id === selected.id ? { ...m, starred: !m.starred } : m));
                setSelected(s => s ? { ...s, starred: !s.starred } : null);
              }}
              onActionDone={() => { load(); setSelected(null); }}
              onMarkUnread={() => {
                setMessages(prev => prev.map(m => m.id === selected.id ? { ...m, read: false } : m));
                setSelected(s => s ? { ...s, read: false } : null);
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

function MessageRow({ msg, selected, checked, souls, onClick, onCheck, onStarToggle, onQuickArchive, onQuickTrash, onQuickRead }: {
  msg: MailMsg; selected: boolean; checked: boolean; souls: any[];
  onClick: () => void;
  onCheck: (e: React.MouseEvent) => void;
  onStarToggle: (e: React.MouseEvent) => void;
  onQuickArchive: (e: React.MouseEvent) => void;
  onQuickTrash: (e: React.MouseEvent) => void;
  onQuickRead: (e: React.MouseEvent) => void;
}) {
  const soul = souls.find(s => s.id === msg.agent_id);
  const date = msg.received_at || msg.created_at;
  const dateStr = date ? formatDate(date) : '';
  const preview = (msg.body_text || msg.body || '').replace(/\n/g, ' ').slice(0, 80);
  const hasAttachments = msg.attachments && msg.attachments.length > 0;

  return (
    <div
      onClick={onClick}
      className={cn(
        'relative w-full text-left px-3 py-3 border-b border-border/40 cursor-pointer transition-colors group',
        selected ? 'bg-primary/5 border-l-2 border-l-primary' : 'hover:bg-accent/40',
        checked && !selected ? 'bg-primary/[0.04]' : '',
        !msg.read && !selected && !checked ? 'bg-primary/[0.02]' : '',
      )}
    >
      <div className="flex items-start gap-2">
        {/* Checkbox — visible on hover or when checked */}
        <button
          onClick={onCheck}
          className={cn(
            'flex shrink-0 items-center justify-center mt-1 transition-opacity',
            checked ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'
          )}
          title={checked ? 'Deselect' : 'Select'}
        >
          {checked
            ? <CheckSquare className="h-4 w-4 text-primary" />
            : <Square className="h-4 w-4 text-muted-foreground" />}
        </button>
        {/* Avatar — hidden when checkbox is visible */}
        <div className={cn(
          'flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold mt-0.5 transition-opacity',
          checked ? 'opacity-0 absolute' : 'group-hover:opacity-0 group-hover:absolute',
          msg.direction === 'outbound' ? 'bg-emerald-500/10 text-emerald-500' : 'bg-primary/10 text-primary'
        )}>
          {(msg.from || 'U').charAt(0).toUpperCase()}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between gap-1">
            <div className="flex items-center gap-1.5 min-w-0">
              {/* Importance indicator */}
              {msg.importance === 'high' && (
                <span title="High importance">
                  <AlertCircle className="h-3 w-3 shrink-0 text-destructive" />
                </span>
              )}
              <span className={cn('text-sm truncate', !msg.read ? 'font-semibold' : 'font-medium')}>
                {msg.from || 'Unknown'}
              </span>
            </div>
            <div className="flex items-center gap-1 shrink-0">
              {hasAttachments && <span title="Has attachments"><Paperclip className="h-3 w-3 text-muted-foreground" /></span>}
              <span className="text-xs text-muted-foreground">{dateStr}</span>
            </div>
          </div>
          <p className={cn('text-xs truncate mt-0.5', !msg.read ? 'text-foreground/90 font-medium' : 'text-muted-foreground')}>
            {msg.subject || '(no subject)'}
          </p>
          <p className="text-xs text-muted-foreground truncate mt-0.5">{preview}</p>
          {soul && <p className="text-xs text-muted-foreground mt-0.5 truncate">via {soul.display_name}</p>}
        </div>
      </div>

      {/* Row-level indicators: unread dot + star */}
      <div className="flex items-center justify-between mt-1">
        {/* Star toggle */}
        <button
          onClick={onStarToggle}
          className={cn(
            'h-5 w-5 flex items-center justify-center rounded transition-colors',
            msg.starred ? 'text-amber-400' : 'text-transparent group-hover:text-muted-foreground hover:text-amber-400'
          )}
          title={msg.starred ? 'Unstar' : 'Star'}
        >
          <Star className={cn('h-3.5 w-3.5', msg.starred && 'fill-current text-amber-400')} />
        </button>

        {/* Hover quick-actions (Gmail-style) */}
        <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
          <button
            onClick={onQuickRead}
            className="h-5 w-5 flex items-center justify-center rounded text-muted-foreground hover:text-foreground hover:bg-accent"
            title={msg.read ? 'Mark unread' : 'Mark read'}
          >
            {msg.read ? <MailCheck className="h-3.5 w-3.5" /> : <MailOpen className="h-3.5 w-3.5" />}
          </button>
          <button
            onClick={onQuickArchive}
            className="h-5 w-5 flex items-center justify-center rounded text-muted-foreground hover:text-foreground hover:bg-accent"
            title="Archive"
          >
            <Archive className="h-3.5 w-3.5" />
          </button>
          <button
            onClick={onQuickTrash}
            className="h-5 w-5 flex items-center justify-center rounded text-muted-foreground hover:text-destructive hover:bg-destructive/10"
            title="Delete"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>

        {/* Unread dot (right side) — only shown when hover quick-actions aren't visible */}
        {!msg.read && (
          <span className="h-2 w-2 rounded-full bg-primary inline-block group-hover:hidden" />
        )}
      </div>
    </div>
  );
}

// ─── Thread Message Card ──────────────────────────────────────────────────────

/** A single message within a thread — collapsed (sender + snippet) or expanded (full body). */
function ThreadMessageCard({ m, expanded, onToggle, souls }: {
  m: MailMsg;
  expanded: boolean;
  onToggle: () => void;
  souls: any[];
}) {
  const soul = souls.find(s => s.id === m.agent_id);
  const date = m.received_at || m.created_at;
  const isVerified = !!(m.body?.includes('✅ DKIM verified') || m.is_verified_thread);
  const isFailed = !!(m.body?.includes('🔴 DKIM FAILED'));
  const isKnown = !!(m.body?.includes('📬 Known sender'));
  const displayBody = parseDisplayBody(m.body || m.body_text || '');
  const hasHtml = !!(m.body_html?.trim());

  return (
    <div className={cn(
      'rounded-xl border bg-card transition-colors',
      expanded ? 'border-border' : 'border-border/60 hover:border-border cursor-pointer',
    )}>
      {/* Collapsed / header row — always visible */}
      <button
        onClick={onToggle}
        className="w-full flex items-center gap-3 px-4 py-3 text-left"
      >
        <div className={cn(
          'flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-semibold',
          m.direction === 'outbound' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-primary/10 text-primary',
        )}>
          {(m.from || 'U').charAt(0).toUpperCase()}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between gap-2">
            <span className="text-sm font-medium truncate">{m.from || 'Unknown'}</span>
            <span className="text-xs text-muted-foreground shrink-0">
              {date ? new Date(date).toLocaleString([], { dateStyle: 'short', timeStyle: 'short' }) : ''}
            </span>
          </div>
          {!expanded && (
            <p className="text-xs text-muted-foreground truncate mt-0.5">
              {hasHtml ? '(HTML message)' : displayBody.slice(0, 100)}
            </p>
          )}
        </div>
        {soul && !expanded && (
          <span className="text-xs text-muted-foreground shrink-0">via {soul.display_name}</span>
        )}
        <ChevronDown className={cn('h-3.5 w-3.5 text-muted-foreground shrink-0 transition-transform', expanded && 'rotate-180')} />
      </button>

      {/* Expanded body */}
      {expanded && (
        <div className="px-4 pb-4">
          {/* Meta */}
          <div className="flex items-center gap-2 flex-wrap mb-3">
            {soul && <span className="text-xs text-muted-foreground">via {soul.display_name}</span>}
            {m.direction === 'outbound' && (
              <span className="rounded-full bg-emerald-500/10 text-emerald-500 px-2 py-0.5 text-xs font-medium">Sent</span>
            )}
          </div>
          <SecurityBadge isVerified={isVerified} isFailed={isFailed} isKnown={isKnown} />
          {/* Body */}
          <HtmlBody
            html={m.body_html}
            text={displayBody}
            className="text-sm leading-relaxed"
          />
          {/* Attachments for this message */}
          {m.attachments && m.attachments.length > 0 && (
            <AttachmentChips msgId={m.id} attachments={m.attachments} />
          )}
        </div>
      )}
    </div>
  );
}

// ─── Attachment Chips ─────────────────────────────────────────────────────────

function AttachmentChips({ msgId, attachments }: {
  msgId: string;
  attachments: Array<{ id?: string; name: string; content_type?: string; size?: number }>;
}) {
  if (attachments.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-2 mt-3 pt-3 border-t border-border/50">
      {attachments.map((att, i) => {
        const url = mailApi.attachmentUrl(msgId, att.name);
        return (
          <a
            key={i}
            href={url}
            download={att.name}
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-1.5 rounded-lg border border-border bg-muted/30 px-2.5 py-1 text-xs hover:bg-accent transition-colors"
          >
            <Paperclip className="h-3 w-3 text-muted-foreground shrink-0" />
            <span className="max-w-40 truncate">{att.name}</span>
            {att.size != null && (
              <span className="text-muted-foreground shrink-0">
                ({att.size < 1024 ? `${att.size}B` : att.size < 1048576 ? `${Math.round(att.size / 1024)}KB` : `${(att.size / 1048576).toFixed(1)}MB`})
              </span>
            )}
          </a>
        );
      })}
    </div>
  );
}

// ─── Message View ─────────────────────────────────────────────────────────────

type ComposePrefill = {
  to: string;
  cc?: string;
  subject: string;
  quotedBody: string;
  mode: 'reply' | 'reply-all' | 'forward';
};

function MessageView({ msg, souls, onClose, onReply, onStarToggle, onActionDone, onMarkUnread }: {
  msg: MailMsg; souls: any[];
  onClose: () => void;
  onReply: (prefill: ComposePrefill) => void;
  onStarToggle: () => void;
  onActionDone: () => void;
  onMarkUnread: () => void;
}) {
  const [showReply, setShowReply] = useState(false);
  const [replyBody, setReplyBody] = useState('');
  const [sending, setSending] = useState(false);
  const [acting, setActing] = useState<string | null>(null);

  // Thread state
  const [thread, setThread] = useState<MailMsg[]>([]);
  const [threadLoading, setThreadLoading] = useState(false);
  // Set of expanded message IDs — most recent starts expanded
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set([msg.id]));

  const soul = souls.find(s => s.id === msg.agent_id);
  const date = msg.received_at || msg.created_at;

  // Parse security info from the structured body
  const isVerified = !!(msg.body?.includes('✅ DKIM verified') || msg.is_verified_thread);
  const isFailed = !!(msg.body?.includes('🔴 DKIM FAILED'));
  const isKnown = !!(msg.body?.includes('📬 Known sender'));

  // Extract the new message content (strip our security wrapper for display)
  const displayBody = parseDisplayBody(msg.body || msg.body_text || '');

  // Fetch thread when msg.thread_id is present
  useEffect(() => {
    if (!msg.thread_id) {
      setThread([msg]);
      setExpandedIds(new Set([msg.id]));
      return;
    }
    setThreadLoading(true);
    mailApi.thread(msg.thread_id)
      .then((msgs: any[]) => {
        const mapped: MailMsg[] = msgs.map((m: any) => ({
          id: m.id,
          from: m.from_address ?? m.from ?? '',
          to: m.to_addresses ?? m.to ?? [],
          cc: m.cc_addresses ?? m.cc ?? [],
          subject: m.subject ?? '',
          body: m.body_text ?? m.body ?? '',
          body_text: m.body_text,
          body_html: m.body_html,
          status: m.status ?? '',
          created_at: m.created_at,
          received_at: m.received_at,
          read: m.is_read ?? m.read ?? false,
          starred: m.is_starred ?? m.starred ?? false,
          agent_id: m.agent_id,
          thread_id: m.thread_id,
          direction: m.direction,
          attachments: m.attachments,
          auth_status: m.auth_status,
          is_verified_thread: m.is_verified_thread,
        }));
        // Sort chronologically
        mapped.sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime());
        setThread(mapped);
        // Expand only the most recent message by default
        if (mapped.length > 0) {
          setExpandedIds(new Set([mapped[mapped.length - 1]!.id]));
        }
      })
      .catch(() => {
        setThread([msg]);
        setExpandedIds(new Set([msg.id]));
      })
      .finally(() => setThreadLoading(false));
  }, [msg.id, msg.thread_id]); // eslint-disable-line react-hooks/exhaustive-deps

  const toggleExpanded = (id: string) => {
    setExpandedIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) { next.delete(id); } else { next.add(id); }
      return next;
    });
  };

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

  // ── Message actions ──
  const act = async (label: string, fn: () => Promise<void>, closeAfter = false) => {
    setActing(label);
    try {
      await fn();
      toast.success(label);
      if (closeAfter) onActionDone();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : `Failed: ${label}`);
    } finally { setActing(null); }
  };

  const handleArchive = () => act('Archived', () => mailApi.archive(msg.id), true);
  const handleTrash   = () => act('Moved to trash', () => mailApi.trash(msg.id), true);
  const handleMarkUnread = async () => {
    try {
      await mailApi.setRead(msg.id, false);
      onMarkUnread();
      toast.success('Marked as unread');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed');
    }
  };
  const handleStar = async () => {
    try {
      await mailApi.setStar(msg.id, !msg.starred);
      onStarToggle();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed');
    }
  };

  const quotedBodyText = `\n\n---\nOn ${date ? new Date(date).toLocaleString() : 'a prior date'}, ${msg.from} wrote:\n${displayBody}`;

  const isThreadView = thread.length > 1;

  return (
    <div className="flex flex-col h-full">
      {/* Top toolbar */}
      <div className="flex items-center gap-2 px-4 py-2.5 border-b border-border shrink-0">
        <button onClick={onClose} className="h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent cursor-pointer">
          <ArrowLeft className="h-4 w-4" />
        </button>
        <div className="flex-1" />
        {acting && <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />}
        <button onClick={handleStar}
          className={cn('h-7 w-7 flex items-center justify-center rounded-md cursor-pointer transition-colors',
            msg.starred ? 'text-amber-400' : 'text-muted-foreground hover:text-amber-400 hover:bg-accent')}>
          <Star className={cn('h-4 w-4', msg.starred && 'fill-current')} />
        </button>
        <button onClick={() => setShowReply(v => !v)}
          className="flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs font-medium hover:bg-accent cursor-pointer transition-colors">
          <Reply className="h-3.5 w-3.5" /> Reply
        </button>

        {/* MoreHorizontal — fully wired actions */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button className="h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent cursor-pointer">
              <MoreHorizontal className="h-4 w-4" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            <DropdownMenuItem onClick={() => setShowReply(true)}>
              <Reply className="h-4 w-4" /> Reply
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => {
              setShowReply(false);
              onReply({ to: msg.from, cc: (msg.cc ?? []).join(', '), subject: `Re: ${msg.subject || ''}`, quotedBody: quotedBodyText, mode: 'reply-all' });
            }}>
              <ReplyAll className="h-4 w-4" /> Reply All
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => {
              setShowReply(false);
              onReply({ to: '', subject: `Fwd: ${msg.subject || ''}`, quotedBody: quotedBodyText, mode: 'forward' });
            }}>
              <Forward className="h-4 w-4" /> Forward
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={handleMarkUnread}>
              <BookOpen className="h-4 w-4" /> Mark as unread
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handleStar}>
              <Star className="h-4 w-4" />
              {msg.starred ? 'Unstar' : 'Star'}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={handleArchive}>
              <Archive className="h-4 w-4" /> Archive
            </DropdownMenuItem>
            <DropdownMenuItem variant="destructive" onClick={handleTrash}>
              <Trash2 className="h-4 w-4" /> Move to trash
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* Message content */}
      <div className="flex-1 overflow-y-auto">
        <div className="px-6 py-5 max-w-4xl">
          {/* Subject + thread count */}
          <div className="flex items-start justify-between gap-3 mb-4">
            <h2 className="text-lg font-semibold leading-tight">
              {msg.subject || '(no subject)'}
            </h2>
            {isThreadView && (
              <span className="shrink-0 rounded-full bg-muted px-2.5 py-0.5 text-xs text-muted-foreground font-medium">
                {thread.length} messages
              </span>
            )}
          </div>

          {threadLoading && (
            <div className="flex items-center gap-2 text-xs text-muted-foreground mb-4">
              <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading thread…
            </div>
          )}

          {/* ── Thread conversation view ── */}
          {isThreadView ? (
            <div className="space-y-2">
              {thread.map(m => (
                <ThreadMessageCard
                  key={m.id}
                  m={m}
                  expanded={expandedIds.has(m.id)}
                  onToggle={() => toggleExpanded(m.id)}
                  souls={souls}
                />
              ))}
            </div>
          ) : (
            /* ── Single message view ── */
            <>
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
                  {msg.cc && msg.cc.length > 0 && (
                    <p className="text-xs text-muted-foreground mt-0.5">
                      Cc: {msg.cc.join(', ')}
                    </p>
                  )}
                  <p className="text-xs text-muted-foreground mt-0.5">{date ? new Date(date).toLocaleString() : ''}</p>
                </div>
                {msg.direction === 'outbound' && (
                  <span className="rounded-full bg-emerald-500/10 text-emerald-500 px-2 py-0.5 text-xs font-medium shrink-0">Sent</span>
                )}
              </div>

              {/* Message body — HTML or plain text */}
              <div className="rounded-xl border border-border bg-card p-5">
                <HtmlBody
                  html={msg.body_html}
                  text={displayBody}
                  className="text-sm leading-relaxed"
                />
              </div>

              {/* Attachments */}
              {msg.attachments && msg.attachments.length > 0 && (
                <AttachmentChips msgId={msg.id} attachments={msg.attachments} />
              )}
            </>
          )}
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

type ComposeAttachment = {
  name: string;
  content_type: string;
  data: string; // base64
};

function ComposePane({ souls, onClose, onSent }: {
  souls: any[]; onClose: () => void; onSent: () => void;
}) {
  // Basic fields
  const [to, setTo] = useState('');
  const [subject, setSubject] = useState('');
  const [bodyHtml, setBodyHtml] = useState('');
  const [bodyText, setBodyText] = useState('');

  // Extended fields
  const [cc, setCc] = useState('');
  const [bcc, setBcc] = useState('');
  const [showCcBcc, setShowCcBcc] = useState(false);
  const [importance, setImportance] = useState<'normal' | 'high' | 'low'>('normal');
  const [identityId, setIdentityId] = useState('');
  const [identities, setIdentities] = useState<MailIdentity[]>([]);
  const [attachments, setAttachments] = useState<ComposeAttachment[]>([]);
  const [draftId, setDraftId] = useState<string | null>(null);

  // UI state
  const [sending, setSending] = useState(false);
  const [savingDraft, setSavingDraft] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Load identities on mount
  useEffect(() => {
    mailApi.identities()
      .then(data => {
        const list = Array.isArray(data) ? data : [];
        setIdentities(list);
        const active = list.find(i => i.is_active) ?? list[0];
        if (active) setIdentityId(active.id);
      })
      .catch(() => {/* non-fatal */});
  }, []);

  const buildPayload = () => ({
    to: to.split(',').map(s => s.trim()).filter(Boolean),
    cc: cc ? cc.split(',').map(s => s.trim()).filter(Boolean) : undefined,
    bcc: bcc ? bcc.split(',').map(s => s.trim()).filter(Boolean) : undefined,
    subject,
    body_html: bodyHtml,
    body_text: bodyText,
    importance: importance !== 'normal' ? importance : undefined,
    identity_id: identityId || undefined,
    attachments: attachments.length > 0 ? attachments : undefined,
  });

  const send = async () => {
    if (!to.trim() || !bodyText.trim()) { toast.error('To and body are required'); return; }
    setSending(true);
    try {
      await mailApi.send(buildPayload());
      toast.success('Message sent');
      onSent();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to send');
    } finally { setSending(false); }
  };

  const saveDraft = async () => {
    setSavingDraft(true);
    try {
      const payload: Record<string, unknown> = {
        to: to.split(',').map(s => s.trim()).filter(Boolean),
        cc: cc ? cc.split(',').map(s => s.trim()).filter(Boolean) : undefined,
        bcc: bcc ? bcc.split(',').map(s => s.trim()).filter(Boolean) : undefined,
        subject,
        body_html: bodyHtml,
        body_text: bodyText,
        importance,
        identity_id: identityId || undefined,
        attachments: attachments.length > 0 ? attachments : undefined,
      };
      if (draftId) {
        await mailApi.draftUpdate(draftId, payload);
      } else {
        const draft = await mailApi.draftSave(payload);
        if (draft?.id) setDraftId(draft.id);
      }
      toast.success('Draft saved');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to save draft');
    } finally { setSavingDraft(false); }
  };

  const handleFileAttach = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? []);
    files.forEach(file => {
      const reader = new FileReader();
      reader.onload = () => {
        const result = reader.result as string;
        // result is "data:<mime>;base64,<data>" — strip the prefix
        const base64 = result.split(',')[1] ?? '';
        setAttachments(prev => [
          ...prev,
          { name: file.name, content_type: file.type || 'application/octet-stream', data: base64 },
        ]);
      };
      reader.readAsDataURL(file);
    });
    // Reset input so the same file can be re-added if removed
    e.target.value = '';
  };

  const removeAttachment = (idx: number) =>
    setAttachments(prev => prev.filter((_, i) => i !== idx));

  const selectedIdentity = identities.find(i => i.id === identityId);

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-2.5 border-b border-border shrink-0">
        <button onClick={onClose} className="h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent cursor-pointer">
          <X className="h-4 w-4" />
        </button>
        <span className="text-sm font-semibold flex-1">New Message</span>
        {/* Importance selector */}
        <select
          value={importance}
          onChange={e => setImportance(e.target.value as 'normal' | 'high' | 'low')}
          className={cn(
            'text-xs rounded-md border border-border bg-background px-2 py-1 outline-none cursor-pointer',
            importance === 'high' && 'text-destructive border-destructive/40',
            importance === 'low' && 'text-muted-foreground',
          )}
          title="Importance"
        >
          <option value="high">! High</option>
          <option value="normal">Normal</option>
          <option value="low">Low</option>
        </select>
      </div>

      {/* Fields */}
      <div className="flex-1 flex flex-col overflow-y-auto">

        {/* From / Identity */}
        {identities.length > 0 && (
          <div className="border-b border-border px-4 py-2 flex items-center gap-2">
            <span className="text-xs text-muted-foreground w-12 shrink-0">From</span>
            <select
              value={identityId}
              onChange={e => setIdentityId(e.target.value)}
              className="flex-1 bg-transparent text-sm outline-none cursor-pointer"
            >
              {identities.map(id => (
                <option key={id.id} value={id.id}>
                  {id.display_name ? `${id.display_name} <${id.address}>` : id.address}
                </option>
              ))}
            </select>
          </div>
        )}

        {/* To + Cc/Bcc toggle */}
        <div className="border-b border-border px-4 py-2.5 flex items-center gap-2">
          <span className="text-xs text-muted-foreground w-12 shrink-0">To</span>
          <input
            value={to}
            onChange={e => setTo(e.target.value)}
            placeholder="recipient@example.com"
            className="flex-1 bg-transparent text-sm outline-none"
          />
          <button
            type="button"
            onClick={() => setShowCcBcc(v => !v)}
            className="text-xs text-muted-foreground hover:text-foreground cursor-pointer flex items-center gap-0.5 shrink-0"
            title={showCcBcc ? 'Hide Cc/Bcc' : 'Show Cc/Bcc'}
          >
            Cc Bcc {showCcBcc ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
          </button>
        </div>

        {/* Cc field */}
        {showCcBcc && (
          <div className="border-b border-border px-4 py-2.5 flex items-center gap-2">
            <span className="text-xs text-muted-foreground w-12 shrink-0">Cc</span>
            <input
              value={cc}
              onChange={e => setCc(e.target.value)}
              placeholder="cc@example.com, another@example.com"
              className="flex-1 bg-transparent text-sm outline-none"
            />
          </div>
        )}

        {/* Bcc field */}
        {showCcBcc && (
          <div className="border-b border-border px-4 py-2.5 flex items-center gap-2">
            <span className="text-xs text-muted-foreground w-12 shrink-0">Bcc</span>
            <input
              value={bcc}
              onChange={e => setBcc(e.target.value)}
              placeholder="bcc@example.com"
              className="flex-1 bg-transparent text-sm outline-none"
            />
          </div>
        )}

        {/* Subject */}
        <div className="border-b border-border px-4 py-2.5 flex items-center gap-2">
          <span className="text-xs text-muted-foreground w-12 shrink-0">Subject</span>
          <input
            value={subject}
            onChange={e => setSubject(e.target.value)}
            placeholder="Subject"
            className="flex-1 bg-transparent text-sm outline-none"
          />
        </div>

        {/* Rich-text body */}
        <div className="flex-1 flex flex-col min-h-0">
          <RichTextEditor
            value={bodyHtml}
            onChange={(html, text) => { setBodyHtml(html); setBodyText(text); }}
            placeholder="Write your message…"
            className="flex-1"
            minHeight={220}
          />
        </div>

        {/* Attachment chips */}
        {attachments.length > 0 && (
          <div className="flex flex-wrap gap-2 px-4 py-2 border-t border-border bg-muted/10">
            {attachments.map((att, idx) => (
              <div
                key={idx}
                className="flex items-center gap-1.5 rounded-lg border border-border bg-background px-2.5 py-1 text-xs"
              >
                <FileText className="h-3 w-3 text-muted-foreground shrink-0" />
                <span className="max-w-32 truncate">{att.name}</span>
                <button
                  type="button"
                  onClick={() => removeAttachment(idx)}
                  className="text-muted-foreground hover:text-destructive cursor-pointer ml-0.5"
                >
                  <X className="h-3 w-3" />
                </button>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Footer */}
      <div className="flex items-center gap-2 px-4 py-3 border-t border-border bg-muted/20 shrink-0">
        <button
          onClick={send}
          disabled={sending || !to.trim() || !bodyText.trim()}
          className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 disabled:opacity-50 cursor-pointer"
        >
          {sending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
          Send
        </button>
        <button
          onClick={saveDraft}
          disabled={savingDraft}
          className="flex items-center gap-1.5 rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer transition-colors disabled:opacity-50"
        >
          {savingDraft ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
          Save Draft
        </button>

        {/* Attach button */}
        <button
          type="button"
          onClick={() => fileInputRef.current?.click()}
          className="h-8 w-8 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent cursor-pointer transition-colors"
          title="Attach files"
        >
          <Paperclip className="h-4 w-4" />
        </button>
        <input
          ref={fileInputRef}
          type="file"
          multiple
          className="hidden"
          onChange={handleFileAttach}
        />

        <div className="flex-1" />
        <button
          onClick={onClose}
          className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer transition-colors"
        >
          Discard
        </button>
      </div>
    </div>
  );
}

// ─── Email Accounts Panel ────────────────────────────────────────────────────

type ProviderPreset = {
  id: string;
  name: string;
  icon: string;
  smtp_host: string;
  smtp_port: number;
  imap_host: string;
  imap_port: number;
};

const PROVIDER_PRESETS: ProviderPreset[] = [
  { id: 'gmail', name: 'Gmail', icon: 'G', smtp_host: 'smtp.gmail.com', smtp_port: 587, imap_host: 'imap.gmail.com', imap_port: 993 },
  { id: 'outlook', name: 'Outlook', icon: 'O', smtp_host: 'smtp.office365.com', smtp_port: 587, imap_host: 'outlook.office365.com', imap_port: 993 },
  { id: 'zoho', name: 'Zoho', icon: 'Z', smtp_host: 'smtp.zoho.com', smtp_port: 587, imap_host: 'imap.zoho.com', imap_port: 993 },
  { id: 'custom', name: 'Custom IMAP/SMTP', icon: '⚙', smtp_host: '', smtp_port: 587, imap_host: '', imap_port: 993 },
];

function getProviderForIdentity(identity: MailIdentity): ProviderPreset | undefined {
  if (identity.smtp_host?.includes('gmail')) return PROVIDER_PRESETS[0];
  if (identity.smtp_host?.includes('office365') || identity.smtp_host?.includes('outlook')) return PROVIDER_PRESETS[1];
  if (identity.smtp_host?.includes('zoho')) return PROVIDER_PRESETS[2];
  if (identity.smtp_host) return PROVIDER_PRESETS[3];
  return undefined;
}

function EmailAccountsPanel({ onClose }: { onClose: () => void }) {
  const [identities, setIdentities] = useState<MailIdentity[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  const loadIdentities = useCallback(() => {
    setLoading(true);
    mailApi.identities()
      .then(data => { setIdentities(Array.isArray(data) ? data : []); })
      .catch(() => toast.error('Failed to load email accounts'))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { loadIdentities(); }, [loadIdentities]);

  const toggleActive = async (identity: MailIdentity) => {
    try {
      await mailApi.updateIdentity(identity.id, { is_active: !identity.is_active });
      setIdentities(prev => prev.map(i => i.id === identity.id ? { ...i, is_active: !i.is_active } : i));
      toast.success(`Account ${!identity.is_active ? 'activated' : 'deactivated'}`);
    } catch {
      toast.error('Failed to update account');
    }
  };

  const editingIdentity = editingId ? identities.find(i => i.id === editingId) : undefined;

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-3 px-4 py-2.5 border-b border-border shrink-0">
        <button onClick={onClose} className="h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent cursor-pointer">
          <ArrowLeft className="h-4 w-4" />
        </button>
        <span className="text-sm font-semibold flex-1">Email Accounts</span>
        {!showForm && !editingId && (
          <button
            onClick={() => setShowForm(true)}
            className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-3 py-1.5 text-xs font-medium hover:bg-primary/90 cursor-pointer"
          >
            <Plus className="h-3.5 w-3.5" /> Add Account
          </button>
        )}
      </div>

      <div className="flex-1 overflow-y-auto">
        {showForm ? (
          <AccountForm
            onCancel={() => setShowForm(false)}
            onSaved={() => { setShowForm(false); loadIdentities(); }}
          />
        ) : editingIdentity ? (
          <AccountForm
            identity={editingIdentity}
            onCancel={() => setEditingId(null)}
            onSaved={() => { setEditingId(null); loadIdentities(); }}
          />
        ) : loading ? (
          <div className="px-6 py-8 space-y-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="h-16 animate-pulse rounded-lg bg-muted" />
            ))}
          </div>
        ) : identities.length === 0 ? (
          <div className="flex-1 flex flex-col items-center justify-center py-16 px-6 text-center">
            <Mail className="h-10 w-10 text-muted-foreground mb-3" />
            <p className="text-sm font-medium mb-1">No email accounts connected</p>
            <p className="text-xs text-muted-foreground mb-4">Connect a Gmail, Outlook, Zoho, or custom IMAP/SMTP account to start sending and receiving mail.</p>
            <button
              onClick={() => setShowForm(true)}
              className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 cursor-pointer"
            >
              <Plus className="h-4 w-4" /> Add Account
            </button>
          </div>
        ) : (
          <div className="px-6 py-4 space-y-3">
            {identities.map(identity => {
              const provider = getProviderForIdentity(identity);
              return (
                <div
                  key={identity.id}
                  className="flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3 group hover:border-primary/30 transition-colors"
                >
                  {/* Provider icon */}
                  <div className={cn(
                    'flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-sm font-bold',
                    provider?.id === 'gmail' ? 'bg-red-500/10 text-red-500' :
                    provider?.id === 'outlook' ? 'bg-blue-500/10 text-blue-500' :
                    provider?.id === 'zoho' ? 'bg-yellow-500/10 text-yellow-600' :
                    'bg-muted text-muted-foreground'
                  )}>
                    {provider?.icon || 'M'}
                  </div>

                  {/* Info */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium truncate">{identity.display_name || identity.address}</span>
                      {provider && provider.id !== 'custom' && (
                        <span className="text-xs text-muted-foreground">{provider.name}</span>
                      )}
                    </div>
                    <p className="text-xs text-muted-foreground truncate">{identity.address}</p>
                  </div>

                  {/* Status badge */}
                  <button
                    onClick={() => toggleActive(identity)}
                    className={cn(
                      'rounded-full px-2.5 py-0.5 text-xs font-medium cursor-pointer transition-colors',
                      identity.is_active
                        ? 'bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20'
                        : 'bg-muted text-muted-foreground hover:bg-muted/80'
                    )}
                  >
                    {identity.is_active ? 'Active' : 'Inactive'}
                  </button>

                  {/* Edit button */}
                  <button
                    onClick={() => setEditingId(identity.id)}
                    className="h-7 w-7 flex items-center justify-center rounded-md text-muted-foreground hover:text-foreground hover:bg-accent cursor-pointer opacity-0 group-hover:opacity-100 transition-opacity"
                    title="Edit"
                  >
                    <Settings className="h-3.5 w-3.5" />
                  </button>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

// ─── Account Form (Add / Edit) ───────────────────────────────────────────────

function AccountForm({ identity, onCancel, onSaved }: {
  identity?: MailIdentity;
  onCancel: () => void;
  onSaved: () => void;
}) {
  const isEdit = !!identity;
  const [provider, setProvider] = useState<string>(() => {
    if (!identity) return '';
    const p = getProviderForIdentity(identity);
    return p?.id || 'custom';
  });
  const [address, setAddress] = useState(identity?.address || '');
  const [displayName, setDisplayName] = useState(identity?.display_name || '');
  const [smtpHost, setSmtpHost] = useState(identity?.smtp_host || '');
  const [smtpPort, setSmtpPort] = useState<number>(identity?.smtp_port || 587);
  const [smtpUser, setSmtpUser] = useState(identity?.smtp_user || '');
  const [smtpPass, setSmtpPass] = useState('');
  const [imapHost, setImapHost] = useState(identity?.imap_host || '');
  const [imapPort, setImapPort] = useState<number>(identity?.imap_port || 993);
  const [imapUser, setImapUser] = useState(identity?.imap_user || '');
  const [imapPass, setImapPass] = useState('');
  const [saving, setSaving] = useState(false);

  const selectProvider = (id: string) => {
    setProvider(id);
    const preset = PROVIDER_PRESETS.find(p => p.id === id);
    if (preset) {
      setSmtpHost(preset.smtp_host);
      setSmtpPort(preset.smtp_port);
      setImapHost(preset.imap_host);
      setImapPort(preset.imap_port);
    }
  };

  const save = async () => {
    if (!address.trim()) { toast.error('Email address is required'); return; }
    if (!displayName.trim()) { toast.error('Display name is required'); return; }
    setSaving(true);
    try {
      if (isEdit) {
        const body: Partial<MailIdentity> & { smtp_pass?: string; imap_pass?: string } = {
          display_name: displayName,
          smtp_host: smtpHost || undefined,
          smtp_port: smtpPort || undefined,
          smtp_user: smtpUser || undefined,
          imap_host: imapHost || undefined,
          imap_port: imapPort || undefined,
          imap_user: imapUser || undefined,
        };
        if (smtpPass) body.smtp_pass = smtpPass;
        if (imapPass) body.imap_pass = imapPass;
        await mailApi.updateIdentity(identity!.id, body);
        toast.success('Account updated');
      } else {
        await mailApi.createIdentity({
          address,
          display_name: displayName,
          identity_type: 'dedicated',
          smtp_host: smtpHost || undefined,
          smtp_port: smtpPort || undefined,
          smtp_user: smtpUser || undefined,
          smtp_pass: smtpPass || undefined,
          imap_host: imapHost || undefined,
          imap_port: imapPort || undefined,
          imap_user: imapUser || undefined,
          imap_pass: imapPass || undefined,
        });
        toast.success('Account added');
      }
      onSaved();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to save account');
    } finally { setSaving(false); }
  };

  return (
    <div className="px-6 py-5 max-w-lg">
      <h3 className="text-sm font-semibold mb-4">{isEdit ? 'Edit Account' : 'Add Email Account'}</h3>

      {/* Provider selection (only for new accounts) */}
      {!isEdit && (
        <div className="mb-5">
          <label className="text-xs font-medium text-muted-foreground mb-2 block">Provider</label>
          <div className="grid grid-cols-2 gap-2">
            {PROVIDER_PRESETS.map(p => (
              <button
                key={p.id}
                onClick={() => selectProvider(p.id)}
                className={cn(
                  'flex items-center gap-2.5 rounded-lg border px-3 py-2.5 text-left cursor-pointer transition-colors',
                  provider === p.id ? 'border-primary bg-primary/5' : 'border-border hover:border-primary/30'
                )}
              >
                <span className={cn(
                  'flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-bold',
                  p.id === 'gmail' ? 'bg-red-500/10 text-red-500' :
                  p.id === 'outlook' ? 'bg-blue-500/10 text-blue-500' :
                  p.id === 'zoho' ? 'bg-yellow-500/10 text-yellow-600' :
                  'bg-muted text-muted-foreground'
                )}>
                  {p.icon}
                </span>
                <span className="text-sm font-medium">{p.name}</span>
              </button>
            ))}
          </div>
          {provider === 'gmail' && (
            <p className="text-xs text-muted-foreground mt-2 px-1">
              Gmail requires an App Password. Go to Google Account &gt; Security &gt; 2-Step Verification &gt; App passwords to generate one.
            </p>
          )}
        </div>
      )}

      {/* Basic fields */}
      <div className="space-y-3 mb-5">
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">Email Address</label>
          <input
            value={address}
            onChange={e => setAddress(e.target.value)}
            placeholder="you@example.com"
            className="qr-input w-full text-sm"
            disabled={isEdit}
          />
        </div>
        <div>
          <label className="text-xs font-medium text-muted-foreground mb-1 block">Display Name</label>
          <input
            value={displayName}
            onChange={e => setDisplayName(e.target.value)}
            placeholder="Your Name"
            className="qr-input w-full text-sm"
          />
        </div>
      </div>

      {/* SMTP Settings */}
      {(provider || isEdit) && (
        <>
          <div className="mb-4">
            <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2">SMTP (Outgoing)</h4>
            <div className="grid grid-cols-3 gap-2 mb-2">
              <div className="col-span-2">
                <input
                  value={smtpHost}
                  onChange={e => setSmtpHost(e.target.value)}
                  placeholder="smtp.example.com"
                  className="qr-input w-full text-sm"
                />
              </div>
              <div>
                <input
                  type="number"
                  value={smtpPort}
                  onChange={e => setSmtpPort(Number(e.target.value))}
                  placeholder="587"
                  className="qr-input w-full text-sm"
                />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-2">
              <input
                value={smtpUser}
                onChange={e => setSmtpUser(e.target.value)}
                placeholder="Username"
                className="qr-input w-full text-sm"
              />
              <input
                type="password"
                value={smtpPass}
                onChange={e => setSmtpPass(e.target.value)}
                placeholder={isEdit ? '••••••••' : 'Password'}
                className="qr-input w-full text-sm"
              />
            </div>
          </div>

          {/* IMAP Settings */}
          <div className="mb-5">
            <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2">IMAP (Incoming)</h4>
            <div className="grid grid-cols-3 gap-2 mb-2">
              <div className="col-span-2">
                <input
                  value={imapHost}
                  onChange={e => setImapHost(e.target.value)}
                  placeholder="imap.example.com"
                  className="qr-input w-full text-sm"
                />
              </div>
              <div>
                <input
                  type="number"
                  value={imapPort}
                  onChange={e => setImapPort(Number(e.target.value))}
                  placeholder="993"
                  className="qr-input w-full text-sm"
                />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-2">
              <input
                value={imapUser}
                onChange={e => setImapUser(e.target.value)}
                placeholder="Username"
                className="qr-input w-full text-sm"
              />
              <input
                type="password"
                value={imapPass}
                onChange={e => setImapPass(e.target.value)}
                placeholder={isEdit ? '••••••••' : 'Password'}
                className="qr-input w-full text-sm"
              />
            </div>
          </div>
        </>
      )}

      {/* Actions */}
      <div className="flex items-center gap-2 pt-2 border-t border-border">
        <button
          onClick={save}
          disabled={saving || !address.trim() || !displayName.trim()}
          className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2 text-sm font-medium hover:bg-primary/90 disabled:opacity-50 cursor-pointer"
        >
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}
          {isEdit ? 'Save Changes' : 'Add Account'}
        </button>
        <button
          onClick={onCancel}
          className="rounded-lg border border-border px-4 py-2 text-sm hover:bg-accent cursor-pointer transition-colors"
        >
          Cancel
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

