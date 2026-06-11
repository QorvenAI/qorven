'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useRef, useState } from 'react';
import {
  AlertCircle,
  CheckCircle2,
  ChevronRight,
  ExternalLink,
  GitPullRequest,
  Loader2,
  Wrench,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import type { BuildEntry } from './code-types';
import { FileChangeChip } from './file-change-chip';

// ─── Feed segment types ───────────────────────────────────────────────────────

interface ToolSegment {
  kind: 'tool';
  index: number;
  tool: string;
  label: string;
  status: 'running' | 'done' | 'error';
  ts?: number;
  durationMs?: number;
  children: BuildEntry[];
}

interface StandaloneSegment {
  kind: 'standalone';
  index: number;
  entry: BuildEntry;
}

type FeedSegment = ToolSegment | StandaloneSegment;

// ─── Grouping (adapted from buildTimeline in command-timeline.tsx) ────────────

const ENTRY_CAP = 500;

function buildActivityFeed(entries: BuildEntry[], running: boolean): FeedSegment[] {
  const capped =
    entries.length > ENTRY_CAP ? entries.slice(entries.length - ENTRY_CAP) : entries;
  const segments: FeedSegment[] = [];
  let current: ToolSegment | null = null;

  for (let i = 0; i < capped.length; i++) {
    const e = capped[i]!;
    if (e.type === 'tool_start') {
      if (current) {
        if (current.ts && e.ts) current.durationMs = e.ts - current.ts;
        segments.push(current);
      }
      current = {
        kind: 'tool',
        index: i,
        tool: e.tool || '',
        label: e.content || e.tool || '',
        status: 'done',
        ts: e.ts,
        children: [],
      };
    } else if (current) {
      if (e.type === 'error') current.status = 'error';
      current.children.push(e);
    } else {
      if (e.type !== 'done') {
        segments.push({ kind: 'standalone', index: i, entry: e });
      }
    }
  }

  if (current) {
    current.status = running ? 'running' : current.status;
    segments.push(current);
  }

  return segments;
}

function formatDuration(ms?: number): string {
  if (!ms || ms < 0) return '';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

// ─── ToolCard — running (amber) → done (emerald) / error (destructive) ────────

function ToolCard({
  seg,
  onFileClick,
}: {
  seg: ToolSegment;
  onFileClick?: (path: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);

  const fileChildren = seg.children.filter(
    (e) => e.type === 'file_chip' || (e.type === 'file_created' && e.path),
  );
  const textChildren = seg.children.filter((e) => e.type === 'text' && e.content.trim());
  const errorChildren = seg.children.filter((e) => e.type === 'error');
  const hasExpandable = seg.children.length > 0;

  const isRunning = seg.status === 'running';
  const isError = seg.status === 'error';

  return (
    <div
      className={cn(
        'rounded-lg border transition-colors',
        isRunning && 'border-amber-500/30 bg-amber-500/10',
        isError && 'border-destructive/30 bg-destructive/10',
        !isRunning && !isError && 'border-border bg-card',
      )}
    >
      {/* Header row */}
      <button
        onClick={() => hasExpandable && setExpanded((v) => !v)}
        className={cn(
          'flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-xs transition-colors',
          hasExpandable ? 'cursor-pointer hover:bg-accent/30' : 'cursor-default',
        )}
      >
        {/* Status icon — flips in-place based on seg.status prop */}
        <span className="shrink-0">
          {isRunning ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin text-amber-500" />
          ) : isError ? (
            <AlertCircle className="h-3.5 w-3.5 text-destructive" />
          ) : (
            <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500" />
          )}
        </span>

        {/* Tool name */}
        {seg.tool ? (
          <span
            className={cn(
              'shrink-0 font-mono font-semibold',
              isRunning
                ? 'text-amber-600 dark:text-amber-400'
                : isError
                  ? 'text-destructive'
                  : 'text-foreground',
            )}
          >
            {seg.tool}
          </span>
        ) : (
          <Wrench className="h-3 w-3 shrink-0 text-muted-foreground" />
        )}

        {/* Arg / target snippet */}
        <span className="flex-1 truncate text-muted-foreground">{seg.label}</span>

        {/* Right-side stats */}
        <div className="ml-auto flex shrink-0 items-center gap-2">
          {fileChildren.length > 0 && (
            <span className="text-2xs font-mono text-emerald-500">
              +{fileChildren.length} {fileChildren.length === 1 ? 'file' : 'files'}
            </span>
          )}
          {errorChildren.length > 0 && !isError && (
            <AlertCircle className="h-3 w-3 text-destructive" />
          )}
          {seg.durationMs !== undefined && !isRunning && (
            <span className="text-2xs font-mono tabular-nums text-muted-foreground/60">
              {formatDuration(seg.durationMs)}
            </span>
          )}
          {hasExpandable && (
            <ChevronRight
              className={cn(
                'h-3 w-3 text-muted-foreground/40 transition-transform',
                expanded && 'rotate-90',
              )}
            />
          )}
        </div>
      </button>

      {/* Expandable body */}
      {expanded && (
        <div className="border-t border-border/50 px-3 pb-2 pt-1.5 space-y-1.5">
          {textChildren.map((e, i) => (
            <div
              key={`t${i}`}
              className="rounded-sm bg-muted/30 px-2 py-1 text-2xs font-mono text-muted-foreground/80 leading-relaxed whitespace-pre-wrap"
            >
              {e.content.trim()}
            </div>
          ))}

          {fileChildren.map((e, i) => {
            const path = e.path || e.content;
            if (!path) return null;
            return (
              <div key={`f${i}`}>
                <FileChangeChip
                  path={path}
                  linesAdded={e.linesAdded}
                  linesRemoved={e.linesRemoved}
                  totalLines={e.totalLines}
                  onClick={onFileClick}
                />
              </div>
            );
          })}

          {errorChildren.map((e, i) => (
            <div key={`e${i}`} className="flex items-start gap-1.5 text-xs text-destructive">
              <AlertCircle className="h-3 w-3 shrink-0 mt-0.5" />
              <span className="break-words">{e.content}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ─── StandaloneItem — text, file chips, PR cards, errors ─────────────────────

function StandaloneItem({
  seg,
  isLast,
  running,
  onFileClick,
  onOpenSession,
}: {
  seg: StandaloneSegment;
  isLast: boolean;
  running: boolean;
  onFileClick?: (path: string) => void;
  onOpenSession?: () => void;
}) {
  const e = seg.entry;
  const showCursor = isLast && running;

  if (e.type === 'text') {
    return (
      <div className="rounded-md bg-muted/30 px-3 py-2 text-xs font-mono text-foreground/80 whitespace-pre-wrap leading-relaxed">
        {e.content}
        {showCursor && (
          <span className="inline-block w-1.5 h-3 bg-foreground/40 align-middle ml-0.5 animate-pulse" />
        )}
      </div>
    );
  }

  if (e.type === 'file_chip') {
    const path = e.path || e.content;
    if (!path) return null;
    return (
      <div className="px-1">
        <FileChangeChip
          path={path}
          linesAdded={e.linesAdded}
          linesRemoved={e.linesRemoved}
          totalLines={e.totalLines}
          onClick={onFileClick}
        />
      </div>
    );
  }

  if (e.type === 'file_created' && e.path) {
    return (
      <button
        onClick={() => e.path && onFileClick?.(e.path)}
        className="flex w-full items-center gap-1.5 rounded-md px-2 py-1 text-xs text-emerald-500 hover:bg-emerald-500/5 transition-colors text-left"
      >
        <CheckCircle2 className="h-3 w-3 shrink-0" />
        <span className="truncate font-mono">{e.path}</span>
      </button>
    );
  }

  if (e.type === 'pr_card') {
    const title = e.prTitle || (e.prNumber ? `PR #${e.prNumber}` : 'Pull request opened');
    const repoLabel =
      e.prRepo ||
      (e.prUrl
        ? (() => {
            try {
              return new URL(e.prUrl!).pathname.split('/').slice(1, 3).join('/');
            } catch {
              return '';
            }
          })()
        : '');

    return (
      <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 p-3 space-y-2">
        <div className="flex items-start gap-2">
          <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-emerald-500/15">
            <GitPullRequest className="h-3.5 w-3.5 text-emerald-600" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-1.5">
              <CheckCircle2 className="h-3 w-3 text-emerald-500 shrink-0" />
              <span className="text-xs font-semibold text-emerald-700 dark:text-emerald-400">
                Pull request opened
              </span>
            </div>
            <p className="mt-0.5 text-xs font-medium truncate">{title}</p>
            {repoLabel && (
              <p className="text-2xs text-muted-foreground font-mono">{repoLabel}</p>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2">
          {e.prUrl && (
            <a
              href={e.prUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1.5 rounded-md border border-emerald-500/30 bg-card px-2.5 py-1 text-xs font-medium text-emerald-600 hover:bg-emerald-500/10 transition-colors"
            >
              <ExternalLink className="h-3 w-3" />
              View PR
            </a>
          )}
          {onOpenSession && (
            <button
              onClick={onOpenSession}
              className="inline-flex items-center gap-1.5 rounded-md border border-border bg-card px-2.5 py-1 text-xs font-medium text-muted-foreground hover:bg-accent hover:text-foreground transition-colors"
            >
              <GitPullRequest className="h-3 w-3" />
              Open session
            </button>
          )}
        </div>
      </div>
    );
  }

  if (e.type === 'error') {
    return (
      <div className="flex items-start gap-1.5 rounded-md border border-destructive/20 bg-destructive/10 px-3 py-2 text-xs text-destructive">
        <AlertCircle className="h-3.5 w-3.5 shrink-0 mt-0.5" />
        <span className="break-words">{e.content}</span>
      </div>
    );
  }

  return null;
}

// ─── ActivityFeed — main export ───────────────────────────────────────────────

export function ActivityFeed({
  entries,
  running,
  onFileClick,
  onOpenSession,
}: {
  entries: BuildEntry[];
  running: boolean;
  onFileClick?: (path: string) => void;
  onOpenSession?: () => void;
}) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const segments = buildActivityFeed(entries, running);

  // Auto-scroll to bottom while build is running
  useEffect(() => {
    if (running) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [entries.length, running]);

  if (segments.length === 0) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center">
        <p className="text-xs text-muted-foreground">No activity yet</p>
      </div>
    );
  }

  return (
    <div className="flex-1 overflow-y-auto px-3 py-3 space-y-2">
      {segments.map((seg, idx) => {
        if (seg.kind === 'tool') {
          return <ToolCard key={seg.index} seg={seg} onFileClick={onFileClick} />;
        }
        return (
          <StandaloneItem
            key={seg.index}
            seg={seg}
            isLast={idx === segments.length - 1}
            running={running}
            onFileClick={onFileClick}
            onOpenSession={onOpenSession}
          />
        );
      })}
      <div ref={bottomRef} />
    </div>
  );
}
