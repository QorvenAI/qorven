'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { permissions } from '@/lib/api';
import type { ApprovalEntry } from '@/store';
import { CheckCircle2, XCircle, ShieldAlert, Loader2, Infinity } from 'lucide-react';

interface Props {
  sessionId: string;
  hideResolvedAfterMs?: number;
}

export function ApprovalCards({ sessionId, hideResolvedAfterMs }: Props) {
  const approvals = useStore((s) => s.approvals);
  if (!sessionId) return null;

  const now = Date.now();
  const mine = Object.values(approvals)
    .filter((a) => a.request.session_id === sessionId)
    .filter((a) => {
      if (!hideResolvedAfterMs || !a.resolved) return true;
      return now - a.createdAt < hideResolvedAfterMs;
    })
    .sort((a, b) => a.createdAt - b.createdAt);

  if (mine.length === 0) return null;

  return (
    <div className="space-y-2">
      {mine.map((entry) => (
        <ApprovalCard key={entry.request.request_id} entry={entry} />
      ))}
    </div>
  );
}

// Maps internal tool names to a human-readable action description.
function toolLabel(tool: string): string {
  const labels: Record<string, string> = {
    gh_push_file:   'Write a file to GitHub',
    gh_open_pr:     'Open a Pull Request on GitHub',
    gh_merge_pr:    'Merge a Pull Request on GitHub',
    gh_create_repo: 'Create a new GitHub repository',
    exec:           'Run a shell command',
    write_file:     'Write a file to disk',
    delete_file:    'Delete a file',
    cron:           'Schedule a recurring task',
    cron_create:    'Create a scheduled job',
  };
  return labels[tool] ?? tool.replace(/_/g, ' ');
}

function ApprovalCard({ entry }: { entry: ApprovalEntry }) {
  const markApprovalResolved = useStore((s) => s.markApprovalResolved);
  const [submitting, setSubmitting] = useState<'allow' | 'allow_always' | 'deny' | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { request, resolved } = entry;

  const send = async (decision: 'allow' | 'allow_always' | 'deny') => {
    if (resolved || submitting) return;
    setSubmitting(decision);
    setError(null);
    try {
      await permissions.reply(request.request_id, { decision });
      markApprovalResolved(request.request_id, decision === 'deny' ? 'deny' : 'allow', { actor: 'me' });
    } catch (e) {
      setError(e instanceof Error ? e.message : 'reply failed');
      setSubmitting(null);
    }
  };

  const isResolved = !!resolved;
  const allowed = resolved?.decision === 'allow';

  return (
    <div
      className={cn(
        'rounded-lg border p-2.5 text-xs transition-opacity',
        isResolved
          ? 'border-border/50 bg-muted/20 opacity-60'
          : 'border-amber-500/50 bg-amber-500/5',
      )}
    >
      {/* Header */}
      <div className="flex items-center gap-1.5 mb-1">
        <ShieldAlert
          className={cn(
            'h-3.5 w-3.5 shrink-0',
            isResolved ? 'text-muted-foreground' : 'text-amber-500',
          )}
        />
        <span className={cn('font-semibold', isResolved ? 'text-muted-foreground' : 'text-amber-500')}>
          {isResolved
            ? (allowed ? 'Allowed' : 'Denied')
            : toolLabel(request.tool)}
        </span>
        {!isResolved && (
          <code className="ml-auto text-2xs font-mono text-muted-foreground/60 shrink-0">
            {request.tool}
          </code>
        )}
      </div>

      {/* Reason — the human-readable explanation from the tool wrapper */}
      {request.reason && (
        <div className="text-muted-foreground mb-1.5 leading-relaxed">
          {request.reason}
        </div>
      )}

      {/* Args detail — collapsed by default to keep things clean */}
      <details className="mb-2 rounded border border-border/50 bg-background/60 text-2xs">
        <summary className="cursor-pointer px-2 py-1 font-medium text-muted-foreground hover:text-foreground select-none">
          details
        </summary>
        <pre className="whitespace-pre-wrap break-all px-2 py-1 font-mono max-h-60 overflow-y-auto">
          {safeStringify(request.args)}
        </pre>
      </details>

      {/* Decision buttons */}
      {!isResolved && (
        <div className="flex items-center gap-1.5">
          <button
            onClick={() => send('allow')}
            disabled={!!submitting}
            className={cn(
              'flex-1 flex items-center justify-center gap-1 px-2 py-1 rounded-md text-xs font-medium',
              'bg-emerald-500/10 text-emerald-600 border border-emerald-500/30',
              'hover:bg-emerald-500/20 disabled:opacity-50 disabled:cursor-not-allowed transition-colors',
            )}
          >
            {submitting === 'allow' ? <Loader2 className="h-3 w-3 animate-spin" /> : <CheckCircle2 className="h-3 w-3" />}
            Allow once
          </button>
          <button
            onClick={() => send('allow_always')}
            disabled={!!submitting}
            title="Allow this action every time — no more prompts for this tool"
            className={cn(
              'flex-1 flex items-center justify-center gap-1 px-2 py-1 rounded-md text-xs font-medium',
              'bg-blue-500/10 text-blue-500 border border-blue-500/30',
              'hover:bg-blue-500/20 disabled:opacity-50 disabled:cursor-not-allowed transition-colors',
            )}
          >
            {submitting === 'allow_always' ? <Loader2 className="h-3 w-3 animate-spin" /> : <Infinity className="h-3 w-3" />}
            Allow always
          </button>
          <button
            onClick={() => send('deny')}
            disabled={!!submitting}
            className={cn(
              'flex items-center justify-center gap-1 px-2 py-1 rounded-md text-xs font-medium',
              'bg-destructive/10 text-destructive border border-destructive/30',
              'hover:bg-destructive/20 disabled:opacity-50 disabled:cursor-not-allowed transition-colors',
            )}
          >
            {submitting === 'deny' ? <Loader2 className="h-3 w-3 animate-spin" /> : <XCircle className="h-3 w-3" />}
            Deny
          </button>
        </div>
      )}

      {/* Resolved footer */}
      {isResolved && (resolved.actor || resolved.note) && (
        <div className="text-2xs text-muted-foreground mt-1">
          {resolved.actor && <span>by {resolved.actor}</span>}
          {resolved.note && <span className="ml-2">— {resolved.note}</span>}
        </div>
      )}

      {error && (
        <div className="mt-1.5 text-2xs text-destructive">
          {error} — tap a button to retry
        </div>
      )}
    </div>
  );
}

function safeStringify(value: unknown): string {
  try {
    const out = JSON.stringify(value, null, 2);
    if (out && out.length > 10_000) return out.slice(0, 10_000) + '\n… [truncated]';
    return out ?? '{}';
  } catch {
    return String(value);
  }
}
