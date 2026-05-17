'use client';

// Copyright 2026 Tekky AI Academy LLP. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { githubApi } from '@/lib/api-github';
import type { GitHubPRReadyProps } from '@/lib/events';
import {
  GitPullRequest, GitMerge, GitBranch, Plus, Minus,
  CheckCircle2, XCircle, Clock, Loader2, ExternalLink,
} from 'lucide-react';

interface Props {
  sessionId: string;
}

export function GitHubPRCards({ sessionId }: Props) {
  const prApprovals = useStore((s) => s.prApprovals);
  const cards = Object.values(prApprovals).filter(
    (p) => !sessionId || p.session_id === sessionId || !p.session_id,
  );
  if (cards.length === 0) return null;
  return (
    <div className="space-y-2">
      {cards.map((p) => (
        <GitHubPRCard key={p.approval_id ?? p.pr_url} pr={p} />
      ))}
    </div>
  );
}

function CIIcon({ status }: { status: string }) {
  if (status === 'success') return <CheckCircle2 className="h-3.5 w-3.5 text-emerald-400" />;
  if (status === 'failure') return <XCircle className="h-3.5 w-3.5 text-destructive" />;
  if (status === 'pending') return <Loader2 className="h-3.5 w-3.5 text-amber-400 animate-spin" />;
  return <Clock className="h-3.5 w-3.5 text-muted-foreground" />;
}

function GitHubPRCard({ pr }: { pr: GitHubPRReadyProps }) {
  const removePRApproval = useStore((s) => s.removePRApproval);
  const [merging, setMerging] = useState(false);
  const [merged, setMerged] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const merge = async () => {
    if (merging || merged) return;
    setMerging(true);
    setError(null);
    try {
      await githubApi.mergePR(pr.owner, pr.repo, pr.pr_number);
      setMerged(true);
      const id = pr.approval_id ?? pr.pr_url;
      setTimeout(() => removePRApproval(id), 3000);
    } catch (e: any) {
      setError(e.message ?? 'Merge failed');
    } finally {
      setMerging(false);
    }
  };

  const dismiss = () => removePRApproval(pr.approval_id ?? pr.pr_url);

  return (
    <div className={cn(
      'rounded-lg border p-3 text-xs transition-opacity',
      merged
        ? 'border-emerald-500/30 bg-emerald-500/5 opacity-60'
        : 'border-primary/30 bg-primary/5',
    )}>
      <div className="flex items-start justify-between gap-2 mb-2">
        <div className="flex items-center gap-1.5">
          <GitPullRequest className="h-3.5 w-3.5 text-primary shrink-0" />
          <span className="font-medium text-primary">PR Ready for Review</span>
        </div>
        <div className="flex items-center gap-1">
          <CIIcon status={pr.ci_status} />
          <span className="text-muted-foreground capitalize">{pr.ci_status}</span>
        </div>
      </div>

      <a href={pr.pr_url} target="_blank" rel="noopener noreferrer"
         className="flex items-start gap-1.5 mb-2 hover:underline font-medium leading-snug">
        <span>#{pr.pr_number} {pr.pr_title}</span>
        <ExternalLink className="h-3 w-3 shrink-0 mt-0.5 text-muted-foreground" />
      </a>

      <div className="flex items-center gap-3 mb-3 text-muted-foreground">
        <span className="flex items-center gap-1">
          <GitBranch className="h-3 w-3" />
          {pr.head_branch} → {pr.base_branch}
        </span>
        {(pr.diff_additions > 0 || pr.diff_deletions > 0) && (
          <span className="flex items-center gap-1.5">
            <span className="flex items-center gap-0.5 text-emerald-400">
              <Plus className="h-3 w-3" />{pr.diff_additions}
            </span>
            <span className="flex items-center gap-0.5 text-destructive">
              <Minus className="h-3 w-3" />{pr.diff_deletions}
            </span>
          </span>
        )}
      </div>

      {merged ? (
        <div className="flex items-center gap-1.5 text-emerald-400 font-medium">
          <CheckCircle2 className="h-3.5 w-3.5" />
          Merged successfully
        </div>
      ) : (
        <div className="flex items-center gap-2">
          <button onClick={merge} disabled={merging || pr.ci_status === 'failure'}
            className={cn(
              'flex-1 flex items-center justify-center gap-1.5 rounded-md px-2 py-1.5 font-medium transition-colors',
              pr.ci_status === 'failure'
                ? 'bg-muted text-muted-foreground cursor-not-allowed'
                : 'bg-primary/10 text-primary hover:bg-primary/20 disabled:opacity-50',
            )}>
            {merging
              ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
              : <GitMerge className="h-3.5 w-3.5" />
            }
            Approve & Merge
          </button>
          <button onClick={dismiss}
            className="rounded-md px-2 py-1.5 text-muted-foreground hover:text-foreground hover:bg-muted transition-colors">
            Dismiss
          </button>
        </div>
      )}

      {error && (
        <div className="mt-1.5 text-destructive">{error} — try again</div>
      )}
    </div>
  );
}
