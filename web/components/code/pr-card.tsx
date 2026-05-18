'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { GitPullRequest, ExternalLink, CheckCircle2 } from 'lucide-react';

export function PrCard({ prUrl, prTitle, prNumber, prRepo, onViewPr }: {
  prUrl?: string;
  prTitle?: string;
  prNumber?: number;
  prRepo?: string;
  onViewPr?: () => void;
}) {
  const title = prTitle || (prNumber ? `PR #${prNumber}` : 'Pull Request opened');
  const repoLabel = prRepo || (prUrl ? new URL(prUrl).pathname.split('/').slice(1, 3).join('/') : '');

  return (
    <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/5 p-3 space-y-2 my-1">
      <div className="flex items-start gap-2">
        <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-emerald-500/15">
          <GitPullRequest className="h-3.5 w-3.5 text-emerald-600" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5">
            <CheckCircle2 className="h-3 w-3 text-emerald-500 shrink-0" />
            <span className="text-xs font-semibold text-emerald-700 dark:text-emerald-400">Pull request opened</span>
          </div>
          <p className="mt-0.5 text-xs font-medium truncate">{title}</p>
          {repoLabel && <p className="text-2xs text-muted-foreground font-mono">{repoLabel}</p>}
        </div>
      </div>

      <div className="flex items-center gap-2">
        {prUrl && (
          <a
            href={prUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 rounded-md border border-emerald-500/30 bg-card px-2.5 py-1 text-xs font-medium text-emerald-600 hover:bg-emerald-500/10 transition-colors"
          >
            <ExternalLink className="h-3 w-3" />
            View PR
          </a>
        )}
        <button
          onClick={onViewPr}
          className="inline-flex items-center gap-1.5 rounded-md border border-border bg-card px-2.5 py-1 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-foreground transition-colors"
        >
          <GitPullRequest className="h-3 w-3" />
          Open session
        </button>
      </div>
    </div>
  );
}
