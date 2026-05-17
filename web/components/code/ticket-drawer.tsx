'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState, useEffect, useRef } from 'react';
import { X, Loader2, File, MessageSquare, Send, User, Bot } from 'lucide-react';
import { cn } from '@/lib/utils';
import { tickets as ticketsApi } from '@/lib/api';
import type { Ticket, TicketComment, TicketFile } from '@/types';

const STATUS_COLOR: Record<string, string> = {
  todo: 'bg-muted text-muted-foreground',
  in_progress: 'bg-blue-500/10 text-blue-500',
  blocked: 'bg-destructive/10 text-destructive',
  done: 'bg-emerald-500/10 text-emerald-600',
};

const PRIORITY_COLOR: Record<string, string> = {
  critical: 'bg-destructive/10 text-destructive',
  high: 'bg-orange-500/10 text-orange-500',
  normal: 'bg-muted text-muted-foreground',
  low: 'bg-muted/50 text-muted-foreground/60',
};

export function TicketDrawer({ ticket, onClose }: { ticket: Ticket; onClose: () => void }) {
  const [comments, setComments] = useState<TicketComment[]>([]);
  const [files, setFiles] = useState<TicketFile[]>([]);
  const [commentBody, setCommentBody] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    ticketsApi.comments(ticket.id).then(setComments).catch(() => {});
    ticketsApi.files(ticket.id).then(setFiles).catch(() => {});
  }, [ticket.id]);

  useEffect(() => {
    const handler = (e: Event) => {
      const d = (e as CustomEvent).detail;
      if (!d) return;
      if (d.ticket_id === ticket.id || d.id === ticket.id) {
        ticketsApi.comments(ticket.id).then(setComments).catch(() => {});
        ticketsApi.files(ticket.id).then(setFiles).catch(() => {});
      }
    };
    window.addEventListener('qorven:ticket_comment', handler);
    window.addEventListener('qorven:ticket_updated', handler);
    return () => {
      window.removeEventListener('qorven:ticket_comment', handler);
      window.removeEventListener('qorven:ticket_updated', handler);
    };
  }, [ticket.id]);

  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'smooth' }); }, [comments]);

  const submit = async () => {
    if (!commentBody.trim()) return;
    setSubmitting(true);
    try {
      await ticketsApi.comment(ticket.id, commentBody.trim());
      setCommentBody('');
      const updated = await ticketsApi.comments(ticket.id);
      setComments(updated);
    } finally {
      setSubmitting(false);
    }
  };

  const OP_ICON: Record<string, string> = { created: '✦', modified: '✎', deleted: '✕' };

  return (
    <div className="fixed inset-y-0 right-0 z-40 flex w-[480px] flex-col border-l border-border bg-background shadow-2xl">
      {/* Header */}
      <div className="flex shrink-0 items-center gap-3 border-b border-border px-4 py-3">
        <span className="font-mono text-xs text-muted-foreground shrink-0">{ticket.slug}</span>
        <h2 className="flex-1 truncate text-sm font-semibold">{ticket.title}</h2>
        <button onClick={onClose} className="rounded p-1 hover:bg-accent transition-colors">
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Meta chips */}
      <div className="flex shrink-0 flex-wrap gap-2 border-b border-border px-4 py-2.5">
        <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium capitalize', STATUS_COLOR[ticket.status])}>
          {ticket.status.replace('_', ' ')}
        </span>
        <span className={cn('rounded-full px-2 py-0.5 text-xs font-medium capitalize', PRIORITY_COLOR[ticket.priority])}>
          {ticket.priority}
        </span>
      </div>

      {/* Description */}
      {ticket.description && (
        <div className="shrink-0 border-b border-border px-4 py-3">
          <p className="text-xs text-muted-foreground leading-relaxed whitespace-pre-wrap">{ticket.description}</p>
        </div>
      )}

      {/* Files touched */}
      {files.length > 0 && (
        <div className="shrink-0 border-b border-border px-4 py-3">
          <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-2">Files</p>
          <div className="space-y-1 max-h-32 overflow-y-auto">
            {files.map(f => (
              <div key={f.id} className="flex items-center gap-2 text-xs">
                <span className="shrink-0 text-muted-foreground/50 font-mono">{OP_ICON[f.operation] ?? '·'}</span>
                <File className="h-3 w-3 shrink-0 text-muted-foreground/60" />
                <span className="truncate font-mono text-xs">{f.path}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Comments */}
      <div className="flex-1 overflow-y-auto px-4 py-3 space-y-3">
        {comments.length === 0 && (
          <div className="flex flex-col items-center justify-center h-full gap-2 text-center">
            <MessageSquare className="h-8 w-8 text-muted-foreground/20" />
            <p className="text-xs text-muted-foreground/60">No comments yet</p>
          </div>
        )}
        {comments.map(c => (
          <div key={c.id} className={cn('flex gap-2.5', c.author_type === 'user' ? 'flex-row-reverse' : '')}>
            <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-muted mt-0.5">
              {c.author_type === 'agent'
                ? <Bot className="h-3.5 w-3.5 text-primary" />
                : <User className="h-3.5 w-3.5 text-muted-foreground" />}
            </div>
            <div className={cn('max-w-[85%] rounded-xl px-3 py-2 text-xs leading-relaxed whitespace-pre-wrap',
              c.author_type === 'user' ? 'bg-primary text-primary-foreground rounded-tr-sm' : 'bg-muted rounded-tl-sm')}>
              {c.body}
            </div>
          </div>
        ))}
        <div ref={bottomRef} />
      </div>

      {/* Comment input */}
      <div className="shrink-0 border-t border-border px-3 py-2.5 flex items-end gap-2">
        <textarea
          value={commentBody}
          onChange={e => setCommentBody(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submit(); } }}
          placeholder="Add a comment…"
          rows={2}
          className="qr-textarea flex-1 resize-none text-xs"
        />
        <button
          onClick={submit}
          disabled={!commentBody.trim() || submitting}
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
        >
          {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
        </button>
      </div>
    </div>
  );
}
