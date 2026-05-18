'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useCallback } from 'react';
import { Send, Trash2, Edit2, Check, X, RefreshCw } from 'lucide-react';
import { type DraftReply, listDrafts, sendDraft, discardDraft, editDraft } from '@/lib/api-inbound';
import { toast } from 'sonner';

export function PendingRepliesPanel() {
  const [drafts, setDrafts] = useState<DraftReply[]>([]);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<string | null>(null);
  const [editContent, setEditContent] = useState('');

  const refresh = useCallback(async () => {
    try {
      const d = await listDrafts();
      setDrafts(d ?? []);
    } catch {
      // silently fail — will show empty state
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const handleSend = async (id: string) => {
    try {
      await sendDraft(id);
      setDrafts((d) => d.filter((x) => x.id !== id));
      toast.success('Reply sent');
    } catch {
      toast.error('Could not send reply. Please try again.');
    }
  };

  const handleDiscard = async (id: string) => {
    try {
      await discardDraft(id);
      setDrafts((d) => d.filter((x) => x.id !== id));
    } catch {
      toast.error('Failed to discard');
    }
  };

  const startEdit = (draft: DraftReply) => {
    setEditing(draft.id);
    setEditContent(draft.draft_content);
  };

  const submitEdit = async (id: string) => {
    try {
      await editDraft(id, editContent);
      setDrafts((d) => d.map((x) => (x.id === id ? { ...x, draft_content: editContent } : x)));
      setEditing(null);
      toast.success('Draft updated');
    } catch {
      toast.error('Failed to update draft');
    }
  };

  if (loading) {
    return <div className="p-4 text-xs text-muted-foreground">Loading drafts...</div>;
  }

  if (drafts.length === 0) {
    return (
      <div className="p-4 text-center text-xs text-muted-foreground">
        No pending draft replies
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-2 p-3">
      <div className="flex items-center justify-between mb-1">
        <span className="text-xs font-medium text-muted-foreground">
          {drafts.length} pending
        </span>
        <button onClick={refresh} className="text-muted-foreground hover:text-foreground transition-colors">
          <RefreshCw className="h-3.5 w-3.5" />
        </button>
      </div>
      {drafts.map((draft) => (
        <div key={draft.id} className="rounded-xl border border-border bg-card p-3 text-xs space-y-2">
          <div className="flex items-center justify-between">
            <span className="font-medium">{draft.sender_name || draft.sender_id}</span>
            <span className="rounded-full px-2 py-0.5 text-[10px] font-medium bg-amber-400/10 text-amber-400">
              {draft.channel}
            </span>
          </div>
          <p className="text-muted-foreground line-clamp-2">{draft.original_message}</p>
          {draft.history_summary && (
            <details className="text-muted-foreground/70">
              <summary className="cursor-pointer hover:text-foreground text-[10px]">
                Conversation history
              </summary>
              <pre className="mt-1 text-[10px] whitespace-pre-wrap font-mono border-t border-border/30 pt-1">
                {draft.history_summary}
              </pre>
            </details>
          )}
          <div className="rounded-lg bg-muted/30 p-2 text-foreground">
            {editing === draft.id ? (
              <textarea
                value={editContent}
                onChange={(e) => setEditContent(e.target.value)}
                rows={4}
                className="w-full bg-transparent resize-none focus:outline-none text-xs"
              />
            ) : (
              <p className="whitespace-pre-wrap">{draft.draft_content}</p>
            )}
          </div>
          <div className="flex items-center gap-1.5">
            {editing === draft.id ? (
              <>
                <button
                  onClick={() => submitEdit(draft.id)}
                  className="flex items-center gap-1 rounded-lg bg-primary px-2.5 py-1 text-primary-foreground hover:bg-primary/90 text-[10px]"
                >
                  <Check className="h-3 w-3" /> Save
                </button>
                <button
                  onClick={() => setEditing(null)}
                  className="flex items-center gap-1 rounded-lg border border-border px-2.5 py-1 hover:bg-accent text-[10px]"
                >
                  <X className="h-3 w-3" /> Cancel
                </button>
              </>
            ) : (
              <>
                <button
                  onClick={() => handleSend(draft.id)}
                  className="flex items-center gap-1 rounded-lg bg-primary px-2.5 py-1 text-primary-foreground hover:bg-primary/90 text-[10px]"
                >
                  <Send className="h-3 w-3" /> Send
                </button>
                <button
                  onClick={() => startEdit(draft)}
                  className="flex items-center gap-1 rounded-lg border border-border px-2.5 py-1 hover:bg-accent text-[10px]"
                >
                  <Edit2 className="h-3 w-3" /> Edit
                </button>
                <button
                  onClick={() => handleDiscard(draft.id)}
                  className="flex items-center gap-1 rounded-lg border border-border px-2.5 py-1 text-muted-foreground hover:text-destructive hover:border-destructive/30 text-[10px]"
                >
                  <Trash2 className="h-3 w-3" /> Discard
                </button>
              </>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
