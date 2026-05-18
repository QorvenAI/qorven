'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { request } from '@/lib/api-core';
import type { GitHubCommitPendingProps } from '@/lib/events';
import {
  GitCommit, GitBranch, Plus, Minus, Loader2, CheckCircle2, X,
} from 'lucide-react';

interface Props {
  sessionId: string;
}

export function CommitApprovalCards({ sessionId }: Props) {
  const commitPendings = useStore((s) => s.commitPendings);
  const cards = Object.values(commitPendings).filter(
    (p) => !sessionId || p.session_id === sessionId || !p.session_id,
  );
  if (cards.length === 0) return null;
  return (
    <div className="space-y-2">
      {cards.map((p) => (
        <CommitApprovalCard key={p.approval_id ?? p.branch} commit={p} />
      ))}
    </div>
  );
}

function CommitApprovalCard({ commit }: { commit: GitHubCommitPendingProps }) {
  const removeCommitPending = useStore((s) => s.removeCommitPending);
  const [pushing, setPushing] = useState(false);
  const [pushed, setPushed] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState(false);

  const push = async () => {
    if (pushing || pushed) return;
    setPushing(true);
    setError(null);
    try {
      await request(`/permissions/${commit.approval_id}/reply`, {
        method: 'POST',
        body: JSON.stringify({ decision: 'allow' }),
      });
      setPushed(true);
      const id = commit.approval_id ?? commit.branch;
      setTimeout(() => removeCommitPending(id), 3000);
    } catch (e: any) {
      setError(e.message ?? 'Push failed');
    } finally {
      setPushing(false);
    }
  };

  const deny = async () => {
    try {
      await request(`/permissions/${commit.approval_id}/reply`, {
        method: 'POST',
        body: JSON.stringify({ decision: 'deny' }),
      });
    } catch { /* ignore */ }
    removeCommitPending(commit.approval_id ?? commit.branch);
  };

  const totalAdd = commit.files.reduce((s, f) => s + f.additions, 0);
  const totalDel = commit.files.reduce((s, f) => s + f.deletions, 0);

  return (
    <div className={cn(
      'rounded-lg border p-3 text-xs transition-opacity',
      pushed
        ? 'border-emerald-500/30 bg-emerald-500/5 opacity-60'
        : 'border-amber-500/30 bg-amber-500/5',
    )}>
      <div className="flex items-center justify-between gap-2 mb-2">
        <div className="flex items-center gap-1.5">
          <GitCommit className="h-3.5 w-3.5 text-amber-400 shrink-0" />
          <span className="font-medium text-amber-400">Commit Ready to Push</span>
        </div>
        <div className="flex items-center gap-1.5 text-muted-foreground">
          <GitBranch className="h-3 w-3" />
          <span>{commit.branch}</span>
        </div>
      </div>

      <p className="mb-2 font-medium leading-snug">{commit.commit_message}</p>

      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2 text-muted-foreground">
          <span className="flex items-center gap-0.5 text-emerald-400">
            <Plus className="h-3 w-3" />{totalAdd}
          </span>
          <span className="flex items-center gap-0.5 text-destructive">
            <Minus className="h-3 w-3" />{totalDel}
          </span>
          <span>{commit.files.length} file{commit.files.length !== 1 ? 's' : ''}</span>
        </div>
        <button onClick={() => setExpanded((v) => !v)}
          className="text-muted-foreground hover:text-foreground transition-colors">
          {expanded ? 'hide files' : 'show files'}
        </button>
      </div>

      {expanded && (
        <div className="mb-3 space-y-0.5 rounded-md bg-background/60 border border-border/50 px-2 py-1.5 max-h-40 overflow-y-auto">
          {commit.files.map((f) => (
            <div key={f.path} className="flex items-center justify-between gap-2">
              <span className="font-mono truncate">{f.path}</span>
              <span className="shrink-0 flex items-center gap-1.5">
                <span className="text-emerald-400">+{f.additions}</span>
                <span className="text-destructive">-{f.deletions}</span>
              </span>
            </div>
          ))}
        </div>
      )}

      {pushed ? (
        <div className="flex items-center gap-1.5 text-emerald-400 font-medium">
          <CheckCircle2 className="h-3.5 w-3.5" />
          Pushed successfully
        </div>
      ) : (
        <div className="flex items-center gap-2">
          <button onClick={push} disabled={pushing}
            className="flex-1 flex items-center justify-center gap-1.5 rounded-md px-2 py-1.5 font-medium bg-amber-500/10 text-amber-400 hover:bg-amber-500/20 disabled:opacity-50 transition-colors">
            {pushing
              ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
              : <GitCommit className="h-3.5 w-3.5" />
            }
            Approve & Push
          </button>
          <button onClick={deny}
            className="flex items-center gap-1.5 rounded-md px-2 py-1.5 font-medium text-muted-foreground hover:text-foreground hover:bg-muted transition-colors">
            <X className="h-3.5 w-3.5" />
            Reject
          </button>
        </div>
      )}

      {error && (
        <div className="mt-1.5 text-destructive">{error} — try again</div>
      )}
    </div>
  );
}
