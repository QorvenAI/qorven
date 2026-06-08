'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useCallback, useEffect, useState, useRef } from 'react';
import { Loader2, Send, MessageSquare, Bot, User } from 'lucide-react';
import { cn } from '@/lib/utils';
import { request } from '@/lib/api-core';

type TaskComment = {
  id: string;
  author_type: 'user' | 'agent';
  author_id: string;
  body: string;
  created_at: string;
};

/**
 * TaskComments — the comment/review thread for a single task. Reads and posts
 * via GET/POST /tasks/{id}/comments. A user-posted comment is stored with
 * author_type 'user' by the backend (it stamps the actor). Used in the Tasks
 * drawer and in the L3 worker monitor's Tasks view.
 */
export function TaskComments({ taskId }: { taskId: string }) {
  const [comments, setComments] = useState<TaskComment[]>([]);
  const [commentBody, setCommentBody] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);

  const reload = useCallback(
    () => request<TaskComment[]>(`/tasks/${taskId}/comments`).then(setComments).catch(() => {}),
    [taskId],
  );

  useEffect(() => { reload(); }, [reload]);
  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'smooth' }); }, [comments]);

  const submitComment = async () => {
    if (!commentBody.trim()) return;
    setSubmitting(true);
    try {
      await request<unknown>(`/tasks/${taskId}/comments`, {
        method: 'POST',
        body: JSON.stringify({ body: commentBody.trim() }),
      });
      setCommentBody('');
      reload();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <div className="flex-1 overflow-y-auto px-4 py-3 space-y-3">
        {comments.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full gap-2 text-center">
            <MessageSquare className="h-8 w-8 text-muted-foreground/20" />
            <p className="text-xs text-muted-foreground/60">No comments yet</p>
          </div>
        ) : (
          comments.map((c) => (
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
          ))
        )}
        <div ref={bottomRef} />
      </div>

      <div className="shrink-0 border-t border-border px-3 py-2.5 flex items-end gap-2">
        <textarea
          value={commentBody}
          onChange={(e) => setCommentBody(e.target.value)}
          onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); submitComment(); } }}
          placeholder="Add a comment or review…"
          rows={2}
          className="qr-textarea flex-1 resize-none text-xs" />
        <button
          onClick={submitComment}
          disabled={!commentBody.trim() || submitting}
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
        >
          {submitting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
        </button>
      </div>
    </div>
  );
}
