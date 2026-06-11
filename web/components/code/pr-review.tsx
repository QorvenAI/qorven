'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useRef, useState } from 'react';
import {
  FileText, Plus, Minus, ChevronRight, Loader2, CheckCircle2,
  XCircle, MessageSquare, X, Send,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { github } from '@/lib/api-workspace';
import type { PRFile, PRDiffLine, PRReviewSubmit, PRReviewComment } from '@/types';

// ─── types ────────────────────────────────────────────────────────────────────

type ReviewEvent = 'APPROVE' | 'REQUEST_CHANGES' | 'COMMENT';

interface PendingComment {
  path: string;
  line: number;
  side: 'LEFT' | 'RIGHT';
  body: string;
}

// key: "<path>:<lineNum>"
type PendingMap = Record<string, PendingComment>;

// ─── helpers ──────────────────────────────────────────────────────────────────

function statusBadge(status: string) {
  const map: Record<string, string> = {
    added:    'bg-emerald-500/10 text-emerald-400 border-emerald-500/30',
    removed:  'bg-destructive/10 text-destructive border-destructive/30',
    modified: 'bg-primary/10 text-primary border-primary/30',
    renamed:  'bg-amber-500/10 text-amber-400 border-amber-500/30',
  };
  const cls = map[status] ?? 'bg-muted text-muted-foreground border-border';
  return (
    <span className={cn('rounded border px-1.5 py-0.5 font-mono text-2xs', cls)}>
      {status}
    </span>
  );
}

// ─── Inline comment bubble ────────────────────────────────────────────────────

function CommentBubble({
  comment,
  onRemove,
}: {
  comment: PendingComment;
  onRemove: () => void;
}) {
  return (
    <tr className="bg-primary/5">
      <td colSpan={5} className="px-3 py-2">
        <div className="flex items-start gap-2 rounded-md border border-primary/20 bg-background p-2">
          <MessageSquare className="h-3.5 w-3.5 shrink-0 mt-0.5 text-primary" />
          <p className="flex-1 text-xs text-foreground whitespace-pre-wrap break-words">{comment.body}</p>
          <button
            onClick={onRemove}
            className="shrink-0 rounded p-0.5 hover:bg-muted transition-colors"
            title="Remove comment"
          >
            <X className="h-3 w-3 text-muted-foreground" />
          </button>
        </div>
      </td>
    </tr>
  );
}

// ─── Inline comment editor ────────────────────────────────────────────────────

function CommentEditor({
  onSave,
  onCancel,
}: {
  onSave: (body: string) => void;
  onCancel: () => void;
}) {
  const [body, setBody] = useState('');
  const ref = useRef<HTMLTextAreaElement>(null);

  useEffect(() => { ref.current?.focus(); }, []);

  const handleKey = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') onCancel();
  };

  return (
    <tr className="bg-muted/30">
      <td colSpan={5} className="px-3 py-2">
        <div className="flex flex-col gap-2 rounded-md border border-border bg-background p-2">
          <textarea
            ref={ref}
            value={body}
            onChange={(e) => setBody(e.target.value)}
            onKeyDown={handleKey}
            rows={3}
            placeholder="Leave a comment on this line…"
            className="w-full resize-none rounded bg-muted/40 p-2 text-xs text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/50"
          />
          <div className="flex items-center gap-2 justify-end">
            <button
              onClick={onCancel}
              className="rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-muted transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={() => { if (body.trim()) onSave(body.trim()); }}
              disabled={!body.trim()}
              className="flex items-center gap-1.5 rounded-md bg-primary/10 text-primary hover:bg-primary/20 disabled:opacity-40 px-2 py-1 text-xs transition-colors"
            >
              <Send className="h-3 w-3" />
              Add
            </button>
          </div>
        </div>
      </td>
    </tr>
  );
}

// ─── Diff table ───────────────────────────────────────────────────────────────

function DiffTable({
  file,
  pending,
  onAddComment,
  onRemoveComment,
}: {
  file: PRFile;
  pending: PendingMap;
  onAddComment: (c: PendingComment) => void;
  onRemoveComment: (key: string) => void;
}) {
  const [hovered, setHovered] = useState<number | null>(null);
  const [editing, setEditing] = useState<number | null>(null);

  if (file.binary) {
    return (
      <div className="flex items-center justify-center py-16 text-xs text-muted-foreground">
        Binary file — no diff available
      </div>
    );
  }

  if (!file.lines || file.lines.length === 0) {
    return (
      <div className="flex items-center justify-center py-16 text-xs text-muted-foreground">
        No changes
      </div>
    );
  }

  const rowKey = (line: PRDiffLine) => `${file.path}:${line.new_line || line.old_line}`;

  const rows: React.ReactNode[] = [];

  file.lines.forEach((line, idx) => {
    const key = rowKey(line);
    const lineNum = line.new_line || line.old_line;
    const isHovered = hovered === idx;
    const isEditing = editing === idx;
    const hasPending = key in pending;

    rows.push(
      <tr
        key={`line-${idx}`}
        onMouseEnter={() => setHovered(idx)}
        onMouseLeave={() => setHovered(null)}
        className={cn(
          'group leading-5',
          line.type === 'add' && 'bg-emerald-500/8',
          line.type === 'del' && 'bg-destructive/8',
        )}
      >
        {/* Old line number */}
        <td className="w-10 select-none pr-2 text-right text-2xs text-muted-foreground/40 border-r border-border/50 pl-2">
          {line.old_line > 0 ? line.old_line : ''}
        </td>
        {/* New line number */}
        <td className="w-10 select-none pr-2 text-right text-2xs text-muted-foreground/40 border-r border-border/50">
          {line.new_line > 0 ? line.new_line : ''}
        </td>
        {/* Comment gutter */}
        <td className="w-6 select-none text-center border-r border-border/50">
          {!isEditing && (
            <button
              onClick={() => setEditing(isEditing ? null : idx)}
              className={cn(
                'flex items-center justify-center h-4 w-4 rounded transition-opacity',
                isHovered || hasPending
                  ? 'opacity-100 text-primary hover:bg-primary/10'
                  : 'opacity-0 group-hover:opacity-100 text-muted-foreground hover:bg-muted',
              )}
              title="Add comment"
            >
              <Plus className="h-3 w-3" />
            </button>
          )}
        </td>
        {/* +/−/space marker */}
        <td className={cn(
          'w-5 select-none text-center text-2xs border-r border-border/50',
          line.type === 'add' && 'text-emerald-400 border-l-2 border-l-emerald-500',
          line.type === 'del' && 'text-destructive border-l-2 border-l-destructive',
          line.type === 'eq' && 'text-transparent',
        )}>
          {line.type === 'add' ? '+' : line.type === 'del' ? '−' : ' '}
        </td>
        {/* Content */}
        <td className={cn(
          'px-3 py-px whitespace-pre overflow-hidden font-mono text-xs',
          line.type === 'add' && 'text-emerald-300',
          line.type === 'del' && 'text-destructive/80 opacity-80',
          line.type === 'eq' && 'text-foreground/60',
        )}>
          {line.content}
        </td>
      </tr>,
    );

    if (isEditing) {
      rows.push(
        <CommentEditor
          key={`editor-${idx}`}
          onSave={(body) => {
            onAddComment({
              path: file.path,
              line: lineNum,
              side: line.type === 'del' ? 'LEFT' : 'RIGHT',
              body,
            });
            setEditing(null);
          }}
          onCancel={() => setEditing(null)}
        />,
      );
    }

    if (hasPending && !isEditing) {
      rows.push(
        <CommentBubble
          key={`comment-${idx}`}
          comment={pending[key]!}
          onRemove={() => onRemoveComment(key)}
        />,
      );
    }
  });

  return (
    <table className="w-full border-collapse font-mono text-xs" style={{ minWidth: '100%' }}>
      <tbody>{rows}</tbody>
    </table>
  );
}

// ─── File tree item ───────────────────────────────────────────────────────────

function FileTreeItem({
  file,
  active,
  pendingCount,
  onClick,
}: {
  file: PRFile;
  active: boolean;
  pendingCount: number;
  onClick: () => void;
}) {
  const parts = file.path.split('/');
  const name = parts.pop() ?? file.path;
  const dir = parts.join('/');

  return (
    <button
      onClick={onClick}
      className={cn(
        'group flex w-full items-start gap-2 rounded px-2 py-1.5 text-left transition-colors',
        active ? 'bg-primary/10 text-foreground' : 'hover:bg-muted text-muted-foreground hover:text-foreground',
      )}
    >
      <FileText className={cn('h-3.5 w-3.5 shrink-0 mt-0.5', active ? 'text-primary' : 'text-muted-foreground')} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5 flex-wrap">
          <span className="text-xs font-medium truncate">{name}</span>
          {statusBadge(file.status)}
          {pendingCount > 0 && (
            <span className="rounded-full bg-primary/20 text-primary px-1.5 py-0 text-2xs font-medium">
              {pendingCount}
            </span>
          )}
        </div>
        {dir && <p className="text-2xs text-muted-foreground font-mono truncate">{dir}/</p>}
        <div className="flex items-center gap-2 mt-0.5">
          {file.additions > 0 && (
            <span className="flex items-center gap-0.5 text-2xs text-emerald-400">
              <Plus className="h-2.5 w-2.5" />{file.additions}
            </span>
          )}
          {file.deletions > 0 && (
            <span className="flex items-center gap-0.5 text-2xs text-destructive">
              <Minus className="h-2.5 w-2.5" />{file.deletions}
            </span>
          )}
        </div>
      </div>
      <ChevronRight className={cn('h-3 w-3 shrink-0 mt-1 transition-transform', active && 'rotate-90')} />
    </button>
  );
}

// ─── Review footer ────────────────────────────────────────────────────────────

function ReviewFooter({
  owner,
  repo,
  prNumber,
  pending,
  onSuccess,
}: {
  owner: string;
  repo: string;
  prNumber: number;
  pending: PendingMap;
  onSuccess: () => void;
}) {
  const [event, setEvent] = useState<ReviewEvent>('COMMENT');
  const [body, setBody] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  const commentCount = Object.keys(pending).length;
  const comments: PRReviewComment[] = Object.values(pending);

  const submit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const payload: PRReviewSubmit = { event, body, comments };
      await github.prReview(owner, repo, prNumber, payload);
      setSuccess(true);
      setBody('');
      onSuccess();
    } catch (e: any) {
      setError(e.message ?? 'Review submission failed');
    } finally {
      setSubmitting(false);
    }
  };

  if (success) {
    return (
      <div className="shrink-0 border-t border-border px-4 py-3 flex items-center gap-2 bg-emerald-500/5">
        <CheckCircle2 className="h-4 w-4 text-emerald-400 shrink-0" />
        <span className="text-xs text-emerald-400 font-medium">Review submitted to GitHub</span>
        <button
          onClick={() => setSuccess(false)}
          className="ml-auto text-xs text-muted-foreground hover:text-foreground transition-colors"
        >
          New review
        </button>
      </div>
    );
  }

  return (
    <div className="shrink-0 border-t border-border bg-muted/20 px-4 py-3 space-y-2">
      {error && (
        <div className="flex items-start gap-2 rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive">
          <XCircle className="h-3.5 w-3.5 shrink-0 mt-0.5" />
          <span>{error}</span>
        </div>
      )}
      <div className="flex items-start gap-2">
        {/* Verdict selector */}
        <select
          value={event}
          onChange={(e) => setEvent(e.target.value as ReviewEvent)}
          disabled={submitting}
          className="qr-select text-xs shrink-0"
        >
          <option value="APPROVE">Approve</option>
          <option value="REQUEST_CHANGES">Request changes</option>
          <option value="COMMENT">Comment only</option>
        </select>
        {/* Summary body */}
        <textarea
          value={body}
          onChange={(e) => setBody(e.target.value)}
          disabled={submitting}
          rows={2}
          placeholder="Review summary (optional)…"
          className="flex-1 resize-none rounded bg-background border border-border px-2 py-1.5 text-xs text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary/50"
        />
        {/* Submit */}
        <button
          onClick={submit}
          disabled={submitting}
          className="flex items-center gap-1.5 rounded-md bg-primary/10 text-primary hover:bg-primary/20 disabled:opacity-40 px-3 py-1.5 text-xs font-medium transition-colors shrink-0 self-end"
        >
          {submitting
            ? <Loader2 className="h-3 w-3 animate-spin" />
            : <Send className="h-3 w-3" />}
          Submit
        </button>
      </div>
      {commentCount > 0 && (
        <p className="text-2xs text-muted-foreground">
          {commentCount} pending inline {commentCount === 1 ? 'comment' : 'comments'} will be included
        </p>
      )}
    </div>
  );
}

// ─── PRReview (main export) ───────────────────────────────────────────────────

export interface PRReviewProps {
  owner: string;
  repo: string;
  prNumber: number;
}

export function PRReview({ owner, repo, prNumber }: PRReviewProps) {
  const [files, setFiles] = useState<PRFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [pending, setPending] = useState<PendingMap>({});

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError(null);
    try {
      const { files: f } = await github.prFiles(owner, repo, prNumber);
      setFiles(f);
      if (f.length > 0 && !selectedPath) setSelectedPath(f[0]!.path);
    } catch (e: any) {
      setLoadError(e.message ?? 'Failed to load files');
    } finally {
      setLoading(false);
    }
  }, [owner, repo, prNumber]);

  useEffect(() => { load(); }, [load]);

  const selectedFile = files.find((f) => f.path === selectedPath) ?? null;

  const addComment = useCallback((c: PendingComment) => {
    const key = `${c.path}:${c.line}`;
    setPending((prev) => ({ ...prev, [key]: c }));
  }, []);

  const removeComment = useCallback((key: string) => {
    setPending((prev) => {
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }, []);

  const clearPending = useCallback(() => setPending({}), []);

  const pendingCountForFile = useCallback((path: string) =>
    Object.values(pending).filter((c) => c.path === path).length,
  [pending]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (loadError) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 py-16">
        <XCircle className="h-5 w-5 text-destructive" />
        <p className="text-xs text-destructive">{loadError}</p>
        <button onClick={load} className="text-xs text-primary hover:underline">Retry</button>
      </div>
    );
  }

  if (files.length === 0) {
    return (
      <div className="flex items-center justify-center py-16">
        <p className="text-xs text-muted-foreground">No changed files in this PR</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full overflow-hidden">
      {/* Two-pane area */}
      <div className="flex flex-1 min-h-0 overflow-hidden">
        {/* File tree (left) */}
        <div className="w-56 shrink-0 flex flex-col border-r border-border overflow-y-auto bg-muted/10">
          <div className="px-3 py-2 border-b border-border">
            <p className="text-2xs font-medium text-muted-foreground uppercase tracking-wide">
              Changed files ({files.length})
            </p>
          </div>
          <div className="flex-1 overflow-y-auto p-1 space-y-0.5">
            {files.map((f) => (
              <FileTreeItem
                key={f.path}
                file={f}
                active={selectedPath === f.path}
                pendingCount={pendingCountForFile(f.path)}
                onClick={() => setSelectedPath(f.path)}
              />
            ))}
          </div>
        </div>

        {/* Diff (right) */}
        <div className="flex flex-col flex-1 min-w-0 overflow-hidden">
          {selectedFile ? (
            <>
              {/* File header */}
              <div className="flex items-center gap-3 shrink-0 border-b border-border bg-muted/20 px-4 py-2">
                <span className="font-mono text-xs text-foreground font-medium truncate flex-1">
                  {selectedFile.path}
                </span>
                <div className="flex items-center gap-1.5">
                  {selectedFile.additions > 0 && (
                    <span className="rounded border border-emerald-500/30 bg-emerald-500/10 px-1.5 py-0.5 font-mono text-2xs text-emerald-400">
                      +{selectedFile.additions}
                    </span>
                  )}
                  {selectedFile.deletions > 0 && (
                    <span className="rounded border border-destructive/30 bg-destructive/10 px-1.5 py-0.5 font-mono text-2xs text-destructive">
                      −{selectedFile.deletions}
                    </span>
                  )}
                </div>
              </div>
              {/* Diff table */}
              <div className="flex-1 overflow-auto">
                <DiffTable
                  file={selectedFile}
                  pending={pending}
                  onAddComment={addComment}
                  onRemoveComment={removeComment}
                />
              </div>
            </>
          ) : (
            <div className="flex items-center justify-center flex-1 text-xs text-muted-foreground">
              Select a file to view the diff
            </div>
          )}
        </div>
      </div>

      {/* Review footer (sticky) */}
      <ReviewFooter
        owner={owner}
        repo={repo}
        prNumber={prNumber}
        pending={pending}
        onSuccess={clearPending}
      />
    </div>
  );
}
