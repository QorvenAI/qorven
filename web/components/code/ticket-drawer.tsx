'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useRef } from 'react';
import { X, Loader2, File, MessageSquare, Send, User, Bot, Pencil, Check, XCircle } from 'lucide-react';
import { cn } from '@/lib/utils';
import { tickets as ticketsApi } from '@/lib/api';
import type { Ticket, TicketComment, TicketFile, TicketStatus, TicketPriority } from '@/types';

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

const STATUS_OPTS: { value: TicketStatus; label: string }[] = [
  { value: 'todo', label: 'Todo' },
  { value: 'in_progress', label: 'In Progress' },
  { value: 'blocked', label: 'Blocked' },
  { value: 'done', label: 'Done' },
];

const PRIORITY_OPTS: { value: TicketPriority; label: string }[] = [
  { value: 'critical', label: 'Critical' },
  { value: 'high', label: 'High' },
  { value: 'normal', label: 'Normal' },
  { value: 'low', label: 'Low' },
];

function dispatchUpdated(id: string) {
  window.dispatchEvent(new CustomEvent('qorven:ticket_updated', { detail: { id } }));
}

export function TicketDrawer({ ticket, onClose }: { ticket: Ticket; onClose: () => void }) {
  const [comments, setComments] = useState<TicketComment[]>([]);
  const [files, setFiles] = useState<TicketFile[]>([]);
  const [commentBody, setCommentBody] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);

  // Optimistic local state for editable fields
  const [localStatus, setLocalStatus] = useState<TicketStatus>(ticket.status);
  const [localPriority, setLocalPriority] = useState<TicketPriority>(ticket.priority);
  const [localTitle, setLocalTitle] = useState(ticket.title);
  const [localDescription, setLocalDescription] = useState(ticket.description ?? '');

  // Edit mode for title + description
  const [editing, setEditing] = useState(false);
  const [editTitle, setEditTitle] = useState(ticket.title);
  const [editDescription, setEditDescription] = useState(ticket.description ?? '');

  // Saving states per field
  const [savingStatus, setSavingStatus] = useState(false);
  const [savingPriority, setSavingPriority] = useState(false);
  const [savingMeta, setSavingMeta] = useState(false);

  // Error state (shown below meta chips)
  const [saveError, setSaveError] = useState('');

  // Sync local state when ticket prop changes (parent reloads after event)
  useEffect(() => {
    setLocalStatus(ticket.status);
    setLocalPriority(ticket.priority);
    setLocalTitle(ticket.title);
    setLocalDescription(ticket.description ?? '');
  }, [ticket.id, ticket.status, ticket.priority, ticket.title, ticket.description]);

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

  const changeStatus = async (newStatus: TicketStatus) => {
    if (savingStatus) return;
    const prev = localStatus;
    setLocalStatus(newStatus);
    setSaveError('');
    setSavingStatus(true);
    try {
      await ticketsApi.update(ticket.id, { status: newStatus });
      dispatchUpdated(ticket.id);
    } catch {
      setLocalStatus(prev);
      setSaveError('Failed to update status — please try again.');
    } finally {
      setSavingStatus(false);
    }
  };

  const changePriority = async (newPriority: TicketPriority) => {
    if (savingPriority) return;
    const prev = localPriority;
    setLocalPriority(newPriority);
    setSaveError('');
    setSavingPriority(true);
    try {
      await ticketsApi.update(ticket.id, { priority: newPriority });
      dispatchUpdated(ticket.id);
    } catch {
      setLocalPriority(prev);
      setSaveError('Failed to update priority — please try again.');
    } finally {
      setSavingPriority(false);
    }
  };

  const startEdit = () => {
    setEditTitle(localTitle);
    setEditDescription(localDescription);
    setSaveError('');
    setEditing(true);
  };

  const cancelEdit = () => {
    setEditing(false);
    setSaveError('');
  };

  const saveMeta = async () => {
    if (savingMeta) return;
    const trimTitle = editTitle.trim();
    if (!trimTitle) return;
    setSaveError('');
    setSavingMeta(true);
    const prevTitle = localTitle;
    const prevDesc = localDescription;
    setLocalTitle(trimTitle);
    setLocalDescription(editDescription.trim());
    try {
      await ticketsApi.update(ticket.id, { title: trimTitle, description: editDescription.trim() || undefined });
      setEditing(false);
      dispatchUpdated(ticket.id);
    } catch {
      setLocalTitle(prevTitle);
      setLocalDescription(prevDesc);
      setSaveError('Failed to save changes — please try again.');
    } finally {
      setSavingMeta(false);
    }
  };

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
        {editing ? (
          <input
            autoFocus
            value={editTitle}
            onChange={e => setEditTitle(e.target.value)}
            onKeyDown={e => {
              if (e.key === 'Enter') { e.preventDefault(); saveMeta(); }
              if (e.key === 'Escape') cancelEdit();
            }}
            className="qr-input flex-1 text-sm font-semibold"
          />
        ) : (
          <h2 className="flex-1 truncate text-sm font-semibold">{localTitle}</h2>
        )}
        {!editing && (
          <button
            onClick={startEdit}
            title="Edit title and description"
            className="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground transition-colors"
          >
            <Pencil className="h-3.5 w-3.5" />
          </button>
        )}
        {editing && (
          <>
            <button
              onClick={saveMeta}
              disabled={!editTitle.trim() || savingMeta}
              title="Save"
              className="rounded p-1 text-emerald-600 hover:bg-emerald-500/10 transition-colors disabled:opacity-50"
            >
              {savingMeta ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}
            </button>
            <button
              onClick={cancelEdit}
              disabled={savingMeta}
              title="Cancel"
              className="rounded p-1 text-muted-foreground hover:bg-accent transition-colors disabled:opacity-50"
            >
              <XCircle className="h-3.5 w-3.5" />
            </button>
          </>
        )}
        <button onClick={onClose} className="rounded p-1 hover:bg-accent transition-colors">
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Meta chips — editable status + priority */}
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border px-4 py-2.5">
        {/* Status select styled as chip */}
        <div className="relative">
          <select
            value={localStatus}
            onChange={e => changeStatus(e.target.value as TicketStatus)}
            disabled={savingStatus}
            className={cn(
              'appearance-none rounded-full px-2 py-0.5 text-xs font-medium capitalize cursor-pointer outline-none transition-opacity',
              'pr-5', // extra right padding for the caret
              STATUS_COLOR[localStatus],
              savingStatus && 'opacity-50 cursor-not-allowed',
            )}
          >
            {STATUS_OPTS.map(o => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
          <span className="pointer-events-none absolute right-1.5 top-1/2 -translate-y-1/2 text-xs opacity-60">▾</span>
          {savingStatus && (
            <Loader2 className="pointer-events-none absolute right-1 top-1/2 -translate-y-1/2 h-3 w-3 animate-spin" />
          )}
        </div>

        {/* Priority select styled as chip */}
        <div className="relative">
          <select
            value={localPriority}
            onChange={e => changePriority(e.target.value as TicketPriority)}
            disabled={savingPriority}
            className={cn(
              'appearance-none rounded-full px-2 py-0.5 text-xs font-medium capitalize cursor-pointer outline-none transition-opacity',
              'pr-5',
              PRIORITY_COLOR[localPriority],
              savingPriority && 'opacity-50 cursor-not-allowed',
            )}
          >
            {PRIORITY_OPTS.map(o => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
          <span className="pointer-events-none absolute right-1.5 top-1/2 -translate-y-1/2 text-xs opacity-60">▾</span>
          {savingPriority && (
            <Loader2 className="pointer-events-none absolute right-1 top-1/2 -translate-y-1/2 h-3 w-3 animate-spin" />
          )}
        </div>

        {saveError && (
          <p className="w-full text-xs text-destructive">{saveError}</p>
        )}
      </div>

      {/* Description — read or edit */}
      {(localDescription || editing) && (
        <div className="shrink-0 border-b border-border px-4 py-3">
          {editing ? (
            <textarea
              value={editDescription}
              onChange={e => setEditDescription(e.target.value)}
              onKeyDown={e => { if (e.key === 'Escape') cancelEdit(); }}
              placeholder="Description (optional)…"
              rows={3}
              className="qr-textarea w-full resize-none text-xs"
            />
          ) : (
            <p className="text-xs text-muted-foreground leading-relaxed whitespace-pre-wrap">{localDescription}</p>
          )}
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
